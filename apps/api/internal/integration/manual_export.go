package integration

import (
	"context"
	"time"
)

// ManualExportAdapter implements Adapter for manual publishing workflows.
type ManualExportAdapter struct{}

func (a *ManualExportAdapter) Kind() string {
	return "manual_export"
}

func (a *ManualExportAdapter) DisplayName() string {
	return "Manual / Export Checklist"
}

func (a *ManualExportAdapter) Description() string {
	return "Checklist and formatted content export for platforms without automated API publishing"
}

func (a *ManualExportAdapter) DefaultCapabilities() []CapabilityReport {
	return []CapabilityReport{
		{Capability: CapReadContent, Status: StatusUnsupported, Details: "Manual workflow has no automated read API"},
		{Capability: CapCreateDraft, Status: StatusSupported, Details: "Platform prepares drafts for manual copying"},
		{Capability: CapPublish, Status: StatusSupported, Details: "Operator manually publishes using step-by-step checklist"},
		{Capability: CapUpdateContent, Status: StatusSupported, Details: "Operator manually applies content updates"},
		{Capability: CapSchedule, Status: StatusUnsupported, Details: "Automated scheduling is not available for manual workflow"},
		{Capability: CapAnalytics, Status: StatusUnsupported, Details: "Manual analytics entry only"},
		{Capability: CapLeadIntake, Status: StatusUnsupported, Details: "No automated lead intake"},
	}
}

func (a *ManualExportAdapter) Check(ctx context.Context, config map[string]any, creds map[string]string) CheckResult {
	now := time.Now().UTC()
	return CheckResult{
		LifecycleStatus: LifecycleConnected,
		Diagnostic:      "Manual export workflow is ready. No credentials required.",
		CheckedAt:       now,
		Capabilities:    a.DefaultCapabilities(),
	}
}
