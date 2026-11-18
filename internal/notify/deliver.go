package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/291-Group/LAN-Orangutan/internal/types"
)

// Version goes in the User-Agent header. The CLI sets it once at startup,
// before any scan runs.
var Version string

// Store is the part of storage the notifier needs. It is an interface so the
// tests can run without a data directory.
type Store interface {
	GetDevices() map[string]*types.Device
	GetWebhooks() []types.Webhook
	RecordWebhookResult(id string, result types.WebhookResult) error
	// AdvanceMissingCheck records now as the time of the latest missing check
	// and returns the previous one (zero if there was none).
	AdvanceMissingCheck(now time.Time) time.Time
}

const (
	// attemptTimeout bounds one delivery attempt, so a receiver that accepts
	// the connection and never answers cannot hold a slot for long.
	attemptTimeout = 10 * time.Second
	// maxAttempts is how many times a failing delivery is tried before the
	// notifier gives up on it.
	maxAttempts = 3
	// maxConcurrent bounds how many webhooks are being posted to at once.
	maxConcurrent = 4
	// maxPerScan caps how many messages one webhook gets from one scan. A
	// network that suddenly looks completely different (a new router, a
	// restored backup) should not turn into a flood of messages.
	maxPerScan = 20
	// dedupeWindow suppresses an identical event for the same device seen
	// again soon after, which happens when two scans overlap. An hour is safe
	// because every real repeat (offline and back, missing and back) takes at
	// least that long.
	dedupeWindow = time.Hour
)

// Notifier detects device events around scans and delivers them to webhooks in
// the background.
type Notifier struct {
	client   *http.Client
	attempts int
	backoff  time.Duration // before the second attempt; doubles each time
	sem      chan struct{}
	wg       sync.WaitGroup

	mu     sync.Mutex
	recent map[string]time.Time
}

// New returns a notifier with the standard timeouts and retry policy.
func New() *Notifier {
	return &Notifier{
		client: &http.Client{
			Timeout: attemptTimeout,
			// A redirect would turn the POST into a GET and "succeed" without
			// delivering anything, so a 3xx is reported instead of followed.
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
		attempts: maxAttempts,
		backoff:  2 * time.Second,
		sem:      make(chan struct{}, maxConcurrent),
		recent:   make(map[string]time.Time),
	}
}

// Default is the notifier shared by every scan in this process, so the limit
// on concurrent deliveries and the repeat suppression apply across all of them.
var Default = New()

// Begin records the device list before a scan. Call Finish on the result when
// the scan is done:
//
//	defer notify.Begin(store).Finish()
func Begin(store Store) *Scan { return Default.Begin(store) }

// Wait blocks until the default notifier has nothing left to deliver, or
// timeout passes. The CLI calls it before exiting so a scan's notifications
// are not cut off with the process.
func Wait(timeout time.Duration) bool { return Default.Wait(timeout) }

// Scan is the state captured at the start of a scan.
type Scan struct {
	n      *Notifier
	store  Store
	before map[string]*types.Device
	at     time.Time
}

// Begin records the device list before a scan.
func (n *Notifier) Begin(store Store) *Scan {
	return &Scan{n: n, store: store, before: store.GetDevices(), at: time.Now()}
}

// Finish compares the device list with the one recorded by Begin, checks for
// expected devices that have gone missing, and hands the events to the webhooks
// that want them. Delivery happens in the background, so Finish returns at once
// and never slows the scan down. It returns the events found, for tests.
func (s *Scan) Finish() []Change {
	now := time.Now()
	after := s.store.GetDevices()
	changes := Detect(s.before, after, s.at, now)
	changes = append(changes, DetectMissing(after, s.store.AdvanceMissingCheck(now), now)...)
	changes = s.n.dedupe(changes, now)
	if len(changes) > 0 {
		s.n.dispatch(s.store, changes)
	}
	return changes
}

// dedupe drops any change already reported within dedupeWindow and marks the
// rest as reported. That happens before forWebhook caps a batch at maxPerScan
// and before a failed delivery drops the remainder of one, so an event cut
// off by either is suppressed for the hour by design: a flood or a dead
// receiver should not come back as a second flood on the next scan.
func (n *Notifier) dedupe(changes []Change, now time.Time) []Change {
	n.mu.Lock()
	defer n.mu.Unlock()
	for k, t := range n.recent {
		if now.Sub(t) >= dedupeWindow {
			delete(n.recent, k)
		}
	}
	out := changes[:0]
	for _, c := range changes {
		k := dedupeKey(c)
		if _, seen := n.recent[k]; seen {
			continue
		}
		n.recent[k] = now
		out = append(out, c)
	}
	return out
}

// dispatch starts one background delivery run per enabled webhook that wants
// any of the changes.
func (n *Notifier) dispatch(store Store, changes []Change) {
	for _, hook := range store.GetWebhooks() {
		if !hook.Enabled {
			continue
		}
		batch := forWebhook(hook, changes)
		if len(batch) == 0 {
			continue
		}
		n.wg.Add(1)
		go func(hook types.Webhook, batch []Change) {
			defer n.wg.Done()
			n.sem <- struct{}{}
			defer func() { <-n.sem }()
			n.deliverBatch(store, hook, batch)
		}(hook, batch)
	}
}

// forWebhook picks the changes a webhook subscribes to. A brand new device is
// also an unexpected one; a webhook that wants both gets only the "new"
// message, since a second message about the same device says nothing more.
func forWebhook(hook types.Webhook, changes []Change) []Change {
	wants := make(map[string]bool, len(hook.Events))
	for _, e := range hook.Events {
		wants[e] = true
	}
	isNew := make(map[string]bool)
	if wants[EventNew] {
		for _, c := range changes {
			if c.Event == EventNew {
				isNew[c.Device.IP] = true
			}
		}
	}
	var out []Change
	for _, c := range changes {
		if !wants[c.Event] {
			continue
		}
		if c.Event == EventUnexpected && isNew[c.Device.IP] {
			continue
		}
		out = append(out, c)
	}
	if len(out) > maxPerScan {
		out = out[:maxPerScan]
	}
	return out
}

// deliverBatch posts a webhook's messages in order. If one cannot be delivered
// after every retry, the rest are dropped: the receiver is down or the URL is
// wrong, and retrying each remaining message would only hold up the queue.
// The last outcome is recorded either way so the UI can show it.
func (n *Notifier) deliverBatch(store Store, hook types.Webhook, batch []Change) {
	for _, c := range batch {
		res := n.Send(context.Background(), hook.URL, NewPayload(c), n.attempts)
		res.Event = c.Event
		_ = store.RecordWebhookResult(hook.ID, res)
		if !res.Success {
			return
		}
	}
}

// SendTest posts a test message to a webhook once, without retries, and
// records the outcome. It runs synchronously so the caller can show the result.
func (n *Notifier) SendTest(ctx context.Context, store Store, hook types.Webhook) types.WebhookResult {
	res := n.Send(ctx, hook.URL, TestPayload(time.Now()), 1)
	res.Event = EventTest
	_ = store.RecordWebhookResult(hook.ID, res)
	return res
}

// SendTest posts a test message with the default notifier.
func SendTest(ctx context.Context, store Store, hook types.Webhook) types.WebhookResult {
	return Default.SendTest(ctx, store, hook)
}

// Send posts one payload, trying up to attempts times with a growing pause in
// between. Only failures that might pass are retried: a network error, a 5xx,
// or 429 Too Many Requests. Any other 4xx means the URL or the request is
// wrong, and sending it again would get the same answer.
func (n *Notifier) Send(ctx context.Context, rawURL string, p Payload, attempts int) types.WebhookResult {
	if attempts < 1 {
		attempts = 1
	}
	wait := n.backoff
	var res types.WebhookResult
	for i := 0; i < attempts; i++ {
		if i > 0 {
			select {
			case <-ctx.Done():
				return res
			case <-time.After(wait):
			}
			wait *= 2
		}
		var retry bool
		res, retry = n.post(ctx, rawURL, p)
		if res.Success || !retry {
			return res
		}
	}
	return res
}

// post makes one attempt and reports the outcome and whether it is worth
// retrying.
func (n *Notifier) post(ctx context.Context, rawURL string, p Payload) (types.WebhookResult, bool) {
	res := types.WebhookResult{Time: time.Now()}
	u, err := url.Parse(rawURL)
	if err != nil {
		res.Error = "not a valid URL"
		return res, false
	}

	var body []byte
	contentType := "application/json"
	if ntfyPlain(u) {
		body = []byte(p.Text)
		contentType = "text/plain; charset=utf-8"
	} else {
		telegramChat(u, &p)
		body, err = json.Marshal(p)
		if err != nil {
			res.Error = err.Error()
			return res, false
		}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, rawURL, bytes.NewReader(body))
	if err != nil {
		res.Error = describeErr(err)
		return res, false
	}
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("User-Agent", userAgent())
	if contentType != "application/json" {
		req.Header.Set("Title", "LAN Orangutan")
	}

	resp, err := n.client.Do(req)
	if err != nil {
		res.Error = describeErr(err)
		return res, true
	}
	defer resp.Body.Close()
	snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))

	res.Status = resp.StatusCode
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		res.Success = true
		res.Body = oneLine(redact(string(snippet), u), 200)
		return res, false
	}
	res.Error = "HTTP " + resp.Status
	if s := oneLine(redact(string(snippet), u), 200); s != "" {
		res.Error += ": " + s
	}
	return res, resp.StatusCode >= 500 || resp.StatusCode == http.StatusTooManyRequests
}

func userAgent() string {
	if Version == "" {
		return "LAN-Orangutan"
	}
	return "LAN-Orangutan/" + Version
}

// describeErr turns a request error into something safe to store and show.
// The http client wraps errors in a *url.Error whose text includes the full
// URL, and a webhook URL can hold a secret, so the wrapper is removed.
func describeErr(err error) string {
	var ue *url.Error
	if errors.As(err, &ue) {
		err = ue.Err
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded) {
		return fmt.Sprintf("no answer within %s", attemptTimeout)
	}
	if ue != nil && ue.Timeout() {
		return fmt.Sprintf("no answer within %s", attemptTimeout)
	}
	return err.Error()
}

// redact removes any part of the webhook URL that a receiver echoed back in its
// error page ("Cannot POST /bot123:abc/sendMessage"), so the stored error
// cannot leak the secret the URL holds.
func redact(s string, u *url.URL) string {
	secrets := []string{u.RawQuery, u.EscapedPath(), u.Path}
	if u.User != nil {
		secrets = append(secrets, u.User.String())
	}
	secrets = append(secrets, strings.Split(u.Path, "/")...)
	for _, secret := range secrets {
		if len(secret) >= 8 {
			s = strings.ReplaceAll(s, secret, "…")
		}
	}
	return s
}

// oneLine collapses a response body to a short single line for display.
func oneLine(s string, max int) string {
	s = strings.Join(strings.Fields(s), " ")
	if len([]rune(s)) > max {
		s = string([]rune(s)[:max]) + "…"
	}
	return s
}

// Wait blocks until every queued delivery has finished, or timeout passes. It
// reports whether everything finished.
func (n *Notifier) Wait(timeout time.Duration) bool {
	done := make(chan struct{})
	go func() {
		n.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return true
	case <-time.After(timeout):
		return false
	}
}
