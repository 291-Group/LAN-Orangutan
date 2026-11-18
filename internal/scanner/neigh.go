package scanner

import (
	"context"
	"os/exec"
	"strings"
	"time"
)

// neighCommandTimeout bounds a neighbour-table read (`ip neigh`, `arp`,
// `ndp`) and a solicitation ping. The tables are held in kernel memory and
// answer immediately, so anything slower than this is a stuck command rather
// than a slow one.
const neighCommandTimeout = 3 * time.Second

func runNeighCommand(ctx context.Context, name string, args ...string) ([]byte, error) {
	c, cancel := context.WithTimeout(ctx, neighCommandTimeout)
	defer cancel()
	return exec.CommandContext(c, name, args...).Output()
}

// parseIPNeighLine splits one line of Linux `ip neigh show` into its address,
// link-layer address and state. A line reads
// "192.168.1.5 dev eth0 lladdr 00:11:22:33:44:55 REACHABLE", with optional
// flags such as "router" before the state. ok is false for a blank or
// malformed line; mac is empty when the kernel never resolved the entry.
func parseIPNeighLine(line string) (ip, mac, state string, ok bool) {
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return "", "", "", false
	}
	for i, f := range fields {
		if f == "lladdr" && i+1 < len(fields) {
			mac = fields[i+1]
			break
		}
	}
	return fields[0], mac, fields[len(fields)-1], true
}

// neighStatePresent reports whether a Linux neighbour state is evidence that
// the device is on the network right now.
//
// Only REACHABLE, DELAY and PROBE qualify. Each means the kernel has had a
// confirmed exchange with the address within the last few seconds, or is in
// the middle of re-confirming one. STALE is the opposite: it means nothing
// has been heard from the address since its last confirmation, and Linux keeps
// such entries indefinitely while the table is below gc_thresh1, which it
// always is on a home network. A powered-off device therefore sits in the
// table as STALE for days, and taking that as presence would re-stamp it as
// seen on every scan and keep it online forever. FAILED and INCOMPLETE mean
// the kernel asked and never got an answer, and PERMANENT and NOARP are
// static entries that say nothing about whether anything is there.
func neighStatePresent(state string) bool {
	switch state {
	case "REACHABLE", "DELAY", "PROBE":
		return true
	}
	return false
}
