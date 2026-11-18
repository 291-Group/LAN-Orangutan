package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/291-Group/LAN-Orangutan/internal/auth"
	"github.com/291-Group/LAN-Orangutan/internal/config"
	"github.com/291-Group/LAN-Orangutan/internal/storage"
	"github.com/291-Group/LAN-Orangutan/internal/types"
)

func newTestAPI(t *testing.T) (*Handler, *storage.Storage) {
	t.Helper()
	dir := t.TempDir()
	store, err := storage.New(filepath.Join(dir, "devices.json"), filepath.Join(dir, "state.json"))
	if err != nil {
		t.Fatalf("storage.New: %v", err)
	}
	cfg := config.Default()
	authn, err := auth.New("", time.Hour)
	if err != nil {
		t.Fatalf("auth.New: %v", err)
	}
	h := NewHandler(store, cfg, authn)
	return h, store
}

func TestAPIGetDevice(t *testing.T) {
	h, store := newTestAPI(t)

	if err := store.MergeDevices([]types.Device{
		{IP: "192.168.1.100", Hostname: "test-dev", Type: "Computer"},
	}); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/device?ip=192.168.1.100", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp types.APIResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if !resp.Success {
		t.Fatalf("expected success, got %v", resp.Error)
	}
}

func TestAPIPostDevice_CustomType(t *testing.T) {
	h, store := newTestAPI(t)

	if err := store.MergeDevices([]types.Device{
		{IP: "192.168.1.101", Hostname: "plug", Type: "Router"},
	}); err != nil {
		t.Fatal(err)
	}

	// Update device with custom type from supported taxonomy
	body := []byte(`{"ip":"192.168.1.101","label":"Smart Plug 1","type":"IoT","custom_type":true}`)
	req := httptest.NewRequest(http.MethodPost, "/api/device", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	dev := store.GetDevice("192.168.1.101")
	if dev == nil {
		t.Fatal("device missing")
	}
	if dev.Label != "Smart Plug 1" {
		t.Errorf("expected label 'Smart Plug 1', got %q", dev.Label)
	}
	if dev.Type != "IoT" {
		t.Errorf("expected Type 'IoT', got %q", dev.Type)
	}
	if !dev.CustomType {
		t.Errorf("expected CustomType true, got %v", dev.CustomType)
	}

	// Rescan should not overwrite custom type
	if err := store.MergeDevices([]types.Device{
		{IP: "192.168.1.101", Hostname: "plug", Type: "Router"},
	}); err != nil {
		t.Fatal(err)
	}
	dev = store.GetDevice("192.168.1.101")
	if dev.Type != "IoT" {
		t.Errorf("rescan overwrote custom type; got %q", dev.Type)
	}
}

func TestAPIPostDevice_InvalidType_Rejected(t *testing.T) {
	h, store := newTestAPI(t)

	if err := store.MergeDevices([]types.Device{
		{IP: "192.168.1.102", Hostname: "outlet"},
	}); err != nil {
		t.Fatal(err)
	}

	// Unsupported arbitrary type should be rejected with HTTP 400
	body := []byte(`{"ip":"192.168.1.102","type":"Smart Outlet","custom_type":true}`)
	req := httptest.NewRequest(http.MethodPost, "/api/device", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request, got %d: %s", rec.Code, rec.Body.String())
	}

	dev := store.GetDevice("192.168.1.102")
	if dev.Type != "" || dev.CustomType {
		t.Errorf("expected type to remain untouched, got type=%q custom=%v", dev.Type, dev.CustomType)
	}
}

func TestAPIPostDevice_TypeTooLong_Rejected(t *testing.T) {
	h, store := newTestAPI(t)

	if err := store.MergeDevices([]types.Device{
		{IP: "192.168.1.104", Hostname: "device"},
	}); err != nil {
		t.Fatal(err)
	}

	tooLong := "ThisDeviceTypeStringIsWayTooLongToBeValid"
	body := []byte(`{"ip":"192.168.1.104","type":"` + tooLong + `"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/device", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestAPIPostDevice_ResetType(t *testing.T) {
	h, store := newTestAPI(t)

	if err := store.MergeDevices([]types.Device{
		{IP: "192.168.1.103", Hostname: "mybox", Type: "Computer"},
	}); err != nil {
		t.Fatal(err)
	}

	// Mark as custom first
	customType := "Server"
	if err := store.UpdateDeviceFields("192.168.1.103", nil, nil, nil, nil, &customType, nil, nil, nil); err != nil {
		t.Fatal(err)
	}

	// Reset via API
	body := []byte(`{"ip":"192.168.1.103","type":"","custom_type":false}`)
	req := httptest.NewRequest(http.MethodPost, "/api/device", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	dev := store.GetDevice("192.168.1.103")
	if dev.Type != "" || dev.CustomType {
		t.Errorf("expected cleared type and custom_type false, got type=%q custom=%v", dev.Type, dev.CustomType)
	}
}

// Scripts filter on "online", so the device list must carry it.
func TestDevicesIncludeOnline(t *testing.T) {
	h, store := newTestAPI(t)
	if err := store.MergeDevices([]types.Device{{IP: "192.168.1.60"}}); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/devices", nil))
	var resp struct {
		Data map[string]struct {
			IP     string `json:"ip"`
			Online bool   `json:"online"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if d, ok := resp.Data["192.168.1.60"]; !ok || !d.Online || d.IP != "192.168.1.60" {
		t.Fatalf("device in API output = %+v (present %v), want online with its fields", d, ok)
	}
}

// A refused value comes back with the code and placeholders the web interface
// translates it from, whether the check lives in storage or in this package.
func TestAPIErrorsCarryCodeAndArgs(t *testing.T) {
	h, _ := newTestAPI(t)

	post := func(path, body string) (int, types.APIResponse) {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader([]byte(body)))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		var resp types.APIResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("bad JSON from %s: %v: %s", path, err, rec.Body.String())
		}
		return rec.Code, resp
	}

	// Storage: adding a group that exists.
	if code, resp := post("/api/groups", `{"action":"add","name":"Den"}`); code != http.StatusOK {
		t.Fatalf("first add: %d %+v", code, resp)
	}
	code, resp := post("/api/groups", `{"action":"add","name":"Den"}`)
	if code != http.StatusBadRequest || resp.Success {
		t.Fatalf("duplicate add: %d %+v", code, resp)
	}
	if resp.Code != "group.exists" || resp.Args["name"] != "Den" || resp.Error == "" {
		t.Errorf("duplicate add = code %q args %v error %q, want group.exists with name Den", resp.Code, resp.Args, resp.Error)
	}

	// This package: a network that is not a CIDR.
	code, resp = post("/api/scan/networks", `{"networks":["nonsense"]}`)
	if code != http.StatusBadRequest || resp.Code != "network.invalid" || resp.Args["value"] != "nonsense" {
		t.Errorf("bad CIDR = %d code %q args %v, want 400 network.invalid with value nonsense", code, resp.Code, resp.Args)
	}

	// A body that is not JSON at all.
	if code, resp := post("/api/groups", `{`); code != http.StatusBadRequest || resp.Code != "request.invalid" {
		t.Errorf("bad body = %d code %q, want 400 request.invalid", code, resp.Code)
	}
}
