package tui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/grumpylabs/kpf/internal/k8s"
)

// ServiceTableRow represents a row in the service table
type ServiceTableRow struct {
	Namespace      string
	Name           string
	Type           string
	ClusterIP      string
	ExternalIP     string
	PortName       string
	Port           int32
	Protocol       string
	Age            string
	IsForwarding   bool
	ForwardingPort int
	Selected       bool
	// Complete service data duplicated per row
	ServiceData    k8s.ServiceInfo  // Complete copy of service data
	PortInfo       *k8s.PortInfo    // Reference to specific port within ServiceData
}

// ServiceTable handles the service list display
type ServiceTable struct {
	rows        []ServiceTableRow
	selectedRow int
	width       int
	height      int
	services    []k8s.ServiceInfo
	offset      int // For scrolling
}

// NewServiceTable creates a new service table
func NewServiceTable() *ServiceTable {
	return &ServiceTable{
		rows:        []ServiceTableRow{},
		selectedRow: 0,
		width:       120,
		height:      30,
		offset:      0,
	}
}

// SetServices updates the table with new service data, creating one row per port
func (t *ServiceTable) SetServices(services []k8s.ServiceInfo) {
	t.services = services
	
	// Calculate total rows needed (one per port)
	totalRows := 0
	for _, svc := range services {
		if len(svc.Ports) == 0 {
			totalRows++ // Service with no ports gets one row
		} else {
			totalRows += len(svc.Ports)
		}
	}
	
	t.rows = make([]ServiceTableRow, 0, totalRows)
	
	for i := range services {
		svc := services[i]
		// Get service age
		age := "unknown"
		if svc.Service != nil && !svc.Service.CreationTimestamp.IsZero() {
			duration := time.Since(svc.Service.CreationTimestamp.Time)
			age = formatAge(duration)
		}

		// Get cluster IP
		clusterIP := "<none>"
		if svc.Service != nil && svc.Service.Spec.ClusterIP != "" && svc.Service.Spec.ClusterIP != "None" {
			clusterIP = svc.Service.Spec.ClusterIP
		}

		// Get external IP
		externalIP := "<none>"
		if svc.Service != nil {
			if len(svc.Service.Status.LoadBalancer.Ingress) > 0 {
				if svc.Service.Status.LoadBalancer.Ingress[0].IP != "" {
					externalIP = svc.Service.Status.LoadBalancer.Ingress[0].IP
				} else if svc.Service.Status.LoadBalancer.Ingress[0].Hostname != "" {
					externalIP = svc.Service.Status.LoadBalancer.Ingress[0].Hostname
				}
			} else if len(svc.Service.Spec.ExternalIPs) > 0 {
				externalIP = svc.Service.Spec.ExternalIPs[0]
			}
		}

		if len(svc.Ports) == 0 {
			// Service with no ports
			row := ServiceTableRow{
				Namespace:      svc.Namespace,
				Name:           svc.Name,
				Type:           svc.Type,
				ClusterIP:      clusterIP,
				ExternalIP:     externalIP,
				PortName:       "<none>",
				Port:           0,
				Protocol:       "",
				Age:            age,
				IsForwarding:   false,
				ForwardingPort: 0,
				Selected:       false, // Will be set properly in SetSelected
				ServiceData:    svc,  // Copy complete service data
				PortInfo:       nil,
			}
			t.rows = append(t.rows, row)
		} else {
			// Create one row per port
			for j := range svc.Ports {
				port := &svc.Ports[j]
				portName := port.Name
				if portName == "" {
					portName = "-"
				}
				
				row := ServiceTableRow{
					Namespace:      svc.Namespace,
					Name:           svc.Name,
					Type:           svc.Type,
					ClusterIP:      clusterIP,
					ExternalIP:     externalIP,
					PortName:       portName,
					Port:           port.Port,
					Protocol:       port.Protocol,
					Age:            age,
					IsForwarding:   port.IsForwarding,
					ForwardingPort: port.ForwardingPort,
					Selected:       false, // Will be set properly in SetSelected
					ServiceData:    svc,  // Copy complete service data
					PortInfo:       port,
				}
				t.rows = append(t.rows, row)
			}
		}
	}
	
	// Simple selection restore - just ensure a valid row is selected
	if t.selectedRow >= len(t.rows) {
		t.selectedRow = len(t.rows) - 1
	}
	if t.selectedRow < 0 {
		t.selectedRow = 0
	}
	
	// Set selection on the current row
	if len(t.rows) > 0 && t.selectedRow >= 0 && t.selectedRow < len(t.rows) {
		t.rows[t.selectedRow].Selected = true
	}
}

// SetSize sets the table dimensions
func (t *ServiceTable) SetSize(width, height int) {
	t.width = width
	t.height = height
}

// SetSelected sets the selected row
func (t *ServiceTable) SetSelected(index int) {
	if index >= 0 && index < len(t.rows) {
		// Clear previous selection
		for i := range t.rows {
			t.rows[i].Selected = false
		}
		
		t.selectedRow = index
		if t.selectedRow < len(t.rows) {
			t.rows[t.selectedRow].Selected = true
		}
		
		// Adjust scroll offset to keep selected row visible
		t.adjustScrollOffset()
	}
}

// GetSelected returns the currently selected service
func (t *ServiceTable) GetSelected() *k8s.ServiceInfo {
	if t.selectedRow >= 0 && t.selectedRow < len(t.rows) {
		return &t.rows[t.selectedRow].ServiceData
	}
	return nil
}

// GetSelectedPort returns the currently selected port info
func (t *ServiceTable) GetSelectedPort() *k8s.PortInfo {
	if t.selectedRow >= 0 && t.selectedRow < len(t.rows) {
		return t.rows[t.selectedRow].PortInfo
	}
	return nil
}

// GetSelectedRow returns the currently selected table row
func (t *ServiceTable) GetSelectedRow() *ServiceTableRow {
	if t.selectedRow >= 0 && t.selectedRow < len(t.rows) {
		return &t.rows[t.selectedRow]
	}
	return nil
}

// GetSelectedIndex returns the currently selected index
func (t *ServiceTable) GetSelectedIndex() int {
	return t.selectedRow
}

// GetRowCount returns the total number of rows
func (t *ServiceTable) GetRowCount() int {
	return len(t.rows)
}

// MoveUp moves selection up
func (t *ServiceTable) MoveUp() {
	if t.selectedRow > 0 {
		t.SetSelected(t.selectedRow - 1)
	}
}

// MoveDown moves selection down
func (t *ServiceTable) MoveDown() {
	if t.selectedRow < len(t.rows)-1 {
		t.SetSelected(t.selectedRow + 1)
	}
}

// adjustScrollOffset adjusts the scroll offset to keep the selected row visible
func (t *ServiceTable) adjustScrollOffset() {
	if t.height <= 3 { // Need space for header + blank line + at least one row
		return
	}
	
	// Available rows = height - 2 (header + blank line)
	availableRows := t.height - 2
	
	// Make sure selected row is visible
	if t.selectedRow < t.offset {
		t.offset = t.selectedRow
	} else if t.selectedRow >= t.offset+availableRows {
		t.offset = t.selectedRow - availableRows + 1
	}
	
	// Don't scroll past the end
	maxOffset := len(t.rows) - availableRows
	if maxOffset < 0 {
		maxOffset = 0
	}
	if t.offset > maxOffset {
		t.offset = maxOffset
	}
}

// Render renders the table
func (t *ServiceTable) Render() string {
	if len(t.rows) == 0 {
		return "No services found"
	}
	
	var content strings.Builder
	
	// Render header row with full width background
	headerLine := fmt.Sprintf("%-35s %-30s %-12s %-16s %-16s %-15s %-8s %-10s %-12s %8s",
		"NAMESPACE", "NAME", "TYPE", "CLUSTER-IP", "EXTERNAL-IP", "PORT-NAME", "PORT", "PROTOCOL", "LOCAL-PORT", "AGE")
	
	// Ensure header background extends to full terminal width
	headerWithBg := adminTableHeaderStyle.Width(t.width).Render(headerLine)
	content.WriteString(headerWithBg)
	content.WriteString("\n\n") // Header + blank line
	
	// Calculate visible rows
	availableRows := t.height - 2 // Subtract header and blank line
	if availableRows <= 0 {
		return content.String()
	}
	
	// Render visible data rows
	endIndex := t.offset + availableRows
	if endIndex > len(t.rows) {
		endIndex = len(t.rows)
	}
	
	for i := t.offset; i < endIndex; i++ {
		row := t.rows[i]
		
		// Status indicator
		statusIndicator := "○" // Gray circle for inactive
		if row.IsForwarding {
			statusIndicator = "●" // Green filled circle for active
		}
		
		// Color the indicator
		var coloredIndicator string
		if row.IsForwarding {
			coloredIndicator = activeStyle.Render(statusIndicator)
		} else {
			coloredIndicator = inactiveStyle.Render(statusIndicator)
		}
		
		// Truncate fields to fit column widths
		namespace := row.Namespace
		name := truncateString(row.Name, 30)
		clusterIP := truncateString(row.ClusterIP, 16)
		externalIP := truncateString(row.ExternalIP, 16)
		portName := truncateString(row.PortName, 15)
		port := ""
		if row.Port > 0 {
			port = fmt.Sprintf("%d", row.Port)
		}
		port = truncateString(port, 8)
		protocol := truncateString(row.Protocol, 10)
		age := truncateString(row.Age, 8)
		
		// Local port column
		localPort := ""
		if row.IsForwarding {
			localPort = fmt.Sprintf(":%d", row.ForwardingPort)
		}
		localPort = truncateString(localPort, 12)
		
		line := fmt.Sprintf("%-35s %-30s %-12s %-16s %-16s %-15s %-8s %-10s %-12s %8s",
			namespace, name, row.Type, clusterIP, externalIP, portName, port, protocol, localPort, age)
		
		// Apply row styling
		var styledLine string
		if row.Selected {
			styledLine = adminSelectedRowStyle.Render(line)
		} else {
			styledLine = adminNormalRowStyle.Render(line)
		}
		
		content.WriteString(coloredIndicator + " " + styledLine + "\n")
	}
	
	return content.String()
}

// SortBy sorts the table by the specified field
func (t *ServiceTable) SortBy(field string, ascending bool) {
	sort.Slice(t.rows, func(i, j int) bool {
		var result bool
		switch field {
		case "namespace":
			result = t.rows[i].Namespace < t.rows[j].Namespace
			if t.rows[i].Namespace == t.rows[j].Namespace {
				result = t.rows[i].Name < t.rows[j].Name
				if t.rows[i].Name == t.rows[j].Name {
					result = t.rows[i].Port < t.rows[j].Port
				}
			}
		case "name":
			result = t.rows[i].Name < t.rows[j].Name
			if t.rows[i].Name == t.rows[j].Name {
				result = t.rows[i].Port < t.rows[j].Port
			}
		case "status":
			// Active ports first when ascending
			if t.rows[i].IsForwarding != t.rows[j].IsForwarding {
				result = t.rows[i].IsForwarding
			} else {
				result = t.rows[i].Name < t.rows[j].Name
				if t.rows[i].Name == t.rows[j].Name {
					result = t.rows[i].Port < t.rows[j].Port
				}
			}
		case "ports":
			// Sort by port number
			if t.rows[i].Port == t.rows[j].Port {
				result = t.rows[i].Name < t.rows[j].Name
			} else {
				result = t.rows[i].Port < t.rows[j].Port
			}
		case "localport":
			// Sort by forwarding port number
			if t.rows[i].ForwardingPort == t.rows[j].ForwardingPort {
				result = t.rows[i].Name < t.rows[j].Name
				if t.rows[i].Name == t.rows[j].Name {
					result = t.rows[i].Port < t.rows[j].Port
				}
			} else {
				result = t.rows[i].ForwardingPort < t.rows[j].ForwardingPort
			}
		default:
			result = t.rows[i].Name < t.rows[j].Name
			if t.rows[i].Name == t.rows[j].Name {
				result = t.rows[i].Port < t.rows[j].Port
			}
		}
		
		if !ascending {
			result = !result
		}
		return result
	})
	
	// Clear and reset selection indicators
	for i := range t.rows {
		t.rows[i].Selected = false
	}
	
	// Reset selection to ensure valid index
	if t.selectedRow >= len(t.rows) {
		t.selectedRow = len(t.rows) - 1
	}
	if t.selectedRow < 0 {
		t.selectedRow = 0
	}
	
	// Set selection on current row
	if len(t.rows) > 0 && t.selectedRow >= 0 && t.selectedRow < len(t.rows) {
		t.rows[t.selectedRow].Selected = true
	}
}

// truncateString truncates a string to the specified length
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-1] + "…"
}