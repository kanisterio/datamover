package controller

import (
	"path"
	"testing"

	api "github.com/kanisterio/datamover/api/v1alpha1"
	"github.com/onsi/gomega"
	"github.com/onsi/gomega/gstruct"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Can only create pods for lifecycle datamover session
func TestMakePodSpecNoLifecycleValidationFail(t *testing.T) {
	dmSession := api.DatamoverSession{}
	_, err := MakePodSpec(dmSession)
	if err == nil {
		t.Errorf("Expected error, got nil")
	}
}

func TestMakePodSpecEmptyValidationFail(t *testing.T) {
	dmSession := api.DatamoverSession{
		Spec: api.DatamoverSessionSpec{
			LifecycleConfig: &api.LifecycleConfig{},
		},
	}
	pod, err := MakePodSpec(dmSession)
	t.Log("pod", pod)
	if err == nil {
		t.Errorf("Expected error, got nil")
	}
}

func TestMakePodSpecNoImplementationValidationFail(t *testing.T) {
	dmSession := api.DatamoverSession{
		Spec: api.DatamoverSessionSpec{
			LifecycleConfig: &api.LifecycleConfig{
				Image: "foo",
			},
		},
	}
	pod, err := MakePodSpec(dmSession)
	t.Log("pod", pod)
	if err == nil {
		t.Errorf("Expected error, got nil")
	}
}

func TestMakePodSpecNoImageValidationFail(t *testing.T) {
	dmSession := api.DatamoverSession{
		Spec: api.DatamoverSessionSpec{
			Implementation:  "foo",
			LifecycleConfig: &api.LifecycleConfig{},
		},
	}
	pod, err := MakePodSpec(dmSession)
	t.Log("pod", pod)
	if err == nil {
		t.Errorf("Expected error, got nil")
	}
}

func TestMakePodSpecImageAndImplementationValidationSufficient(t *testing.T) {
	matcher := gomega.NewWithT(t)
	imageName := "foo_image"
	implementation := "foo_impl"
	name := "foo_datamover"
	dmSession := api.DatamoverSession{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: api.DatamoverSessionSpec{
			Implementation: implementation,
			LifecycleConfig: &api.LifecycleConfig{
				Image: imageName,
			},
		},
	}
	pod, err := MakePodSpec(dmSession)
	t.Log("pod", pod)

	matcher.Expect(err).To(gomega.BeNil())

	podChecksNoPodOptions(t, pod, name, imageName, implementation)
	matcher.Expect(getMainContainer(*pod).Env).To(gomega.ContainElement(corev1.EnvVar{Name: "PROTOCOLS", Value: ""}))
	matcher.Expect(len(getMainContainer(*pod).VolumeMounts)).To(gomega.Equal(1))
	matcher.Expect(len(getMainContainer(*pod).Env)).To(gomega.Equal(2))
}

func assertDefaultLabels(t *testing.T, pod *corev1.Pod, name string) {
	matcher := gomega.NewWithT(t)
	matcher.Expect(pod.ObjectMeta.Labels).To(gstruct.MatchKeys(gstruct.IgnoreExtras, gstruct.Keys{
		api.DatamoverSessionSelectorLabel: gomega.Equal(name),
		api.DatamoverSessionLabel:         gomega.Equal(name),
	}))
}

func assertDefaultRestartPolicy(t *testing.T, pod *corev1.Pod) {
	matcher := gomega.NewWithT(t)
	matcher.Expect(pod.Spec.RestartPolicy).To(gomega.Equal(corev1.RestartPolicyNever))
}

func getMainContainer(pod corev1.Pod) *corev1.Container {
	for _, container := range pod.Spec.Containers {
		if container.Name == api.DefaultContainerName {
			return &container
		}
	}
	return nil
}

func defaultMainContainerChecks(t *testing.T, pod *corev1.Pod, imageName, implementation string) {
	matcher := gomega.NewWithT(t)
	mainContainer := getMainContainer(*pod)
	if mainContainer == nil {
		t.Log("Main container not found")
		t.FailNow()
		return
	}

	matcher.Expect(mainContainer.Name).To(gomega.Equal(api.DefaultContainerName))
	matcher.Expect(mainContainer.Image).To(gomega.Equal(imageName))
	matcher.Expect(mainContainer.Env).To(gomega.ContainElements(corev1.EnvVar{Name: api.ImplementationEnvVarName, Value: implementation}))
	matcher.Expect(mainContainer.ReadinessProbe).To(gomega.Equal(readinessProbe()))

	envs := mainContainer.Env
	matcher.Expect(envs).To(gomega.ContainElement(corev1.EnvVar{Name: "DATAMOVER_NAME", Value: "foo_impl"}))
}

func assertDefaultInitContainerChecks(t *testing.T, pod *corev1.Pod) {
	matcher := gomega.NewWithT(t)
	matcher.Expect(pod.Spec.InitContainers).To(gomega.HaveLen(1))
	initContainer := pod.Spec.InitContainers[0]
	matcher.Expect(initContainer.Name).To(gomega.Equal(sessionDataContainerName))
	matcher.Expect(initContainer.Image).To(gomega.Equal(sessionDataContainerImage))
	matcher.Expect(*initContainer.RestartPolicy).To(gomega.Equal(corev1.ContainerRestartPolicyAlways))
	matcher.Expect(initContainer.ImagePullPolicy).To(gomega.Equal(corev1.PullPolicy("")))
}

func basePodChecks(t *testing.T, pod *corev1.Pod, name, imageName, implementation string) {
	assertDefaultLabels(t, pod, name)
	assertDefaultRestartPolicy(t, pod)
	defaultMainContainerChecks(t, pod, imageName, implementation)
	assertDefaultInitContainerChecks(t, pod)
}

func podChecksNoExtraContainers(t *testing.T, pod *corev1.Pod, name, imageName, implementation string) {
	matcher := gomega.NewWithT(t)
	matcher.Expect(pod.Spec.Containers).To(gomega.HaveLen(1))
	basePodChecks(t, pod, name, imageName, implementation)
}

func podChecksNoPodOptions(t *testing.T, pod *corev1.Pod, name, imageName, implementation string) {
	matcher := gomega.NewWithT(t)
	podChecksNoExtraContainers(t, pod, name, imageName, implementation)

	matcher.Expect(pod.Spec.Containers).To(gomega.HaveLen(1))

	defaultMainContainerChecks(t, pod, imageName, implementation)

	assertDefaultInitContainerChecks(t, pod)

	matcher.Expect(*pod.Spec.AutomountServiceAccountToken).To(gomega.BeFalse())
	mainContainer := getMainContainer(*pod)
	matcher.Expect(mainContainer.LivenessProbe).To(gomega.BeNil())
	matcher.Expect(mainContainer.StartupProbe).To(gomega.BeNil())
	matcher.Expect(mainContainer.Command).To(gomega.BeEmpty())
	matcher.Expect(mainContainer.Args).To(gomega.BeEmpty())
	matcher.Expect(mainContainer.ImagePullPolicy).To(gomega.Equal(corev1.PullPolicy("")))
}

func TestMakePodSpecBaseValues(t *testing.T) {
	matcher := gomega.NewWithT(t)

	imageName := "foo_image"
	implementation := "foo_impl"
	name := "foo_datamover"
	dmSession := api.DatamoverSession{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: api.DatamoverSessionSpec{
			Implementation: implementation,
			LifecycleConfig: &api.LifecycleConfig{
				Image: imageName,
			},

			Configuration: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: "config-map",
				},
			},
			ConfigurationSecrets: map[string]corev1.SecretVolumeSource{
				"foo": {
					SecretName: "secret-foo",
				},
				"bar": {
					SecretName: "secret-bar",
				},
			},
			ClientSecretRef: &corev1.SecretVolumeSource{
				SecretName: "client-secret-name",
			},
			Env: map[string]string{
				"FOO": "bar",
			},
		},
	}

	pod, err := MakePodSpec(dmSession)
	t.Log("pod", pod)

	matcher.Expect(err).To(gomega.BeNil())

	podChecksNoPodOptions(t, pod, name, imageName, implementation)
	matcher.Expect(getMainContainer(*pod).Env).To(gomega.ContainElement(corev1.EnvVar{Name: "PROTOCOLS", Value: ""}))

	volumes := pod.Spec.Volumes
	matcher.Expect(volumes).To(gomega.HaveLen(5))

	t.Log("Checking volumes")
	assertConfigMapVolume(t, volumes, "config-map")
	assertClientsSecretVolume(t, volumes, "client-secret-name")
	assertConfigSecretVolume(t, volumes, "foo", "secret-foo")
	assertConfigSecretVolume(t, volumes, "bar", "secret-bar")
	assertEmptyDirVolume(t, volumes)

	t.Log("Checking main container envs")
	envs := getMainContainer(*pod).Env
	matcher.Expect(len(envs)).To(gomega.Equal(3))
	matcher.Expect(envs).To(gomega.ContainElement(corev1.EnvVar{Name: "FOO", Value: "bar"}))

	t.Log("Checking main container mounts")
	mainContainerMounts := getMainContainer(*pod).VolumeMounts
	matcher.Expect(mainContainerMounts).To(gomega.HaveLen(5))
	assertConfigMapVolumeMount(t, mainContainerMounts)
	assertClientsSecretVolumeMount(t, mainContainerMounts)
	assertConfigSecretVolumeMount(t, mainContainerMounts, "foo")
	assertConfigSecretVolumeMount(t, mainContainerMounts, "bar")
	assertEmptyDirVolumeMount(t, mainContainerMounts)

	t.Log("Checking init container mounts")
	initContainerMounts := pod.Spec.InitContainers[0].VolumeMounts
	matcher.Expect(initContainerMounts).To(gomega.HaveLen(1))
	assertEmptyDirVolumeMount(t, initContainerMounts)
}

func TestMakePodSpecProtocolsAndProbes(t *testing.T) {
	matcher := gomega.NewWithT(t)

	startupProbe := corev1.Probe{ProbeHandler: corev1.ProbeHandler{Exec: &corev1.ExecAction{Command: []string{"startup"}}}}
	livenessProbe := corev1.Probe{ProbeHandler: corev1.ProbeHandler{Exec: &corev1.ExecAction{Command: []string{"liveness"}}}}

	imageName := "foo_image"
	implementation := "foo_impl"
	name := "foo_datamover"
	dmSession := api.DatamoverSession{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: api.DatamoverSessionSpec{
			Implementation: implementation,
			LifecycleConfig: &api.LifecycleConfig{
				Image: imageName,
				ServicePorts: []corev1.ServicePort{
					{Name: "foo", Port: 1},
					{Name: "bar", Port: 2},
				},
				StartupProbe:  &startupProbe,
				LivenessProbe: &livenessProbe,
			},
		},
	}

	pod, err := MakePodSpec(dmSession)
	t.Log("pod", pod)

	matcher.Expect(err).To(gomega.BeNil())

	podChecksNoExtraContainers(t, pod, name, imageName, implementation)

	matcher.Expect(getMainContainer(*pod).Env).To(gomega.ContainElement(corev1.EnvVar{Name: "PROTOCOLS", Value: "foo:1;bar:2"}))
	matcher.Expect(*getMainContainer(*pod).StartupProbe).To(gomega.Equal(startupProbe))
	matcher.Expect(*getMainContainer(*pod).LivenessProbe).To(gomega.Equal(livenessProbe))
}

func TestMakePodSpecPodOptionsBase(t *testing.T) {
	matcher := gomega.NewWithT(t)

	trueVal := true
	truePtr := &trueVal
	intVal := int64(1)
	intPtr := &intVal
	resources := corev1.ResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("100")}}
	labels := map[string]string{"label_name": "label_val"}
	annotations := map[string]string{"ann_name": "ann_val"}
	priorityClassName := "class"
	podSecContext := corev1.PodSecurityContext{RunAsNonRoot: truePtr}
	conSecContext := corev1.SecurityContext{RunAsUser: intPtr}
	serviceAccountName := "foo_acct"

	imageName := "foo_image"
	implementation := "foo_impl"
	name := "foo_datamover"
	dmSession := api.DatamoverSession{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: api.DatamoverSessionSpec{
			Implementation: implementation,
			LifecycleConfig: &api.LifecycleConfig{
				Image: imageName,
				PodOptions: api.PodOptions{
					Resources:                resources,
					Labels:                   labels,
					Annotations:              annotations,
					PriorityClassName:        priorityClassName,
					PodSecurityContext:       &podSecContext,
					ContainerSecurityContext: &conSecContext,
					ShareProcessNamespace:    truePtr,
					ServiceAccount:           serviceAccountName,
					ImagePullPolicy:          corev1.PullIfNotPresent,
				},
			},
		},
	}

	pod, err := MakePodSpec(dmSession)
	t.Log("pod", pod)

	matcher.Expect(err).To(gomega.BeNil())

	podChecksNoExtraContainers(t, pod, name, imageName, implementation)

	matcher.Expect(getMainContainer(*pod).Env).To(gomega.ContainElement(corev1.EnvVar{Name: "PROTOCOLS", Value: ""}))

	matcher.Expect(pod.ObjectMeta.Labels).To(gstruct.MatchKeys(gstruct.IgnoreExtras, gstruct.Keys{
		"label_name": gomega.Equal("label_val"),
	}))
	matcher.Expect(pod.ObjectMeta.Annotations).To(gstruct.MatchKeys(gstruct.IgnoreExtras, gstruct.Keys{
		"ann_name": gomega.Equal("ann_val"),
	}))

	matcher.Expect(getMainContainer(*pod).Resources).To(gomega.Equal(resources))
	matcher.Expect(pod.Spec.PriorityClassName).To(gomega.Equal(priorityClassName))
	matcher.Expect(pod.Spec.ShareProcessNamespace).To(gomega.Equal(truePtr))
	matcher.Expect(pod.Spec.ServiceAccountName).To(gomega.Equal(serviceAccountName))
	matcher.Expect(pod.Spec.AutomountServiceAccountToken).To(gomega.Equal(truePtr))
	matcher.Expect(getMainContainer(*pod).ImagePullPolicy).To(gomega.Equal(corev1.PullIfNotPresent))

	matcher.Expect(*pod.Spec.SecurityContext).To(gomega.Equal(podSecContext))
	matcher.Expect(*getMainContainer(*pod).SecurityContext).To(gomega.Equal(conSecContext))
}

func TestMakePodSpecPodOptionsExtraResources(t *testing.T) {
	matcher := gomega.NewWithT(t)

	extraContainer := corev1.Container{
		Name:  "sidecar",
		Image: "foo",
	}

	extraVolume := corev1.Volume{
		Name: "extra_tmp",
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	}

	imageName := "foo_image"
	implementation := "foo_impl"
	name := "foo_datamover"
	dmSession := api.DatamoverSession{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: api.DatamoverSessionSpec{
			Implementation: implementation,
			LifecycleConfig: &api.LifecycleConfig{
				Image: imageName,
				PodOptions: api.PodOptions{
					ExtraVolumes:    []corev1.Volume{extraVolume},
					ExtraContainers: []corev1.Container{extraContainer},
				},
			},
		},
	}
	pod, err := MakePodSpec(dmSession)
	t.Log("pod", pod)

	matcher.Expect(err).To(gomega.BeNil())

	basePodChecks(t, pod, name, imageName, implementation)

	matcher.Expect(getMainContainer(*pod).VolumeMounts).To(gomega.HaveLen(2))
	matcher.Expect(pod.Spec.Volumes).To(gomega.HaveLen(2))

	matcher.Expect(pod.Spec.Volumes).To(gomega.ContainElement(extraVolume))
	assertCustomEmptyDirVolumeMount(t, getMainContainer(*pod).VolumeMounts, "extra_tmp")
	assertExtraContainer(t, *pod, extraContainer)
}

func TestMakePodSpecPodOptionsPodOverride(t *testing.T) {
	matcher := gomega.NewWithT(t)

	imageName := "foo_image"
	implementation := "foo_impl"
	name := "foo_datamover"
	dmSession := api.DatamoverSession{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: api.DatamoverSessionSpec{
			Implementation: implementation,
			LifecycleConfig: &api.LifecycleConfig{
				Image: imageName,
				PodOptions: api.PodOptions{
					PodOverride: api.PodOverride{
						"containers": []map[string]string{
							{
								"name":  "extra_container",
								"image": "foo",
							},
							{
								"name":  api.DefaultContainerName,
								"image": "image_override",
							}},
						"serviceAccountName": "override_sa",
						"imagePullSecrets": []map[string]string{
							{
								"name": "image_pull_secret",
							}},
					},
				},
			},
		},
	}
	pod, err := MakePodSpec(dmSession)
	t.Log("pod", pod)

	matcher.Expect(err).To(gomega.BeNil())

	// Labels are unchanged
	assertDefaultLabels(t, pod, name)
	assertDefaultRestartPolicy(t, pod)

	// Did not override init containers
	assertDefaultInitContainerChecks(t, pod)

	matcher.Expect(pod.Spec.Containers).To(gomega.HaveLen(2))
	matcher.Expect(getMainContainer(*pod).Image).To(gomega.Equal("image_override"))

	matcher.Expect(pod.Spec.ServiceAccountName).To(gomega.Equal("override_sa"))
	matcher.Expect(pod.Spec.Containers).To(gomega.ContainElement(corev1.Container{Name: "extra_container", Image: "foo"}))

	matcher.Expect(pod.Spec.ImagePullSecrets).To(gomega.ContainElement(corev1.LocalObjectReference{Name: "image_pull_secret"}))
}

func assertExtraContainer(t *testing.T, pod corev1.Pod, extraContainer corev1.Container) {
	matcher := gomega.NewWithT(t)
	for _, container := range pod.Spec.Containers {
		if container.Name == extraContainer.Name {
			matcher.Expect(container).To(gomega.Equal(extraContainer))
			return
		}
	}
	t.Log("Cannot find extra container")
	t.Fail()
}

func assertConfigMapVolume(t *testing.T, volumes []corev1.Volume, cmName string) {
	matcher := gomega.NewWithT(t)
	for _, vol := range volumes {
		// FIXME: this should be a const
		if vol.Name == "config" {
			matcher.Expect(vol.VolumeSource.ConfigMap).To(gomega.Not(gomega.BeNil()))
			matcher.Expect(vol.VolumeSource.ConfigMap.Name).To(gomega.Equal(cmName))
			return
		}
	}
	t.Log("Config map voume not found")
	t.Fail()
}

func assertClientsSecretVolume(t *testing.T, volumes []corev1.Volume, secretName string) {
	matcher := gomega.NewWithT(t)
	for _, vol := range volumes {
		// FIXME: this should be a const
		if vol.Name == "client-creds" {
			matcher.Expect(vol.VolumeSource.Secret).To(gomega.Not(gomega.BeNil()))
			matcher.Expect(vol.VolumeSource.Secret.SecretName).To(gomega.Equal(secretName))
			return
		}
	}
	t.Log("Clients secret voume not found")
	t.Fail()
}

func assertConfigSecretVolume(t *testing.T, volumes []corev1.Volume, volumeName string, secretName string) {
	matcher := gomega.NewWithT(t)
	for _, vol := range volumes {
		// FIXME: this should be a const
		if vol.Name == volumeName {
			matcher.Expect(vol.VolumeSource.Secret).To(gomega.Not(gomega.BeNil()))
			matcher.Expect(vol.VolumeSource.Secret.SecretName).To(gomega.Equal(secretName))
			return
		}
	}
	t.Log("Secret voume not found")
	t.Fail()
}

func assertEmptyDirVolume(t *testing.T, volumes []corev1.Volume) {
	matcher := gomega.NewWithT(t)
	for _, vol := range volumes {
		// FIXME: this should be a const
		if vol.Name == "session-data" {
			matcher.Expect(vol.VolumeSource.EmptyDir).To(gomega.Not(gomega.BeNil()))
			return
		}
	}
	t.Log("Empty dir voume not found")
	t.Fail()
}

func assertConfigMapVolumeMount(t *testing.T, mounts []corev1.VolumeMount) {
	matcher := gomega.NewWithT(t)
	for _, mount := range mounts {
		// FIXME: const
		if mount.Name == "config" {
			matcher.Expect(mount.MountPath).To(gomega.Equal("/etc/config"))
			return
		}
	}
	t.Log("Config map voume mount not found")
	t.Fail()
}

func assertClientsSecretVolumeMount(t *testing.T, mounts []corev1.VolumeMount) {
	matcher := gomega.NewWithT(t)
	for _, mount := range mounts {
		// FIXME: const
		if mount.Name == "client-creds" {
			matcher.Expect(mount.MountPath).To(gomega.Equal("/etc/client_credentials"))
			return
		}
	}
	t.Log("Credentials secret voume mount not found")
	t.Fail()
}

func assertConfigSecretVolumeMount(t *testing.T, mounts []corev1.VolumeMount, volumeName string) {
	matcher := gomega.NewWithT(t)
	for _, mount := range mounts {
		// FIXME: const
		if mount.Name == volumeName {
			matcher.Expect(mount.MountPath).To(gomega.Equal(path.Join("/etc/secrets", volumeName)))
			return
		}
	}
	t.Log("Secret voume mount not found")
	t.Fail()
}

func assertEmptyDirVolumeMount(t *testing.T, mounts []corev1.VolumeMount) {
	matcher := gomega.NewWithT(t)
	for _, mount := range mounts {
		// FIXME: const
		if mount.Name == "session-data" {
			matcher.Expect(mount.MountPath).To(gomega.Equal("/etc/session"))
			return
		}
	}
	t.Log("Empty dir voume mount not found")
	t.Fail()
}

func assertCustomEmptyDirVolumeMount(t *testing.T, mounts []corev1.VolumeMount, volumeName string) {
	matcher := gomega.NewWithT(t)
	for _, vol := range mounts {
		// FIXME: this should be a const
		if vol.Name == volumeName {
			matcher.Expect(vol.MountPath).To(gomega.Equal("/mnt/volumes/extra_tmp"))
			return
		}
	}
	t.Log("Empty dir voume not found")
	t.Fail()
}
