package cli

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/291-Group/LAN-Orangutan/internal/storage"
	"github.com/291-Group/LAN-Orangutan/internal/uerr"
)

// The Settings page keeps its changes as overrides in the data directory, so
// the config file stays the user's and the app still does what the page says.
// These commands edit the same overrides, which is why a change here shows up
// in a running server and on its Settings page without a restart.

// The clamping limits are the Settings page's, kept in step by hand because
// the API package cannot be imported here without dragging in the server.
const (
	settingsMinInterval  = 10
	settingsMaxInterval  = 86400
	settingsMaxRetention = 36500
	settingsMaxDevices   = 1000000
)

// settingKeys lists every setting the page can change, in the order they are
// printed. The theme is not here: the browser keeps it, not the server.
var settingKeys = []string{
	"continuous_scan", "scan_interval", "enable_service_detection",
	"networks", "exclude_networks", "only_configured_networks",
	"max_devices", "retention_days",
}

var settingsFormat string

var settingsCmd = &cobra.Command{
	Use:   "settings",
	Short: "Show or change the settings the Settings page can change",
	Long: `Show or change the settings the web interface's Settings page can change.

A change made here is saved the same way the page saves it, as an override in
the data directory, so a running "orangutan serve" picks it up without a
restart and the config file is left as it is. "settings reset" removes the
override and the setting follows the config file again.

Keys:
  continuous_scan            yes or no
  scan_interval              seconds between automatic scans (10 to 86400)
  enable_service_detection   yes or no
  networks                   extra networks to scan, comma separated CIDRs
  exclude_networks           detected networks never to scan, comma separated
  only_configured_networks   yes or no
  max_devices                0 for no limit
  retention_days             0 to keep devices forever

The theme is chosen in each browser and is not a server setting.`,
	Args: cobra.NoArgs,
	RunE: runSettingsList,
}

var settingsListCmd = &cobra.Command{
	Use:   "list",
	Short: "Show the settings in effect and which ones are overrides",
	Args:  cobra.NoArgs,
	RunE:  runSettingsList,
}

var settingsSetCmd = &cobra.Command{
	Use:   "set <key> <value>",
	Short: "Change a setting",
	Example: `  orangutan settings set scan_interval 600
  orangutan settings set continuous_scan no
  orangutan settings set networks 192.168.10.0/24,10.0.5.0/24
  orangutan settings set networks ""        # Back to the config file's list`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := openStore()
		if err != nil {
			return err
		}
		return setSetting(store, args[0], args[1])
	},
}

var settingsResetCmd = &cobra.Command{
	Use:   "reset [key]",
	Short: "Remove an override, or all of them, so the config file applies again",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := openStore()
		if err != nil {
			return err
		}
		if len(args) == 0 {
			if err := store.ClearScanOverrides(true, true, true); err != nil {
				return fmt.Errorf("failed to save settings: %w", err)
			}
			if err := store.SetNetworkOverrides(nil, nil, nil); err != nil {
				return fmt.Errorf("failed to save settings: %w", err)
			}
			if err := store.SetLimitOverrides(nil, nil); err != nil {
				return fmt.Errorf("failed to save settings: %w", err)
			}
			fmt.Println("Removed every override; the config file applies again")
			return nil
		}
		return resetSetting(store, args[0])
	},
}

func init() {
	for _, c := range []*cobra.Command{settingsCmd, settingsListCmd} {
		c.Flags().StringVar(&settingsFormat, "format", "table", "output format: table or json")
	}
	settingsCmd.AddCommand(settingsListCmd, settingsSetCmd, settingsResetCmd)
	rootCmd.AddCommand(settingsCmd)
}

// settingRow is one line of "settings list": the value in effect, the config
// file's value, and whether an override is what made the difference.
type settingRow struct {
	Key        string `json:"key"`
	Value      string `json:"value"`
	Default    string `json:"default"`
	Overridden bool   `json:"overridden"`
}

func settingRows(store *storage.Storage) []settingRow {
	continuous, interval, serviceDetection := store.GetScanOverrides()
	networks, exclude, onlyConfigured := store.GetNetworkOverrides()
	retention, maxDevices := store.GetLimitOverrides()
	rows := []settingRow{
		{"continuous_scan", yesNo(store.ContinuousScanEnabled(cfg.Scanning.ContinuousScan)), yesNo(cfg.Scanning.ContinuousScan), continuous != nil},
		{"scan_interval", strconv.Itoa(store.ScanIntervalOrDefault(cfg.Scanning.ScanInterval)), strconv.Itoa(cfg.Scanning.ScanInterval), interval != nil},
		{"enable_service_detection", yesNo(store.ServiceDetectionEnabled(cfg.Scanning.EnableServiceDetection)), yesNo(cfg.Scanning.EnableServiceDetection), serviceDetection != nil},
		{"networks", listOrNone(store.NetworksOrDefault(cfg.Scanning.Networks)), listOrNone(cfg.Scanning.Networks), networks != nil},
		{"exclude_networks", listOrNone(store.ExcludeNetworksOrDefault(cfg.Scanning.ExcludeNetworks)), listOrNone(cfg.Scanning.ExcludeNetworks), exclude != nil},
		{"only_configured_networks", yesNo(store.OnlyConfiguredOrDefault(cfg.Scanning.OnlyConfiguredNetworks)), yesNo(cfg.Scanning.OnlyConfiguredNetworks), onlyConfigured != nil},
		{"max_devices", strconv.Itoa(store.MaxDevicesOrDefault(cfg.Storage.MaxDevices)), strconv.Itoa(cfg.Storage.MaxDevices), maxDevices != nil},
		{"retention_days", strconv.Itoa(store.RetentionDaysOrDefault(cfg.Storage.RetentionDays)), strconv.Itoa(cfg.Storage.RetentionDays), retention != nil},
	}
	return rows
}

func runSettingsList(cmd *cobra.Command, args []string) error {
	format, err := outputFormat(settingsFormat, "table", "json")
	if err != nil {
		return err
	}
	store, err := openStore()
	if err != nil {
		return err
	}
	rows := settingRows(store)
	if format == "json" {
		return printJSON(rows)
	}
	table := make([][]string, 0, len(rows))
	for _, r := range rows {
		source := "config"
		if r.Overridden {
			source = "override"
		}
		table = append(table, []string{r.Key, r.Value, source})
	}
	return printTable(os.Stdout, []string{"SETTING", "VALUE", "SOURCE"}, table)
}

// setSetting parses value for key the way the Settings page would, clamps a
// number the page would clamp, and saves the override.
func setSetting(store *storage.Storage, key, value string) error {
	key = strings.ToLower(strings.TrimSpace(key))
	value = strings.TrimSpace(value)
	var err error
	switch key {
	case "continuous_scan":
		var on bool
		if on, err = parseYesNo(key, value); err == nil {
			err = saveSetting(store.SetContinuousScan(on))
		}
	case "enable_service_detection":
		var on bool
		if on, err = parseYesNo(key, value); err == nil {
			err = saveSetting(store.SetServiceDetection(on))
		}
	case "only_configured_networks":
		var on bool
		if on, err = parseYesNo(key, value); err == nil {
			networks, exclude, _ := store.GetNetworkOverrides()
			err = saveSetting(store.SetNetworkOverrides(networks, exclude, &on))
		}
	case "scan_interval":
		var n int
		if n, err = parseClamped(key, value, settingsMinInterval, settingsMaxInterval); err == nil {
			err = saveSetting(store.SetScanInterval(n))
		}
	case "max_devices":
		var n int
		if n, err = parseClamped(key, value, 0, settingsMaxDevices); err == nil {
			retention, _ := store.GetLimitOverrides()
			err = saveSetting(store.SetLimitOverrides(retention, &n))
		}
	case "retention_days":
		var n int
		if n, err = parseClamped(key, value, 0, settingsMaxRetention); err == nil {
			_, maxDevices := store.GetLimitOverrides()
			err = saveSetting(store.SetLimitOverrides(&n, maxDevices))
		}
	case "networks", "exclude_networks":
		if value == "" {
			return resetSetting(store, key)
		}
		var list []string
		if list, err = parseCIDRList(value); err == nil {
			networks, exclude, onlyConfigured := store.GetNetworkOverrides()
			if key == "networks" {
				networks = &list
			} else {
				exclude = &list
			}
			err = saveSetting(store.SetNetworkOverrides(networks, exclude, onlyConfigured))
		}
	default:
		return unknownSetting(key)
	}
	if err != nil {
		return err
	}
	for _, r := range settingRows(store) {
		if r.Key == key {
			if r.Value != value && (key == "scan_interval" || key == "max_devices" || key == "retention_days") {
				fmt.Printf("Set %s to %s (adjusted from %s)\n", key, r.Value, value)
			} else {
				fmt.Printf("Set %s to %s\n", key, r.Value)
			}
		}
	}
	return nil
}

func resetSetting(store *storage.Storage, key string) error {
	key = strings.ToLower(strings.TrimSpace(key))
	var err error
	switch key {
	case "continuous_scan":
		err = store.ClearScanOverrides(true, false, false)
	case "scan_interval":
		err = store.ClearScanOverrides(false, true, false)
	case "enable_service_detection":
		err = store.ClearScanOverrides(false, false, true)
	case "networks", "exclude_networks", "only_configured_networks":
		networks, exclude, onlyConfigured := store.GetNetworkOverrides()
		switch key {
		case "networks":
			networks = nil
		case "exclude_networks":
			exclude = nil
		default:
			onlyConfigured = nil
		}
		err = store.SetNetworkOverrides(networks, exclude, onlyConfigured)
	case "max_devices", "retention_days":
		retention, maxDevices := store.GetLimitOverrides()
		if key == "max_devices" {
			maxDevices = nil
		} else {
			retention = nil
		}
		err = store.SetLimitOverrides(retention, maxDevices)
	default:
		return unknownSetting(key)
	}
	if err != nil {
		return fmt.Errorf("failed to save settings: %w", err)
	}
	for _, r := range settingRows(store) {
		if r.Key == key {
			fmt.Printf("Reset %s; it follows the config file again (%s)\n", key, r.Value)
		}
	}
	return nil
}

func saveSetting(err error) error {
	if err != nil {
		return fmt.Errorf("failed to save settings: %w", err)
	}
	return nil
}

func unknownSetting(key string) error {
	return fmt.Errorf("unknown setting %q (one of %s)", key, strings.Join(settingKeys, ", "))
}

func parseYesNo(key, value string) (bool, error) {
	switch strings.ToLower(value) {
	case "yes", "y", "true", "on", "1":
		return true, nil
	case "no", "n", "false", "off", "0":
		return false, nil
	}
	return false, fmt.Errorf("%s takes yes or no, not %q", key, value)
}

// parseClamped reads a whole number and keeps it within the range the Settings
// page allows, the way the page does, rather than refusing it.
func parseClamped(key, value string, min, max int) (int, error) {
	n, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("%s takes a whole number, not %q", key, value)
	}
	if n < min {
		n = min
	}
	if n > max {
		n = max
	}
	return n, nil
}

// parseCIDRList splits a comma separated list and checks each entry the way
// the Settings page does, dropping blanks and repeats.
func parseCIDRList(value string) ([]string, error) {
	seen := map[string]bool{}
	out := []string{}
	for _, raw := range strings.Split(value, ",") {
		c := strings.TrimSpace(raw)
		if c == "" {
			continue
		}
		if _, _, err := net.ParseCIDR(c); err != nil {
			return nil, uerr.Newf("network.invalid", "%q is not a valid network (use CIDR form, e.g. 192.168.1.0/24)", "value", c)
		}
		if !seen[c] {
			seen[c] = true
			out = append(out, c)
		}
	}
	return out, nil
}
