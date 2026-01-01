package cli

import (
	"encoding/csv"
	"fmt"
	"net"
	"os"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/291-Group/LAN-Orangutan/internal/storage"
	"github.com/291-Group/LAN-Orangutan/internal/types"
)

var (
	listOnline  bool
	listOffline bool
	listGroup   string
	listFormat  string
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List discovered devices",
	Long:  `List all discovered devices with optional filtering by status or group.`,
	RunE:  runList,
}

func init() {
	listCmd.Flags().BoolVar(&listOnline, "online", false, "Show only online devices")
	listCmd.Flags().BoolVar(&listOffline, "offline", false, "Show only offline devices")
	listCmd.Flags().StringVar(&listGroup, "group", "", "Filter by group")
	listCmd.Flags().StringVar(&listFormat, "format", "table", "Output format (table, csv, json)")
}

func runList(cmd *cobra.Command, args []string) error {
	// Initialize storage
	store, err := storage.New(cfg.DevicesFile(), cfg.StateFile())
	if err != nil {
		return fmt.Errorf("failed to initialize storage: %w", err)
	}

	devices := store.GetDevices()

	// Convert to slice and filter
	var filtered []*types.Device
	for _, d := range devices {
		// Apply filters
		if listOnline && !d.IsOnline() {
			continue
		}
		if listOffline && d.IsOnline() {
			continue
		}
		if listGroup != "" && !strings.EqualFold(d.Group, listGroup) {
			continue
		}
		filtered = append(filtered, d)
	}

	// Sort by IP
	sort.Slice(filtered, func(i, j int) bool {
		return ipToSortKey(filtered[i].IP) < ipToSortKey(filtered[j].IP)
	})

	// Output based on format
	switch listFormat {
	case "csv":
		return outputCSV(filtered)
	case "json":
		return outputJSON(filtered)
	default:
		return outputTable(filtered)
	}
}

func outputTable(devices []*types.Device) error {
	if len(devices) == 0 {
		fmt.Println("No devices found")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "IP\tMAC\tHOSTNAME\tVENDOR\tLABEL\tGROUP\tSTATUS")
	fmt.Fprintln(w, "--\t---\t--------\t------\t-----\t-----\t------")

	for _, d := range devices {
		status := "offline"
		if d.IsRecent() {
			status = "online"
		} else if d.IsOnline() {
			status = "seen"
		}

		hostname := d.Hostname
		if len(hostname) > 25 {
			hostname = hostname[:22] + "..."
		}

		vendor := d.Vendor
		if len(vendor) > 20 {
			vendor = vendor[:17] + "..."
		}

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			d.IP, d.MAC, hostname, vendor, d.Label, d.Group, status)
	}

	return w.Flush()
}

func outputCSV(devices []*types.Device) error {
	w := csv.NewWriter(os.Stdout)
	defer w.Flush()

	// Header
	if err := w.Write([]string{"IP", "MAC", "Hostname", "Vendor", "Label", "Notes", "Group", "First Seen", "Last Seen"}); err != nil {
		return err
	}

	for _, d := range devices {
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
		}); err != nil {
			return err
		}
	}

	return nil
}

func outputJSON(devices []*types.Device) error {
	// Simple JSON output
	fmt.Println("[")
	for i, d := range devices {
		comma := ","
		if i == len(devices)-1 {
			comma = ""
		}
		fmt.Printf("  {\"ip\": %q, \"mac\": %q, \"hostname\": %q, \"vendor\": %q, \"label\": %q, \"group\": %q}%s\n",
			d.IP, d.MAC, d.Hostname, d.Vendor, d.Label, d.Group, comma)
	}
	fmt.Println("]")
	return nil
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
