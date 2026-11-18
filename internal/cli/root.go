// Package cli implements the command-line interface
package cli

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/291-Group/LAN-Orangutan/internal/config"
	"github.com/291-Group/LAN-Orangutan/internal/notify"
	"github.com/291-Group/LAN-Orangutan/internal/storage"
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

	// Execute prints errors itself; without this cobra prints them a second
	// time.
	SilenceErrors: true,

	// Flags have parsed by the time this runs, so anything that fails from here
	// is a runtime problem rather than a usage mistake. Silencing usage now
	// keeps a real error visible instead of burying it under the flag listing,
	// while a genuine flag mistake still gets the full help.
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		cmd.SilenceUsage = true
	},
}

// exitCode is an error that carries the status to exit with and has already
// been explained on stderr, for outcomes that are neither success nor failure,
// such as a scan that only the ARP table could answer.
type exitCode int

func (e exitCode) Error() string { return fmt.Sprintf("exit status %d", int(e)) }

// Execute adds all child commands and runs the CLI
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		var code exitCode
		if errors.As(err, &code) {
			os.Exit(int(code))
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", config.GetDefaultConfigFile(), "config file path")
	rootCmd.SetVersionTemplate("LAN Orangutan {{.Version}}\n")

	// Add subcommands
	rootCmd.AddCommand(scanCmd)
	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(deviceCmd)
	rootCmd.AddCommand(groupsCmd)
	rootCmd.AddCommand(locationsCmd)
	rootCmd.AddCommand(snapshotCmd)
	rootCmd.AddCommand(backupCmd)
	rootCmd.AddCommand(restoreCmd)
	rootCmd.AddCommand(reportCmd)
	rootCmd.AddCommand(tokenCmd)
	rootCmd.AddCommand(webhookCmd)
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(networksCmd)
	rootCmd.AddCommand(exportCmd)
	rootCmd.AddCommand(configCmd)
	rootCmd.AddCommand(serveCmd)
	rootCmd.AddCommand(versionCmd)
}

func initConfig() {
	// A missing file is fine when it is the default location, since most
	// installs run on defaults. A path the user typed is another matter: a
	// typo there would otherwise be obeyed silently, as an empty config.
	if rootCmd.PersistentFlags().Changed("config") {
		if _, err := os.Stat(cfgFile); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: config file %s cannot be read (%v); using the defaults\n", cfgFile, err)
		}
	}

	var err error
	cfg, err = config.Load(cfgFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	// Environment variables override the file, so containers can be configured
	// without mounting one.
	cfg.ApplyEnv()

	// The webhook User-Agent carries the version. It is final once every
	// package has initialised, which is the case by the time this runs.
	notify.Version = Version
}

// openStore opens the device list and the settings overrides in the data
// directory. Every command that reads or changes devices goes through here, so
// a data directory problem reads the same everywhere.
func openStore() (*storage.Storage, error) {
	store, err := storage.New(cfg.DevicesFile(), cfg.StateFile())
	if err != nil {
		return nil, fmt.Errorf("failed to initialize storage: %w", err)
	}
	return store, nil
}
