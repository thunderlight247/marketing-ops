// Package integration defines the adapter interface, registry, and read-only
// capability verification logic for external content and marketing platforms.
package integration

import (
	"context"
	"time"
)

// Capability constants match the CHECK constraint in migration 0002_domain.sql.
const (
	CapReadContent   = "read_content"
	CapCreateDraft   = "create_draft"
	CapPublish       = "publish"
	CapUpdateContent = "update_content"
	CapSchedule      = "schedule"
	CapAnalytics     = "analytics"
	CapLeadIntake    = "lead_intake"
)

// AllCapabilities lists all 7 capabilities in canonical order.
var AllCapabilities = []string{
	CapReadContent,
	CapCreateDraft,
	CapPublish,
	CapUpdateContent,
	CapSchedule,
	CapAnalytics,
	CapLeadIntake,
}

// Capability status constants match 0002_domain.sql.
const (
	StatusSupported   = "supported"
	StatusUnsupported = "unsupported"
	StatusUnverified  = "unverified"
	StatusFailed      = "failed"
)

// Lifecycle status constants match 0002_domain.sql.
const (
	LifecycleUnconfigured = "unconfigured"
	LifecycleConnected    = "connected"
	LifecycleLimited      = "limited"
	LifecycleError        = "error"
	LifecycleDisconnected = "disconnected"
)

// CapabilityReport holds the verification outcome for a single capability.
type CapabilityReport struct {
	Capability string `json:"capability"`
	Status     string `json:"status"`
	Details    string `json:"details"`
}

// CheckResult is the outcome of a read-only integration health and capability check.
type CheckResult struct {
	LifecycleStatus string             `json:"lifecycleStatus"`
	Capabilities    []CapabilityReport `json:"capabilities"`
	Diagnostic      string             `json:"diagnostic"`
	CheckedAt       time.Time          `json:"checkedAt"`
}

// AdapterDescriptor provides metadata for displaying available adapters in the UI.
type AdapterDescriptor struct {
	Kind        string `json:"kind"`
	DisplayName string `json:"displayName"`
	Description string `json:"description"`
}

// Adapter is implemented by each supported platform or publishing workflow.
type Adapter interface {
	Kind() string
	DisplayName() string
	Description() string

	// DefaultCapabilities returns the baseline capabilities before any check is run.
	DefaultCapabilities() []CapabilityReport

	// Check performs a strictly read-only verification against configuration
	// and decrypted credentials (if present). It must never modify, create,
	// or delete external resources, and must sanitize diagnostic messages to
	// ensure no tokens or secrets are exposed.
	Check(ctx context.Context, config map[string]any, creds map[string]string) CheckResult
}
