package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/291-Group/LAN-Orangutan/internal/types"
)

// snapshotDir is where saved snapshots live: a "snapshots" subdirectory of the
// data directory, next to devices.json.
func (s *Storage) snapshotDir() string {
	return filepath.Join(filepath.Dir(s.devicesFile), "snapshots")
}

// snapshotPath maps a snapshot id to its file, rejecting anything that is not a
// plain numeric id so a crafted id cannot escape the snapshots directory.
func (s *Storage) snapshotPath(id string) (string, error) {
	if id == "" || strings.IndexFunc(id, func(r rune) bool { return r < '0' || r > '9' }) >= 0 {
		return "", fmt.Errorf("invalid snapshot id")
	}
	return filepath.Join(s.snapshotDir(), id+".json"), nil
}

func readSnapshotFile(path string) (*types.Snapshot, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var snap types.Snapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return nil, err
	}
	return &snap, nil
}

// SaveSnapshot captures the current device list under the given name and returns
// its metadata. A blank name defaults to the current date and time.
func (s *Storage) SaveSnapshot(name string) (types.SnapshotMeta, error) {
	s.mu.RLock()
	devices := make([]types.Device, 0, len(s.devices))
	for _, d := range s.devices {
		devices = append(devices, *d)
	}
	s.mu.RUnlock()

	now := time.Now()
	name = strings.TrimSpace(name)
	if name == "" {
		name = now.Format("2006-01-02 15:04")
	}
	snap := types.Snapshot{
		ID:        strconv.FormatInt(now.UnixNano(), 10),
		Name:      name,
		CreatedAt: now,
		Devices:   devices,
	}

	if err := os.MkdirAll(s.snapshotDir(), 0755); err != nil {
		return types.SnapshotMeta{}, err
	}
	data, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return types.SnapshotMeta{}, err
	}
	if err := atomicWrite(filepath.Join(s.snapshotDir(), snap.ID+".json"), data); err != nil {
		return types.SnapshotMeta{}, err
	}
	return types.SnapshotMeta{
		ID:          snap.ID,
		Name:        snap.Name,
		CreatedAt:   snap.CreatedAt,
		DeviceCount: len(devices),
	}, nil
}

// ListSnapshots returns the metadata of every saved snapshot, newest first. A
// snapshot file that cannot be read is skipped rather than failing the listing.
func (s *Storage) ListSnapshots() ([]types.SnapshotMeta, error) {
	entries, err := os.ReadDir(s.snapshotDir())
	if err != nil {
		if os.IsNotExist(err) {
			return []types.SnapshotMeta{}, nil
		}
		return nil, err
	}
	metas := []types.SnapshotMeta{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		snap, err := readSnapshotFile(filepath.Join(s.snapshotDir(), e.Name()))
		if err != nil {
			continue
		}
		metas = append(metas, types.SnapshotMeta{
			ID:          snap.ID,
			Name:        snap.Name,
			CreatedAt:   snap.CreatedAt,
			DeviceCount: len(snap.Devices),
		})
	}
	sort.Slice(metas, func(i, j int) bool { return metas[i].CreatedAt.After(metas[j].CreatedAt) })
	return metas, nil
}

// GetSnapshot loads a full snapshot (including its devices) by id.
func (s *Storage) GetSnapshot(id string) (*types.Snapshot, error) {
	path, err := s.snapshotPath(id)
	if err != nil {
		return nil, err
	}
	return readSnapshotFile(path)
}

// DeleteSnapshot removes a saved snapshot by id.
func (s *Storage) DeleteSnapshot(id string) error {
	path, err := s.snapshotPath(id)
	if err != nil {
		return err
	}
	return os.Remove(path)
}
