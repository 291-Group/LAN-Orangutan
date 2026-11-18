package notify

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/291-Group/LAN-Orangutan/internal/types"
)

// fakeStore is an in-memory Store.
type fakeStore struct {
	mu        sync.Mutex
	devices   map[string]*types.Device
	hooks     []types.Webhook
	results   map[string][]types.WebhookResult
	lastCheck time.Time
}

func newFakeStore(hooks ...types.Webhook) *fakeStore {
	return &fakeStore{devices: map[string]*types.Device{}, hooks: hooks, results: map[string][]types.WebhookResult{}}
}

func (f *fakeStore) GetDevices() map[string]*types.Device {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make(map[string]*types.Device, len(f.devices))
	for k, v := range f.devices {
		cp := *v
		out[k] = &cp
	}
	return out
}

func (f *fakeStore) setDevice(d types.Device) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.devices[d.IP] = &d
}

func (f *fakeStore) GetWebhooks() []types.Webhook {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]types.Webhook(nil), f.hooks...)
}

func (f *fakeStore) RecordWebhookResult(id string, r types.WebhookResult) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.results[id] = append(f.results[id], r)
	return nil
}

func (f *fakeStore) resultsFor(id string) []types.WebhookResult {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]types.WebhookResult(nil), f.results[id]...)
}

func (f *fakeStore) AdvanceMissingCheck(now time.Time) time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	prev := f.lastCheck
	f.lastCheck = now
	return prev
}

// fastNotifier retries without the real pauses.
func fastNotifier() *Notifier {
	n := New()
	n.backoff = 5 * time.Millisecond
	return n
}

func samplePayload() Payload {
	return NewPayload(Change{Event: EventNew, Device: types.Device{IP: "10.0.0.5"}, Time: t0})
}

func TestSendPostsJSON(t *testing.T) {
	var gotBody map[string]interface{}
	var gotCT, gotUA string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotCT = r.Header.Get("Content-Type")
		gotUA = r.Header.Get("User-Agent")
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
	}))
	defer srv.Close()

	Version = "9.9.9"
	defer func() { Version = "" }()

	res := fastNotifier().Send(context.Background(), srv.URL+"/hook", samplePayload(), 3)
	if !res.Success || res.Status != 200 {
		t.Fatalf("want success, got %+v", res)
	}
	if gotCT != "application/json" {
		t.Errorf("Content-Type = %q", gotCT)
	}
	if gotUA != "LAN-Orangutan/9.9.9" {
		t.Errorf("User-Agent = %q", gotUA)
	}
	if gotBody["event"] != EventNew || gotBody["text"] == "" {
		t.Errorf("body = %v", gotBody)
	}
}

func TestSendRetriesOn500ThenGivesUp(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		http.Error(w, "broken", http.StatusInternalServerError)
	}))
	defer srv.Close()

	res := fastNotifier().Send(context.Background(), srv.URL, samplePayload(), 3)
	if res.Success || res.Status != 500 || !strings.Contains(res.Error, "500") {
		t.Fatalf("want a recorded 500 failure, got %+v", res)
	}
	if n := atomic.LoadInt32(&calls); n != 3 {
		t.Fatalf("want 3 attempts, got %d", n)
	}
}

func TestSendRetriesThenSucceeds(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) < 3 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	res := fastNotifier().Send(context.Background(), srv.URL, samplePayload(), 3)
	if !res.Success || res.Status != 204 {
		t.Fatalf("want success on the third try, got %+v", res)
	}
}

func TestSendDoesNotRetryClientErrors(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	res := fastNotifier().Send(context.Background(), srv.URL, samplePayload(), 3)
	if res.Success || res.Status != 404 {
		t.Fatalf("got %+v", res)
	}
	if n := atomic.LoadInt32(&calls); n != 1 {
		t.Fatalf("a 404 should not be retried, got %d attempts", n)
	}
}

// A receiver that echoes the request path in its error page must not get the
// secret in the URL into the stored result.
func TestSendErrorNeverHoldsTheURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Cannot POST "+r.URL.Path+"?"+r.URL.RawQuery, http.StatusNotFound)
	}))
	defer srv.Close()

	res := fastNotifier().Send(context.Background(), srv.URL+"/bot123456:SECRETTOKEN/sendMessage?chat_id=4242424242", samplePayload(), 1)
	if strings.Contains(res.Error, "SECRETTOKEN") || strings.Contains(res.Error, "4242424242") {
		t.Fatalf("error leaks the URL: %q", res.Error)
	}

	// An unreachable host: the http client's own error text includes the URL.
	res = fastNotifier().Send(context.Background(), "http://127.0.0.1:1/hook/SECRETTOKEN", samplePayload(), 1)
	if res.Success || res.Error == "" || strings.Contains(res.Error, "SECRETTOKEN") {
		t.Fatalf("unreachable: got %+v", res)
	}
}

func TestSendDoesNotFollowRedirects(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/old" {
			http.Redirect(w, r, "/new", http.StatusFound)
		}
	}))
	defer srv.Close()
	res := fastNotifier().Send(context.Background(), srv.URL+"/old", samplePayload(), 1)
	if res.Success || res.Status != http.StatusFound {
		t.Fatalf("a redirect should be reported, not followed: %+v", res)
	}
}

func TestSendTelegramAndNtfyBodies(t *testing.T) {
	var body []byte
	var ct, title string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ = io.ReadAll(r.Body)
		ct = r.Header.Get("Content-Type")
		title = r.Header.Get("Title")
	}))
	defer srv.Close()

	// Point the well-known hosts at the test server.
	n := fastNotifier()
	n.client.Transport = &http.Transport{
		DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, network, strings.TrimPrefix(srv.URL, "http://"))
		},
	}

	res := n.Send(context.Background(), "http://api.telegram.org/botX/sendMessage?chat_id=777", samplePayload(), 1)
	if !res.Success {
		t.Fatalf("telegram: %+v", res)
	}
	var tg map[string]interface{}
	_ = json.Unmarshal(body, &tg)
	if tg["chat_id"] != "777" || tg["text"] == "" {
		t.Errorf("telegram body = %s", body)
	}

	res = n.Send(context.Background(), "http://ntfy.sh/mytopic", samplePayload(), 1)
	if !res.Success {
		t.Fatalf("ntfy: %+v", res)
	}
	if !strings.HasPrefix(ct, "text/plain") || string(body) != samplePayload().Text || title == "" {
		t.Errorf("ntfy: Content-Type %q, Title %q, body %q", ct, title, body)
	}
}

// Finish must return straight away even when a receiver is slow, and the
// outcome is recorded once delivery completes.
func TestFinishNeverBlocksAndRecordsResult(t *testing.T) {
	release := make(chan struct{})
	var got int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release
		atomic.AddInt32(&got, 1)
	}))
	defer srv.Close()

	hook := types.Webhook{ID: "h1", URL: srv.URL, Events: AllEvents, Enabled: true}
	off := types.Webhook{ID: "h2", URL: srv.URL, Events: AllEvents, Enabled: false}
	store := newFakeStore(hook, off)
	store.setDevice(types.Device{IP: "10.0.0.1", MAC: "aa:aa:aa:aa:aa:01", Expected: true, LastSeen: time.Now()})

	n := fastNotifier()
	scan := n.Begin(store)
	store.setDevice(types.Device{IP: "10.0.0.5", MAC: "bb:bb:bb:bb:bb:05", LastSeen: time.Now()})

	start := time.Now()
	changes := scan.Finish()
	if time.Since(start) > time.Second {
		t.Fatalf("Finish blocked for %s", time.Since(start))
	}
	if len(changes) != 2 {
		t.Fatalf("want new+unexpected, got %v", eventsOf(changes))
	}

	close(release)
	if !n.Wait(5 * time.Second) {
		t.Fatal("delivery did not finish")
	}
	// The webhook wants both events, so the new device is one message.
	if c := atomic.LoadInt32(&got); c != 1 {
		t.Fatalf("receiver got %d messages, want 1", c)
	}
	res := store.resultsFor("h1")
	if len(res) != 1 || !res[0].Success || res[0].Event != EventNew {
		t.Fatalf("recorded results = %+v", res)
	}
	if len(store.resultsFor("h2")) != 0 {
		t.Fatal("a disabled webhook must not be sent to")
	}

	// The same scan result again is not news.
	if again := n.Begin(store).Finish(); len(again) != 0 {
		t.Fatalf("second scan should be quiet, got %v", eventsOf(again))
	}
}

// When a webhook keeps failing, the rest of that scan's messages are dropped
// rather than each being retried, and the failure is recorded.
func TestBatchGivesUpAfterFailure(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	hook := types.Webhook{ID: "h1", URL: srv.URL, Events: []string{EventNew}, Enabled: true}
	store := newFakeStore(hook)
	n := fastNotifier()
	n.deliverBatch(store, hook, []Change{
		{Event: EventNew, Device: types.Device{IP: "10.0.0.5"}, Time: t0},
		{Event: EventNew, Device: types.Device{IP: "10.0.0.6"}, Time: t0},
	})
	if c := atomic.LoadInt32(&calls); c != maxAttempts {
		t.Fatalf("want %d attempts for the first message only, got %d", maxAttempts, c)
	}
	res := store.resultsFor("h1")
	if len(res) != 1 || res[0].Success || res[0].Status != 503 {
		t.Fatalf("recorded = %+v", res)
	}
}

func TestSendTestRecordsResult(t *testing.T) {
	var body map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&body)
	}))
	defer srv.Close()
	hook := types.Webhook{ID: "h1", URL: srv.URL, Events: AllEvents, Enabled: false}
	store := newFakeStore(hook)
	res := fastNotifier().SendTest(context.Background(), store, hook)
	if !res.Success || res.Event != EventTest {
		t.Fatalf("got %+v", res)
	}
	if body["event"] != EventTest {
		t.Fatalf("body = %v", body)
	}
	if len(store.resultsFor("h1")) != 1 {
		t.Fatal("test result not recorded")
	}
}
