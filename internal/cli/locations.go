package cli

import (
	"fmt"
	"os"
	"strconv"

	"github.com/spf13/cobra"
)

var locationsFormat string

var locationsCmd = &cobra.Command{
	Use:   "locations",
	Short: "List, add, rename, delete or prune device locations",
	Long: `With no subcommand (or with "list"), lists the locations and how many devices
are in each. With --format json the output is the same object as
GET /api/locations.

Renaming a location moves every device in it. Deleting one never deletes
devices; they are left without a location. A device is put in a location with
"orangutan device set <ip> --location <name>", which also adds a new place.

Unlike groups there is no built-in list to reset to, since rooms are personal;
"prune" removes the places no device is in any more.`,
	Args: cobra.NoArgs,
	RunE: runLocationsList,
}

var locationsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List the locations and how many devices are in each",
	Args:  cobra.NoArgs,
	RunE:  runLocationsList,
}

var locationsAddCmd = &cobra.Command{
	Use:   "add <name>",
	Short: "Add a location",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := openStore()
		if err != nil {
			return err
		}
		if err := store.AddLocation(args[0]); err != nil {
			return err
		}
		fmt.Printf("Added location %q\n", args[0])
		return nil
	},
}

var locationsRenameCmd = &cobra.Command{
	Use:   "rename <old> <new>",
	Short: "Rename a location, moving every device in it",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := openStore()
		if err != nil {
			return err
		}
		if err := store.RenameLocation(args[0], args[1]); err != nil {
			return err
		}
		fmt.Printf("Renamed %q to %q\n", args[0], args[1])
		return nil
	},
}

var locationsDeleteCmd = &cobra.Command{
	Use:   "delete <name>",
	Short: "Delete a location (its devices are kept, without a location)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := openStore()
		if err != nil {
			return err
		}
		stored, n := foldedCount(store.LocationCounts(), args[0])
		if err := store.DeleteLocation(args[0]); err != nil {
			return err
		}
		if n > 0 {
			fmt.Printf("Deleted location %q; %s now without a location\n", stored, count(n, "device is", "devices are"))
		} else {
			fmt.Printf("Deleted location %q\n", stored)
		}
		return nil
	},
}

var locationsPruneCmd = &cobra.Command{
	Use:   "prune",
	Short: "Delete every location that no device is in",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := openStore()
		if err != nil {
			return err
		}
		counts := store.LocationCounts()
		var removed []string
		for _, l := range store.GetLocations() {
			if counts[l] > 0 {
				continue
			}
			if err := store.DeleteLocation(l); err != nil {
				return err
			}
			removed = append(removed, l)
		}
		if len(removed) == 0 {
			fmt.Println("Every location has at least one device; nothing to prune")
			return nil
		}
		fmt.Printf("Deleted %s:\n", count(len(removed), "empty location", "empty locations"))
		for _, l := range removed {
			fmt.Printf("  %s\n", l)
		}
		return nil
	},
}

func init() {
	for _, c := range []*cobra.Command{locationsCmd, locationsListCmd} {
		c.Flags().StringVar(&locationsFormat, "format", "table", "Output format (table, json)")
	}
	locationsCmd.AddCommand(locationsListCmd, locationsAddCmd, locationsRenameCmd, locationsDeleteCmd, locationsPruneCmd)
}

func runLocationsList(cmd *cobra.Command, args []string) error {
	format, err := outputFormat(locationsFormat, "table", "json")
	if err != nil {
		return err
	}
	store, err := openStore()
	if err != nil {
		return err
	}
	locs := store.GetLocations()
	counts := store.LocationCounts()
	if format == "json" {
		return printJSON(map[string]any{
			"locations": nonNilList(locs),
			"counts":    counts,
		})
	}
	if len(locs) == 0 {
		fmt.Println(`No locations yet. Add one with "orangutan locations add <name>".`)
		return nil
	}
	rows := make([][]string, 0, len(locs))
	for _, l := range locs {
		rows = append(rows, []string{l, strconv.Itoa(counts[l])})
	}
	return printTable(os.Stdout, []string{"LOCATION", "DEVICES"}, rows)
}
