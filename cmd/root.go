package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	kubeconfig string
	namespace  string
	appVersion string = "dev"
	appCommit  string = "unknown"
	appDate    string = "unknown"
)

func SetVersionInfo(version, commit, date string) {
	appVersion = version
	appCommit = commit
	appDate = date
}

var rootCmd = &cobra.Command{
	Use:   "kpf",
	Short: "Kubernetes Port Forwarding TUI",
	Long:  `A TUI application for managing Kubernetes port forwarding with an interactive interface.`,
	Run: func(cmd *cobra.Command, args []string) {
		runTUI()
	},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)

	rootCmd.PersistentFlags().StringVar(&kubeconfig, "kubeconfig", "", "path to kubeconfig file (default: $HOME/.kube/config)")
	rootCmd.PersistentFlags().StringVarP(&namespace, "namespace", "n", "", "kubernetes namespace (default: all namespaces)")

	viper.BindPFlag("kubeconfig", rootCmd.PersistentFlags().Lookup("kubeconfig"))
	viper.BindPFlag("namespace", rootCmd.PersistentFlags().Lookup("namespace"))
}

func initConfig() {
	viper.SetEnvPrefix("KPF")
	viper.AutomaticEnv()

	if kubeconfig == "" {
		if home := os.Getenv("HOME"); home != "" {
			viper.SetDefault("kubeconfig", fmt.Sprintf("%s/.kube/config", home))
		}
	}
}
