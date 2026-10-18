package cli

import (
	"path/filepath"
	"testing"

	"github.com/291-Group/LAN-Orangutan/internal/storage"
)

// `orangutan config` has to keep working when the data directory cannot be read,
// because the config file is still worth printing. In that case every value falls
// back to the config default and nothing is marked as overridden.
func TestEffectiveValuesFallBackWhenStoreIsNil(t *testing.T) {
	if got := effBool(nil, true, storeContinuous); !got {
		t.Error("effBool with no store should return the config default")
	}
	if got := effInt(nil, 300, storeInterval); got != 300 {
		t.Errorf("effInt with no store = %d, want the config default 300", got)
	}
	if got := effList(nil, []string{"10.0.0.0/24"}, storeNetworks); len(got) != 1 {
		t.Errorf("effList with no store = %v, want the config default", got)
	}
	if mark(nil, true, storeContinuous) != "" || markInt(nil, 300, storeInterval) != "" ||
		markList(nil, nil, storeNetworks) != "" {
		t.Error("nothing should be marked as overridden when there is no store")
	}
}

// With a store and no override saved, the config value wins and is not marked.
func TestNoOverrideMeansConfigValueAndNoMark(t *testing.T) {
	s := newConfigTestStore(t)

	if got := effInt(s, 300, storeInterval); got != 300 {
		t.Errorf("effInt = %d, want 300 from config", got)
	}
	if markInt(s, 300, storeInterval) != "" {
		t.Error("an unset override should not be marked")
	}
}

// The case this was written for: a value changed from the Settings page must be
// reported by `orangutan config`, and flagged as coming from there.
func TestOverrideIsReportedAndMarked(t *testing.T) {
	s := newConfigTestStore(t)
	if err := s.SetScanInterval(90); err != nil {
		t.Fatalf("SetScanInterval: %v", err)
	}

	if got := effInt(s, 300, storeInterval); got != 90 {
		t.Errorf("effInt = %d, want the override 90, not the config default", got)
	}
	if markInt(s, 300, storeInterval) == "" {
		t.Error("an override should be marked so its source is obvious")
	}
}

func TestListRendersEmptyReadably(t *testing.T) {
	if got := listOrNone(nil); got != "(none)" {
		t.Errorf("listOrNone(nil) = %q, want \"(none)\"", got)
	}
	if got := listOrNone([]string{"a", "b"}); got != "a, b" {
		t.Errorf("listOrNone = %q, want \"a, b\"", got)
	}
}

func newConfigTestStore(t *testing.T) *storage.Storage {
	t.Helper()
	dir := t.TempDir()
	s, err := storage.New(filepath.Join(dir, "devices.json"), filepath.Join(dir, "state.json"))
	if err != nil {
		t.Fatalf("storage.New: %v", err)
	}
	return s
}
