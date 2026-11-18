package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/291-Group/LAN-Orangutan/internal/types"
)

var exportForce bool

var exportCmd = &cobra.Command{
	Use:   "export <file>",
	Short: "Export devices to a CSV or JSON file",
	Long: `Writes every device to a file. The format follows the extension: .json gives
the same output as "orangutan list --format json", anything else gives CSV
with the same columns as the browser's CSV download.

An existing file is not overwritten unless --force is given.`,
	Example: `  orangutan export devices.csv
  orangutan export devices.json --force`,
	Args: cobra.ExactArgs(1),
	RunE: runExport,
}

func init() {
	exportCmd.Flags().BoolVar(&exportForce, "force", false, "Overwrite the file if it exists")
}

// outputFile resolves where a command writes its file and checks that this is
// possible before any work is done: the directory has to exist, and an
// existing file is only replaced when the user said so with --force.
func outputFile(path string, force bool) (string, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("invalid path: %w", err)
	}
	if _, err := os.Stat(filepath.Dir(absPath)); os.IsNotExist(err) {
		return "", fmt.Errorf("directory does not exist: %s", filepath.Dir(absPath))
	}
	if _, err := os.Stat(absPath); err == nil && !force {
		return "", fmt.Errorf("%s already exists; use --force to overwrite it", absPath)
	}
	return absPath, nil
}

func runExport(cmd *cobra.Command, args []string) error {
	absPath, err := outputFile(args[0], exportForce)
	if err != nil {
		return err
	}

	store, err := openStore()
	if err != nil {
		return err
	}

	var deviceList []*types.Device
	for _, d := range store.GetDevices() {
		deviceList = append(deviceList, d)
	}
	sortByIP(deviceList)

	file, err := os.Create(absPath)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	if strings.EqualFold(filepath.Ext(absPath), ".json") {
		err = writeDevicesJSON(file, deviceList)
	} else {
		err = writeDevicesCSV(file, deviceList)
	}
	if err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	fmt.Printf("Exported %d devices to %s\n", len(deviceList), absPath)
	return nil
}
