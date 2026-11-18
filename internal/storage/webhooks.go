package storage

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/291-Group/LAN-Orangutan/internal/notify"
	"github.com/291-Group/LAN-Orangutan/internal/types"
	"github.com/291-Group/LAN-Orangutan/internal/uerr"
)

// MaxWebhooks bounds the webhook list. A handful is plenty for any home or
// office; the cap only keeps one scan from fanning out to an absurd number.
const MaxWebhooks = 20

// maxWebhookNameLen bounds a webhook's optional name, which is only a label for
// the list.
const maxWebhookNameLen = 64

// GetWebhooks returns the webhooks in the order they were added. The slice and
// everything in it are copies, so the caller can read them freely.
func (s *Storage) GetWebhooks() []types.Webhook {
	defer s.readLock()()
	out := make([]types.Webhook, 0, len(s.state.Webhooks))
	for _, w := range s.state.Webhooks {
		out = append(out, copyWebhook(w))
	}
	return out
}

// GetWebhook returns one webhook by ID, or false if there is none.
func (s *Storage) GetWebhook(id string) (types.Webhook, bool) {
	defer s.readLock()()
	if i := s.webhookIndexLocked(id); i >= 0 {
		return copyWebhook(s.state.Webhooks[i]), true
	}
	return types.Webhook{}, false
}

// AddWebhook validates and stores a new webhook, enabled, and returns it with
// its new ID.
func (s *Storage) AddWebhook(name, url string, events []string) (types.Webhook, error) {
	w, err := newWebhook(name, url, events)
	if err != nil {
		return types.Webhook{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.syncLocked()
	if len(s.state.Webhooks) >= MaxWebhooks {
		return types.Webhook{}, uerr.Newf("webhook.tooMany", "there can be at most %d webhooks", "max", MaxWebhooks)
	}
	if s.webhookURLIndexLocked(w.URL, -1) >= 0 {
		return types.Webhook{}, errDuplicateWebhookURL()
	}
	s.state.Webhooks = append(s.state.Webhooks, w)
	if err := s.saveState(); err != nil {
		return types.Webhook{}, err
	}
	return copyWebhook(w), nil
}

func newWebhook(name, url string, events []string) (types.Webhook, error) {
	cleanName, err := validateWebhookName(name)
	if err != nil {
		return types.Webhook{}, err
	}
	cleanURL, err := notify.ValidateURL(url)
	if err != nil {
		return types.Webhook{}, err
	}
	cleanEvents, err := notify.NormalizeEvents(events)
	if err != nil {
		return types.Webhook{}, err
	}
	id, err := newWebhookID()
	if err != nil {
		return types.Webhook{}, err
	}
	return types.Webhook{ID: id, Name: cleanName, URL: cleanURL, Events: cleanEvents, Enabled: true}, nil
}

// UpdateWebhook changes the fields that are not nil. Setting a new URL replaces
// the old one outright (the old one is never shown, so there is nothing to
// edit in place) and clears the last result, which belonged to the old URL.
func (s *Storage) UpdateWebhook(id string, name, url *string, events *[]string, enabled *bool) error {
	var cleanName, cleanURL string
	var cleanEvents []string
	var err error
	if name != nil {
		if cleanName, err = validateWebhookName(*name); err != nil {
			return err
		}
	}
	if url != nil {
		if cleanURL, err = notify.ValidateURL(*url); err != nil {
			return err
		}
	}
	if events != nil {
		if cleanEvents, err = notify.NormalizeEvents(*events); err != nil {
			return err
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.syncLocked()
	i := s.webhookIndexLocked(id)
	if i < 0 {
		return uerr.New("webhook.notFound", "webhook not found")
	}
	if url != nil && s.webhookURLIndexLocked(cleanURL, i) >= 0 {
		return errDuplicateWebhookURL()
	}
	w := &s.state.Webhooks[i]
	if name != nil {
		w.Name = cleanName
	}
	if url != nil && cleanURL != w.URL {
		w.URL = cleanURL
		w.LastResult = nil
	}
	if events != nil {
		w.Events = cleanEvents
	}
	if enabled != nil {
		w.Enabled = *enabled
	}
	return s.saveState()
}

// DeleteWebhook removes a webhook.
func (s *Storage) DeleteWebhook(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.syncLocked()
	i := s.webhookIndexLocked(id)
	if i < 0 {
		return uerr.New("webhook.notFound", "webhook not found")
	}
	s.state.Webhooks = append(s.state.Webhooks[:i], s.state.Webhooks[i+1:]...)
	if len(s.state.Webhooks) == 0 {
		s.state.Webhooks = nil
	}
	return s.saveState()
}

// RecordWebhookResult saves the outcome of the latest delivery to a webhook. A
// webhook deleted while its delivery was in flight is silently skipped.
func (s *Storage) RecordWebhookResult(id string, result types.WebhookResult) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.syncLocked()
	i := s.webhookIndexLocked(id)
	if i < 0 {
		return nil
	}
	r := result
	s.state.Webhooks[i].LastResult = &r
	return s.saveState()
}

// AdvanceMissingCheck records now as the time expected devices were last
// checked for having gone missing, and returns the time of the previous check
// (zero if there has never been one). Doing both under one lock, against the
// state file shared with any other process, is what keeps two scans from
// checking the same stretch of time twice.
func (s *Storage) AdvanceMissingCheck(now time.Time) time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.syncLocked()
	prev := s.state.MissingCheckedAt
	if !now.After(prev) {
		// The clock went backwards, or another process got here first with a
		// later time; keep the later mark so no window is checked twice.
		return now
	}
	s.state.MissingCheckedAt = now
	_ = s.saveState()
	return prev
}

func (s *Storage) webhookIndexLocked(id string) int {
	for i, w := range s.state.Webhooks {
		if w.ID == id {
			return i
		}
	}
	return -1
}

// webhookURLIndexLocked returns the index of the webhook posting to url, or -1.
// The webhook at skip is left out, so an update can keep its own URL. Two
// webhooks with one URL would send every event twice, which nobody wants.
func (s *Storage) webhookURLIndexLocked(url string, skip int) int {
	url = strings.TrimSpace(url)
	for i, w := range s.state.Webhooks {
		if i != skip && strings.TrimSpace(w.URL) == url {
			return i
		}
	}
	return -1
}

func errDuplicateWebhookURL() error {
	return uerr.New("webhook.duplicateUrl", "a webhook with that URL already exists")
}

func copyWebhook(w types.Webhook) types.Webhook {
	w.Events = append([]string(nil), w.Events...)
	if w.LastResult != nil {
		r := *w.LastResult
		w.LastResult = &r
	}
	return w
}

func validateWebhookName(name string) (string, error) {
	for _, r := range name {
		if unicode.IsControl(r) {
			return "", uerr.New("webhook.nameControlChars", "the name cannot contain control characters")
		}
	}
	name = strings.TrimSpace(name)
	if len([]rune(name)) > maxWebhookNameLen {
		return "", uerr.Newf("webhook.nameTooLong", "the name cannot be longer than %d characters", "max", maxWebhookNameLen)
	}
	return name, nil
}

func newWebhookID() (string, error) {
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("could not create a webhook ID: %w", err)
	}
	return hex.EncodeToString(b), nil
}
