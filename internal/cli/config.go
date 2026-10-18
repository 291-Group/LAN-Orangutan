package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/291-Group/LAN-Orangutan/internal/auth"
	"github.com/291-Group/LAN-Orangutan/internal/storage"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Display current configuration",
	RunE:  runConfig,
}

func runConfig(cmd *cobra.Command, args []string) error {
	// Settings changed from the Settings page are saved as overrides in the data
	// directory, not written back to the config file, so the file alone no longer
	// says what the app will actually do. Resolve through storage when it opens so
	// this command reports the EFFECTIVE value and marks where it came from.
	// Degrade gracefully: with no readable data directory the config file is still
	// the whole truth, which is exactly what we then print.
	store, _ := storage.New(cfg.DevicesFile(), cfg.StateFile())

	fmt.Println("=== LAN Orangutan Configuration ===")
	fmt.Printf("Config file: %s\n", cfgFile)
	if store != nil {
		fmt.Printf("Overrides:   %s\n", cfg.StateFile())
	}
	fmt.Println()

	fmt.Println("[server]")
	fmt.Printf("  port = %d\n", cfg.Server.Port)
	fmt.Printf("  bind_address = %s\n", cfg.Server.BindAddress)
	fmt.Printf("  enable_api = %v\n", cfg.Server.EnableAPI)
	// Never print the password or its hash. Show only whether one is set, and
	// where it came from, which is what someone checking their setup needs.
	fmt.Printf("  password = %s\n", passwordSummary())
	fmt.Printf("  session_hours = %d\n", cfg.Server.SessionHours)
	fmt.Printf("  allow_insecure = %v\n", cfg.Server.AllowInsecure)
	fmt.Println()

	fmt.Println("[scanning]")
	fmt.Printf("  continuous_scan = %v%s\n", effBool(store, cfg.Scanning.ContinuousScan, storeContinuous), mark(store, cfg.Scanning.ContinuousScan, storeContinuous))
	fmt.Printf("  scan_interval = %d%s\n", effInt(store, cfg.Scanning.ScanInterval, storeInterval), markInt(store, cfg.Scanning.ScanInterval, storeInterval))
	fmt.Printf("  min_scan_interval = %d\n", cfg.Scanning.MinScanInterval)
	fmt.Printf("  enable_service_detection = %v%s\n", effBool(store, cfg.Scanning.EnableServiceDetection, storeServiceDetection), mark(store, cfg.Scanning.EnableServiceDetection, storeServiceDetection))
	fmt.Printf("  networks = %s%s\n", listOrNone(effList(store, cfg.Scanning.Networks, storeNetworks)), markList(store, cfg.Scanning.Networks, storeNetworks))
	fmt.Printf("  exclude_networks = %s%s\n", listOrNone(effList(store, cfg.Scanning.ExcludeNetworks, storeExclude)), markList(store, cfg.Scanning.ExcludeNetworks, storeExclude))
	fmt.Printf("  only_configured_networks = %v%s\n", effBool(store, cfg.Scanning.OnlyConfiguredNetworks, storeOnlyConfigured), mark(store, cfg.Scanning.OnlyConfiguredNetworks, storeOnlyConfigured))
	// Reserved keys: parsed and preserved, but nothing consumes them yet.
	fmt.Printf("  enable_port_scan = %v (reserved, not wired up yet)\n", cfg.Scanning.EnablePortScan)
	fmt.Printf("  port_scan_range = %s (reserved, not wired up yet)\n", cfg.Scanning.PortScanRange)
	fmt.Println()

	fmt.Println("[storage]")
	fmt.Printf("  max_devices = %d%s\n", effInt(store, cfg.Storage.MaxDevices, storeMaxDevices), markInt(store, cfg.Storage.MaxDevices, storeMaxDevices))
	fmt.Printf("  retention_days = %d%s\n", effInt(store, cfg.Storage.RetentionDays, storeRetention), markInt(store, cfg.Storage.RetentionDays, storeRetention))
	fmt.Printf("  data_dir = %s\n", cfg.Storage.DataDir)
	fmt.Println()

	fmt.Println("[tailscale]")
	fmt.Printf("  enable = %v\n", cfg.Tailscale.Enable)
	fmt.Printf("  auto_detect = %v\n", cfg.Tailscale.AutoDetect)
	fmt.Println()

	fmt.Println("[ui]")
	fmt.Printf("  theme = %s\n", cfg.UI.Theme)

	return nil
}

// passwordSummary describes the password state without revealing it.
func passwordSummary() string {
	if cfg.Server.Password != "" {
		return "(set in config or environment)"
	}
	if auth.LoadHash(cfg.PasswordFile()) != "" {
		return "(set during first run setup)"
	}
	if cfg.RequiresSetup() {
		return "(not set - you will be asked to create one on first visit)"
	}
	return "(not set - not required with this bind address)"
}

// Resolving an effective value needs the config default and the matching storage
// resolver. These small adapters name each pairing once so the printing above
// stays readable, and so a nil store (unreadable data directory) simply falls
// back to the config value.

func storeContinuous(s *storage.Storage, d bool) bool       { return s.ContinuousScanEnabled(d) }
func storeServiceDetection(s *storage.Storage, d bool) bool { return s.ServiceDetectionEnabled(d) }
func storeOnlyConfigured(s *storage.Storage, d bool) bool   { return s.OnlyConfiguredOrDefault(d) }
func storeInterval(s *storage.Storage, d int) int           { return s.ScanIntervalOrDefault(d) }
func storeMaxDevices(s *storage.Storage, d int) int         { return s.MaxDevicesOrDefault(d) }
func storeRetention(s *storage.Storage, d int) int          { return s.RetentionDaysOrDefault(d) }
func storeNetworks(s *storage.Storage, d []string) []string { return s.NetworksOrDefault(d) }
func storeExclude(s *storage.Storage, d []string) []string  { return s.ExcludeNetworksOrDefault(d) }

func effBool(s *storage.Storage, def bool, f func(*storage.Storage, bool) bool) bool {
	if s == nil {
		return def
	}
	return f(s, def)
}

func effInt(s *storage.Storage, def int, f func(*storage.Storage, int) int) int {
	if s == nil {
		return def
	}
	return f(s, def)
}

func effList(s *storage.Storage, def []string, f func(*storage.Storage, []string) []string) []string {
	if s == nil {
		return def
	}
	return f(s, def)
}

// mark* append a note when the effective value differs from the config file, so
// it is obvious that the Settings page, not the file, is deciding this one.
const overridden = "   <- set in Settings"

func mark(s *storage.Storage, def bool, f func(*storage.Storage, bool) bool) string {
	if s != nil && f(s, def) != def {
		return overridden
	}
	return ""
}

func markInt(s *storage.Storage, def int, f func(*storage.Storage, int) int) string {
	if s != nil && f(s, def) != def {
		return overridden
	}
	return ""
}

func markList(s *storage.Storage, def []string, f func(*storage.Storage, []string) []string) string {
	if s != nil && strings.Join(f(s, def), ",") != strings.Join(def, ",") {
		return overridden
	}
	return ""
}

// listOrNone renders an empty list as something readable rather than a blank.
func listOrNone(v []string) string {
	if len(v) == 0 {
		return "(none)"
	}
	return strings.Join(v, ", ")
}
