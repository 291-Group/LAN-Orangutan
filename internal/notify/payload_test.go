package notify

import (
	"encoding/json"
	"net/url"
	"strings"
	"testing"

	"github.com/291-Group/LAN-Orangutan/internal/types"
)

func TestPayloadShape(t *testing.T) {
	c := Change{
		Event: EventMoved,
		Device: types.Device{
			IP: "192.168.1.35", MAC: "aa:bb:cc:dd:ee:ff", Hostname: "iphone",
			CustomHostname: "Living Room TV", Vendor: "Apple", Label: "TV", Group: "Mobile", Type: "Phone",
		},
		PreviousIP: "192.168.1.20",
		Time:       t0,
	}
	data, err := json.Marshal(NewPayload(c))
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]interface{}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got["event"] != "device.moved" || got["time"] != "2026-10-20T12:00:00Z" || got["previous_ip"] != "192.168.1.20" {
		t.Errorf("top-level fields wrong: %v", got)
	}
	if got["text"] == "" || got["text"] != got["content"] {
		t.Errorf("text and content must carry the same message: %q / %q", got["text"], got["content"])
	}
	if want := "Device changed address: TV moved from 192.168.1.20 to 192.168.1.35"; got["text"] != want {
		t.Errorf("text = %q, want %q", got["text"], want)
	}
	dev := got["device"].(map[string]interface{})
	wantDev := map[string]string{
		"ip": "192.168.1.35", "mac": "aa:bb:cc:dd:ee:ff", "hostname": "Living Room TV",
		"vendor": "Apple", "label": "TV", "group": "Mobile", "type": "Phone",
	}
	for k, v := range wantDev {
		if dev[k] != v {
			t.Errorf("device.%s = %v, want %q", k, dev[k], v)
		}
	}
	if _, ok := got["chat_id"]; ok {
		t.Error("chat_id should only appear for Telegram")
	}

	// Not a move: no previous_ip at all.
	c.Event = EventNew
	c.PreviousIP = ""
	data, _ = json.Marshal(NewPayload(c))
	if strings.Contains(string(data), "previous_ip") {
		t.Errorf("previous_ip should be omitted: %s", data)
	}
}

func TestMessages(t *testing.T) {
	d := types.Device{IP: "10.0.0.5", Vendor: "Raspberry Pi"}
	tests := []struct {
		event, want string
	}{
		{EventNew, "New device on the network: Raspberry Pi (10.0.0.5)"},
		{EventUnexpected, "Unexpected device online: Raspberry Pi (10.0.0.5)"},
	}
	for _, tt := range tests {
		if got := Message(Change{Event: tt.event, Device: d}); got != tt.want {
			t.Errorf("%s: got %q, want %q", tt.event, got, tt.want)
		}
	}
	named := types.Device{IP: "10.0.0.6", Hostname: "nas", Vendor: "Synology", Expected: true, LastSeen: t0}
	if got := Message(Change{Event: EventMissing, Device: named}); !strings.HasPrefix(got, "Expected device missing: nas (10.0.0.6, Synology), last seen ") {
		t.Errorf("missing: got %q", got)
	}
	if got := Message(Change{Event: EventNew, Device: types.Device{IP: "10.0.0.7"}}); got != "New device on the network: 10.0.0.7" {
		t.Errorf("bare device: got %q", got)
	}
}

func TestMaskURL(t *testing.T) {
	tests := map[string]string{
		"https://api.telegram.org/bot123:ABC/sendMessage?chat_id=42": "https://api.telegram.org/…",
		"https://discord.com/api/webhooks/1/secret":                  "https://discord.com/…",
		"http://homeassistant.local:8123/api/webhook/abc":            "http://homeassistant.local:8123/…",
		"https://ntfy.sh":               "https://ntfy.sh",
		"https://ntfy.sh/":              "https://ntfy.sh",
		"https://user:pass@example.com": "https://example.com/…",
		"HTTPS://Example.com/x":         "https://Example.com/…",
		"not a url":                     "…",
	}
	for in, want := range tests {
		got := MaskURL(in)
		if got != want {
			t.Errorf("MaskURL(%q) = %q, want %q", in, got, want)
		}
		if strings.Contains(got, "secret") || strings.Contains(got, "ABC") || strings.Contains(got, "pass") {
			t.Errorf("MaskURL(%q) leaked a secret: %q", in, got)
		}
	}
}

func TestValidateURL(t *testing.T) {
	good := []string{
		"https://api.telegram.org/bot1:x/sendMessage?chat_id=1",
		"  http://192.168.1.10:8123/api/webhook/x  ",
		"https://ntfy.sh/topic",
	}
	for _, u := range good {
		if _, err := ValidateURL(u); err != nil {
			t.Errorf("ValidateURL(%q): %v", u, err)
		}
	}
	bad := []string{"", "ftp://example.com/x", "example.com/hook", "https://", "https:///path", "javascript:alert(1)", "https://exa mple.com", "file:///etc/passwd"}
	for _, u := range bad {
		if _, err := ValidateURL(u); err == nil {
			t.Errorf("ValidateURL(%q) should fail", u)
		}
	}
}

func TestTelegramChatCopiedIntoBody(t *testing.T) {
	u, _ := url.Parse("https://api.telegram.org/bot1:x/sendMessage?chat_id=-1001&message_thread_id=7")
	p := NewPayload(Change{Event: EventNew, Device: types.Device{IP: "10.0.0.5"}, Time: t0})
	telegramChat(u, &p)
	if p.ChatID != "-1001" || p.MessageThreadID != "7" {
		t.Errorf("chat not copied: %+v", p)
	}
	other, _ := url.Parse("https://example.com/hook?chat_id=5")
	p2 := NewPayload(Change{Event: EventNew, Device: types.Device{IP: "10.0.0.5"}, Time: t0})
	telegramChat(other, &p2)
	if p2.ChatID != "" {
		t.Error("chat_id must only be copied for Telegram")
	}
}

func TestNtfyPlain(t *testing.T) {
	cases := map[string]bool{
		"https://ntfy.sh/mytopic":                   true,
		"https://NTFY.sh/mytopic":                   true,
		"https://ntfy.sh/mytopic?tpl=1&m={{.text}}": false,
		"https://ntfy.example.com/mytopic":          false,
		"https://example.com/ntfy.sh":               false,
		"https://ntfy.sh/mytopic?template=yes&m=hi": false,
	}
	for raw, want := range cases {
		u, _ := url.Parse(raw)
		if got := ntfyPlain(u); got != want {
			t.Errorf("ntfyPlain(%q) = %v, want %v", raw, got, want)
		}
	}
}
