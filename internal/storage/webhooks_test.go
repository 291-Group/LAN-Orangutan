package storage

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"fmt"
	"github.com/291-Group/LAN-Orangutan/internal/types"
	"github.com/291-Group/LAN-Orangutan/internal/uerr"
)

const testHookURL = "https://api.telegram.org/bot123:ABC/sendMessage?chat_id=42"

func TestWebhookCRUD(t *testing.T) {
	s := newTestStorage(t)

	if got := s.GetWebhooks(); len(got) != 0 {
		t.Fatalf("new store has webhooks: %v", got)
	}

	w, err := s.AddWebhook(" Family ", "  "+testHookURL+" ", []string{"new", "moved"})
	if err != nil {
		t.Fatalf("AddWebhook: %v", err)
	}
	if w.ID == "" || w.Name != "Family" || w.URL != testHookURL || !w.Enabled {
		t.Errorf("added webhook = %+v", w)
	}
	if !reflect.DeepEqual(w.Events, []string{"device.new", "device.moved"}) {
		t.Errorf("events = %v", w.Events)
	}

	off := false
	events := []string{"missing"}
	if err := s.UpdateWebhook(w.ID, nil, nil, &events, &off); err != nil {
		t.Fatalf("UpdateWebhook: %v", err)
	}
	got, ok := s.GetWebhook(w.ID)
	if !ok || got.Enabled || !reflect.DeepEqual(got.Events, []string{"device.missing"}) || got.Name != "Family" {
		t.Errorf("after update = %+v", got)
	}

	if err := s.RecordWebhookResult(w.ID, types.WebhookResult{Time: time.Now(), Success: true, Status: 200}); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.GetWebhook(w.ID); got.LastResult == nil || got.LastResult.Status != 200 {
		t.Errorf("result not recorded: %+v", got.LastResult)
	}

	// A new URL replaces the old one and clears the result that belonged to it.
	newURL := "https://discord.com/api/webhooks/1/abc"
	if err := s.UpdateWebhook(w.ID, nil, &newURL, nil, nil); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.GetWebhook(w.ID); got.URL != newURL || got.LastResult != nil {
		t.Errorf("after URL change = %+v", got)
	}

	if err := s.DeleteWebhook(w.ID); err != nil {
		t.Fatalf("DeleteWebhook: %v", err)
	}
	if len(s.GetWebhooks()) != 0 {
		t.Error("webhook not deleted")
	}
	if err := s.DeleteWebhook(w.ID); err == nil {
		t.Error("deleting an unknown webhook should fail")
	}
	// A result for a webhook deleted mid-delivery is dropped quietly.
	if err := s.RecordWebhookResult(w.ID, types.WebhookResult{Success: true}); err != nil {
		t.Errorf("RecordWebhookResult on a deleted webhook: %v", err)
	}
}

func TestWebhookValidation(t *testing.T) {
	s := newTestStorage(t)
	cases := []struct {
		name, url string
		events    []string
		want      string
	}{
		{"", "ftp://example.com", []string{"new"}, "http"},
		{"", "https://", []string{"new"}, "host"},
		{"", "https://example.com/hook", []string{"boom"}, "unknown event"},
		{"", "https://example.com/hook", nil, "at least one"},
		{strings.Repeat("x", 65), "https://example.com/hook", []string{"new"}, "longer"},
	}
	for _, c := range cases {
		if _, err := s.AddWebhook(c.name, c.url, c.events); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("AddWebhook(%q, %v): err = %v, want it to mention %q", c.url, c.events, err, c.want)
		}
	}
	bad := "javascript:alert(1)"
	w, _ := s.AddWebhook("", "https://example.com/hook", []string{"new"})
	if err := s.UpdateWebhook(w.ID, nil, &bad, nil, nil); err == nil {
		t.Error("UpdateWebhook accepted a bad URL")
	}
	if err := s.UpdateWebhook("nope", nil, nil, nil, nil); err == nil {
		t.Error("UpdateWebhook accepted an unknown ID")
	}
}

func TestWebhookLimit(t *testing.T) {
	s := newTestStorage(t)
	for i := 0; i < MaxWebhooks; i++ {
		if _, err := s.AddWebhook("", fmt.Sprintf("https://example.com/hook/%d", i), []string{"new"}); err != nil {
			t.Fatal(err)
		}
	}
	_, err := s.AddWebhook("", "https://example.com/hook/last", []string{"new"})
	if e, ok := uerr.Of(err); !ok || e.Code != "webhook.tooMany" || e.Args["max"] != MaxWebhooks {
		t.Errorf("expected the webhook cap to be enforced with webhook.tooMany, got %v", err)
	}
}

// Two webhooks posting to one URL would send every event twice, so the same
// URL (compared trimmed) is refused on add and on update, but a webhook may
// keep its own URL through an update.
func TestWebhookDuplicateURLRefused(t *testing.T) {
	s := newTestStorage(t)
	first, err := s.AddWebhook("a", "https://example.com/hook", []string{"new"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.AddWebhook("b", "https://example.com/other", []string{"new"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.AddWebhook("c", "  https://example.com/hook ", []string{"new"})
	if e, ok := uerr.Of(err); !ok || e.Code != "webhook.duplicateUrl" {
		t.Errorf("adding a duplicate URL should fail with webhook.duplicateUrl, got %v", err)
	}
	err = s.UpdateWebhook(second.ID, nil, strptr("https://example.com/hook"), nil, nil)
	if e, ok := uerr.Of(err); !ok || e.Code != "webhook.duplicateUrl" {
		t.Errorf("updating onto another webhook's URL should fail, got %v", err)
	}
	if err := s.UpdateWebhook(first.ID, strptr("renamed"), strptr("https://example.com/hook"), nil, nil); err != nil {
		t.Errorf("a webhook should be able to keep its own URL: %v", err)
	}
	if len(s.GetWebhooks()) != 2 {
		t.Errorf("expected 2 webhooks, got %d", len(s.GetWebhooks()))
	}
}

// Webhooks live in the state file, so they survive a restart and a CLI change
// shows up in a running server (another Storage on the same files).
func TestWebhooksPersistAndSyncAcrossProcesses(t *testing.T) {
	dir := t.TempDir()
	devices, state := filepath.Join(dir, "devices.json"), filepath.Join(dir, "state.json")
	server, err := New(devices, state)
	if err != nil {
		t.Fatal(err)
	}
	_ = server.GetWebhooks() // the server has read the (empty) state

	cli, err := New(devices, state)
	if err != nil {
		t.Fatal(err)
	}
	w, err := cli.AddWebhook("ha", "http://homeassistant.local:8123/api/webhook/x", []string{"new"})
	if err != nil {
		t.Fatal(err)
	}

	got := server.GetWebhooks()
	if len(got) != 1 || got[0].ID != w.ID {
		t.Fatalf("server did not pick up the CLI's webhook: %+v", got)
	}

	reopened, err := New(devices, state)
	if err != nil {
		t.Fatal(err)
	}
	if got := reopened.GetWebhooks(); len(got) != 1 || got[0].URL != w.URL {
		t.Fatalf("webhook did not persist: %+v", got)
	}
}

func TestGetWebhooksReturnsCopies(t *testing.T) {
	s := newTestStorage(t)
	if _, err := s.AddWebhook("", "https://example.com/hook", []string{"new", "missing"}); err != nil {
		t.Fatal(err)
	}
	got := s.GetWebhooks()
	got[0].Events[0] = "changed"
	if s.GetWebhooks()[0].Events[0] != "device.new" {
		t.Error("GetWebhooks handed out the stored slice")
	}
}

func TestAdvanceMissingCheck(t *testing.T) {
	s := newTestStorage(t)
	t1 := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	if prev := s.AdvanceMissingCheck(t1); !prev.IsZero() {
		t.Fatalf("first check: prev = %v, want zero", prev)
	}
	t2 := t1.Add(5 * time.Minute)
	if prev := s.AdvanceMissingCheck(t2); !prev.Equal(t1) {
		t.Fatalf("second check: prev = %v, want %v", prev, t1)
	}
	// A time that is not later (clock went back) checks an empty window.
	if prev := s.AdvanceMissingCheck(t1); !prev.Equal(t1) {
		t.Fatalf("backwards: prev = %v, want an empty window at %v", prev, t1)
	}
	if prev := s.AdvanceMissingCheck(t2.Add(time.Minute)); !prev.Equal(t2) {
		t.Fatalf("after going back the mark should be unchanged: prev = %v, want %v", prev, t2)
	}
}
