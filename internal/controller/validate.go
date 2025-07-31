package controller

import (
	"fmt"

	api "github.com/kanisterio/datamover/api/v1alpha1"
)

func validateSession(dmSession api.DatamoverSession) error {
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

		err = validateSessionForPod(dmSession)
		if err != nil {
			return err
		}
	}
	return nil
}

func validatePodLabels(dmSession api.DatamoverSession) error {
	if dmSession.Spec.LifecycleConfig.PodOptions.Labels[datamoverSessionSelectorLabel] != "" {
		return fmt.Errorf("Label %s not allowed", datamoverSessionSelectorLabel)
	}

	if dmSession.Spec.LifecycleConfig.PodOptions.Labels[datamoverSessionLabel] != "" {
		return fmt.Errorf("Label %s not allowed", datamoverSessionLabel)
	}
	return nil
}

func validateEnvs(dmSession api.DatamoverSession) error {
	if dmSession.Spec.Env[api.ProtocolsEnvVarName] != "" {
		return fmt.Errorf("Env %s not allowed", api.ProtocolsEnvVarName)
	}
	return nil
}
