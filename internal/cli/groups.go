package cli

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/291-Group/LAN-Orangutan/internal/storage"
)

var groupsFormat string

var groupsCmd = &cobra.Command{
	Use:   "groups",
	Short: "List, add, rename or delete device groups",
	Long: `With no subcommand (or with "list"), lists the device groups and how many
devices are in each. With --format json the output is the same object as
GET /api/groups.

Renaming a group keeps every device in it. Deleting a group never deletes
devices; they are left without a group.`,
	Args: cobra.NoArgs,
	RunE: runGroupsList,
}

var groupsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List the groups and how many devices are in each",
	Args:  cobra.NoArgs,
	RunE:  runGroupsList,
}

var groupsAddCmd = &cobra.Command{
	Use:   "add <name>",
	Short: "Add a group",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := openStore()
		if err != nil {
			return err
		}
		if err := store.AddGroup(args[0]); err != nil {
			return err
		}
		fmt.Printf("Added group %q\n", args[0])
		return nil
	},
}

var groupsRenameCmd = &cobra.Command{
	Use:   "rename <old> <new>",
	Short: "Rename a group, keeping every device in it",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := openStore()
		if err != nil {
			return err
		}
		if err := store.RenameGroup(args[0], args[1]); err != nil {
			return err
		}
		fmt.Printf("Renamed %q to %q\n", args[0], args[1])
		return nil
	},
}

var groupsDeleteCmd = &cobra.Command{
	Use:   "delete <name>",
	Short: "Delete a group (its devices are kept, without a group)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := openStore()
		if err != nil {
			return err
		}
		stored, n := foldedCount(store.GroupCounts(), args[0])
		if err := store.DeleteGroup(args[0]); err != nil {
			return err
		}
		if n > 0 {
			fmt.Printf("Deleted group %q; %s now without a group\n", stored, count(n, "device is", "devices are"))
		} else {
			fmt.Printf("Deleted group %q\n", stored)
		}
		return nil
	},
}

var groupsResetCmd = &cobra.Command{
	Use:   "reset",
	Short: "Go back to the built-in groups",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := openStore()
		if err != nil {
			return err
		}
		if err := store.SetGroups(nil); err != nil {
			return err
		}
		fmt.Println("Groups reset to the built-in list")
		return nil
	},
}

func init() {
	for _, c := range []*cobra.Command{groupsCmd, groupsListCmd} {
		c.Flags().StringVar(&groupsFormat, "format", "table", "Output format (table, json)")
	}
	groupsCmd.AddCommand(groupsListCmd, groupsAddCmd, groupsRenameCmd, groupsDeleteCmd, groupsResetCmd)
}

// foldedCount looks a name up in a per-name count the way storage matches
// names: trimmed and regardless of case. Storage deletes "iot" and "IoT" as
// the same group, so the number reported has to be found the same way.
// foldedCount finds name in counts the way storage matches it, trimmed and
// without regard to case, and returns the spelling on record with its count,
// so "groups delete iot" reports "IoT" and the right number.
func foldedCount(counts map[string]int, name string) (string, int) {
	name = strings.TrimSpace(name)
	for k, n := range counts {
		if strings.EqualFold(k, name) {
			return k, n
		}
	}
	return name, 0
}

func runGroupsList(cmd *cobra.Command, args []string) error {
	format, err := outputFormat(groupsFormat, "table", "json")
	if err != nil {
		return err
	}
	store, err := openStore()
	if err != nil {
		return err
	}
	groups := store.GetGroups()
	counts := store.GroupCounts()
	if format == "json" {
		return printJSON(map[string]any{
			"groups":     nonNilList(groups),
			"defaults":   storage.DefaultGroups(),
			"customised": store.GroupsCustomised(),
			"counts":     counts,
		})
	}
	rows := make([][]string, 0, len(groups))
	for _, g := range groups {
		rows = append(rows, []string{g, strconv.Itoa(counts[g])})
	}
	return printTable(os.Stdout, []string{"GROUP", "DEVICES"}, rows)
}

// nonNilList keeps an empty list as "[]" in JSON rather than "null".
func nonNilList(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
