# KPF - Kubernetes Port Forwarding TUI

A terminal user interface (TUI) application for managing Kubernetes port forwarding with an interactive interface.

## Features

- Interactive TUI built with bubbletea
- List all services across namespaces or filter by namespace
- View service details including ports and types
- Start/stop port forwarding with visual indicators
- Automatic local port assignment
- Multiple concurrent port forwards support
- Real-time status updates

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
- `Enter` - View service details
- `r` - Refresh service list
- `q` - Quit application
- `?` - Show help

### Detail View
- `s` - Start port forwarding
- `x` - Stop port forwarding
- `Esc/b` - Back to list
- `q` - Quit application

## Visual Indicators

- Green dot (●) - Port forwarding is active
- Service details show the local port when forwarding is active
- Status messages appear for actions and errors

## Architecture

The project follows a clean architecture pattern:

```
.
├── cmd/           # CLI command processing (cobra/viper)
├── internal/
│   ├── k8s/       # Kubernetes client and service listing
│   ├── tui/       # Bubbletea TUI components
│   └── portforward/ # Port forwarding management
└── pkg/           # Reusable packages
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