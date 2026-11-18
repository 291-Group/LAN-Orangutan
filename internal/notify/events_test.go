package notify

import (
	"reflect"
	"testing"
	"time"

	"github.com/291-Group/LAN-Orangutan/internal/types"
	"github.com/291-Group/LAN-Orangutan/internal/uerr"
	"strings"
)

var t0 = time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)

func devs(list ...types.Device) map[string]*types.Device {
	m := make(map[string]*types.Device, len(list))
	for i := range list {
		d := list[i]
		m[d.IP] = &d
	}
	return m
}

// eventsOf flattens changes to "event ip" strings for easy comparison.
func eventsOf(changes []Change) []string {
	out := []string{}
	for _, c := range changes {
		s := c.Event + " " + c.Device.IP
		if c.PreviousIP != "" {
			s += " from " + c.PreviousIP
		}
		out = append(out, s)
	}
	return out
}

func TestDetect(t *testing.T) {
	scanStart := t0
	scanEnd := t0.Add(30 * time.Second)
	online := scanStart.Add(-5 * time.Minute)
	offline := scanStart.Add(-3 * time.Hour)
	seenNow := scanEnd

	known := types.Device{IP: "10.0.0.1", MAC: "aa:aa:aa:aa:aa:01", LastSeen: online}

	tests := []struct {
		name   string
		before map[string]*types.Device
		after  map[string]*types.Device
		want   []string
	}{
		{
			name:   "first scan of an empty inventory is a baseline",
			before: devs(),
			after:  devs(types.Device{IP: "10.0.0.5", LastSeen: seenNow}),
			want:   []string{},
		},
		{
			name:   "brand new device is new and unexpected",
			before: devs(known),
			after:  devs(known, types.Device{IP: "10.0.0.5", MAC: "bb:bb:bb:bb:bb:05", LastSeen: seenNow}),
			want:   []string{"device.new 10.0.0.5", "device.unexpected 10.0.0.5"},
		},
		{
			name:   "unexpected device still online fires nothing",
			before: devs(known),
			after:  devs(types.Device{IP: "10.0.0.1", MAC: known.MAC, LastSeen: seenNow}),
			want:   []string{},
		},
		{
			name:   "unexpected device back after being offline",
			before: devs(types.Device{IP: "10.0.0.1", MAC: known.MAC, LastSeen: offline}),
			after:  devs(types.Device{IP: "10.0.0.1", MAC: known.MAC, LastSeen: seenNow}),
			want:   []string{"device.unexpected 10.0.0.1"},
		},
		{
			name:   "expected device back after being offline is not unexpected",
			before: devs(types.Device{IP: "10.0.0.1", MAC: known.MAC, Expected: true, LastSeen: offline}),
			after:  devs(types.Device{IP: "10.0.0.1", MAC: known.MAC, Expected: true, LastSeen: seenNow}),
			want:   []string{},
		},
		{
			name:   "offline device not seen by this scan fires nothing",
			before: devs(types.Device{IP: "10.0.0.1", MAC: known.MAC, LastSeen: offline}),
			after:  devs(types.Device{IP: "10.0.0.1", MAC: known.MAC, LastSeen: offline}),
			want:   []string{},
		},
		{
			name:   "online device that moved is moved only",
			before: devs(types.Device{IP: "10.0.0.1", MAC: known.MAC, Expected: true, LastSeen: online}),
			after: devs(types.Device{IP: "10.0.0.9", MAC: known.MAC, Expected: true, LastSeen: seenNow,
				AddressHistory: []types.AddressChange{{IP: "10.0.0.1", ChangedAt: scanEnd}}}),
			want: []string{"device.moved 10.0.0.9 from 10.0.0.1"},
		},
		{
			name:   "unexpected device that was offline and comes back at a new IP",
			before: devs(types.Device{IP: "10.0.0.1", MAC: known.MAC, LastSeen: offline}),
			after: devs(types.Device{IP: "10.0.0.9", MAC: known.MAC, LastSeen: seenNow,
				AddressHistory: []types.AddressChange{{IP: "10.0.0.1", ChangedAt: scanEnd}}}),
			want: []string{"device.moved 10.0.0.9 from 10.0.0.1", "device.unexpected 10.0.0.9"},
		},
		{
			name: "old address history is not a new move",
			before: devs(types.Device{IP: "10.0.0.9", MAC: known.MAC, Expected: true, LastSeen: online,
				AddressHistory: []types.AddressChange{{IP: "10.0.0.1", ChangedAt: offline}}}),
			after: devs(types.Device{IP: "10.0.0.9", MAC: known.MAC, Expected: true, LastSeen: seenNow,
				AddressHistory: []types.AddressChange{{IP: "10.0.0.1", ChangedAt: offline}}}),
			want: []string{},
		},
		{
			name:   "device without a MAC at a new IP is new",
			before: devs(types.Device{IP: "10.0.0.1", LastSeen: online}),
			after:  devs(types.Device{IP: "10.0.0.1", LastSeen: seenNow}, types.Device{IP: "10.0.0.2", LastSeen: seenNow}),
			want:   []string{"device.new 10.0.0.2", "device.unexpected 10.0.0.2"},
		},
		{
			name:   "IPv6 privacy address is not reported",
			before: devs(known),
			after:  devs(known, types.Device{IP: "2001:db8::1234", MAC: "f6:c0:74:52:09:ae", LastSeen: seenNow}),
			want:   []string{},
		},
		{
			name:   "IPv6 device with a hardware MAC is reported",
			before: devs(known),
			after:  devs(known, types.Device{IP: "2001:db8::10", MAC: "dc:a6:32:e8:95:09", LastSeen: seenNow}),
			want:   []string{"device.new 2001:db8::10", "device.unexpected 2001:db8::10"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := eventsOf(Detect(tt.before, tt.after, scanStart, scanEnd))
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

// Running the same comparison again after a scan has already been reported
// must find nothing: each event is one transition, not a state.
func TestDetectNoRepeatOnNextScan(t *testing.T) {
	before := devs(types.Device{IP: "10.0.0.1", MAC: "aa:aa:aa:aa:aa:01", LastSeen: t0.Add(-5 * time.Minute)})
	after1 := devs(
		types.Device{IP: "10.0.0.1", MAC: "aa:aa:aa:aa:aa:01", LastSeen: t0},
		types.Device{IP: "10.0.0.5", MAC: "bb:bb:bb:bb:bb:05", LastSeen: t0},
	)
	if got := Detect(before, after1, t0.Add(-time.Second), t0); len(got) != 2 {
		t.Fatalf("first scan: want new+unexpected, got %v", eventsOf(got))
	}
	next := t0.Add(5 * time.Minute)
	after2 := devs(
		types.Device{IP: "10.0.0.1", MAC: "aa:aa:aa:aa:aa:01", LastSeen: next},
		types.Device{IP: "10.0.0.5", MAC: "bb:bb:bb:bb:bb:05", LastSeen: next},
	)
	if got := Detect(after1, after2, next.Add(-time.Second), next); len(got) != 0 {
		t.Fatalf("second scan should be quiet, got %v", eventsOf(got))
	}
}

func TestDetectMissing(t *testing.T) {
	exp := func(ip string, lastSeen time.Time) types.Device {
		return types.Device{IP: ip, Expected: true, LastSeen: lastSeen}
	}
	tests := []struct {
		name       string
		devices    map[string]*types.Device
		since, now time.Time
		want       []string
	}{
		{
			name:    "crossed the threshold since the last check",
			devices: devs(exp("10.0.0.1", t0.Add(-62*time.Minute))),
			since:   t0.Add(-5 * time.Minute), now: t0,
			want: []string{"device.missing 10.0.0.1"},
		},
		{
			name:    "already missing at the last check",
			devices: devs(exp("10.0.0.1", t0.Add(-2*time.Hour))),
			since:   t0.Add(-5 * time.Minute), now: t0,
			want: []string{},
		},
		{
			name:    "still online",
			devices: devs(exp("10.0.0.1", t0.Add(-30*time.Minute))),
			since:   t0.Add(-5 * time.Minute), now: t0,
			want: []string{},
		},
		{
			name:    "not expected",
			devices: devs(types.Device{IP: "10.0.0.1", LastSeen: t0.Add(-62 * time.Minute)}),
			since:   t0.Add(-5 * time.Minute), now: t0,
			want: []string{},
		},
		{
			name:    "no previous check is a baseline",
			devices: devs(exp("10.0.0.1", t0.Add(-62*time.Minute))),
			since:   time.Time{}, now: t0,
			want: []string{},
		},
		{
			name:    "crossed while nothing was checking (server was down)",
			devices: devs(exp("10.0.0.1", t0.Add(-3*time.Hour))),
			since:   t0.Add(-4 * time.Hour), now: t0,
			want: []string{"device.missing 10.0.0.1"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := eventsOf(DetectMissing(tt.devices, tt.since, tt.now))
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

// Consecutive checks cover consecutive windows, so a device crossing the
// threshold is reported by exactly one of them.
func TestDetectMissingFiresOncePerDisappearance(t *testing.T) {
	devices := devs(types.Device{IP: "10.0.0.1", Expected: true, LastSeen: t0})
	fired := 0
	prev := t0
	for i := 1; i <= 30; i++ {
		now := t0.Add(time.Duration(i) * 5 * time.Minute)
		fired += len(DetectMissing(devices, prev, now))
		prev = now
	}
	if fired != 1 {
		t.Fatalf("missing fired %d times over 2.5 hours, want 1", fired)
	}
}

func TestDedupeSuppressesRepeats(t *testing.T) {
	n := New()
	c := Change{Event: EventNew, Device: types.Device{IP: "10.0.0.5", MAC: "bb:bb:bb:bb:bb:05"}}
	if got := n.dedupe([]Change{c}, t0); len(got) != 1 {
		t.Fatalf("first report should pass, got %d", len(got))
	}
	if got := n.dedupe([]Change{c}, t0.Add(time.Minute)); len(got) != 0 {
		t.Fatalf("an overlapping scan should not report it again, got %d", len(got))
	}
	if got := n.dedupe([]Change{c}, t0.Add(2*time.Hour)); len(got) != 1 {
		t.Fatalf("after the window it may fire again, got %d", len(got))
	}
}

func TestNormalizeEvents(t *testing.T) {
	got, err := NormalizeEvents([]string{"moved", " NEW ", "device.new", "missing"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{EventNew, EventMissing, EventMoved}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
	_, err = NormalizeEvents([]string{"new", "exploded"})
	if err == nil {
		t.Fatal("an unknown event should be rejected")
	}
	// The message names both spellings, since both are accepted, and the code
	// carries what was typed.
	if !strings.Contains(err.Error(), "new") || !strings.Contains(err.Error(), "device.new") {
		t.Errorf("error should mention the short and full forms: %v", err)
	}
	if e, ok := uerr.Of(err); !ok || e.Code != "event.unknown" || e.Args["event"] != "exploded" {
		t.Errorf("unknown event should carry event.unknown and the typed name: %v", err)
	}
	_, err = NormalizeEvents([]string{"", " "})
	if e, ok := uerr.Of(err); !ok || e.Code != "event.none" {
		t.Errorf("no events should be rejected with event.none: %v", err)
	}
}

func TestForWebhookCollapsesNewAndUnexpected(t *testing.T) {
	d := types.Device{IP: "10.0.0.5"}
	changes := []Change{
		{Event: EventNew, Device: d},
		{Event: EventUnexpected, Device: d},
		{Event: EventUnexpected, Device: types.Device{IP: "10.0.0.6"}},
	}
	both := types.Webhook{Events: []string{EventNew, EventUnexpected}}
	if got := eventsOf(forWebhook(both, changes)); !reflect.DeepEqual(got, []string{"device.new 10.0.0.5", "device.unexpected 10.0.0.6"}) {
		t.Errorf("both: got %v", got)
	}
	onlyUnexpected := types.Webhook{Events: []string{EventUnexpected}}
	if got := forWebhook(onlyUnexpected, changes); len(got) != 2 {
		t.Errorf("unexpected only: got %v", eventsOf(got))
	}
	var many []Change
	for i := 0; i < 50; i++ {
		many = append(many, Change{Event: EventMissing, Device: types.Device{IP: "10.0.1." + string(rune('a'+i))}})
	}
	if got := forWebhook(types.Webhook{Events: []string{EventMissing}}, many); len(got) != maxPerScan {
		t.Errorf("cap: got %d, want %d", len(got), maxPerScan)
	}
}
