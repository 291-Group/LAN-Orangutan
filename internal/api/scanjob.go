package api

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// scanJobTimeout caps how long a single network may be scanned for before it is
// abandoned. Large networks are slow, so this is deliberately generous.
const scanJobTimeout = 10 * time.Minute

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
// must hold h.jobMu.
func (h *Handler) startScanJob(networks []string) *scanJob {
	ctx, cancel := context.WithCancel(context.Background())

	job := &scanJob{
		id:        fmt.Sprintf("scan-%d", time.Now().UnixNano()),
		networks:  networks,
		cancel:    cancel,
		status:    "running",
		startedAt: time.Now(),
		results:   make([]networkScanSummary, 0, len(networks)),
	}

	go job.run(ctx, h)
	return job
}

// run scans each network in turn, recording the outcome of each. A network that
// is rate limited or fails does not abort the job, so one bad interface cannot
// hide results from the others.
func (j *scanJob) run(ctx context.Context, h *Handler) {
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

		scanCtx, cancel := context.WithTimeout(ctx, scanJobTimeout)
		result, err := h.scanNetwork(scanCtx, cidr)
		cancel()

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

		summary.Status = "scanned"
		summary.DeviceCount = result.DeviceCount
		summary.Duration = result.Duration
		j.addResult(summary, result.DeviceCount)
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
