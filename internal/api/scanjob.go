package api

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// percentUnknown is reported when a network has never been scanned before and
// there is therefore no timing history to estimate progress from.
const percentUnknown = -1

// scanJob tracks a scan running in the background. Scans of large networks take
// minutes, which is far too long to hold an HTTP request open, so the scan runs
// detached and the UI polls for progress.
type scanJob struct {
	id       string
	networks []string
	cancel   context.CancelFunc

	mu               sync.RWMutex
	status           string // running, done, cancelled or failed
	currentNetwork   string
	networkIndex     int // 1-based; 0 before the first network starts
	deviceCount      int
	startedAt        time.Time
	networkStartedAt time.Time
	// estimatedSeconds is how long the current network took to scan last time,
	// or 0 when it has never been scanned before.
	estimatedSeconds float64
	results          []networkScanSummary
	err              string

	// automatic marks a scan the background scanner started, as opposed to one
	// the user clicked. Set once at creation. The UI uses it to stay quiet for
	// automatic scans instead of popping the progress overlay on its own.
	automatic bool
}

// scanProgress is the snapshot of a job returned to the UI.
type scanProgress struct {
	JobID          string  `json:"job_id"`
	Status         string  `json:"status"`
	CurrentNetwork string  `json:"current_network,omitempty"`
	NetworkIndex   int     `json:"network_index"`
	NetworkCount   int     `json:"network_count"`
	DeviceCount    int     `json:"device_count"`
	Elapsed        float64 `json:"elapsed"`
	// Percent is the estimated completion of the whole job, or -1 when the
	// current network has no timing history and progress cannot be estimated.
	Percent float64 `json:"percent"`
	// Remaining is the estimated number of seconds left, omitted when unknown.
	Remaining *float64             `json:"remaining,omitempty"`
	Networks  []networkScanSummary `json:"networks"`
	Error     string               `json:"error,omitempty"`
	// Automatic is true for a scan the background scanner started. The UI does
	// not show its progress overlay for these, so an automatic scan never pops
	// a dialog with a Cancel button on its own.
	Automatic bool `json:"automatic"`
}

// snapshot returns the current progress of the job. The estimate is based on
// how long each network took to scan previously, which is real measured data,
// but it is only ever an estimate: nmap cannot report incremental progress for
// a ping sweep, so there is nothing more accurate to use.
func (j *scanJob) snapshot() scanProgress {
	j.mu.RLock()
	defer j.mu.RUnlock()

	p := scanProgress{
		JobID:          j.id,
		Status:         j.status,
		CurrentNetwork: j.currentNetwork,
		NetworkIndex:   j.networkIndex,
		NetworkCount:   len(j.networks),
		DeviceCount:    j.deviceCount,
		Elapsed:        time.Since(j.startedAt).Seconds(),
		Percent:        percentUnknown,
		Networks:       append([]networkScanSummary(nil), j.results...),
		Error:          j.err,
		Automatic:      j.automatic,
	}

	// Only a job that ran to completion is 100%. A cancelled or failed job
	// stopped wherever it stopped, so leave its progress unknown rather than
	// reporting it as finished.
	if j.status == "done" {
		p.Percent = 100
		return p
	}
	if j.status != "running" {
		return p
	}

	// Without timing history for the current network there is no honest way to
	// estimate progress, so report it as unknown and let the UI show an
	// indeterminate state rather than invent a number.
	if j.estimatedSeconds <= 0 || j.networkIndex == 0 {
		return p
	}

	// Weight every network equally: finished ones count in full, and the
	// current one counts as its own elapsed fraction. Cap the fraction just
	// below 1 so a network that overruns its estimate does not appear complete
	// while it is still working.
	fraction := time.Since(j.networkStartedAt).Seconds() / j.estimatedSeconds
	if fraction > 0.99 {
		fraction = 0.99
	}
	p.Percent = (float64(j.networkIndex-1) + fraction) / float64(len(j.networks)) * 100

	remaining := (1 - fraction) * j.estimatedSeconds
	p.Remaining = &remaining
	return p
}

// startScanJob begins scanning the given networks in the background. The caller
// must hold h.jobMu. automatic marks a scan the background scanner started, so
// the UI can stay quiet for it.
func (h *Handler) startScanJob(networks []string, automatic bool) *scanJob {
	// Descend from h.scanCtx (the server's shutdown context) rather than
	// context.Background(), so cancelling it at shutdown cancels a running scan.
	ctx, cancel := context.WithCancel(h.scanCtx)

	job := &scanJob{
		id:        fmt.Sprintf("scan-%d", time.Now().UnixNano()),
		networks:  networks,
		cancel:    cancel,
		status:    "running",
		startedAt: time.Now(),
		results:   make([]networkScanSummary, 0, len(networks)),
		automatic: automatic,
	}

	h.scanWG.Add(1)
	go func() {
		defer h.scanWG.Done()
		job.run(ctx, h)
	}()
	return job
}

// run scans each network in turn, recording the outcome of each. A network that
// is rate limited or fails does not abort the job, so one bad interface cannot
// hide results from the others.
func (j *scanJob) run(ctx context.Context, h *Handler) {
	var scannedNetworks []string
	for i, cidr := range j.networks {
		if ctx.Err() != nil {
			j.finish("cancelled", "")
			return
		}

		j.beginNetwork(cidr, i+1, h.store.GetLastDuration(cidr))

		summary := networkScanSummary{Network: cidr}

		lastScan := h.store.GetLastScan(cidr)
		if canScan, waitTime := h.scanner.CheckRateLimit(lastScan); !canScan {
			summary.Status = "skipped"
			summary.Error = "rate limited, wait " + waitTime.Round(time.Second).String()
			j.addResult(summary, 0)
			continue
		}

		// scanNetwork applies its own per-network timeout.
		result, err := h.scanNetwork(ctx, cidr)

		if err != nil {
			// A cancelled job surfaces as a scan error, but it is not a failure.
			if ctx.Err() != nil {
				j.finish("cancelled", "")
				return
			}
			summary.Status = "failed"
			summary.Error = err.Error()
			j.addResult(summary, 0)
			continue
		}

		scannedNetworks = append(scannedNetworks, cidr)
		summary.Status = "scanned"
		summary.DeviceCount = result.DeviceCount
		summary.Duration = result.Duration
		j.addResult(summary, result.DeviceCount)
	}

	// Supplemental discovery, once for the whole job: IPv4 neighbors already
	// known to the host, IPv6 neighbors (an IPv6 subnet cannot be swept), and
	// mDNS announcements. They merge as secondary sources. A cancelled job skips them.
	if ctx.Err() == nil {
		if ipv4 := h.scanner.DiscoverIPv4(ctx, scannedNetworks); len(ipv4) > 0 {
			if err := h.store.MergeSupplemental(ipv4); err == nil {
				j.addResult(networkScanSummary{Network: "IPv4 neighbors", Status: "scanned", DeviceCount: len(ipv4)}, len(ipv4))
			}
		}
	}
	if ctx.Err() == nil {
		if ipv6 := h.scanner.DiscoverIPv6(ctx); len(ipv6) > 0 {
			if err := h.store.MergeIPv6Neighbors(ipv6); err == nil {
				j.addResult(networkScanSummary{Network: "IPv6 neighbors", Status: "scanned", DeviceCount: len(ipv6)}, len(ipv6))
			}
		}
	}
	if ctx.Err() == nil {
		if mdns := h.scanner.DiscoverMDNS(ctx); len(mdns) > 0 {
			if err := h.store.MergeSupplemental(mdns); err == nil {
				j.addResult(networkScanSummary{Network: "mDNS", Status: "scanned", DeviceCount: len(mdns)}, len(mdns))
			}
		}
	}

	j.finish("done", "")
}

// beginNetwork records that the job has started scanning a network.
func (j *scanJob) beginNetwork(cidr string, index int, estimate float64) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.currentNetwork = cidr
	j.networkIndex = index
	j.networkStartedAt = time.Now()
	j.estimatedSeconds = estimate
}

// addResult records the outcome of one network.
func (j *scanJob) addResult(summary networkScanSummary, devices int) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.results = append(j.results, summary)
	j.deviceCount += devices
}

// finish marks the job as no longer running.
func (j *scanJob) finish(status, err string) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.status = status
	j.err = err
	j.currentNetwork = ""
}

// isRunning reports whether the job is still in progress.
func (j *scanJob) isRunning() bool {
	j.mu.RLock()
	defer j.mu.RUnlock()
	return j.status == "running"
}

// adopt marks a running job as user-initiated. When someone clicks Scan while
// the background scanner is mid-scan, the manual request attaches to that scan
// (rather than being rejected) and adopts it so the progress overlay follows it
// to completion instead of treating it as a silent automatic scan.
func (j *scanJob) adopt() {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.automatic = false
}

// StartBackgroundScanner re-scans the detected networks every interval so the
// device list stays current on its own, without anyone clicking Scan or keeping
// a browser open. It reuses the same job path, the one-scan-at-a-time guard and
// the per-network rate limiting as a manual scan, and stops when ctx is
// cancelled. A non-positive interval disables it. The ticker always runs so the
// user can switch continuous scanning on and off at runtime; runBackgroundScan
// skips the actual scan while the setting is off.
func (h *Handler) StartBackgroundScanner(ctx context.Context) {
	go func() {
		// Scan shortly after startup so a freshly started server does not sit
		// empty until the first interval elapses.
		startup := time.NewTimer(5 * time.Second)
		defer startup.Stop()
		select {
		case <-ctx.Done():
			return
		case <-startup.C:
		}
		h.runBackgroundScan()

		// The interval is read fresh before each wait, so a change from the
		// Settings page takes effect on the next cycle without a restart. A
		// per-cycle timer rather than a fixed ticker is what makes that possible.
		for {
			timer := time.NewTimer(h.backgroundScanInterval())
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
				h.runBackgroundScan()
			}
		}
	}()
}

// minBackgroundScanSeconds floors the automatic scan interval so a stray small
// override cannot spin the loop; the per-network rate limit already prevents any
// one network from being scanned more often than min_scan_interval.
const minBackgroundScanSeconds = 10

// backgroundScanInterval is how long to wait between automatic scans: the UI
// override if set, otherwise the configured interval, floored for safety.
func (h *Handler) backgroundScanInterval() time.Duration {
	secs := h.store.ScanIntervalOrDefault(h.cfg.Scanning.ScanInterval)
	if secs < minBackgroundScanSeconds {
		secs = minBackgroundScanSeconds
	}
	return time.Duration(secs) * time.Second
}

// runBackgroundScan starts one scan of all detected networks, unless a scan is
// already running. The job itself skips any network that was scanned too
// recently, so this never scans a network more often than the rate limit
// allows, however short the interval.
func (h *Handler) runBackgroundScan() {
	// The ticker always runs so the setting can be toggled at runtime; skip the
	// actual scan when continuous scanning is currently switched off.
	if !h.store.ContinuousScanEnabled(h.cfg.Scanning.ContinuousScan) {
		return
	}

	networks, err := h.resolveScanTargets("all")
	if err != nil || len(networks) == 0 {
		return
	}

	h.jobMu.Lock()
	defer h.jobMu.Unlock()

	if h.job != nil && h.job.isRunning() {
		return
	}
	h.job = h.startScanJob(networks, true)
}
