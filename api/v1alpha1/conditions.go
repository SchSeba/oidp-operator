package v1alpha1

// MachineConfigPoolConditionType valid conditions of a MachineConfigPool
type PlatformConditionType string

const (
	PlatformRunning PlatformConditionType = "Creating"
	PlatformCreated PlatformConditionType = "Created"
	PlatformFailed  PlatformConditionType = "Failed"
)
