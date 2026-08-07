package storage

import (
	"path/filepath"
	"sync"
	"testing"

	"github.com/291-Group/LAN-Orangutan/internal/types"
)

// TestConcurrentReadDuringMerge guards against the data race where a reader of
// GetDevices touches device fields while a scan merge mutates the same structs.
// It must stay race-clean: run the package with -race.
func TestConcurrentReadDuringMerge(t *testing.T) {
	dir := t.TempDir()
	s, err := New(filepath.Join(dir, "devices.json"), filepath.Join(dir, "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.MergeDevices([]types.Device{{IP: "10.0.0.1", Hostname: "a"}}); err != nil {
		t.Fatal(err)
	}

	const iterations = 2000
	var wg sync.WaitGroup

	// Reader: mimic the web handler reading fields after GetDevices returns.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			for _, d := range s.GetDevices() {
				_ = d.Hostname + d.MAC + d.Vendor
			}
		}
	}()

	// Writer: mimic a scan merging new data for the same IP.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			_ = s.MergeDevices([]types.Device{{IP: "10.0.0.1", Hostname: "bbbbbbbb", MAC: "cc", Vendor: "dd"}})
		}
	}()

	wg.Wait()
}
