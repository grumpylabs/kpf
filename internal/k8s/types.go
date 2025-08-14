package k8s

import (
	"time"
	
	corev1 "k8s.io/api/core/v1"
)

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
	Name           string
	Port           int32
	TargetPort     int32
	Protocol       string
	IsForwarding   bool
	ForwardingPort int
}

type DeploymentInfo struct {
	Name            string
	Replicas        int32
	ReadyReplicas   int32
	UpdatedReplicas int32
	Image           string
	CreatedAt       time.Time
}
