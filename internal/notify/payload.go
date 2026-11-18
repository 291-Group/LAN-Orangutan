package notify

import (
	"fmt"
	"net/url"
	"strings"
	"time"
	"unicode"

	"github.com/291-Group/LAN-Orangutan/internal/scanner"
	"github.com/291-Group/LAN-Orangutan/internal/types"

	"github.com/291-Group/LAN-Orangutan/internal/uerr"
)

// Payload is the JSON body posted to a webhook.
//
// The one-line message goes in both "text" and "content" so the common chat
// services show it without any setup: Slack and Telegram read "text", Discord
// reads "content". Anything else (Home Assistant, a script) can use the
// structured fields.
type Payload struct {
	Event      string        `json:"event"`
	Time       string        `json:"time"`
	Text       string        `json:"text"`
	Content    string        `json:"content"`
	Device     PayloadDevice `json:"device"`
	PreviousIP string        `json:"previous_ip,omitempty"`
	// ChatID is filled only for Telegram, copied from the chat_id in the
	// webhook URL. See telegramChat.
	ChatID string `json:"chat_id,omitempty"`
	// MessageThreadID is Telegram's topic within a group, copied from the URL
	// the same way.
	MessageThreadID string `json:"message_thread_id,omitempty"`
}

// PayloadDevice is the device an event is about.
type PayloadDevice struct {
	IP string `json:"ip"`
	// MAC is empty when the scan could not read it (no sudo).
	MAC string `json:"mac"`
	// Hostname is the name as the dashboard shows it: the user's own name for
	// the device when set, otherwise the scanned one.
	Hostname string `json:"hostname"`
	Vendor   string `json:"vendor"`
	Label    string `json:"label"`
	Group    string `json:"group"`
	Type     string `json:"type"`
}

// NewPayload builds the body for one change. Messages are in English on
// purpose: they are read in whatever service receives them, not in this app.
func NewPayload(c Change) Payload {
	d := c.Device
	return Payload{
		Event:   c.Event,
		Time:    c.Time.Format(time.RFC3339),
		Text:    Message(c),
		Content: Message(c),
		Device: PayloadDevice{
			IP:       d.IP,
			MAC:      d.MAC,
			Hostname: d.DisplayHostname(),
			Vendor:   scanner.ResolveVendor(d.Vendor, d.MAC),
			Label:    d.Label,
			Group:    d.Group,
			Type:     d.Type,
		},
		PreviousIP: c.PreviousIP,
	}
}

// TestPayload is what the Send test button posts. It uses a made-up device so a
// receiver that parses the device fields sees the real shape.
func TestPayload(now time.Time) Payload {
	return NewPayload(Change{
		Event: EventTest,
		Device: types.Device{
			IP:       "192.0.2.10",
			MAC:      "00:00:5e:00:53:01",
			Hostname: "example-device",
		},
		Time: now,
	})
}

// Message is the one-line, human-readable description of a change.
func Message(c Change) string {
	d := c.Device
	name := deviceName(&d)
	switch c.Event {
	case EventNew:
		return fmt.Sprintf("New device on the network: %s", name)
	case EventUnexpected:
		return fmt.Sprintf("Unexpected device online: %s", name)
	case EventMissing:
		return fmt.Sprintf("Expected device missing: %s, last seen %s", name, d.LastSeen.Local().Format("2006-01-02 15:04"))
	case EventMoved:
		return fmt.Sprintf("Device changed address: %s moved from %s to %s", deviceTitle(&d), c.PreviousIP, d.IP)
	case EventTest:
		return "Test notification from LAN Orangutan. This webhook is working."
	}
	return fmt.Sprintf("%s: %s", c.Event, name)
}

// deviceTitle is the best name for a device: the user's label, then its
// hostname as shown, then its vendor, then its address.
func deviceTitle(d *types.Device) string {
	if d.Label != "" {
		return d.Label
	}
	if h := d.DisplayHostname(); h != "" {
		return h
	}
	if v := scanner.ResolveVendor(d.Vendor, d.MAC); v != "" && v != "Unknown" {
		return v
	}
	return d.IP
}

// deviceName is the title followed by the address and vendor, when those add
// anything: "Living Room TV (192.168.1.20, Apple)".
func deviceName(d *types.Device) string {
	title := deviceTitle(d)
	var extra []string
	if title != d.IP {
		extra = append(extra, d.IP)
	}
	if v := scanner.ResolveVendor(d.Vendor, d.MAC); v != "" && v != "Unknown" && v != title {
		extra = append(extra, v)
	}
	if len(extra) == 0 {
		return title
	}
	return title + " (" + strings.Join(extra, ", ") + ")"
}

// maxURLLen bounds a webhook URL. Real ones are well under this; it only stops
// a pasted blob from bloating the state file.
const maxURLLen = 2048

// ValidateURL checks a webhook URL is an absolute http or https URL with a host,
// and returns it trimmed.
func ValidateURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", uerr.New("url.empty", "enter the webhook URL")
	}
	if len(raw) > maxURLLen {
		return "", uerr.Newf("url.tooLong", "the URL is longer than %d characters", "max", maxURLLen)
	}
	for _, r := range raw {
		if unicode.IsSpace(r) || unicode.IsControl(r) {
			return "", uerr.New("url.spaces", "the URL cannot contain spaces")
		}
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", uerr.New("url.invalid", "not a valid URL")
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", uerr.New("url.scheme", "the URL must start with http:// or https://")
	}
	if u.Hostname() == "" {
		return "", uerr.New("url.noHost", "the URL has no host")
	}
	return raw, nil
}

// MaskURL shows only the scheme and host of a webhook URL, since the rest often
// holds a secret: a Telegram bot token, a Discord or Slack webhook key. Anything
// hidden is marked with an ellipsis, as in https://api.telegram.org/…
func MaskURL(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" {
		return "…"
	}
	masked := strings.ToLower(u.Scheme) + "://" + u.Host
	if (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" || u.User != nil {
		masked += "/…"
	}
	return masked
}

// isHost reports whether a webhook URL points at the given host.
func isHost(u *url.URL, host string) bool {
	return strings.EqualFold(u.Hostname(), host)
}

// telegramChat copies chat_id (and message_thread_id) from a Telegram Bot API
// URL into the payload. Telegram's documentation lists the query string and a
// JSON body as ways to pass parameters but does not promise to merge the two,
// so the chat is also put in the JSON body, where sendMessage is documented to
// read it. The user still only pastes
// https://api.telegram.org/bot<token>/sendMessage?chat_id=<id>.
func telegramChat(u *url.URL, p *Payload) {
	if !isHost(u, "api.telegram.org") {
		return
	}
	q := u.Query()
	p.ChatID = q.Get("chat_id")
	p.MessageThreadID = q.Get("message_thread_id")
}

// ntfyPlain reports whether a webhook is an ntfy.sh topic that should get the
// message as plain text. ntfy publishes the request body as the notification,
// so a JSON body would show up as raw JSON. A URL that turns on ntfy's own
// templating (tpl or template) is left alone, since it is asking for the JSON.
func ntfyPlain(u *url.URL) bool {
	if !isHost(u, "ntfy.sh") {
		return false
	}
	q := u.Query()
	_, tpl := q["tpl"]
	_, template := q["template"]
	return !tpl && !template
}
