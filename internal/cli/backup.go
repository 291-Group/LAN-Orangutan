package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/291-Group/LAN-Orangutan/internal/types"
)

var (
	backupForce bool
	restoreYes  bool
)

var backupCmd = &cobra.Command{
	Use:   "backup [file]",
	Short: "Back up devices, settings, webhooks and snapshots to a file",
	Long: `Writes your devices (with labels, notes and groups), settings, webhooks and
snapshots to a single file, the same file "Download backup" in Settings
produces. Defaults to lan-orangutan-backup-<date>.json.

Your password and the API token are credentials, not data, and are not
included. Webhook URLs are, and they can hold a secret (a Telegram bot token,
a Discord key), so keep the file private.

An existing file is not overwritten unless --force is given.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runBackup,
}

var restoreCmd = &cobra.Command{
	Use:   "restore <file>",
	Short: "Restore from a backup file, replacing what is there now",
	Args:  cobra.ExactArgs(1),
	RunE:  runRestore,
}

func init() {
	backupCmd.Flags().BoolVar(&backupForce, "force", false, "Overwrite the file if it exists")
	restoreCmd.Flags().BoolVarP(&restoreYes, "yes", "y", false, "Confirm replacing the current devices, settings and snapshots")
}

func runBackup(cmd *cobra.Command, args []string) error {
	path := "lan-orangutan-backup-" + time.Now().Format("2006-01-02") + ".json"
	if len(args) == 1 {
		path = args[0]
	}
	absPath, err := outputFile(path, backupForce)
	if err != nil {
		return err
	}
	store, err := openStore()
	if err != nil {
		return err
	}
	b := store.ExportBackup()
	data, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to build the backup: %w", err)
	}
	// The backup holds notes, the device list and webhook URLs, so keep it
	// private.
	if err := os.WriteFile(absPath, data, 0o600); err != nil {
		return fmt.Errorf("failed to write the backup: %w", err)
	}
	fmt.Printf("Backed up %d devices and %d snapshots to %s\n", len(b.Devices), len(b.Snapshots), absPath)
	return nil
}

func runRestore(cmd *cobra.Command, args []string) error {
	data, err := os.ReadFile(args[0])
	if err != nil {
		return fmt.Errorf("failed to read the backup: %w", err)
	}
	var b types.Backup
	if err := json.Unmarshal(data, &b); err != nil {
		return fmt.Errorf("that is not a valid backup file: %w", err)
	}
	// The same guard as the web restore: a real backup always carries its
	// creation time, so some unrelated JSON file cannot wipe everything.
	if b.CreatedAt.IsZero() {
		return fmt.Errorf("that file does not look like a LAN Orangutan backup")
	}
	if !restoreYes {
		return fmt.Errorf("this replaces your current devices, settings and snapshots with the backup from %s (%d devices). Run again with --yes to go ahead",
			formatTime(b.CreatedAt), len(b.Devices))
	}

	store, err := openStore()
	if err != nil {
		return err
	}
	if err := store.ImportBackup(b); err != nil {
		return fmt.Errorf("failed to restore: %w", err)
	}
	fmt.Printf("Restored %d devices and %d snapshots\n", len(b.Devices), len(b.Snapshots))
	// A running server re-reads the devices and state files whenever they
	// change, so the restored devices, scan settings, groups, locations and
	// webhooks are live at once. The storage limits are the exception: it
	// applies retention_days and max_devices once at startup.
	fmt.Println("A running server picks this up on its own, except the storage limits (retention days, maximum devices): restart it if the backup changed those.")
	return nil
}
