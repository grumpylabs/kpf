package tui

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/grumpylabs/kpf/internal/k8s"
)

type servicesLoadedMsg struct {
	services []k8s.ServiceInfo
}

type portForwardStartedMsg struct {
	localPort int
}

type portForwardStoppedMsg struct{}

type portConflictMsg struct {
	servicePort int
	remotePort  int
}


type errorMsg struct {
	err error
}

type clusterInfoMsg struct {
	context string
}

type deploymentLoadedMsg struct {
	deployment *k8s.DeploymentInfo
}


func (m *Model) loadServices() tea.Msg {
	ctx := context.Background()
	services, err := m.client.GetServices(ctx)
	if err != nil {
		return errorMsg{err: err}
	}

	for i := range services {
		// Check if any port is forwarding
		services[i].IsForwarding = m.forwardManager.IsServiceForwarding(services[i].Namespace, services[i].Name)
		
		// Update per-port forwarding status
		for j := range services[i].Ports {
			port := &services[i].Ports[j]
			port.IsForwarding = m.forwardManager.IsForwarding(services[i].Namespace, services[i].Name, int(port.Port))
			if port.IsForwarding {
				port.ForwardingPort = m.forwardManager.GetLocalPort(services[i].Namespace, services[i].Name, int(port.Port))
			} else {
				port.ForwardingPort = 0 // Ensure it's cleared when not forwarding
			}
		}
		
		// For backward compatibility, set service-level forwarding info from first active port
		if services[i].IsForwarding {
			for _, port := range services[i].Ports {
				if port.IsForwarding {
					services[i].ForwardingPort = port.ForwardingPort
					break
				}
			}
		}
	}

	return servicesLoadedMsg{services: services}
}

func (m *Model) startPortForward() tea.Msg {
	selectedRow := m.table.GetSelectedRow()
	if selectedRow == nil {
		return errorMsg{err: fmt.Errorf("no row selected")}
	}

	svc := &selectedRow.ServiceData
	if svc.Name == "" {
		return errorMsg{err: fmt.Errorf("no service info available")}
	}

	// If no specific port (service with no ports)
	if selectedRow.PortInfo == nil {
		return errorMsg{err: fmt.Errorf("service has no ports to forward")}
	}

	port := selectedRow.PortInfo
	
	if selectedRow.IsForwarding {
		// Port is active, stop it
		m.forwardManager.StopForward(svc.Namespace, svc.Name, int(port.Port))
		return portForwardStoppedMsg{}
	} else {
		// Port is inactive, start it
		localPort, err := m.forwardManager.StartForwardWithLocalPort(context.Background(), svc.Namespace, svc.Name, int(port.Port), int(port.Port))
		if err != nil {
			if fmt.Sprintf("port %d is already in use", port.Port) == err.Error() {
				return portConflictMsg{servicePort: int(port.Port), remotePort: int(port.Port)}
			}
			return errorMsg{err: err}
		}
		return portForwardStartedMsg{localPort: localPort}
	}
}



func (m *Model) startPortForwardWithUserPort() tea.Msg {
	svc := m.table.GetSelected()
	if svc == nil {
		return errorMsg{err: fmt.Errorf("no service selected")}
	}

	// Parse user input
	var userPort int
	if _, err := fmt.Sscanf(m.portInput, "%d", &userPort); err != nil {
		return errorMsg{err: fmt.Errorf("invalid port number: %s", m.portInput)}
	}

	// Validate port range
	if userPort < 1024 || userPort > 65535 {
		return errorMsg{err: fmt.Errorf("port must be between 1024-65535")}
	}

	// Try to start port forward with user's port
	localPort, err := m.forwardManager.StartForwardWithLocalPort(context.Background(), svc.Namespace, svc.Name, m.remotePort, userPort)
	if err != nil {
		return errorMsg{err: err}
	}

	// Clear port input state and return to list view
	m.portInput = ""
	m.viewMode = listView
	
	return portForwardStartedMsg{localPort: localPort}
}

func (m *Model) loadClusterInfo() tea.Msg {
	context := "default"

	if m.client != nil {
		// Get server host from the REST config
		config := m.client.GetConfig()
		if config != nil && config.Host != "" {
			// Extract hostname or IP from the server URL
			host := config.Host
			// Remove https:// prefix if present
			if strings.HasPrefix(host, "https://") {
				host = host[8:]
			}
			if strings.HasPrefix(host, "http://") {
				host = host[7:]
			}
			// Remove port if present for cleaner display
			if colonIndex := strings.LastIndex(host, ":"); colonIndex > 0 {
				// Only remove port if it looks like a port (not IPv6)
				if !strings.Contains(host, "[") {
					host = host[:colonIndex]
				}
			}
			context = host
		}
	}

	return clusterInfoMsg{context: context}
}

func (m *Model) loadDeploymentInfo() tea.Msg {
	svc := &m.detailViewService
	if svc.Name == "" {
		return deploymentLoadedMsg{deployment: nil}
	}

	ctx := context.Background()
	deployment, err := m.client.GetDeploymentForService(ctx, svc.Namespace, svc.Name)
	if err != nil {
		// Don't treat this as an error, just no deployment found
		return deploymentLoadedMsg{deployment: nil}
	}

	return deploymentLoadedMsg{deployment: deployment}
}
