package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// webhookResponse is the shape of GET/POST /api/webhooks.
type webhookResponse struct {
	Success bool   `json:"success"`
	Error   string `json:"error"`
	Data    struct {
		Webhooks []webhookView `json:"webhooks"`
		Events   []string      `json:"events"`
		Result   *struct {
			Success bool   `json:"success"`
			Status  int    `json:"status"`
			Event   string `json:"event"`
			Error   string `json:"error"`
		} `json:"result"`
	} `json:"data"`
}

func callWebhooks(t *testing.T, h *Handler, method, path, body string) (int, webhookResponse) {
	t.Helper()
	var req *http.Request
	if body == "" {
		req = httptest.NewRequest(method, path, nil)
	} else {
		req = httptest.NewRequest(method, path, bytes.NewReader([]byte(body)))
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var resp webhookResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("bad JSON from %s %s: %v: %s", method, path, err, rec.Body.String())
	}
	return rec.Code, resp
}

func TestAPIWebhooksCRUDMasksURL(t *testing.T) {
	h, store := newTestAPI(t)

	code, resp := callWebhooks(t, h, http.MethodGet, "/api/webhooks", "")
	if code != http.StatusOK || len(resp.Data.Webhooks) != 0 || len(resp.Data.Events) != 4 {
		t.Fatalf("empty list: %d %+v", code, resp)
	}

	secret := "https://api.telegram.org/bot123:SECRET/sendMessage?chat_id=42"
	code, resp = callWebhooks(t, h, http.MethodPost, "/api/webhooks",
		`{"action":"add","url":"`+secret+`","name":"Phone"}`)
	if code != http.StatusOK || len(resp.Data.Webhooks) != 1 {
		t.Fatalf("add: %d %+v", code, resp)
	}
	got := resp.Data.Webhooks[0]
	if got.URL != "https://api.telegram.org/…" || got.Name != "Phone" || !got.Enabled || len(got.Events) != 4 {
		t.Errorf("added = %+v", got)
	}

	// The full URL is stored but never returned.
	if stored := store.GetWebhooks(); stored[0].URL != secret {
		t.Errorf("stored URL = %q", stored[0].URL)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/webhooks", nil))
	if strings.Contains(rec.Body.String(), "SECRET") || strings.Contains(rec.Body.String(), "chat_id") {
		t.Fatalf("GET leaked the URL: %s", rec.Body.String())
	}

	code, resp = callWebhooks(t, h, http.MethodPost, "/api/webhooks",
		`{"action":"update","id":"`+got.ID+`","enabled":false,"events":["missing"]}`)
	if code != http.StatusOK || resp.Data.Webhooks[0].Enabled || resp.Data.Webhooks[0].Events[0] != "device.missing" {
		t.Fatalf("update: %d %+v", code, resp)
	}

	code, resp = callWebhooks(t, h, http.MethodPost, "/api/webhooks", `{"action":"delete","id":"`+got.ID+`"}`)
	if code != http.StatusOK || len(resp.Data.Webhooks) != 0 {
		t.Fatalf("delete: %d %+v", code, resp)
	}
}

func TestAPIWebhooksRejectsBadInput(t *testing.T) {
	h, _ := newTestAPI(t)
	cases := []string{
		`{"action":"add","url":"ftp://example.com/x"}`,
		`{"action":"add","url":"https://example.com/x","events":[]}`,
		`{"action":"add","url":"https://example.com/x","events":["exploded"]}`,
		`{"action":"update","id":"nope","enabled":true}`,
		`{"action":"frobnicate"}`,
		`not json`,
	}
	for _, body := range cases {
		code, resp := callWebhooks(t, h, http.MethodPost, "/api/webhooks", body)
		if code != http.StatusBadRequest || resp.Success {
			t.Errorf("%s: got %d %+v, want 400", body, code, resp)
		}
	}
	if code, _ := callWebhooks(t, h, http.MethodDelete, "/api/webhooks", `{}`); code != http.StatusMethodNotAllowed {
		t.Errorf("DELETE: got %d", code)
	}
}

func TestAPIWebhookTest(t *testing.T) {
	h, store := newTestAPI(t)
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		if r.URL.Path == "/broken" {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	ok, err := store.AddWebhook("", srv.URL+"/hook", []string{"new"})
	if err != nil {
		t.Fatal(err)
	}
	broken, err := store.AddWebhook("", srv.URL+"/broken", []string{"new"})
	if err != nil {
		t.Fatal(err)
	}

	code, resp := callWebhooks(t, h, http.MethodPost, "/api/webhooks/test", `{"id":"`+ok.ID+`"}`)
	if code != http.StatusOK || resp.Data.Result == nil || !resp.Data.Result.Success || resp.Data.Result.Event != "test" {
		t.Fatalf("test ok: %d %+v", code, resp)
	}
	if lr := resp.Data.Webhooks[0].LastResult; lr == nil || !lr.Success {
		t.Errorf("last result not shown: %+v", resp.Data.Webhooks[0])
	}

	code, resp = callWebhooks(t, h, http.MethodPost, "/api/webhooks/test", `{"id":"`+broken.ID+`"}`)
	if code != http.StatusOK || resp.Data.Result == nil || resp.Data.Result.Success || resp.Data.Result.Status != 404 {
		t.Fatalf("test broken: %d %+v", code, resp)
	}
	// A test is one attempt, not a retry loop.
	if c := atomic.LoadInt32(&calls); c != 2 {
		t.Errorf("receiver got %d requests, want 2", c)
	}

	if code, _ := callWebhooks(t, h, http.MethodPost, "/api/webhooks/test", `{"id":"missing"}`); code != http.StatusNotFound {
		t.Errorf("unknown id: got %d", code)
	}
}
