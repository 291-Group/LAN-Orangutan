package cli

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/291-Group/LAN-Orangutan/internal/scanner"
	"github.com/291-Group/LAN-Orangutan/internal/types"
)

var (
	listOnline   bool
	listOffline  bool
	listGroup    string
	listLocation string
	listType     string
	listFormat   string
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List discovered devices",
	Long: `List the discovered devices, sorted by IP address.

Narrow the list with --online or --offline, or by --group, --location or
--type (all matched regardless of case). A device is online when it was seen
within the last hour, the same rule the web interface and the API use.

The csv output has the same columns as "orangutan export"; the json output is
an array with one object per device, with first_seen and last_seen in RFC 3339.`,
	Args: cobra.NoArgs,
	RunE: runList,
}

func init() {
	listCmd.Flags().BoolVar(&listOnline, "online", false, "Show only online devices")
	listCmd.Flags().BoolVar(&listOffline, "offline", false, "Show only offline devices")
	listCmd.Flags().StringVar(&listGroup, "group", "", "Filter by group")
	listCmd.Flags().StringVar(&listLocation, "location", "", "Filter by location")
	listCmd.Flags().StringVar(&listType, "type", "", "Filter by device type")
	listCmd.Flags().StringVar(&listFormat, "format", "table", "Output format (table, csv, json)")
}

func runList(cmd *cobra.Command, args []string) error {
	format, err := outputFormat(listFormat, "table", "csv", "json")
	if err != nil {
		return err
	}
	if listOnline && listOffline {
		return fmt.Errorf("choose one of --online or --offline")
	}

	store, err := openStore()
	if err != nil {
		return err
	}

	var filtered []*types.Device
	for _, d := range store.GetDevices() {
		if listOnline && !d.IsOnline() {
			continue
		}
		if listOffline && d.IsOnline() {
			continue
		}
		if listGroup != "" && !strings.EqualFold(d.Group, listGroup) {
			continue
		}
		if listLocation != "" && !strings.EqualFold(d.Location, listLocation) {
			continue
		}
		if listType != "" && !strings.EqualFold(d.Type, listType) {
			continue
		}
		filtered = append(filtered, d)
	}
	sortByIP(filtered)

	switch format {
	case "csv":
		return writeDevicesCSV(os.Stdout, filtered)
	case "json":
		return writeDevicesJSON(os.Stdout, filtered)
	default:
		return outputTable(filtered)
	}
}

func outputTable(devices []*types.Device) error {
	if len(devices) == 0 {
		fmt.Println("No devices found")
		return nil
	}

	rows := make([][]string, 0, len(devices))
	for _, d := range devices {
		rows = append(rows, []string{
			d.IP,
			d.MAC,
			truncate(d.DisplayHostname(), 25),
			truncate(scanner.ResolveVendor(d.Vendor, d.MAC), 20),
			d.Label,
			d.Group,
			d.Location,
			deviceStatus(d),
		})
	}
	return printTable(os.Stdout, []string{"IP", "MAC", "HOSTNAME", "VENDOR", "LABEL", "GROUP", "LOCATION", "STATUS"}, rows)
}

// writeDevicesCSV writes devices with the same columns and header as
// "orangutan export" and the browser's CSV download, so the three files are
// interchangeable.
func writeDevicesCSV(out io.Writer, devices []*types.Device) error {
	w := csv.NewWriter(out)
	if err := w.Write([]string{
		"IP Address", "MAC Address", "Hostname", "Vendor", "Label",
		"Notes", "Group", "Location", "First Seen", "Last Seen", "Status",
	}); err != nil {
		return fmt.Errorf("failed to write header: %w", err)
	}
	for _, d := range devices {
		if err := w.Write([]string{
			d.IP,
			d.MAC,
			d.DisplayHostname(),
			scanner.ResolveVendor(d.Vendor, d.MAC),
			d.Label,
			d.Notes,
			d.Group,
			d.Location,
			d.FirstSeen.Local().Format(csvTimeLayout),
			d.LastSeen.Local().Format(csvTimeLayout),
			deviceStatus(d),
		}); err != nil {
			return fmt.Errorf("failed to write row: %w", err)
		}
	}
	w.Flush()
	return w.Error()
}

// deviceJSON is one device as "list --format json" and "export <file>.json"
// print it: the fields a script filters on, with the hostname already resolved
// to the one the web interface shows.
type deviceJSON struct {
	IP              string `json:"ip"`
	MAC             string `json:"mac"`
	Hostname        string `json:"hostname"`
	ScannedHostname string `json:"scanned_hostname"`
	CustomHostname  string `json:"custom_hostname"`
	Vendor          string `json:"vendor"`
	Type            string `json:"type"`
	Group           string `json:"group"`
	Location        string `json:"location"`
	Label           string `json:"label"`
	Notes           string `json:"notes"`
	Expected        bool   `json:"expected"`
	Online          bool   `json:"online"`
	Status          string `json:"status"`
	FirstSeen       string `json:"first_seen"`
	LastSeen        string `json:"last_seen"`
}

func toDeviceJSON(d *types.Device) deviceJSON {
	return deviceJSON{
		IP:              d.IP,
		MAC:             d.MAC,
		Hostname:        d.DisplayHostname(),
		ScannedHostname: d.Hostname,
		CustomHostname:  d.CustomHostname,
		Vendor:          scanner.ResolveVendor(d.Vendor, d.MAC),
		Type:            d.Type,
		Group:           d.Group,
		Location:        d.Location,
		Label:           d.Label,
		Notes:           d.Notes,
		Expected:        d.Expected,
		Online:          d.IsOnline(),
		Status:          deviceStatus(d),
		FirstSeen:       d.FirstSeen.Format(time.RFC3339),
		LastSeen:        d.LastSeen.Format(time.RFC3339),
	}
}

func writeDevicesJSON(out io.Writer, devices []*types.Device) error {
	// An empty list is "[]", not "null", so a script can always range over it.
	list := make([]deviceJSON, 0, len(devices))
	for _, d := range devices {
		list = append(list, toDeviceJSON(d))
	}
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(list)
}

// sortByIP orders devices numerically by IPv4 address, so 192.168.1.9 comes
// before 192.168.1.10. Anything that is not an IPv4 address sorts last.
func sortByIP(devices []*types.Device) {
	sort.Slice(devices, func(i, j int) bool {
		return ipToSortKey(devices[i].IP) < ipToSortKey(devices[j].IP)
	})
}

// ipToSortKey converts an IP to a sortable integer
func ipToSortKey(ipStr string) int64 {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return 0x7FFFFFFFFFFFFFFF // Sort invalid IPs last
	}
	ip = ip.To4()
	if ip == nil {
		return 0x7FFFFFFFFFFFFFFF
	}
	return int64(ip[0])<<24 | int64(ip[1])<<16 | int64(ip[2])<<8 | int64(ip[3])
}
