package storage

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/291-Group/LAN-Orangutan/internal/types"
)

// ExportBackup gathers the whole device list, the settings overrides (scan
// state), and every saved snapshot into a portable Backup. Maps are deep-copied
// so the result can be marshalled without holding the lock.
func (s *Storage) ExportBackup() types.Backup {
	s.mu.RLock()
	devices := make(map[string]*types.Device, len(s.devices))
	for ip, d := range s.devices {
		dc := *d
		devices[ip] = &dc
	}
	st := *s.state
	st.LastScan = make(map[string]time.Time, len(s.state.LastScan))
	for k, v := range s.state.LastScan {
		st.LastScan[k] = v
	}
	st.LastDuration = make(map[string]float64, len(s.state.LastDuration))
	for k, v := range s.state.LastDuration {
		st.LastDuration[k] = v
	}
	s.mu.RUnlock()

	snaps := []types.Snapshot{}
	if metas, err := s.ListSnapshots(); err == nil {
		for _, m := range metas {
			if full, err := s.GetSnapshot(m.ID); err == nil {
				snaps = append(snaps, *full)
			}
		}
	}

	return types.Backup{
		CreatedAt: time.Now(),
		Devices:   devices,
		State:     &st,
		Snapshots: snaps,
	}
}

// ImportBackup replaces the current device list, settings overrides, and saved
// snapshots with those from the backup, and persists them. This is destructive:
// it is the "restore" half of a backup and overwrites what is there.
func (s *Storage) ImportBackup(b types.Backup) error {
	s.mu.Lock()
	if b.Devices != nil {
		s.devices = b.Devices
	} else {
		s.devices = make(map[string]*types.Device)
	}
	if b.State != nil {
		s.state = b.State
	}
	if s.state.LastScan == nil {
		s.state.LastScan = make(map[string]time.Time)
	}
	if s.state.LastDuration == nil {
		s.state.LastDuration = make(map[string]float64)
	}
	errD := s.saveDevices()
	errS := s.saveState()
	s.mu.Unlock()
	if errD != nil {
		return errD
	}
	if errS != nil {
		return errS
	}
	return s.replaceSnapshots(b.Snapshots)
}

// replaceSnapshots clears the saved snapshots and writes the given ones. Each
// gets a freshly generated, guaranteed-numeric id rather than trusting the
// backup's, so a crafted backup cannot write outside the snapshots directory.
func (s *Storage) replaceSnapshots(snaps []types.Snapshot) error {
	dir := s.snapshotDir()
	if entries, err := os.ReadDir(dir); err == nil {
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
				_ = os.Remove(filepath.Join(dir, e.Name()))
			}
		}
	}
	if len(snaps) == 0 {
		return nil
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	base := time.Now().UnixNano()
	for i, snap := range snaps {
		snap.ID = strconv.FormatInt(base+int64(i), 10) // unique and numeric
		data, err := json.MarshalIndent(snap, "", "  ")
		if err != nil {
			continue
		}
		path, err := s.snapshotPath(snap.ID)
		if err != nil {
			continue
		}
		if err := atomicWrite(path, data); err != nil {
			return err
		}
	}
	return nil
}
