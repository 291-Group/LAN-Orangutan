package api

import (
	"encoding/json"
	"net/http"

	"github.com/291-Group/LAN-Orangutan/internal/notify"
	"github.com/291-Group/LAN-Orangutan/internal/types"
	"github.com/291-Group/LAN-Orangutan/internal/uerr"
)

// webhookView is a webhook as the API returns it. The URL is masked to its
// scheme and host, because the rest often holds a secret (a Telegram bot token,
// a Discord webhook key) and an API response can end up in a log or a
// screenshot. Editing a webhook's URL replaces it rather than revealing it.
type webhookView struct {
	ID         string               `json:"id"`
	Name       string               `json:"name"`
	URL        string               `json:"url"`
	Events     []string             `json:"events"`
	Enabled    bool                 `json:"enabled"`
	LastResult *types.WebhookResult `json:"last_result,omitempty"`
}

func viewWebhook(w types.Webhook) webhookView {
	return webhookView{
		ID:         w.ID,
		Name:       w.Name,
		URL:        notify.MaskURL(w.URL),
		Events:     w.Events,
		Enabled:    w.Enabled,
		LastResult: w.LastResult,
	}
}

// webhooksView returns the webhook list and the events a webhook can choose.
func (h *Handler) webhooksView() map[string]interface{} {
	hooks := h.store.GetWebhooks()
	views := make([]webhookView, 0, len(hooks))
	for _, w := range hooks {
		views = append(views, viewWebhook(w))
	}
	return map[string]interface{}{
		"webhooks": views,
		"events":   notify.AllEvents,
	}
}

// handleWebhooks handles GET/POST /api/webhooks. POST takes an action, like
// /api/groups: "add" (url, optional name and events), "update" (id plus any of
// name, url, events, enabled) and "delete" (id).
func (h *Handler) handleWebhooks(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.success(w, h.webhooksView())

	case http.MethodPost:
		var body struct {
			Action  string    `json:"action"`
			ID      string    `json:"id"`
			Name    *string   `json:"name"`
			URL     *string   `json:"url"`
			Events  *[]string `json:"events"`
			Enabled *bool     `json:"enabled"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			h.fail(w, http.StatusBadRequest, errInvalidBody)
			return
		}

		var err error
		switch body.Action {
		case "add":
			name, url, events := "", "", notify.AllEvents
			if body.Name != nil {
				name = *body.Name
			}
			if body.URL != nil {
				url = *body.URL
			}
			if body.Events != nil {
				events = *body.Events
			}
			_, err = h.store.AddWebhook(name, url, events)
		case "update":
			err = h.store.UpdateWebhook(body.ID, body.Name, body.URL, body.Events, body.Enabled)
		case "delete":
			err = h.store.DeleteWebhook(body.ID)
		default:
			h.fail(w, http.StatusBadRequest, invalidAction("add, update, delete"))
			return
		}
		if err != nil {
			// Bad URL, unknown event, unknown ID: all input problems.
			h.fail(w, http.StatusBadRequest, err)
			return
		}
		h.success(w, h.webhooksView())

	default:
		h.error(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// handleWebhookTest handles POST /api/webhooks/test, which sends a test message
// to one webhook and waits for the answer, so the Settings page can say
// straight away whether it worked. A delivery that fails is still a successful
// request; the outcome is in the result.
func (h *Handler) handleWebhookTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var body struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		h.fail(w, http.StatusBadRequest, errInvalidBody)
		return
	}
	hook, ok := h.store.GetWebhook(body.ID)
	if !ok {
		h.fail(w, http.StatusNotFound, uerr.New("webhook.notFound", "webhook not found"))
		return
	}
	result := notify.SendTest(r.Context(), h.store, hook)
	h.success(w, map[string]interface{}{
		"result":   result,
		"webhooks": h.webhooksView()["webhooks"],
	})
}
