package podoverride

import (
	"encoding/json"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/strategicpatch"
)

func OverridePodSpec(podSpec corev1.PodSpec, override map[string]any) (corev1.PodSpec, error) {
	// Merge default specs and override specs with StrategicMergePatch
	mergedPatch, err := strategicMergeJSONPatch(podSpec, override)
	if err != nil {
		return corev1.PodSpec{}, err
	}

	// Convert merged json to corev1.PodSPec object
	newPodSpec := corev1.PodSpec{}
	err = json.Unmarshal(mergedPatch, &newPodSpec)
	if err != nil {
		return newPodSpec, err
	}
	return newPodSpec, err
}

func strategicMergeJSONPatch(original, override interface{}) ([]byte, error) {
	// Convert override specs to json
	overrideJSON, err := json.Marshal(override)
	if err != nil {
		return nil, err
	}

	// Convert original specs to json
	originalJSON, err := json.Marshal(original)
	if err != nil {
		return nil, err
	}

	// Merge json specs with StrategicMerge
	mergedPatch, err := strategicpatch.StrategicMergePatch(originalJSON, overrideJSON, corev1.PodSpec{})
	if err != nil {
		return nil, err
	}
	return mergedPatch, nil
}
