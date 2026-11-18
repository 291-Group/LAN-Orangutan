package cli

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/291-Group/LAN-Orangutan/internal/network"
	"github.com/291-Group/LAN-Orangutan/internal/report"
)

var reportForce bool

var reportCmd = &cobra.Command{
	Use:   "report [file]",
	Short: "Save a shareable HTML report of your network",
	Long: `Writes the same single-file HTML report as "Report" in the web interface.
It needs nothing else to open. Defaults to lan-orangutan-report-<date>.html.

An existing file is not overwritten unless --force is given.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runReport,
}

func init() {
	reportCmd.Flags().BoolVar(&reportForce, "force", false, "Overwrite the file if it exists")
}

func runReport(cmd *cobra.Command, args []string) error {
	path := "lan-orangutan-report-" + time.Now().Format("2006-01-02") + ".html"
	if len(args) == 1 {
		path = args[0]
	}
	absPath, err := outputFile(path, reportForce)
	if err != nil {
		return err
	}
	store, err := openStore()
	if err != nil {
		return err
	}
	networkCount := 0
	if nets, err := network.DetectNetworks(); err == nil {
		networkCount = len(network.WithConfigured(nets, networkFilter(store, cfg)))
	}

	f, err := os.Create(absPath)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	data := report.NewData(store.GetDevices(), store.GetStats(), networkCount, time.Now())
	if err := report.Render(f, data); err != nil {
		f.Close()
		return fmt.Errorf("failed to write the report: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("failed to write the report: %w", err)
	}
	fmt.Printf("Saved a report of %d devices to %s\n", data.Total, absPath)
	return nil
}
