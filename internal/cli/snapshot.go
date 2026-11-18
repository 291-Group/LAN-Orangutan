package cli

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/291-Group/LAN-Orangutan/internal/snapshot"
	"github.com/291-Group/LAN-Orangutan/internal/storage"
	"github.com/291-Group/LAN-Orangutan/internal/types"
)

var (
	snapshotListFormat    string
	snapshotCompareFormat string
)

var snapshotCmd = &cobra.Command{
	Use:   "snapshot",
	Short: "Save, list, compare or delete snapshots of the device list",
	Long: `A snapshot is a saved copy of the device list. Compare two of them, or one
against the network now, to see what appeared, what disappeared and what changed.

Snapshots can be referred to by their number in "orangutan snapshot list"
(1 is the newest) or by their ID.`,
	Args: cobra.NoArgs,
	RunE: runSnapshotList,
}

var snapshotSaveCmd = &cobra.Command{
	Use:   "save [name]",
	Short: "Save a snapshot of the current device list",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := openStore()
		if err != nil {
			return err
		}
		name := ""
		if len(args) == 1 {
			name = args[0]
		}
		meta, err := store.SaveSnapshot(name)
		if err != nil {
			return err
		}
		fmt.Printf("Saved snapshot %q with %d devices\n", meta.Name, meta.DeviceCount)
		return nil
	},
}

var snapshotListCmd = &cobra.Command{
	Use:   "list",
	Short: "List saved snapshots, newest first",
	Long: `List saved snapshots, newest first. With --format json the output is the same
array as GET /api/snapshots.`,
	Args: cobra.NoArgs,
	RunE: runSnapshotList,
}

var snapshotCompareCmd = &cobra.Command{
	Use:   "compare <from> [to]",
	Short: "Compare a snapshot with a later one, or with the network now",
	Long: `Compare a snapshot with a later one, or with the network now, listing what is
new, what is gone and what changed. The comparison always runs from the older
snapshot to the newer, whichever order they are given in. With --format json
the output is the same object as GET /api/snapshots/compare.`,
	Example: `  orangutan snapshot compare 2        # second newest against now
  orangutan snapshot compare 3 1      # third newest against the newest`,
	Args: cobra.RangeArgs(1, 2),
	RunE: runSnapshotCompare,
}

var snapshotDeleteCmd = &cobra.Command{
	Use:   "delete <snapshot>",
	Short: "Delete a saved snapshot",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := openStore()
		if err != nil {
			return err
		}
		meta, err := resolveSnapshot(store, args[0])
		if err != nil {
			return err
		}
		if err := store.DeleteSnapshot(meta.ID); err != nil {
			return err
		}
		fmt.Printf("Deleted snapshot %q\n", meta.Name)
		return nil
	},
}

func init() {
	for _, c := range []*cobra.Command{snapshotCmd, snapshotListCmd} {
		c.Flags().StringVar(&snapshotListFormat, "format", "table", "Output format (table, json)")
	}
	snapshotCompareCmd.Flags().StringVar(&snapshotCompareFormat, "format", "table", "Output format (table, json)")
	snapshotCmd.AddCommand(snapshotSaveCmd, snapshotListCmd, snapshotCompareCmd, snapshotDeleteCmd)
}

func runSnapshotList(cmd *cobra.Command, args []string) error {
	format, err := outputFormat(snapshotListFormat, "table", "json")
	if err != nil {
		return err
	}
	store, err := openStore()
	if err != nil {
		return err
	}
	metas, err := store.ListSnapshots()
	if err != nil {
		return err
	}
	if format == "json" {
		if metas == nil {
			metas = []types.SnapshotMeta{}
		}
		return printJSON(metas)
	}
	if len(metas) == 0 {
		fmt.Println(`No snapshots yet. Save one with "orangutan snapshot save".`)
		return nil
	}
	rows := make([][]string, 0, len(metas))
	for i, m := range metas {
		rows = append(rows, []string{strconv.Itoa(i + 1), m.Name, formatTime(m.CreatedAt), strconv.Itoa(m.DeviceCount), m.ID})
	}
	return printTable(os.Stdout, []string{"#", "NAME", "SAVED", "DEVICES", "ID"}, rows)
}

// resolveSnapshot finds a snapshot by its position in the list (1 = newest)
// or by its ID.
func resolveSnapshot(store *storage.Storage, ref string) (types.SnapshotMeta, error) {
	metas, err := store.ListSnapshots()
	if err != nil {
		return types.SnapshotMeta{}, err
	}
	for _, m := range metas {
		if m.ID == ref {
			return m, nil
		}
	}
	if n, err := strconv.Atoi(ref); err == nil && n >= 1 && n <= len(metas) {
		return metas[n-1], nil
	}
	return types.SnapshotMeta{}, fmt.Errorf("no snapshot %q; see \"orangutan snapshot list\"", ref)
}

func runSnapshotCompare(cmd *cobra.Command, args []string) error {
	format, err := outputFormat(snapshotCompareFormat, "table", "json")
	if err != nil {
		return err
	}
	store, err := openStore()
	if err != nil {
		return err
	}
	fromMeta, err := resolveSnapshot(store, args[0])
	if err != nil {
		return err
	}

	toLabel := "now"
	var after []types.Device
	if len(args) == 2 && args[1] != "current" && args[1] != "now" {
		toMeta, err := resolveSnapshot(store, args[1])
		if err != nil {
			return err
		}
		// "New" and "gone" only mean something from the older list to the
		// newer, so a pair given newest first is turned round rather than
		// reported backwards.
		if toMeta.CreatedAt.Before(fromMeta.CreatedAt) {
			fmt.Fprintf(os.Stderr, "Note: %q is newer than %q; comparing from the older one\n", fromMeta.Name, toMeta.Name)
			fromMeta, toMeta = toMeta, fromMeta
		}
		to, err := store.GetSnapshot(toMeta.ID)
		if err != nil {
			return err
		}
		after = to.Devices
		toLabel = fmt.Sprintf("%q", toMeta.Name)
	} else {
		for _, d := range store.GetDevices() {
			after = append(after, *d)
		}
	}
	from, err := store.GetSnapshot(fromMeta.ID)
	if err != nil {
		return err
	}

	diff := snapshot.Compare(from.Devices, after)
	if format == "json" {
		return printJSON(diff)
	}
	fmt.Printf("From %q to %s\n\n", fromMeta.Name, toLabel)
	if len(diff.Added)+len(diff.Removed)+len(diff.Changed) == 0 {
		fmt.Println("No changes.")
		return nil
	}
	describe := func(d types.Device) string {
		s := d.IP
		if name := d.DisplayHostname(); name != "" {
			s += "  " + name
		}
		if d.Label != "" {
			s += "  (" + d.Label + ")"
		}
		return s
	}
	if len(diff.Added) > 0 {
		fmt.Printf("New (%d)\n", len(diff.Added))
		for _, d := range diff.Added {
			fmt.Println("  + " + describe(d))
		}
		fmt.Println()
	}
	if len(diff.Removed) > 0 {
		fmt.Printf("Gone (%d)\n", len(diff.Removed))
		for _, d := range diff.Removed {
			fmt.Println("  - " + describe(d))
		}
		fmt.Println()
	}
	if len(diff.Changed) > 0 {
		fmt.Printf("Changed (%d)\n", len(diff.Changed))
		for _, c := range diff.Changed {
			var parts []string
			for _, f := range c.Fields {
				parts = append(parts, fmt.Sprintf("%s: %s -> %s", f, orDash(snapshot.FieldValue(c.Before, f)), orDash(snapshot.FieldValue(c.After, f))))
			}
			fmt.Printf("  ~ %s  %s\n", describe(c.After), strings.Join(parts, ", "))
		}
	}
	return nil
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
