/*
Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/strategicpatch"
)

const (
	DatamoverSessionKind = "DatamoverSession"
)

// TODO: make the whole thing immutable

// DatamoverSessionSpec defines the desired state of DatamoverSession
type DatamoverSessionSpec struct {
	Implementation string `json:"implementation"`

	// Configmap in the same namespace referencing implementation specific configuration
	// This configmap will be mointed to /etc/config dir
	Configuration *corev1.ConfigMapVolumeSource `json:"config,omitempty"`
	// A list of secrets to extend implementation specific configuration
	// These secrets will be mounted to /etc/secrets/<secret-name> dirs
	ConfigurationSecrets map[string]corev1.SecretVolumeSource `json:"secrets,omitempty"`
	// ClientSecretRef contains client credentials information
	// This secret will be mounted to /etc/client_credentials dir
	ClientSecretRef *corev1.SecretVolumeSource `json:"clientSecretRef,omitempty"`
	// Implementation specific env variables to pass to the session pod
	Env map[string]string `json:"env,omitempty"`

	//TODO: dynamic configmap separate from the main config??

	LifecycleConfig *LifecycleConfig `json:"lifecycle,omitempty"`
}

type LifecycleConfig struct {
	Image string `json:"image"`
	// Ports to expose via service, service will not be created if empty
	ServicePorts []corev1.ServicePort `json:"servicePorts,omitempty"`

	// NetworkPolicy controls whether network policy should be created
	NetworkPolicy NetworkPolicyConfig `json:"networkPolicy,omitempty"`

	// TODO: configurable sidecar readiness timeouts?

	// Extra configurations to pass to session pod
	PodOptions PodOptions `json:"podOptions,omitempty"`

	// Startup probe to control datamover session lifecycle
	// More info: https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle#container-probes
	StartupProbe *corev1.Probe `json:"startupProbe,omitempty"`
	// Liveness probe to control datamover session lifecycle
	// More info: https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle#container-probes
	LivenessProbe *corev1.Probe `json:"livenessProbe,omitempty"`
}

type NetworkPolicyConfig struct {
	Enabled bool `json:"enabled,omitempty"`

	From []networkingv1.NetworkPolicyPeer `json:"from,omitempty"`
}

type PodOptions struct {
	// Fine tune resources of the pod
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`

	// Additional volumes to be mounted to the session pod
	ExtraVolumes []corev1.Volume `json:"extraVolumes,omitempty"`

	// Labels to add to the session pod
	Labels map[string]string `json:"labels,omitempty"`
	// Annotations to add to the session pod
	Annotations map[string]string `json:"annotations,omitempty"`

	// Additional containers to run in the pod
	ExtraContainers []corev1.Container `json:"extraContainers,omitempty"`

	// FIXME: support sidecar (restarting init) containers

	// Pod priorityClassName
	PriorityClassName string `json:"priorityClassName,omitempty"`

	// Pod security context
	PodSecurityContext       *corev1.PodSecurityContext `json:"podSecurityContext,omitempty"`
	ContainerSecurityContext *corev1.SecurityContext    `json:"containerSecurityContext,omitempty"`

	ShareProcessNamespace *bool  `json:"shareProcessNamespace,omitempty"`
	ServiceAccount        string `json:"serviceAccount,omitempty"`

	ImagePullPolicy corev1.PullPolicy `json:"imagePullPolicy,omitempty"`

	// temporary support for podOverride overriding the entire thing
	// TODO: rework when addressing podOverride rework
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Schemaless
	PodOverride PodOverride `json:"podOverride,omitempty"`
}

type PodOverride strategicpatch.JSONMap

// DatamoverSessionStatus defines the observed state of DatamoverSession
type DatamoverSessionStatus struct {
	SessionInfo SessionInfo              `json:"sessionInfo,omitempty"`
	Progress    DatamoverSessionProgress `json:"progress,omitempty"`
}

// SessionInfo contains information to generate endpoint URL to connect to
type SessionInfo struct {
	PodName     string `json:"podName,omitempty"`
	ServiceName string `json:"serviceName,omitempty"`
	// TODO: we can also record timestamp on when data was last updated
	// taking it from the file metadata
	SessionData string `json:"data,omitempty"`
}

// DatamoverSessionProgress is the field users would check to know the state of DatamoverSession
type DatamoverSessionProgress string

// TODO: this could be expressed with conditions
const (
	ProgressNone             DatamoverSessionProgress = ""
	ProgressValidationFailed DatamoverSessionProgress = "ValidationFailed"
	ProgressResourcesCreated DatamoverSessionProgress = "ResourcesCreated"
	ProgressReadinessFailure DatamoverSessionProgress = "ReadinessFailure"
	ProgressReady            DatamoverSessionProgress = "Ready"
	ProgressSessionFailure   DatamoverSessionProgress = "SessionFailure"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// DatamoverSession is the Schema for the datamoversessions API
type DatamoverSession struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="Value is immutable"
	Spec   DatamoverSessionSpec   `json:"spec,omitempty"`
	Status DatamoverSessionStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
// DatamoverSessionList contains a list of DatamoverSession
type DatamoverSessionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DatamoverSession `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DatamoverSession{}, &DatamoverSessionList{})
}
