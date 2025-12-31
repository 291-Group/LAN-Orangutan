// Package cli implements the command-line interface
package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/291-Group/LAN-Orangutan/internal/config"
)

var (
	cfgFile string
	cfg     *config.Config
)

// rootCmd represents the base command
var rootCmd = &cobra.Command{
	Use:   "orangutan",
	Short: "LAN Orangutan - Network Discovery Tool",
	Long: `LAN Orangutan is a network discovery and monitoring tool.
It scans your local network to find devices and provides
a web interface for viewing and managing discovered devices.`,
}

// Execute adds all child commands and runs the CLI
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", config.DefaultConfigFile, "config file path")

	// Add subcommands
	rootCmd.AddCommand(scanCmd)
	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(networksCmd)
	rootCmd.AddCommand(exportCmd)
	rootCmd.AddCommand(configCmd)
	rootCmd.AddCommand(serveCmd)
	rootCmd.AddCommand(versionCmd)
}

func initConfig() {
	var err error
	cfg, err = config.Load(cfgFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}
}
