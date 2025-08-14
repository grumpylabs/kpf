package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/grumpylabs/kpf/internal/k8s"
	"github.com/grumpylabs/kpf/internal/portforward"
)

type viewMode int

const (
	listView viewMode = iota
	detailView
	helpView
	portInputView
	errorModalView
)

// Model represents the main TUI model
type Model struct {
	client         *k8s.Client
	table          *ServiceTable
	viewMode       viewMode
	ready          bool
	width          int
	height         int
	keys           keyMap
	forwardManager *portforward.Manager
	err            error
	statusMessage  string
	kubeconfig     string
	namespace      string
	context        string
	lastRefresh    time.Time
	sortField      string // "namespace", "name", "status"
	sortAscending  bool
	
	// Port input state
	portInput     string
	conflictPort  int
	remotePort    int
	
	
	// Error modal state
	errorMessage  string
	previousView  viewMode
	
	// Detail view state
	deploymentInfo   *k8s.DeploymentInfo
	detailViewService k8s.ServiceInfo // Store a copy of the service we're viewing details for
	
	// Active forwards counter (atomic for thread safety)
	activeForwardsCount int64
	
	// Version info
	commitHash     string
}

type keyMap struct {
	Up       key.Binding
	Down     key.Binding
	Enter    key.Binding
	Back     key.Binding
	Forward  key.Binding
	Refresh  key.Binding
	Quit     key.Binding
	PageUp   key.Binding
	PageDown key.Binding
	Home     key.Binding
	End      key.Binding
	Detail   key.Binding
	SortNamespace key.Binding
	SortName      key.Binding
	SortStatus    key.Binding
	SortPorts     key.Binding
	SortLocalPort key.Binding
	Help         key.Binding
}

var keys = keyMap{
	Up: key.NewBinding(
		key.WithKeys("up", "k"),
		key.WithHelp("↑/k", "up"),
	),
	Down: key.NewBinding(
		key.WithKeys("down", "j"),
		key.WithHelp("↓/j", "down"),
	),
	PageUp: key.NewBinding(
		key.WithKeys("pgup", "ctrl+u"),
		key.WithHelp("pgup", "page up"),
	),
	PageDown: key.NewBinding(
		key.WithKeys("pgdown", "ctrl+d"),
		key.WithHelp("pgdn", "page down"),
	),
	Home: key.NewBinding(
		key.WithKeys("home", "ctrl+a"),
		key.WithHelp("home", "go to top"),
	),
	End: key.NewBinding(
		key.WithKeys("end", "ctrl+e"),
		key.WithHelp("end", "go to bottom"),
	),
	Enter: key.NewBinding(
		key.WithKeys("enter"),
		key.WithHelp("enter", "view details"),
	),
	Back: key.NewBinding(
		key.WithKeys("esc", "b"),
		key.WithHelp("esc/b", "back"),
	),
	Detail: key.NewBinding(
		key.WithKeys("d"),
		key.WithHelp("d", "details"),
	),
	Forward: key.NewBinding(
		key.WithKeys("f", "F"),
		key.WithHelp("f", "toggle forward"),
	),
	Refresh: key.NewBinding(
		key.WithKeys("r", "ctrl+r"),
		key.WithHelp("r", "refresh"),
	),
	Quit: key.NewBinding(
		key.WithKeys("ctrl+c"),
		key.WithHelp("ctrl+c", "quit"),
	),
	SortNamespace: key.NewBinding(
		key.WithKeys("N"),
		key.WithHelp("N", "sort by namespace"),
	),
	SortName: key.NewBinding(
		key.WithKeys("M"), // M for naME to avoid conflict with N
		key.WithHelp("M", "sort by name"),
	),
	SortStatus: key.NewBinding(
		key.WithKeys("S"),
		key.WithHelp("S", "sort by status"),
	),
	SortPorts: key.NewBinding(
		key.WithKeys("P"),
		key.WithHelp("P", "sort by ports"),
	),
	SortLocalPort: key.NewBinding(
		key.WithKeys("L"),
		key.WithHelp("L", "sort by local port"),
	),
	Help: key.NewBinding(
		key.WithKeys("?", "h"),
		key.WithHelp("?/h", "help"),
	),
}

// NewModel creates a new TUI model
func NewModel(client *k8s.Client, kubeconfigArg string, commitHash string) *Model {
	// Use the kubeconfig path passed from CLI args, with fallback
	kubeconfig := kubeconfigArg
	if kubeconfig == "" {
		kubeconfig = os.Getenv("KUBECONFIG")
		if kubeconfig == "" {
			home := os.Getenv("HOME")
			if home != "" {
				kubeconfig = filepath.Join(home, ".kube", "config")
			}
		}
	}

	// Get context from kubeconfig (simplified - you might want to parse the actual file)
	context := "kubernetes-cluster"

	// Get namespace
	namespace := os.Getenv("KPF_NAMESPACE")
	if namespace == "" {
		namespace = "all"
	}

	table := NewServiceTable()

	return &Model{
		client:         client,
		table:          table,
		viewMode:       listView,
		keys:           keys,
		forwardManager: portforward.NewManager(client),
		kubeconfig:     kubeconfig,
		namespace:      namespace,
		context:        context,
		lastRefresh:    time.Now(),
		sortField:      "namespace",
		sortAscending:  true,
		commitHash:     commitHash,
	}
}

// Init initializes the model
func (m *Model) Init() tea.Cmd {
	return tea.Batch(
		m.loadServices,
		m.loadClusterInfo,
	)
}

// Update handles TUI updates
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		
		// Calculate available space for table content
		// Let's be more precise about the header counting:
		// 1. Admin header: 1 line
		// 2. "Services": 1 line  
		// 3. "Environment": 1 line
		// 4. "Services count": 1 line
		// 5. Blank line: 1 line
		// Total: 5 lines for section headers
		// Footer: 1 line + 1 blank line above it
		// Be conservative and reserve extra space
		sectionHeaderHeight := 5
		footerHeight := 3  // Footer + blank line above it + extra padding
		availableHeight := msg.Height - sectionHeaderHeight - footerHeight
		
		// Ensure we have at least some space for the table
		if availableHeight < 5 {
			availableHeight = 5
		}
		
		m.table.SetSize(msg.Width, availableHeight)
		m.ready = true
		return m, nil

	case tea.KeyMsg:
		// Ctrl+C should always quit from anywhere
		if key.Matches(msg, m.keys.Quit) {
			m.forwardManager.StopAll()
			return m, tea.Quit
		}
		
		if m.viewMode == listView {
			switch {
			case msg.String() == "q" || msg.Type == tea.KeyEsc:
				// q and esc quit from main menu
				m.forwardManager.StopAll()
				return m, tea.Quit

			case key.Matches(msg, m.keys.Up):
				m.table.MoveUp()

			case key.Matches(msg, m.keys.Down):
				m.table.MoveDown()

			case key.Matches(msg, m.keys.PageUp):
				// Move up by 10 rows
				for i := 0; i < 10; i++ {
					m.table.MoveUp()
				}

			case key.Matches(msg, m.keys.PageDown):
				// Move down by 10 rows
				for i := 0; i < 10; i++ {
					m.table.MoveDown()
				}

			case key.Matches(msg, m.keys.Home):
				// Go to first item
				m.table.SetSelected(0)

			case key.Matches(msg, m.keys.End):
				// Go to last item
				lastIndex := m.table.GetRowCount() - 1
				if lastIndex >= 0 {
					m.table.SetSelected(lastIndex)
				}

			case key.Matches(msg, m.keys.Enter):
				if m.table.GetSelected() != nil {
					m.viewMode = detailView
					m.deploymentInfo = nil // Clear previous deployment info
					return m, m.loadDeploymentInfo
				}

			case key.Matches(msg, m.keys.Forward):
				if m.table.GetSelected() != nil {
					return m, m.startPortForward
				}

			case key.Matches(msg, m.keys.Detail):
				selectedService := m.table.GetSelected()
				if selectedService != nil {
					m.detailViewService = *selectedService // Store a copy of the service we're viewing
					m.viewMode = detailView
					m.deploymentInfo = nil // Clear previous deployment info
					return m, m.loadDeploymentInfo
				}

			case key.Matches(msg, m.keys.Refresh):
				m.statusMessage = ""
				m.lastRefresh = time.Now()
				return m, m.loadServices

			case key.Matches(msg, m.keys.SortNamespace):
				if m.sortField == "namespace" {
					m.sortAscending = !m.sortAscending
				} else {
					m.sortField = "namespace"
					m.sortAscending = true
				}
				m.table.SortBy(m.sortField, m.sortAscending)
				return m, nil

			case key.Matches(msg, m.keys.SortName):
				if m.sortField == "name" {
					m.sortAscending = !m.sortAscending
				} else {
					m.sortField = "name"
					m.sortAscending = true
				}
				m.table.SortBy(m.sortField, m.sortAscending)
				return m, nil

			case key.Matches(msg, m.keys.SortStatus):
				if m.sortField == "status" {
					m.sortAscending = !m.sortAscending
				} else {
					m.sortField = "status"
					m.sortAscending = true
				}
				m.table.SortBy(m.sortField, m.sortAscending)
				return m, nil

			case key.Matches(msg, m.keys.SortPorts):
				if m.sortField == "ports" {
					m.sortAscending = !m.sortAscending
				} else {
					m.sortField = "ports"
					m.sortAscending = true
				}
				m.table.SortBy(m.sortField, m.sortAscending)
				return m, nil

			case key.Matches(msg, m.keys.SortLocalPort):
				if m.sortField == "localport" {
					m.sortAscending = !m.sortAscending
				} else {
					m.sortField = "localport"
					m.sortAscending = true
				}
				m.table.SortBy(m.sortField, m.sortAscending)
				return m, nil

			case key.Matches(msg, m.keys.Help):
				m.viewMode = helpView
				return m, nil
			}
		} else if m.viewMode == helpView {
			switch {
			case msg.String() == "q" || msg.Type == tea.KeyEsc || key.Matches(msg, m.keys.Help):
				// q, esc, or ? go back to list view
				m.viewMode = listView
				return m, nil
			}
		} else if m.viewMode == detailView {
			switch {
			case msg.String() == "q" || msg.Type == tea.KeyEsc || key.Matches(msg, m.keys.Back):
				// q, esc, or b go back to list view
				m.viewMode = listView
				return m, nil

			case key.Matches(msg, m.keys.Forward):
				return m, m.startPortForward
			}
		} else if m.viewMode == portInputView {
			switch {
			case msg.Type == tea.KeyEnter:
				// Try to start port forward with user-provided port
				return m, m.startPortForwardWithUserPort

			case msg.Type == tea.KeyEsc || msg.String() == "q":
				// Cancel and go back to list view
				m.viewMode = listView
				m.portInput = ""
				return m, nil

			case msg.Type == tea.KeyBackspace:
				if len(m.portInput) > 0 {
					m.portInput = m.portInput[:len(m.portInput)-1]
				}
				return m, nil

			case msg.Type == tea.KeyRunes:
				// Handle numeric input for port
				for _, r := range msg.Runes {
					if r >= '0' && r <= '9' && len(m.portInput) < 5 { // Limit to 5 digits (max port 65535)
						m.portInput += string(r)
					}
				}
				return m, nil

			default:
				return m, nil
			}
		} else if m.viewMode == errorModalView {
			switch {
			case msg.Type == tea.KeyEnter, msg.Type == tea.KeyEsc, msg.String() == " ", msg.String() == "q":
				// Any of these keys dismiss the error modal
				m.viewMode = m.previousView
				m.errorMessage = ""
				return m, nil

			default:
				return m, nil
			}
		}

	case servicesLoadedMsg:
		m.table.SetServices(msg.services)
		m.table.SortBy(m.sortField, m.sortAscending)
		
		// Initialize active forwards counter
		activeCount := int64(0)
		for _, row := range m.table.rows {
			if row.IsForwarding {
				activeCount++
			}
		}
		atomic.StoreInt64(&m.activeForwardsCount, activeCount)
		
		return m, nil

	case clusterInfoMsg:
		m.context = msg.context
		return m, nil

	case portForwardStartedMsg:
		// Update the current row's state directly without rebuilding table
		selectedRow := m.table.GetSelectedRow()
		if selectedRow != nil && selectedRow.PortInfo != nil {
			selectedRow.IsForwarding = true
			selectedRow.ForwardingPort = msg.localPort
			selectedRow.PortInfo.IsForwarding = true
			selectedRow.PortInfo.ForwardingPort = msg.localPort
			// Also update the service data copy
			for i := range selectedRow.ServiceData.Ports {
				if selectedRow.ServiceData.Ports[i].Port == selectedRow.PortInfo.Port {
					selectedRow.ServiceData.Ports[i].IsForwarding = true
					selectedRow.ServiceData.Ports[i].ForwardingPort = msg.localPort
					break
				}
			}
			// Increment active forwards counter
			atomic.AddInt64(&m.activeForwardsCount, 1)
		}
		return m, nil

	case portForwardStoppedMsg:
		// Update the current row's state directly without rebuilding table
		selectedRow := m.table.GetSelectedRow()
		if selectedRow != nil && selectedRow.PortInfo != nil {
			selectedRow.IsForwarding = false
			selectedRow.ForwardingPort = 0
			selectedRow.PortInfo.IsForwarding = false
			selectedRow.PortInfo.ForwardingPort = 0
			// Also update the service data copy
			for i := range selectedRow.ServiceData.Ports {
				if selectedRow.ServiceData.Ports[i].Port == selectedRow.PortInfo.Port {
					selectedRow.ServiceData.Ports[i].IsForwarding = false
					selectedRow.ServiceData.Ports[i].ForwardingPort = 0
					break
				}
			}
			// Decrement active forwards counter
			atomic.AddInt64(&m.activeForwardsCount, -1)
		}
		return m, nil

	case portConflictMsg:
		// Port conflict detected - switch to port input view
		m.viewMode = portInputView
		m.conflictPort = msg.servicePort
		m.remotePort = msg.remotePort
		m.portInput = ""
		return m, nil


	case deploymentLoadedMsg:
		m.deploymentInfo = msg.deployment
		return m, nil


	case errorMsg:
		// Show error modal instead of just storing the error
		m.previousView = m.viewMode
		m.viewMode = errorModalView
		m.errorMessage = msg.err.Error()
		m.err = msg.err // Keep this for compatibility
		return m, nil
	}

	return m, tea.Batch(cmds...)
}

// View renders the TUI
func (m *Model) View() string {
	if !m.ready {
		return "\n  Initializing..."
	}

	switch m.viewMode {
	case detailView:
		return m.renderDetailView()
	case helpView:
		return m.renderHelpView()
	case portInputView:
		return m.renderPortInputView()
	case errorModalView:
		return m.renderErrorModal()
	default:
		return m.renderListView()
	}
}

func (m *Model) renderListView() string {
	// Header with cluster info (matching admin format)
	header := m.renderAdminHeader()

	// Section headers (matching admin yellow style)  
	sectionHeaders := m.renderSectionHeaders()

	// Table content (includes header and data rows with proper scrolling)
	tableContent := m.table.Render()

	// Build main content
	content := header + sectionHeaders + tableContent

	// Generate footer with navigation (matching admin footer exactly)
	selected := m.table.GetSelectedIndex() + 1
	total := m.table.GetRowCount()
	sortIndicator := ""
	if m.sortField != "" {
		direction := "^"
		if !m.sortAscending {
			direction = "v"
		}
		sortIndicator = fmt.Sprintf(" [%s%s]", m.sortField, direction)
	}
	footerText := fmt.Sprintf("[%d/%d]%s ↑↓/jk:navigate f:toggle-forward enter:details N/M/S/P/L:sort r:refresh ?/h:help q:quit", selected, total, sortIndicator)

	return RenderWithFooter(content, footerText, m.width, m.height)
}

func (m *Model) renderHelpView() string {
	// Header
	header := m.renderAdminHeader()

	// Help content
	helpContent := `
Navigation:
  ↑/k              Move up
  ↓/j              Move down  
  pgup/pgdn        Page up/down
  home/end         Go to top/bottom
  enter            View service details
  esc/q            Back/Quit

Port Forwarding:
  f/F              Toggle port forward for selected service

Sorting:
  N                Sort by namespace
  M                Sort by name
  S                Sort by status (forwarding active/inactive)
  P                Sort by ports
  L                Sort by local port

Other:
  r                Refresh service list
  ?/h              Show/hide this help
  Ctrl+C           Quit application

Status Indicators:
  ●                Port forwarding active (green)
  ○                Port forwarding inactive (gray)
`

	content := header + sectionHeaderStyle.Render("Help") + "\n" + helpContent

	footerText := "Press ?/h or ESC to close help"
	return RenderWithFooter(content, footerText, m.width, m.height)
}

func (m *Model) renderAdminHeader() string {
	// Match admin header format exactly: "KPF: Now(2025-08-13 13:50:25/1755111025)"
	now := time.Now()
	humanTime := now.Format("2006-01-02 15:04:05")
	epochTime := now.Unix()

	commitInfo := ""
	if m.commitHash != "" && m.commitHash != "unknown" {
		commitInfo = fmt.Sprintf(" (%s)", m.commitHash)
	}
	leftSide := fmt.Sprintf("KPF%s: Now(%s/%d)", commitInfo, humanTime, epochTime)

	// Right side with cluster info and kubeconfig path
	kubeconfigPath := m.kubeconfig
	if kubeconfigPath != "" {
		// Show just the filename, not the full path for space
		if lastSlash := strings.LastIndex(kubeconfigPath, "/"); lastSlash != -1 {
			kubeconfigPath = kubeconfigPath[lastSlash+1:]
		}
	}
	
	// If we have the original kubeconfig from CLI args, prefer showing that filename
	rightSide := fmt.Sprintf("[ Cluster: %s | Config: %s ]", m.context, kubeconfigPath)

	// Calculate spacing
	leftLen := len(leftSide)
	rightLen := len(rightSide)
	availableSpace := m.width - 2

	var headerText string
	if leftLen+rightLen+2 <= availableSpace {
		spacing := availableSpace - leftLen - rightLen
		headerText = leftSide + strings.Repeat(" ", spacing) + rightSide
	} else {
		headerText = leftSide
	}

	return adminHeaderStyle.Width(m.width).Render(headerText) + "\n"
}

func (m *Model) renderSectionHeaders() string {
	var content strings.Builder

	// Services section header (matching admin yellow style)
	content.WriteString(sectionHeaderStyle.Render("Services") + "\n")

	// Environment/namespace info (matching admin style)
	envText := "Environment: all"
	if m.namespace != "" && m.namespace != "all" {
		envText = fmt.Sprintf("Environment: %s", m.namespace)
	}
	content.WriteString(sectionHeaderStyle.Render(envText) + "\n")

	// Services count (matching admin style)
	services := m.table.services
	activeCount := atomic.LoadInt64(&m.activeForwardsCount)
	countText := fmt.Sprintf("Services (%d) - Active Forwards (%d)", len(services), activeCount)
	content.WriteString(sectionHeaderStyle.Render(countText) + "\n")

	// Blank line before table
	content.WriteString("\n")

	return content.String()
}



// formatAge formats a duration into k9s-style age string
func formatAge(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	} else if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	} else if d < 24*time.Hour {
		return fmt.Sprintf("%dh", int(d.Hours()))
	} else {
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}

func (m *Model) renderDetailView() string {
	svc := &m.detailViewService
	if svc.Name == "" {
		return "No service selected"
	}

	// Header
	header := m.renderAdminHeader()
	sectionHeaders := m.renderSectionHeaders()

	// Service details box
	serviceDetails := m.renderServiceDetails(*svc)
	serviceBox := detailBoxStyle.Width(m.width-4).Render(serviceDetails)
	
	// Deployment details box (if available)
	deploymentBox := ""
	if m.deploymentInfo != nil {
		deploymentDetails := m.renderDeploymentDetails(*m.deploymentInfo)
		deploymentBox = "\n\n" + detailBoxStyle.Width(m.width-4).Render(deploymentDetails)
	}
	
	content := header + sectionHeaders + serviceBox + deploymentBox

	// Footer for detail view
	footerText := "f:toggle-forward q/esc:back Ctrl+C:exit"

	return RenderWithFooter(content, footerText, m.width, m.height)
}

func (m *Model) renderServiceDetails(svc k8s.ServiceInfo) string {
	var s strings.Builder

	s.WriteString(detailLabelStyle.Render("Service") + ": " + svc.Name + "\n")
	s.WriteString(detailLabelStyle.Render("Namespace") + ": " + svc.Namespace + "\n")
	s.WriteString(detailLabelStyle.Render("Type") + ": " + svc.Type + "\n\n")

	s.WriteString(detailLabelStyle.Render("Ports") + ":\n")
	for _, port := range svc.Ports {
		portInfo := fmt.Sprintf("  • %s: %d", port.Name, port.Port)
		if port.TargetPort != 0 && port.TargetPort != port.Port {
			portInfo += fmt.Sprintf(" → %d", port.TargetPort)
		}
		portInfo += fmt.Sprintf(" (%s)", port.Protocol)
		
		// Add per-port forwarding status
		if port.IsForwarding {
			portInfo += fmt.Sprintf(" [FORWARDING → localhost:%d]", port.ForwardingPort)
		} else {
			portInfo += " [INACTIVE]"
		}
		
		s.WriteString(portInfo + "\n")
	}

	return s.String()
}


func (m *Model) renderDeploymentDetails(dep k8s.DeploymentInfo) string {
	var s strings.Builder

	s.WriteString(detailLabelStyle.Render("Deployment") + ": " + dep.Name + "\n")
	s.WriteString(detailLabelStyle.Render("Replicas") + ": " + fmt.Sprintf("%d/%d ready", dep.ReadyReplicas, dep.Replicas) + "\n")
	s.WriteString(detailLabelStyle.Render("Updated") + ": " + fmt.Sprintf("%d replicas", dep.UpdatedReplicas) + "\n")
	s.WriteString(detailLabelStyle.Render("Image") + ": " + dep.Image + "\n")
	
	// Format age
	age := time.Since(dep.CreatedAt)
	s.WriteString(detailLabelStyle.Render("Age") + ": " + formatAge(age))

	return s.String()
}

func padRight(s string, n int) string {
	if len(s) >= n {
		return s[:n]
	}
	return s + strings.Repeat(" ", n-len(s))
}

func (m *Model) renderPortInputView() string {
	// Header
	header := m.renderAdminHeader()

	// Port conflict message
	svc := m.table.GetSelected()
	if svc == nil {
		return "Error: No service selected"
	}

	content := header + sectionHeaderStyle.Render("Port Conflict") + "\n\n"
	content += fmt.Sprintf("Port %d is already in use for service %s/%s\n\n", m.conflictPort, svc.Namespace, svc.Name)
	content += "Please enter an alternative local port number:\n\n"
	
	// Input box with cursor
	inputBox := fmt.Sprintf("Port: %s", m.portInput)
	if len(m.portInput) == 0 {
		inputBox += "_" // Show cursor when empty
	} else {
		inputBox += "_" // Always show cursor for active input
	}
	content += inputBox + "\n\n"
	
	// Validation message
	if len(m.portInput) > 0 {
		var port int
		if n, err := fmt.Sscanf(m.portInput, "%d", &port); n != 1 || err != nil {
			content += errorStyle.Render("Invalid port number") + "\n"
		} else if port > 65535 || port < 1024 {
			content += errorStyle.Render("Port must be between 1024-65535") + "\n"
		}
	}

	footerText := "Enter: confirm  Esc: cancel"
	return RenderWithFooter(content, footerText, m.width, m.height)
}


func (m *Model) renderErrorModal() string {
	// Get the background view to show behind the modal
	var backgroundView string
	switch m.previousView {
	case detailView:
		backgroundView = m.renderDetailView()
	case helpView:
		backgroundView = m.renderHelpView()
	case portInputView:
		backgroundView = m.renderPortInputView()
	default:
		backgroundView = m.renderListView()
	}

	// Create the error modal overlay
	modalWidth := 60
	modalHeight := 8

	// Center the modal
	startCol := (m.width - modalWidth) / 2
	startRow := (m.height - modalHeight) / 2

	// Create the modal content
	title := errorStyle.Render("⚠ Error")
	message := strings.Join(wordWrap(m.errorMessage, modalWidth-4), "\n")
	
	modalContent := fmt.Sprintf("%s\n\n%s\n\nPress Enter/Esc/Space to dismiss", title, message)

	// Style the modal box
	modal := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#FF0000")).
		Background(lipgloss.Color("#000000")).
		Foreground(lipgloss.Color("#FFFFFF")).
		Width(modalWidth-4).
		Padding(1, 2).
		Render(modalContent)

	// Split background into lines
	backgroundLines := strings.Split(backgroundView, "\n")
	
	// Overlay the modal on the background
	modalLines := strings.Split(modal, "\n")
	for i, modalLine := range modalLines {
		row := startRow + i
		if row >= 0 && row < len(backgroundLines) {
			// Replace part of the background line with the modal line
			backgroundLine := backgroundLines[row]
			if len(backgroundLine) > startCol {
				before := ""
				after := ""
				if startCol > 0 {
					before = backgroundLine[:startCol]
				}
				if startCol+len(modalLine) < len(backgroundLine) {
					after = backgroundLine[startCol+len(modalLine):]
				}
				backgroundLines[row] = before + modalLine + after
			}
		}
	}

	return strings.Join(backgroundLines, "\n")
}

// wordWrap wraps text to specified width
func wordWrap(text string, width int) []string {
	words := strings.Fields(text)
	if len(words) == 0 {
		return []string{}
	}

	var lines []string
	var currentLine strings.Builder

	for _, word := range words {
		if currentLine.Len() == 0 {
			currentLine.WriteString(word)
		} else if currentLine.Len()+1+len(word) <= width {
			currentLine.WriteString(" " + word)
		} else {
			lines = append(lines, currentLine.String())
			currentLine.Reset()
			currentLine.WriteString(word)
		}
	}

	if currentLine.Len() > 0 {
		lines = append(lines, currentLine.String())
	}

	return lines
}

