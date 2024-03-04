package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// MachineConfigPoolCondition contains condition information for an MachineConfigPool.
type PlatformCondition struct {
	// type of the condition, currently ('Done', 'Updating', 'Failed').
	// +optional
	Type PlatformConditionType `json:"type"`

	// status of the condition, one of ('True', 'False', 'Unknown').
	// +optional
	Status corev1.ConditionStatus `json:"status"`

	// lastTransitionTime is the timestamp corresponding to the last status
	// change of this condition.
	// +nullable
	// +optional
	LastTransitionTime metav1.Time `json:"lastTransitionTime"`

	// reason is a brief machine readable explanation for the condition's last
	// transition.
	// +optional
	Reason string `json:"reason"`

	// message is a human readable description of the details of the last
	// transition, complementing reason.
	// +optional
	Message string `json:"message"`
}

// MachineConfigPoolConditionType valid conditions of a MachineConfigPool
type PlatformConditionType string

const (
	// MachineConfigPoolUpdated means MachineConfigPool is updated completely.
	// When the all the machines in the pool are updated to the correct machine config.
	PlatformCreated PlatformConditionType = "Created"

	// MachineConfigPoolUpdating means MachineConfigPool is updating.
	// When at least one of machine is not either not updated or is in the process of updating
	// to the desired machine config.
	PlatformUpdating PlatformConditionType = "Updating"

	PlatformFailed PlatformConditionType = "Failed"
)
