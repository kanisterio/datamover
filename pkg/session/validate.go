package session

import (
	"fmt"

	"errors"
	api "github.com/kanisterio/datamover/api/v1alpha1"
)

func ValidateSession(dmSession api.DatamoverSession) error {
	// TODO: better chained validation if end up adding more validators
	// TODO: support validation of non-lifecycle sessions??
	if dmSession.Spec.LifecycleConfig != nil {
		err := validateEnvs(dmSession)
		if err != nil {
			return err
		}

		err = validatePodLabels(dmSession)
		if err != nil {
			return err
		}

		err = ValidateSessionForPod(dmSession)
		if err != nil {
			return err
		}
		err = validateNetworkPolicyConfig(dmSession)
		if err != nil {
			return err
		}
	}
	return nil
}

func validatePodLabels(dmSession api.DatamoverSession) error {
	if dmSession.Spec.LifecycleConfig.PodOptions.Labels[api.DatamoverSessionSelectorLabel] != "" {
		return fmt.Errorf("Label %s not allowed", api.DatamoverSessionSelectorLabel)
	}

	if dmSession.Spec.LifecycleConfig.PodOptions.Labels[api.DatamoverSessionLabel] != "" {
		return fmt.Errorf("Label %s not allowed", api.DatamoverSessionLabel)
	}
	return nil
}

func validateEnvs(dmSession api.DatamoverSession) error {
	if dmSession.Spec.Env[api.ProtocolsEnvVarName] != "" {
		return fmt.Errorf("Env %s not allowed", api.ProtocolsEnvVarName)
	}
	return nil
}

func validateNetworkPolicyConfig(dmSession api.DatamoverSession) error {
	if dmSession.Spec.LifecycleConfig.NetworkPolicy.Enabled {
		if len(dmSession.Spec.LifecycleConfig.ServicePorts) == 0 {
			return errors.New("ServicePorts should be set to create a network policy")
		}
	}
	return nil
}

func ValidateSessionForPod(dmSession api.DatamoverSession) error {
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
