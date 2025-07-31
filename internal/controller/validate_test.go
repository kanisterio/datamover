package controller

import (
	api "github.com/kanisterio/datamover/api/v1alpha1"
	"testing"
)

func TestValidatePassNoLifecycle(t *testing.T) {
	session := api.DatamoverSession{}
	err := validateSession(session)
	if err != nil {
		t.Errorf("Validation failed %v", err)
	}
}

func TestValidateFailLifecycleNoImplementation(t *testing.T) {
	session := api.DatamoverSession{
		Spec: api.DatamoverSessionSpec{
			LifecycleConfig: &api.LifecycleConfig{},
		},
	}
	err := validateSession(session)
	if err == nil {
		t.Errorf("Validation without implementation value passed, but should have failed")
	}
}

func TestValidatePassLifecycleNoLabels(t *testing.T) {
	session := api.DatamoverSession{
		Spec: api.DatamoverSessionSpec{
			Implementation: "foo",
			LifecycleConfig: &api.LifecycleConfig{
				Image: "image",
			},
		},
	}
	err := validateSession(session)
	if err != nil {
		t.Errorf("Validation failed %v", err)
	}
}

func TestValidatePassLifecycleValidLabels(t *testing.T) {
	session := api.DatamoverSession{
		Spec: api.DatamoverSessionSpec{
			Implementation: "foo",
			LifecycleConfig: &api.LifecycleConfig{
				Image: "image",
				PodOptions: api.PodOptions{
					Labels: map[string]string{
						"my_label": "value",
					},
				},
			},
		},
	}
	err := validateSession(session)
	if err != nil {
		t.Errorf("Validation failed %v", err)
	}
}

func TestValidateFailLifecycleInValidLabels(t *testing.T) {
	session := api.DatamoverSession{
		Spec: api.DatamoverSessionSpec{
			Implementation: "foo",
			LifecycleConfig: &api.LifecycleConfig{
				PodOptions: api.PodOptions{
					Labels: map[string]string{
						datamoverSessionSelectorLabel: "value",
					},
				},
			},
		},
	}
	err := validateSession(session)
	if err == nil {
		t.Errorf("Validation with %s label passed, but should have failed", datamoverSessionSelectorLabel)
	}

	session = api.DatamoverSession{
		Spec: api.DatamoverSessionSpec{
			Implementation: "foo",
			LifecycleConfig: &api.LifecycleConfig{
				PodOptions: api.PodOptions{
					Labels: map[string]string{
						datamoverSessionLabel: "value",
					},
				},
			},
		},
	}
	err = validateSession(session)
	if err == nil {
		t.Errorf("Validation with %v label passed, but should have failed", datamoverSessionLabel)
	}
}

// If lifecycle is not enabled, PROTOCOLS env is allowed
func TestValidatePassNoLifecycleInValidEnv(t *testing.T) {
	session := api.DatamoverSession{
		Spec: api.DatamoverSessionSpec{
			Env: map[string]string{
				api.ProtocolsEnvVarName: "foo",
			},
		},
	}
	err := validateSession(session)
	if err != nil {
		t.Errorf("Validation failed")
	}
}

// If lifecycle is enabled, PROTOCOLS env is not allowed
func TestValidateFailLifecycleInValidEnv(t *testing.T) {
	session := api.DatamoverSession{
		Spec: api.DatamoverSessionSpec{
			Implementation: "foo",
			Env: map[string]string{
				api.ProtocolsEnvVarName: "foo",
			},
			LifecycleConfig: &api.LifecycleConfig{},
		},
	}
	err := validateSession(session)
	if err == nil {
		t.Errorf("Validation with %v env passed, but should have failed", api.ProtocolsEnvVarName)
	}
}
