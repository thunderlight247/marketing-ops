package leadengine

import (
	"sync"
	"time"
)

// DispatchReport is the outcome of the most recent cold-outreach run for one
// workspace. Skipped holds the reason each lead was not sent in that run, keyed
// by lead ID, so the dashboard can explain silent-looking skips: contacts that
// were already messaged, opt-outs, and safety checks that could not be
// answered. It lives in memory because it describes this process's last run —
// it is not outreach history (that stays in lead_outreach_logs).
type DispatchReport struct {
	WorkspaceID string
	StartedAt   time.Time
	FinishedAt  time.Time
	Sent        int
	Failed      int
	// Unconfirmed counts failed sends whose outcome is unknown (timeout, 5xx,
	// transport error). They are never retried automatically, so the operator
	// has to check the contact; provable refusals are not included.
	Unconfirmed int
	Skips       map[string]string
	Halted      string
}

var dispatchReports = struct {
	sync.RWMutex
	byWorkspace map[string]DispatchReport
}{byWorkspace: map[string]DispatchReport{}}

func recordDispatchReport(r DispatchReport) {
	skips := make(map[string]string, len(r.Skips))
	for id, reason := range r.Skips {
		skips[id] = reason
	}
	r.Skips = skips
	dispatchReports.Lock()
	defer dispatchReports.Unlock()
	dispatchReports.byWorkspace[r.WorkspaceID] = r
}

// LastDispatchReport returns a copy of the workspace's most recent dispatch
// report, if this process has run one.
func LastDispatchReport(workspaceID string) (DispatchReport, bool) {
	dispatchReports.RLock()
	defer dispatchReports.RUnlock()
	r, ok := dispatchReports.byWorkspace[workspaceID]
	if !ok {
		return DispatchReport{}, false
	}
	skips := make(map[string]string, len(r.Skips))
	for id, reason := range r.Skips {
		skips[id] = reason
	}
	r.Skips = skips
	return r, true
}
