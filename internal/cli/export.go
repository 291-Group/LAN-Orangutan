package cli

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/spf13/cobra"

	"github.com/291-Group/LAN-Orangutan/internal/storage"
	"github.com/291-Group/LAN-Orangutan/internal/types"
)

var exportCmd = &cobra.Command{
	Use:   "export <file>",
	Short: "Export devices to CSV file",
	Args:  cobra.ExactArgs(1),
	RunE:  runExport,
}

func runExport(cmd *cobra.Command, args []string) error {
	outputPath := args[0]

	// Validate path (prevent path traversal)
	absPath, err := filepath.Abs(outputPath)
	if err != nil {
		return fmt.Errorf("invalid path: %w", err)
	}

	// Ensure parent directory exists
	dir := filepath.Dir(absPath)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return fmt.Errorf("directory does not exist: %s", dir)
	}

	// Initialize storage
	store, err := storage.New(cfg.DevicesFile(), cfg.StateFile())
	if err != nil {
		return fmt.Errorf("failed to initialize storage: %w", err)
	}

	devices := store.GetDevices()

	// Convert to slice and sort by IP
	var deviceList []*types.Device
	for _, d := range devices {
		deviceList = append(deviceList, d)
	}
	sort.Slice(deviceList, func(i, j int) bool {
		return ipToSortKey(deviceList[i].IP) < ipToSortKey(deviceList[j].IP)
	})

	// Create output file
	file, err := os.Create(absPath)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()

	// Write CSV
	w := csv.NewWriter(file)
	defer w.Flush()

	// Header
	if err := w.Write([]string{
		"IP Address",
		"MAC Address",
		"Hostname",
		"Vendor",
		"Label",
		"Notes",
		"Group",
		"First Seen",
		"Last Seen",
		"Status",
	}); err != nil {
		return fmt.Errorf("failed to write header: %w", err)
	}

	// Data rows
	for _, d := range deviceList {
		status := "offline"
		if d.IsOnline() {
			status = "online"
		}

		if err := w.Write([]string{
			d.IP,
			d.MAC,
			d.Hostname,
			d.Vendor,
			d.Label,
			d.Notes,
			d.Group,
			d.FirstSeen.Format("2006-01-02 15:04:05"),
			d.LastSeen.Format("2006-01-02 15:04:05"),
			status,
		}); err != nil {
			return fmt.Errorf("failed to write row: %w", err)
		}
	}

	fmt.Printf("Exported %d devices to %s\n", len(deviceList), absPath)
	return nil
}
