package v1alpha1

const (
	ProtocolsEnvVarName      = "PROTOCOLS"
	ImplementationEnvVarName = "DATAMOVER_NAME"
	DefaultContainerName     = "main"
	// FIXME: domain for labels
	DatamoverSessionSelectorLabel = "datamover/service_label"
	DatamoverSessionLabel         = "datamover/session"
)
