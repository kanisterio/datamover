package client

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/pkg/errors"

	api "github.com/kanisterio/datamover/api/v1alpha1"
	"github.com/kanisterio/datamover/pkg/podoverride"
	"github.com/kanisterio/datamover/pkg/session"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

const NamePrefix string = "dm-client-"
const MainContainerName string = "container"

type ClientCredentialsConfig interface {
	GetVolumes() ([]corev1.Volume, []corev1.VolumeMount, error)
}

type ClientCredentialsSecret struct {
	SecretName string
}

const (
	tokenProjectionPath     = "token"
	tokenProjectionAudience = "datamover"
	tokenMoutName           = "client-token"
	tokenMountPath          = "/etc/client-token"
)

const (
	secretMountName = "client-secret"
	secretMountPath = "/etc/client-secret"
)

const (
	envSessionURL  = "SESSION_URL"
	envSessionData = "SESSION_DATA"
)

type ClientCredentialsToken struct {
	ExpirationSeconds *int64
}

func (config ClientCredentialsSecret) GetVolumes() ([]corev1.Volume, []corev1.VolumeMount, error) {
	if config.SecretName == "" {
		return nil, nil, errors.New("Credential secret is empty")
	}
	volumes := []corev1.Volume{}
	volumeMounts := []corev1.VolumeMount{}

	if config.SecretName == "" {
		return nil, nil, errors.New("SecretName should be set for CredentialsMode == secret")
	}
	volumes = append(volumes, corev1.Volume{
		Name: secretMountName,
		VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				SecretName: config.SecretName,
			},
		},
	})
	volumeMounts = append(volumeMounts, corev1.VolumeMount{
		Name:      secretMountName,
		MountPath: secretMountPath,
	})
	return volumes, volumeMounts, nil
}

func (config ClientCredentialsToken) GetVolumes() ([]corev1.Volume, []corev1.VolumeMount, error) {
	volumes := []corev1.Volume{}
	volumeMounts := []corev1.VolumeMount{}
	volumes = append(volumes, corev1.Volume{
		Name: tokenMoutName,
		VolumeSource: corev1.VolumeSource{
			Projected: &corev1.ProjectedVolumeSource{
				Sources: []corev1.VolumeProjection{
					{
						ServiceAccountToken: &corev1.ServiceAccountTokenProjection{
							Path:              tokenProjectionPath,
							ExpirationSeconds: config.ExpirationSeconds,
							Audience:          tokenProjectionAudience,
						},
					},
				},
			},
		},
	})
	volumeMounts = append(volumeMounts, corev1.VolumeMount{
		Name:      tokenMoutName,
		MountPath: tokenMountPath,
	})
	// Emptydir volume for client secret so the token auth can put credentials here
	volumes = append(volumes, corev1.Volume{
		Name: secretMountName,
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{
				Medium: corev1.StorageMediumMemory,
			},
		},
	})
	volumeMounts = append(volumeMounts, corev1.VolumeMount{
		Name:      secretMountName,
		MountPath: secretMountPath,
	})
	return volumes, volumeMounts, nil
}

type CreateClientArgs struct {
	Operation         Operation
	Namespace         string
	Image             string
	SessionNamespace  string
	SessionName       string
	GenerateName      string
	ConfigMap         *string
	Secrets           map[string]string
	CredentialsConfig ClientCredentialsConfig
	Env               []corev1.EnvVar
	PodOptions        api.PodOptions
}

func CreateClientPod(
	ctx context.Context,
	cli kubernetes.Interface,
	dynCli dynamic.Interface,
	clientArgs CreateClientArgs,
) (*corev1.Pod, error) {
	// FIXME: require readiness check beforehand instead of waiting here
	sessionConfig, err := session.GetConfig(ctx, dynCli, clientArgs.SessionName, clientArgs.SessionNamespace)
	if err != nil {
		return nil, errors.Wrap(err, "Cannot extract datamover session config")
	}

	pod, err := MakeClientPod(clientArgs, *sessionConfig)
	if err != nil {
		return nil, errors.Wrap(err, "Failed to generate client pod spec")
	}
	pod, err = cli.CoreV1().Pods(clientArgs.Namespace).Create(ctx, pod, metav1.CreateOptions{})
	if err != nil {
		return nil, errors.Wrap(err, "Failed to run client pod")
	}
	return pod, nil
}

func MakeClientPod(
	clientArgs CreateClientArgs,
	sessionConfig session.SessionConfig,
) (*corev1.Pod, error) {
	secretVolumes, secretVolumeMounts := makeSecretVolumes(clientArgs.Secrets)

	clientVolumes, clientVolumeMounts, err := clientArgs.CredentialsConfig.GetVolumes()
	if err != nil {
		return nil, errors.Wrap(err, "Failed to generate credentials volumes config")
	}

	configVolumes, configVolumeMounts := makeConfigVolumes(clientArgs.ConfigMap)

	operationVolumes, operationVolumeMounts, operationVolumeDevices := clientArgs.Operation.MakeVolumes()

	// TODO: maybe replace configmap controls with envs???
	argsEnvs := clientArgs.Env

	envs := slices.Concat(sessionConfigEnvs(sessionConfig), argsEnvs)

	args := clientArgs.Operation.MakeArgs()

	extraVolumes := clientArgs.PodOptions.ExtraVolumes
	extraVolumeMounts := []corev1.VolumeMount{}
	for _, vol := range extraVolumes {
		extraVolumeMounts = append(extraVolumeMounts, corev1.VolumeMount{
			Name:      vol.Name,
			MountPath: "/mnt/volumes/" + vol.Name,
		})
	}

	// TODO: handle name clashes in volumes better
	err = validateVolumeNames([][]corev1.Volume{secretVolumes, clientVolumes, operationVolumes, configVolumes, extraVolumes})
	if err != nil {
		return nil, err
	}

	volumes := slices.Concat(secretVolumes, clientVolumes, operationVolumes, configVolumes, extraVolumes)

	volumeMounts := slices.Concat(secretVolumeMounts, clientVolumeMounts, operationVolumeMounts, configVolumeMounts, extraVolumeMounts)

	operationContainers := clientArgs.Operation.MakeContainers()
	operationInitContainers := clientArgs.Operation.MakeInitContainers()

	mainContainer := corev1.Container{
		Name:          MainContainerName,
		Image:         clientArgs.Image,
		Args:          args,
		Env:           envs,
		VolumeMounts:  volumeMounts,
		VolumeDevices: operationVolumeDevices,

		SecurityContext: clientArgs.PodOptions.ContainerSecurityContext,
		Resources:       clientArgs.PodOptions.Resources,
	}

	containers := slices.Concat(
		[]corev1.Container{mainContainer},
		operationContainers,
		clientArgs.PodOptions.ExtraContainers)

	// TODO: can we use pod runner??
	podSpec := corev1.PodSpec{
		RestartPolicy: corev1.RestartPolicyNever,
		Volumes:       volumes,

		Containers:     containers,
		InitContainers: operationInitContainers,

		SecurityContext:    clientArgs.PodOptions.PodSecurityContext,
		PriorityClassName:  clientArgs.PodOptions.PriorityClassName,
		ServiceAccountName: clientArgs.PodOptions.ServiceAccount,
	}

	genName := clientArgs.GenerateName
	if genName == "" {
		genName = NamePrefix
	}

	// podOverride to make it work with the K10 prototype code
	// TODO: rework when reviewing podOverride/podOptions
	podSpec, err = podoverride.OverridePodSpec(podSpec, clientArgs.PodOptions.PodOverride)
	if err != nil {
		return nil, err
	}

	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: genName,
			Namespace:    clientArgs.Namespace,
			Labels:       clientArgs.PodOptions.Labels,
			Annotations:  clientArgs.PodOptions.Annotations,
		},
		Spec: podSpec,
	}, nil
}

func sessionConfigEnvs(sessionConfig session.SessionConfig) []corev1.EnvVar {
	service := sessionConfig.Service
	url := ""
	protocols := ""
	if service != nil {
		ports := service.Spec.Ports
		stringProtocols := []string{}
		for _, port := range ports {
			stringProtocols = append(stringProtocols, port.Name+":"+strconv.Itoa(int(port.Port)))
		}
		protocols = strings.Join(stringProtocols, ";")

		url = fmt.Sprintf("%s.%s.svc.cluster.local", service.Name, service.Namespace)
	}

	return []corev1.EnvVar{
		{Name: api.ImplementationEnvVarName, Value: sessionConfig.Implementation},
		// TODO: do we want to validate that session url is set?
		{Name: envSessionURL, Value: url},
		{Name: api.ProtocolsEnvVarName, Value: protocols},
		// TODO: currently passing session data as an env variable
		// We could have used a config map, but it's a single use pod
		// and using extra resources would be harder to maintain
		{Name: envSessionData, Value: sessionConfig.SessionData},
	}
}

func validateVolumeNames(volumeLists [][]corev1.Volume) error {
	uniqNames := map[string]bool{}
	for _, list := range volumeLists {
		for _, volume := range list {
			if uniqNames[volume.Name] {
				return fmt.Errorf("Duplicate volume name: %s", volume.Name)
			} else {
				uniqNames[volume.Name] = true
			}
		}
	}
	return nil
}

func makeSecretVolumes(secrets map[string]string) ([]corev1.Volume, []corev1.VolumeMount) {
	secretVolumes := []corev1.Volume{}
	secretVolumeMounts := []corev1.VolumeMount{}
	for name, secret := range secrets {
		vol := corev1.Volume{
			Name: name,
			VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{
				SecretName: secret,
			}},
		}
		mount := corev1.VolumeMount{
			Name:      name,
			MountPath: "/etc/secrets/" + name,
		}
		secretVolumes = append(secretVolumes, vol)
		secretVolumeMounts = append(secretVolumeMounts, mount)
	}
	return secretVolumes, secretVolumeMounts
}

func makeConfigVolumes(configMap *string) ([]corev1.Volume, []corev1.VolumeMount) {
	configVolumes := []corev1.Volume{}
	configVolumeMounts := []corev1.VolumeMount{}
	if configMap != nil {
		configVolumes = append(configVolumes, corev1.Volume{
			Name: "config",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: *configMap,
					},
				},
			},
		})
		configVolumeMounts = append(configVolumeMounts, corev1.VolumeMount{
			Name:      "config",
			MountPath: "/etc/config",
		})
	}
	return configVolumes, configVolumeMounts
}
