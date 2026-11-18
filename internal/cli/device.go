package cli

import (
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/291-Group/LAN-Orangutan/internal/scanner"
	"github.com/291-Group/LAN-Orangutan/internal/types"
)

var (
	deviceLabel      string
	deviceHostname   string
	deviceNotes      string
	deviceGroup      string
	deviceLocation   string
	deviceType       string
	deviceExpected   bool
	deviceShowFormat string
	expectAllClear   bool
)

var deviceCmd = &cobra.Command{
	Use:   "device",
	Short: "Show, edit or delete a device",
	Long: `Show, edit or delete a single device, identified by its IP address.

Everything the edit dialog in the web interface can change can be changed here,
and it is safe to do while "orangutan serve" is running on the same data.`,
	// Without an action of its own cobra prints the help for any arguments,
	// so a misspelt subcommand would exit 0. With one, NoArgs rejects it.
	// SilenceUsage here because NoArgs fails before PersistentPreRun gets to
	// set it, and the error line says all that is needed.
	Args:         cobra.NoArgs,
	SilenceUsage: true,
	RunE:         func(cmd *cobra.Command, args []string) error { return cmd.Help() },
}

var deviceShowCmd = &cobra.Command{
	Use:   "show <ip>",
	Short: "Show everything stored for a device",
	Long: `Show everything stored for a device. With --format json the output is the
same object as GET /api/device?ip=<ip>: the stored record plus "online".`,
	Args: cobra.ExactArgs(1),
	RunE: runDeviceShow,
}

var deviceSetCmd = &cobra.Command{
	Use:   "set <ip>",
	Short: "Change a device's label, hostname, notes, group, location, type or expected status",
	Long: `Change one or more of a device's fields. Only the flags you pass are changed.

  --label      A short tag shown next to the device in the list ("NAS",
               "Guest"). Pass "" to remove it.
  --hostname   Your name for the device, shown in place of the scanned one.
               Scans never overwrite it. Pass "" to go back to the scanned name.
  --notes      Free text kept with the device: where it is plugged in, who
               owns it, a serial number. Pass "" to clear.
  --group      One of the groups in "orangutan groups" ("" for none).
  --location   Where the device is. A new place is added to the list.
  --type       One of the built-in types, or "auto" to let scans detect it again.
  --expected   Mark the device as part of your known network (--expected=false to undo).`,
	Example: `  orangutan device set 192.168.1.20 --hostname "Living Room TV" --group Family
  orangutan device set 192.168.1.20 --hostname ""
  orangutan device set 192.168.1.25 --location "Living Room"
  orangutan device set 192.168.1.30 --type Printer --expected
  orangutan device set 192.168.1.30 --label NAS --notes "Rack, shelf 2"`,
	Args: cobra.ExactArgs(1),
	RunE: runDeviceSet,
}

var deviceDeleteCmd = &cobra.Command{
	Use:   "delete <ip>",
	Short: "Delete a device from the list",
	Args:  cobra.ExactArgs(1),
	RunE:  runDeviceDelete,
}

var deviceExpectAllCmd = &cobra.Command{
	Use:   "expect-all",
	Short: "Mark every current device as expected",
	Long: `Mark every device in the list as expected, so from now on only new devices
are flagged as unexpected and only expected ones going offline are flagged as
missing. The same as "Mark all as expected" in the web interface.

With --clear, the mark is removed from every device instead.`,
	Args: cobra.NoArgs,
	RunE: runDeviceExpectAll,
}

func init() {
	deviceShowCmd.Flags().StringVar(&deviceShowFormat, "format", "table", "Output format (table, json)")

	f := deviceSetCmd.Flags()
	f.StringVar(&deviceLabel, "label", "", `Short tag shown in the list ("" to remove)`)
	f.StringVar(&deviceHostname, "hostname", "", `Custom hostname ("" to use the scanned one)`)
	f.StringVar(&deviceNotes, "notes", "", `Free-text notes kept with the device ("" to clear)`)
	f.StringVar(&deviceGroup, "group", "", `Group ("" for none)`)
	f.StringVar(&deviceLocation, "location", "", `Location ("" for none)`)
	f.StringVar(&deviceType, "type", "", fmt.Sprintf("Device type (%s, or auto)", strings.Join(scanner.KnownTypes, ", ")))
	f.BoolVar(&deviceExpected, "expected", false, "Mark as expected (--expected=false to clear)")

	deviceExpectAllCmd.Flags().BoolVar(&expectAllClear, "clear", false, "Remove the expected mark from every device instead")

	deviceCmd.AddCommand(deviceShowCmd, deviceSetCmd, deviceDeleteCmd, deviceExpectAllCmd)
}

func runDeviceShow(cmd *cobra.Command, args []string) error {
	format, err := outputFormat(deviceShowFormat, "table", "json")
	if err != nil {
		return err
	}
	store, err := openStore()
	if err != nil {
		return err
	}
	d := store.GetDevice(args[0])
	if d == nil {
		return fmt.Errorf("no device at %s", args[0])
	}
	if format == "json" {
		// The same shape as /api/device: the record plus whether it is online.
		return printJSON(struct {
			*types.Device
			Online bool `json:"online"`
		}{d, d.IsOnline()})
	}

	hostname := d.DisplayHostname()
	if d.CustomHostname != "" {
		scanned := d.Hostname
		if scanned == "" {
			scanned = "none"
		}
		hostname += fmt.Sprintf("  (yours; scanned: %s)", scanned)
	}
	typ := d.Type
	if d.CustomType {
		typ += "  (set by you)"
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	row := func(k, v string) {
		if v == "" {
			v = "-"
		}
		fmt.Fprintf(w, "%s\t%s\n", k, v)
	}
	row("IP", d.IP)
	row("IPv6", d.IPv6)
	row("MAC", d.MAC)
	row("Hostname", hostname)
	row("Vendor", scanner.ResolveVendor(d.Vendor, d.MAC))
	row("Type", typ)
	row("Label", d.Label)
	row("Group", d.Group)
	row("Location", d.Location)
	row("Expected", yesNo(d.Expected))
	row("Status", deviceStatus(d))
	row("First seen", formatTime(d.FirstSeen))
	row("Last seen", formatTime(d.LastSeen))
	for _, a := range d.AddressHistory {
		row("Was at", fmt.Sprintf("%s until %s", a.IP, formatTime(a.ChangedAt)))
	}
	if err := w.Flush(); err != nil {
		return err
	}

	// Notes can span lines, which would throw off the column alignment above,
	// so they come last, indented, on their own.
	if d.Notes != "" {
		fmt.Println("Notes")
		for _, line := range strings.Split(strings.TrimRight(d.Notes, "\n"), "\n") {
			fmt.Printf("  %s\n", line)
		}
	}
	return nil
}

func runDeviceSet(cmd *cobra.Command, args []string) error {
	f := cmd.Flags()
	var label, hostname, notes, group, location, typ *string
	var custom, expected *bool
	if f.Changed("label") {
		label = &deviceLabel
	}
	if f.Changed("hostname") {
		hostname = &deviceHostname
	}
	if f.Changed("notes") {
		notes = &deviceNotes
	}
	if f.Changed("group") {
		group = &deviceGroup
	}
	if f.Changed("location") {
		location = &deviceLocation
	}
	typeCleared := false
	if f.Changed("type") {
		t := deviceType
		if strings.EqualFold(strings.TrimSpace(t), "auto") {
			t = ""
			typeCleared = true
		}
		isCustom := t != ""
		typ, custom = &t, &isCustom
	}
	if f.Changed("expected") {
		expected = &deviceExpected
	}
	if label == nil && hostname == nil && notes == nil && group == nil && location == nil && typ == nil && expected == nil {
		return fmt.Errorf("nothing to change: pass at least one of --label, --hostname, --notes, --group, --location, --type, --expected")
	}

	store, err := openStore()
	if err != nil {
		return err
	}
	if store.GetDevice(args[0]) == nil {
		return fmt.Errorf("no device at %s", args[0])
	}
	if err := store.UpdateDeviceFields(args[0], label, notes, group, expected, typ, custom, hostname, location); err != nil {
		return err
	}
	fmt.Printf("Updated %s\n", args[0])
	if typeCleared {
		fmt.Println("Type cleared; the next scan will detect it again")
	}
	return nil
}

func runDeviceDelete(cmd *cobra.Command, args []string) error {
	store, err := openStore()
	if err != nil {
		return err
	}
	if store.GetDevice(args[0]) == nil {
		return fmt.Errorf("no device at %s", args[0])
	}
	if err := store.DeleteDevice(args[0]); err != nil {
		return err
	}
	fmt.Printf("Deleted %s\n", args[0])
	return nil
}

func runDeviceExpectAll(cmd *cobra.Command, args []string) error {
	store, err := openStore()
	if err != nil {
		return err
	}
	// Count before the change so the message can say what actually changed.
	already, changed := 0, 0
	for _, d := range store.GetDevices() {
		if d.Expected == !expectAllClear {
			already++
		} else {
			changed++
		}
	}
	if err := store.MarkAllExpected(!expectAllClear); err != nil {
		return err
	}
	if expectAllClear {
		fmt.Printf("Cleared the expected mark from %s, %d not marked\n", count(changed, "device", "devices"), already)
	} else {
		fmt.Printf("Marked %s as expected, %d already marked\n", count(changed, "device", "devices"), already)
	}
	return nil
}
