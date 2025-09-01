package k8s

import (
	"time"
	
	corev1 "k8s.io/api/core/v1"
)

// ForwardingState represents the state of port forwarding
type ForwardingState int

const (
	ForwardingStateInactive ForwardingState = iota
	ForwardingStatePending
	ForwardingStateActive
	ForwardingStateFailed
)

// String returns a string representation of the forwarding state
func (s ForwardingState) String() string {
	switch s {
	case ForwardingStateInactive:
		return "inactive"
	case ForwardingStatePending:
		return "pending"
	case ForwardingStateActive:
		return "active"
	case ForwardingStateFailed:
		return "failed"
	default:
		return "unknown"
	}
}

type ServiceInfo struct {
	Name           string
	Namespace      string
	Type           string
	Ports          []PortInfo
	Service        *corev1.Service
	IsForwarding   bool
	ForwardingPort int
}

type PortInfo struct {
	Name              string
	Port              int32
	TargetPort        int32
	Protocol          string
	ForwardingState   ForwardingState
	ForwardingPort    int
	ForwardStartTime  time.Time
	FailureReason     string
	FailureTime       time.Time
}

type DeploymentInfo struct {
	Name            string
	Replicas        int32
	ReadyReplicas   int32
	UpdatedReplicas int32
	Image           string
	CreatedAt       time.Time
}
