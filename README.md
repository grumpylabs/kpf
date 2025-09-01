# KPF - Kubernetes Port Forwarding TUI

A terminal user interface (TUI) application for managing Kubernetes port forwarding with an interactive interface.

## Features

- Interactive TUI built with bubbletea
- List all services across namespaces or filter by namespace
- Advanced filtering with autocomplete (by status, type, name, protocol)
- View service details including ports and types
- Start/stop port forwarding with visual indicators
- Automatic local port assignment with conflict resolution
- Multiple concurrent port forwards support
- Real-time status updates with detailed failure reporting
- Sortable columns (namespace, name, status, ports, local port)
- Port forwarding state tracking (inactive, pending, active, failed)

## Installation

```bash
go build -o kpf .
```

## Usage

```bash
# Use default kubeconfig (~/.kube/config)
./kpf

# Specify custom kubeconfig
./kpf --kubeconfig /path/to/config

# Filter by namespace
./kpf --namespace my-namespace
./kpf -n my-namespace
```

## Keyboard Shortcuts

### List View
- `↑/k` - Move up
- `↓/j` - Move down
- `pgup/pgdn` - Page up/down
- `home/end` - Go to top/bottom
- `Enter/d` - View service details
- `f` - Toggle port forwarding for selected service
- `/` - Open filter menu
- `r` - Refresh service list
- `?/h` - Show help
- `q/Esc` - Quit application

### Sorting
- `N` - Sort by namespace
- `M` - Sort by name
- `S` - Sort by status
- `P` - Sort by ports
- `L` - Sort by local port

### Filter View
- `←/→` - Change filter type (status/type/name/protocol/all)
- `Tab` - Autocomplete/cycle through suggestions
- `Enter` - Apply filter
- `C` - Clear all filters
- `Esc` - Cancel filter

### Detail View
- `f` - Toggle port forwarding
- `Esc/b` - Back to list
- `q` - Quit application

### Port Input View (conflict resolution)
- `0-9` - Enter port number
- `Enter` - Confirm port
- `Esc` - Cancel

## Visual Indicators

- **Green dot (●)** - Port forwarding is active
- **Yellow dot (●)** - Port forwarding is establishing (pending)
- **Red dot (●)** - Port forwarding failed
- **Gray dot (○)** - Port forwarding inactive
- **Service types**: C=ClusterIP, N=NodePort, L=LoadBalancer, E=ExternalName
- **Local port column** shows the forwarded port when active (e.g., `:8080`)
- **[FILTERED]** indicator shows when filters are active
- **Sort indicators** show current sort field and direction (e.g., `[name^]`)
- **Status messages** appear for actions, errors, and timeouts

## Advanced Filtering

KPF includes a powerful filtering system with autocomplete:

### Filter Types
- **status** - Filter by forwarding status (`active`, `inactive`, `pending`, `failed`)
- **type** - Filter by service type (`ClusterIP`, `NodePort`, `LoadBalancer`, `ExternalName`)
- **name** - Filter by service name (partial matching supported)
- **protocol** - Filter by port protocol (`TCP`, `UDP`)
- **all** - Search across all fields

### How to Use Filters
1. Press `/` to enter filter mode
2. Use `←/→` arrows to select filter type
3. Start typing to see matching suggestions
4. Use `Tab` to cycle through autocomplete suggestions
5. Press `Enter` to apply the filter
6. Press `C` to clear all active filters

### Examples
- Filter for active forwards: Select `status` type, then type `active`
- Find NodePort services: Select `type` type, then type `NodePort`
- Search by name: Select `name` type, then type part of service name

## Architecture

The project follows a clean architecture pattern:

```
.
├── cmd/           # CLI command processing (cobra/viper)
├── internal/
│   ├── k8s/       # Kubernetes client and service listing
│   ├── tui/       # Bubbletea TUI components
│   └── portforward/ # Port forwarding management
```

## Requirements

- Go 1.24+
- Access to a Kubernetes cluster
- Valid kubeconfig file

## Dependencies

- `github.com/charmbracelet/bubbletea` - TUI framework
- `github.com/charmbracelet/bubbles` - TUI components
- `github.com/charmbracelet/lipgloss` - TUI styling
- `github.com/spf13/cobra` - CLI framework
- `github.com/spf13/viper` - Configuration management
- `k8s.io/client-go` - Kubernetes Go client
