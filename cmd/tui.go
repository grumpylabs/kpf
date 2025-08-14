package cmd

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/grumpylabs/kpf/internal/k8s"
	"github.com/grumpylabs/kpf/internal/tui"
	"github.com/spf13/viper"
)

func runTUI() {
	kubeconfigPath := viper.GetString("kubeconfig")
	namespace := viper.GetString("namespace")

	// Set environment variable for the TUI to pick up
	if namespace != "" {
		os.Setenv("KPF_NAMESPACE", namespace)
	}

	client, err := k8s.NewClient(kubeconfigPath, namespace)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create Kubernetes client: %v\n", err)
		os.Exit(1)
	}

	// Get short commit hash
	shortCommit := appCommit
	if len(shortCommit) > 7 {
		shortCommit = shortCommit[:7]
	}
	
	model := tui.NewModel(client, kubeconfigPath, shortCommit)
	program := tea.NewProgram(model, tea.WithAltScreen())

	if _, err := program.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running TUI: %v\n", err)
		os.Exit(1)
	}
}
