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
	rows         []ServiceTableRow
	filteredRows []ServiceTableRow // Filtered rows for display
	selectedRow  int
	width        int
	height       int
	services     []k8s.ServiceInfo
	offset       int // For scrolling
	filters      map[string]string // Active filters
}

// NewServiceTable creates a new service table
func NewServiceTable() *ServiceTable {
	return &ServiceTable{
		rows:         []ServiceTableRow{},
		filteredRows: []ServiceTableRow{},
		selectedRow:  0,
		width:        120,
		height:       30,
		offset:       0,
		filters:      make(map[string]string),
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
	
	// Apply existing filters
	t.ApplyFilters(t.filters)
	
	// Simple selection restore - just ensure a valid row is selected
	if t.selectedRow >= len(t.getActiveRows()) {
		t.selectedRow = len(t.getActiveRows()) - 1
	}
	if t.selectedRow < 0 {
		t.selectedRow = 0
	}
	
	// Set selection on the current row
	rows := t.getActiveRows()
	if len(rows) > 0 && t.selectedRow >= 0 && t.selectedRow < len(rows) {
		for i := range rows {
			rows[i].Selected = (i == t.selectedRow)
		}
	}
}

// SetSize sets the table dimensions
func (t *ServiceTable) SetSize(width, height int) {
	t.width = width
	t.height = height
}

// SetSelected sets the selected row
func (t *ServiceTable) SetSelected(index int) {
	rows := t.getActiveRows()
	if index >= 0 && index < len(rows) {
		// Clear previous selection
		for i := range rows {
			rows[i].Selected = false
		}
		
		t.selectedRow = index
		if t.selectedRow < len(rows) {
			rows[t.selectedRow].Selected = true
		}
		
		// Adjust scroll offset to keep selected row visible
		t.adjustScrollOffset()
	}
}

// GetSelected returns the currently selected service
func (t *ServiceTable) GetSelected() *k8s.ServiceInfo {
	rows := t.getActiveRows()
	if t.selectedRow >= 0 && t.selectedRow < len(rows) {
		return &rows[t.selectedRow].ServiceData
	}
	return nil
}

// GetSelectedPort returns the currently selected port info
func (t *ServiceTable) GetSelectedPort() *k8s.PortInfo {
	rows := t.getActiveRows()
	if t.selectedRow >= 0 && t.selectedRow < len(rows) {
		return rows[t.selectedRow].PortInfo
	}
	return nil
}

// GetSelectedRow returns the currently selected table row
func (t *ServiceTable) GetSelectedRow() *ServiceTableRow {
	rows := t.getActiveRows()
	if t.selectedRow >= 0 && t.selectedRow < len(rows) {
		return &rows[t.selectedRow]
	}
	return nil
}

// GetSelectedIndex returns the currently selected index
func (t *ServiceTable) GetSelectedIndex() int {
	return t.selectedRow
}

// GetRowCount returns the total number of rows
func (t *ServiceTable) GetRowCount() int {
	return len(t.getActiveRows())
}

// MoveUp moves selection up
func (t *ServiceTable) MoveUp() {
	if t.selectedRow > 0 {
		t.SetSelected(t.selectedRow - 1)
	}
}

// MoveDown moves selection down
func (t *ServiceTable) MoveDown() {
	rows := t.getActiveRows()
	if t.selectedRow < len(rows)-1 {
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
	rows := t.getActiveRows()
	maxOffset := len(rows) - availableRows
	if maxOffset < 0 {
		maxOffset = 0
	}
	if t.offset > maxOffset {
		t.offset = maxOffset
	}
}

// Render renders the table
func (t *ServiceTable) Render() string {
	rows := t.getActiveRows()
	if len(rows) == 0 {
		if len(t.filters) > 0 {
			return "No services match the current filters"
		}
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
	activeRows := t.getActiveRows()
	endIndex := t.offset + availableRows
	if endIndex > len(activeRows) {
		endIndex = len(activeRows)
	}
	
	for i := t.offset; i < endIndex; i++ {
		row := activeRows[i]
		
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
	// Sort all rows first
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
	
	// Also sort filtered rows if filters are active
	if len(t.filters) > 0 {
		sort.Slice(t.filteredRows, func(i, j int) bool {
			var result bool
			switch field {
			case "namespace":
				result = t.filteredRows[i].Namespace < t.filteredRows[j].Namespace
				if t.filteredRows[i].Namespace == t.filteredRows[j].Namespace {
					result = t.filteredRows[i].Name < t.filteredRows[j].Name
					if t.filteredRows[i].Name == t.filteredRows[j].Name {
						result = t.filteredRows[i].Port < t.filteredRows[j].Port
					}
				}
			case "name":
				result = t.filteredRows[i].Name < t.filteredRows[j].Name
				if t.filteredRows[i].Name == t.filteredRows[j].Name {
					result = t.filteredRows[i].Port < t.filteredRows[j].Port
				}
			case "status":
				// Active ports first when ascending
				if t.filteredRows[i].IsForwarding != t.filteredRows[j].IsForwarding {
					result = t.filteredRows[i].IsForwarding
				} else {
					result = t.filteredRows[i].Name < t.filteredRows[j].Name
					if t.filteredRows[i].Name == t.filteredRows[j].Name {
						result = t.filteredRows[i].Port < t.filteredRows[j].Port
					}
				}
			case "ports":
				// Sort by port number
				if t.filteredRows[i].Port == t.filteredRows[j].Port {
					result = t.filteredRows[i].Name < t.filteredRows[j].Name
				} else {
					result = t.filteredRows[i].Port < t.filteredRows[j].Port
				}
			case "localport":
				// Sort by forwarding port number
				if t.filteredRows[i].ForwardingPort == t.filteredRows[j].ForwardingPort {
					result = t.filteredRows[i].Name < t.filteredRows[j].Name
					if t.filteredRows[i].Name == t.filteredRows[j].Name {
						result = t.filteredRows[i].Port < t.filteredRows[j].Port
					}
				} else {
					result = t.filteredRows[i].ForwardingPort < t.filteredRows[j].ForwardingPort
				}
			default:
				result = t.filteredRows[i].Name < t.filteredRows[j].Name
				if t.filteredRows[i].Name == t.filteredRows[j].Name {
					result = t.filteredRows[i].Port < t.filteredRows[j].Port
				}
			}
			
			if !ascending {
				result = !result
			}
			return result
		})
	}
	
	// Clear and reset selection indicators
	rows := t.getActiveRows()
	for i := range rows {
		rows[i].Selected = false
	}
	
	// Reset selection to ensure valid index
	if t.selectedRow >= len(rows) {
		t.selectedRow = len(rows) - 1
	}
	if t.selectedRow < 0 {
		t.selectedRow = 0
	}
	
	// Set selection on current row
	if len(rows) > 0 && t.selectedRow >= 0 && t.selectedRow < len(rows) {
		rows[t.selectedRow].Selected = true
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

// ApplyFilters applies the given filters to the service table
func (t *ServiceTable) ApplyFilters(filters map[string]string) {
	t.filters = filters
	
	// If no filters, show all rows
	if len(filters) == 0 {
		t.filteredRows = make([]ServiceTableRow, len(t.rows))
		copy(t.filteredRows, t.rows)
		t.adjustAfterFilter()
		return
	}
	
	// Filter rows based on active filters
	t.filteredRows = []ServiceTableRow{}
	for _, row := range t.rows {
		if t.matchesFilters(row, filters) {
			t.filteredRows = append(t.filteredRows, row)
		}
	}
	
	t.adjustAfterFilter()
}

// matchesFilters checks if a row matches all active filters
func (t *ServiceTable) matchesFilters(row ServiceTableRow, filters map[string]string) bool {
	for filterType, filterValue := range filters {
		filterValue = strings.ToLower(filterValue)
		
		switch filterType {
		case "status":
			// Filter by forwarding status
			if filterValue == "active" && !row.IsForwarding {
				return false
			}
			if filterValue == "inactive" && row.IsForwarding {
				return false
			}
			
		case "type":
			// Filter by service type
			if !strings.Contains(strings.ToLower(row.Type), filterValue) {
				return false
			}
			
		case "name":
			// Filter by service name (partial match)
			if !strings.Contains(strings.ToLower(row.Name), filterValue) {
				return false
			}
			
		case "protocol":
			// Filter by protocol
			if !strings.EqualFold(row.Protocol, filterValue) {
				return false
			}
			
		default:
			// Generic search across all fields
			found := false
			searchFields := []string{
				row.Namespace,
				row.Name,
				row.Type,
				row.ClusterIP,
				row.ExternalIP,
				row.PortName,
				fmt.Sprintf("%d", row.Port),
				row.Protocol,
			}
			
			for _, field := range searchFields {
				if strings.Contains(strings.ToLower(field), filterValue) {
					found = true
					break
				}
			}
			
			if !found {
				return false
			}
		}
	}
	
	return true
}

// adjustAfterFilter adjusts selection and scrolling after filtering
func (t *ServiceTable) adjustAfterFilter() {
	// Reset selection to first filtered row
	if len(t.filteredRows) > 0 {
		t.selectedRow = 0
		// Clear all selected flags
		for i := range t.filteredRows {
			t.filteredRows[i].Selected = false
		}
		t.filteredRows[0].Selected = true
	} else {
		t.selectedRow = -1
	}
	
	// Reset scroll offset
	t.offset = 0
}

// getActiveRows returns the currently active row set (filtered or all)
func (t *ServiceTable) getActiveRows() []ServiceTableRow {
	if len(t.filters) > 0 {
		return t.filteredRows
	}
	return t.rows
}