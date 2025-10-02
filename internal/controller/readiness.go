package controller

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"

	api "github.com/kanisterio/datamover/api/v1alpha1"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	sessionDataContainerName = "session-data-read"
	// TODO: is there an alternative?
	// FIXME: do we want to make this configurable??
	sessionDataContainerImage = "busybox:latest"

	sessionDataVolumeName       = "session-data"
	sessionDataVolumeMountPoint = "/etc/session"

	// TODO: make this configurable
	sessionDataReadinessTimeout  = 600
	sessionDataReadinessInterval = 1
)

// Checking whether session container is started and is passing the probes
func isPodReady(pod corev1.Pod) bool {
	if pod.Status.Phase == corev1.PodRunning {
		log.Log.Info("Pod status is ready now")
		return mainContainerReady(pod) && sidecarContainerReady(pod)
	}
	return false
}

func mainContainerReady(pod corev1.Pod) bool {
	for _, containerStatus := range pod.Status.ContainerStatuses {
		if containerStatus.Name == api.DefaultContainerName {
			// All probes need to succeed
			return *containerStatus.Started && containerStatus.Ready
		}
	}
	return false
}

func sidecarContainerReady(pod corev1.Pod) bool {
	for _, containerStatus := range pod.Status.InitContainerStatuses {
		if containerStatus.Name == sessionDataContainerName {
			// All probes need to succeed
			return *containerStatus.Started && containerStatus.Ready
		}
	}
	return false
}

// Data being nil means that logs were not read successfully
func (r *DatamoverSessionReconciler) fetchSessionData(ctx context.Context, pod corev1.Pod) (*string, error) {
	// Currently using a single output run from sessionDataContainer sidecar
	// TODO: support dynamic session data
	return r.fetchSessionDataUsingSidecar(ctx, pod)
}

// Reasons for nil data:
// Sidecar container not ready or missing
// Error in k8s api fetching logs
// Logs do not container start and stop sequences
// Data may be empty and non-nil
func (r *DatamoverSessionReconciler) fetchSessionDataUsingSidecar(ctx context.Context, pod corev1.Pod) (*string, error) {
	for _, containerStatus := range pod.Status.InitContainerStatuses {
		if containerStatus.Name == sessionDataContainerName {
			logs, err := r.getContainerLogs(ctx, pod.Name, pod.Namespace, sessionDataContainerName)
			if err != nil {
				return nil, errors.Wrap(err, "Failed to read pod logs")
			}

			log.Log.Info("Pod logs: ", "log", logs)

			data := getDataFromLogs(logs)
			return data, nil
		}
	}
	return nil, nil
}

func getDataFromLogs(logs string) *string {
	start := strings.LastIndex(logs, "---")
	end := strings.LastIndex(logs, "___")

	if start == -1 || end == -1 {
		return nil
	}

	data := strings.TrimSpace(logs[start+3 : end])

	return &data
}

func (r *DatamoverSessionReconciler) getPodErrors(ctx context.Context, podName, podNamespace string) (string, error) {
	var pod corev1.Pod
	err := r.Get(ctx, types.NamespacedName{Name: podName, Namespace: podNamespace}, &pod)
	if err != nil {
		return "", err
	}

	containerErrors := ""

	for _, conStatus := range pod.Status.ContainerStatuses {
		if conStatus.Name == api.DefaultContainerName && !conStatus.Ready {
			if conStatus.State.Waiting != nil {
				containerErrors = fmt.Sprintf("Waiting to run main container: %s %s",
					conStatus.State.Waiting.Reason,
					conStatus.State.Waiting.Message)
			}
			if conStatus.State.Terminated != nil {
				containerErrors = fmt.Sprintf("Main container terminated: %s %s",
					conStatus.State.Terminated.Reason,
					conStatus.State.Terminated.Message)

			}
		}
	}
	podLogs, err := r.getContainerLogsReader(ctx, podName, podNamespace, api.DefaultContainerName)
	if err != nil {
		return containerErrors, err
	}
	defer podLogs.Close()

	errorLines := []string{}
	scanner := bufio.NewScanner(podLogs)
	for scanner.Scan() {
		line := scanner.Text()
		if isErrorLine(line) {
			errorLines = append(errorLines, line)
		}
	}
	return containerErrors + "\n" + strings.Join(errorLines, "\n"), nil
}

func isErrorLine(line string) bool {
	return strings.Contains(line, "ERROR")
}

func (r *DatamoverSessionReconciler) getContainerLogs(ctx context.Context, podName, podNamespace, containerName string) (string, error) {
	podLogs, err := r.getContainerLogsReader(ctx, podName, podNamespace, containerName)
	if err != nil {
		return "", err
	}
	defer podLogs.Close()

	buf := new(bytes.Buffer)
	_, err = io.Copy(buf, podLogs)
	if err != nil {
		return "", errors.New("error in copy information from podLogs to buf")
	}
	str := buf.String()
	return str, nil
}

func (r *DatamoverSessionReconciler) getContainerLogsReader(ctx context.Context, podName, podNamespace, containerName string) (io.ReadCloser, error) {
	config := r.RestConfig
	clientset, err := kubernetes.NewForConfig(&config)

	if err != nil {
		return nil, err
	}

	podLogOpts := corev1.PodLogOptions{Container: containerName}
	req := clientset.CoreV1().Pods(podNamespace).GetLogs(podName, &podLogOpts)

	podLogs, err := req.Stream(ctx)
	if err != nil {
		log.Log.Error(err, "Error reading pod logs")
		return nil, errors.New("error in opening stream")
	}
	return podLogs, nil
}

func sessionDataContainer() corev1.Container {
	restartAlways := corev1.ContainerRestartPolicyAlways
	return corev1.Container{
		Name:  sessionDataContainerName,
		Image: sessionDataContainerImage,
		// TODO: this container will run indefinitely. Maybe there is a way to prevent that?
		Command: []string{
			"sh",
			"-c",
			"echo '---' ; while [ ! -f /etc/session/ready ];" +
				" do sleep 1; done;" +
				"if [ -f /etc/session/data ]; then cat /etc/session/data | base64 -w0; fi;" +
				" echo '___';" +
				" tail -f /dev/null "},
		VolumeMounts:   []corev1.VolumeMount{sessionDataVolumeMount()},
		RestartPolicy:  &restartAlways,
		ReadinessProbe: readinessProbe(),
		// TODO: resources limit??
	}
}

func readinessProbe() *corev1.Probe {
	return &corev1.Probe{
		TimeoutSeconds: sessionDataReadinessTimeout,
		PeriodSeconds:  sessionDataReadinessInterval,
		ProbeHandler: corev1.ProbeHandler{
			Exec: &corev1.ExecAction{
				Command: []string{"cat", "/etc/session/ready"},
			},
		},
	}
}

func sessionDataVolumeMount() corev1.VolumeMount {
	return corev1.VolumeMount{
		Name:      sessionDataVolumeName,
		MountPath: sessionDataVolumeMountPoint,
	}
}

func sessionDataVolume() ([]corev1.Volume, []corev1.VolumeMount) {
	volumes := []corev1.Volume{{
		Name: sessionDataVolumeName,
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{
				Medium: corev1.StorageMediumMemory,
			},
		},
	}}
	volumeMounts := []corev1.VolumeMount{sessionDataVolumeMount()}
	return volumes, volumeMounts
}
