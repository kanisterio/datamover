package controller

import (
	"context"
	"slices"
	"strconv"
	"strings"

	api "github.com/kanisterio/datamover/api/v1alpha1"
	"github.com/kanisterio/datamover/pkg/podoverride"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	defaultContainerName = "main"
)

func (r *DatamoverSessionReconciler) CreatePod(ctx context.Context, dmSession api.DatamoverSession) error {
	podSpec, err := MakePodSpec(dmSession)
	if err != nil {
		return err
	}

	if err := controllerutil.SetControllerReference(&dmSession, podSpec, r.Scheme); err != nil {
		return err
	}

	if err := r.Create(ctx, podSpec); err != nil {
		return errors.Wrap(err, "Failed to create pod")
	}
	log.Log.Info("Created pod.")

	return nil
}

func (r *DatamoverSessionReconciler) DeletePod(ctx context.Context, pod *corev1.Pod) error {
	return r.Delete(ctx, pod)
}

func MakePodSpec(dmSession api.DatamoverSession) (*corev1.Pod, error) {
	if err := validateSessionForPod(dmSession); err != nil {
		return nil, errors.Wrap(err, "Session spec is invalid for pod creation")
	}
	volumes, mounts := makePodVolumes(dmSession)
	// Make sure labels include selector
	labels := dmSession.Spec.LifecycleConfig.PodOptions.Labels
	if labels == nil {
		labels = map[string]string{}
	}

	labels[datamoverSessionSelectorLabel] = dmSession.Name
	labels[datamoverSessionLabel] = dmSession.Name

	// Create env vars by adding protocols and dmSession address to envs from spec
	env := dmSession.Spec.Env
	if env == nil {
		env = map[string]string{}
	}
	env[api.ImplementationEnvVarName] = dmSession.Spec.Implementation
	env[api.ProtocolsEnvVarName] = formatProtocolsVar(dmSession.Spec.LifecycleConfig.ServicePorts)

	env_vars := []corev1.EnvVar{}
	for key, val := range env {
		env_vars = append(env_vars, corev1.EnvVar{
			Name:  key,
			Value: val,
		})
	}

	mainContainer := corev1.Container{
		// TODO: make a const
		Name:  defaultContainerName,
		Image: dmSession.Spec.LifecycleConfig.Image,
		// FIXME: ImagePullPolicy
		ImagePullPolicy: dmSession.Spec.LifecycleConfig.PodOptions.ImagePullPolicy,
		VolumeMounts:    mounts,
		ReadinessProbe:  readinessProbe(),
		StartupProbe:    dmSession.Spec.LifecycleConfig.StartupProbe,
		LivenessProbe:   dmSession.Spec.LifecycleConfig.LivenessProbe,
		Env:             env_vars,
		Resources:       dmSession.Spec.LifecycleConfig.PodOptions.Resources,
		SecurityContext: dmSession.Spec.LifecycleConfig.PodOptions.ContainerSecurityContext,
	}

	sessionDataContainer := sessionDataContainer()

	genPodName := dmSession.GenerateName
	if genPodName == "" {
		genPodName = dmSession.Name
	}

	serviceAccountName := dmSession.Spec.LifecycleConfig.PodOptions.ServiceAccount
	automount := serviceAccountName != ""
	automountServiceAccountToken := &automount

	podSpec := corev1.PodSpec{
		Volumes:               volumes,
		PriorityClassName:     dmSession.Spec.LifecycleConfig.PodOptions.PriorityClassName,
		SecurityContext:       dmSession.Spec.LifecycleConfig.PodOptions.PodSecurityContext,
		ShareProcessNamespace: dmSession.Spec.LifecycleConfig.PodOptions.ShareProcessNamespace,
		Containers:            append(dmSession.Spec.LifecycleConfig.PodOptions.ExtraContainers, mainContainer),
		// TODO: support sidecar (restarting init) containers
		InitContainers: []corev1.Container{sessionDataContainer},
		// TODO: think of lifecycle management which can support restart
		RestartPolicy:                corev1.RestartPolicyNever,
		ServiceAccountName:           serviceAccountName,
		AutomountServiceAccountToken: automountServiceAccountToken,
	}

	// podOverride to make it work with the K10 prototype code
	// TODO: rework when reviewing podOverride/podOptions
	podSpec, err := podoverride.OverridePodSpec(podSpec, dmSession.Spec.LifecycleConfig.PodOptions.PodOverride)
	if err != nil {
		return nil, err
	}

	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: genPodName,
			Namespace:    dmSession.Namespace,
			Labels:       labels,
			Annotations:  dmSession.Spec.LifecycleConfig.PodOptions.Annotations,
		},
		Spec: podSpec,
	}, nil
}

func formatProtocolsVar(ports []corev1.ServicePort) string {
	configs := make([]string, len(ports))
	for i, port := range ports {
		portNum := port.TargetPort.IntValue()
		if portNum == 0 {
			portNum = int(port.Port)
		}
		configs[i] = port.Name + ":" + strconv.Itoa(portNum)
	}
	return strings.Join(configs, ";")
}

func makePodVolumes(dmSession api.DatamoverSession) ([]corev1.Volume, []corev1.VolumeMount) {
	extraVolumes := dmSession.Spec.LifecycleConfig.PodOptions.ExtraVolumes
	extraVolumeMounts := podOptionsVolumeMounts(extraVolumes)
	configmapVolumes, configmapVolumeMounts := configMapVolume(dmSession.Spec.Configuration)
	clientSecretVolumes, clientSecretVolumeMounts := clientSecretVolume(dmSession.Spec.ClientSecretRef)
	sessionDataVolumes, sessionDataVolumeMounts := sessionDataVolume()
	secretVolumes, secretVolumeMounts := configSecretVolumes(dmSession.Spec.ConfigurationSecrets)

	return slices.Concat(extraVolumes, configmapVolumes, clientSecretVolumes, sessionDataVolumes, secretVolumes),
		slices.Concat(extraVolumeMounts, configmapVolumeMounts, clientSecretVolumeMounts, sessionDataVolumeMounts, secretVolumeMounts)
}

func podOptionsVolumeMounts(volumes []corev1.Volume) []corev1.VolumeMount {
	mounts := []corev1.VolumeMount{}
	for _, vol := range volumes {
		mounts = append(mounts, corev1.VolumeMount{
			Name:      vol.Name,
			MountPath: "/mnt/volumes/" + vol.Name,
		})
	}
	return mounts
}

func configSecretVolumes(secretsMap map[string]corev1.SecretVolumeSource) ([]corev1.Volume, []corev1.VolumeMount) {
	volumes := []corev1.Volume{}
	volumeMounts := []corev1.VolumeMount{}
	for name, secret := range secretsMap {
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      name,
			MountPath: "/etc/secrets/" + name,
		})
		volumes = append(volumes, getSecretVolume(name, secret))
	}
	return volumes, volumeMounts
}

func configMapVolume(ref *corev1.ConfigMapVolumeSource) ([]corev1.Volume, []corev1.VolumeMount) {
	volumes := []corev1.Volume{}
	volumeMounts := []corev1.VolumeMount{}
	name := "config"
	mountPoint := "/etc/config"
	if ref != nil {
		volumes = append(volumes, getConfigMapVolume(name, *ref))
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      name,
			MountPath: mountPoint,
		})
	}
	return volumes, volumeMounts
}

func clientSecretVolume(ref *corev1.SecretVolumeSource) ([]corev1.Volume, []corev1.VolumeMount) {
	volumes := []corev1.Volume{}
	volumeMounts := []corev1.VolumeMount{}
	name := "client-creds"
	mountPoint := "/etc/client_credentials"
	if ref != nil {
		volumes = append(volumes, getSecretVolume(name, *ref))
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      name,
			MountPath: mountPoint,
		})
	}
	return volumes, volumeMounts
}

func getConfigMapVolume(name string, ref corev1.ConfigMapVolumeSource) corev1.Volume {
	return corev1.Volume{
		Name:         name,
		VolumeSource: corev1.VolumeSource{ConfigMap: &ref},
	}
}

func getSecretVolume(name string, ref corev1.SecretVolumeSource) corev1.Volume {
	return corev1.Volume{
		Name:         name,
		VolumeSource: corev1.VolumeSource{Secret: &ref},
	}
}

func validateSessionForPod(dmSession api.DatamoverSession) error {
	if dmSession.Spec.LifecycleConfig == nil {
		return errors.New("Can only create pods for lifecycle session")
	}
	image := dmSession.Spec.LifecycleConfig.Image
	implementation := dmSession.Spec.Implementation
	if implementation == "" {
		return errors.New("Session must have implementation set")
	}
	if image == "" {
		return errors.New("Session must have lifecycle.image set")
	}
	return nil
}
