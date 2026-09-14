package cli

import (
	"github.com/291-Group/LAN-Orangutan/internal/config"
	"github.com/291-Group/LAN-Orangutan/internal/network"
	"github.com/291-Group/LAN-Orangutan/internal/storage"
)

// networkFilter builds the effective scan-target filter for CLI commands,
// preferring the UI overrides (saved in the data dir) over the config defaults,
// so the command line agrees with the dashboard about which networks are in
// scope.
func networkFilter(store *storage.Storage, cfg *config.Config) network.Filter {
	return network.Filter{
		Configured:     store.NetworksOrDefault(cfg.Scanning.Networks),
		Excluded:       store.ExcludeNetworksOrDefault(cfg.Scanning.ExcludeNetworks),
		OnlyConfigured: store.OnlyConfiguredOrDefault(cfg.Scanning.OnlyConfiguredNetworks),
	}
}
