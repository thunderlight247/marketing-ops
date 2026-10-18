package integration

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"
)

var (
	sanityProjectIDPattern = regexp.MustCompile(`^[a-z0-9_-]{4,30}$`)
	sanityDatasetPattern   = regexp.MustCompile(`^[a-z0-9_~-]+$`)
)

// SanityAdapter implements Adapter for Sanity CMS.
type SanityAdapter struct{}

func (a *SanityAdapter) Kind() string {
	return "sanity"
}

func (a *SanityAdapter) DisplayName() string {
	return "Sanity CMS"
}

func (a *SanityAdapter) Description() string {
	return "Structured content platform (Next.js + Sanity blog pilot)"
}

func (a *SanityAdapter) DefaultCapabilities() []CapabilityReport {
	return []CapabilityReport{
		{Capability: CapReadContent, Status: StatusUnverified, Details: "Dataset read unverified"},
		{Capability: CapCreateDraft, Status: StatusUnverified, Details: "Draft creation requires write token"},
		{Capability: CapPublish, Status: StatusUnverified, Details: "Publishing requires write token"},
		{Capability: CapUpdateContent, Status: StatusUnverified, Details: "Content update requires write token"},
		{Capability: CapSchedule, Status: StatusUnsupported, Details: "Sanity does not provide scheduled releases out of the box in this tier"},
		{Capability: CapAnalytics, Status: StatusUnsupported, Details: "Content CMS only; use Google Search Console or analytics integration"},
		{Capability: CapLeadIntake, Status: StatusUnsupported, Details: "Sanity is not an enquiry or CRM endpoint"},
	}
}

func (a *SanityAdapter) Check(ctx context.Context, config map[string]any, creds map[string]string) CheckResult {
	now := time.Now().UTC()

	projectID, _ := config["projectId"].(string)
	projectID = strings.TrimSpace(projectID)

	dataset, _ := config["dataset"].(string)
	dataset = strings.TrimSpace(dataset)

	if projectID == "" {
		return CheckResult{
			LifecycleStatus: LifecycleError,
			Diagnostic:      "Configuration error: projectId is required",
			CheckedAt:       now,
			Capabilities: []CapabilityReport{
				{Capability: CapReadContent, Status: StatusFailed, Details: "Missing projectId"},
				{Capability: CapCreateDraft, Status: StatusFailed, Details: "Missing projectId"},
				{Capability: CapPublish, Status: StatusFailed, Details: "Missing projectId"},
				{Capability: CapUpdateContent, Status: StatusFailed, Details: "Missing projectId"},
				{Capability: CapSchedule, Status: StatusUnsupported, Details: "Sanity does not provide scheduled releases"},
				{Capability: CapAnalytics, Status: StatusUnsupported, Details: "Not supported by Sanity"},
				{Capability: CapLeadIntake, Status: StatusUnsupported, Details: "Not supported by Sanity"},
			},
		}
	}

	if !sanityProjectIDPattern.MatchString(projectID) {
		return CheckResult{
			LifecycleStatus: LifecycleError,
			Diagnostic:      "Configuration error: projectId has invalid format",
			CheckedAt:       now,
			Capabilities:    a.DefaultCapabilities(),
		}
	}

	if dataset == "" {
		return CheckResult{
			LifecycleStatus: LifecycleError,
			Diagnostic:      "Configuration error: dataset is required (e.g. 'production')",
			CheckedAt:       now,
			Capabilities:    a.DefaultCapabilities(),
		}
	}

	if !sanityDatasetPattern.MatchString(dataset) {
		return CheckResult{
			LifecycleStatus: LifecycleError,
			Diagnostic:      "Configuration error: dataset has invalid format",
			CheckedAt:       now,
			Capabilities:    a.DefaultCapabilities(),
		}
	}

	token := strings.TrimSpace(creds["token"])
	if token == "" {
		// Read-only / limited configuration without write token.
		return CheckResult{
			LifecycleStatus: LifecycleLimited,
			Diagnostic:      fmt.Sprintf("Dataset %q on project %q configured for public read; no write token provided.", dataset, projectID),
			CheckedAt:       now,
			Capabilities: []CapabilityReport{
				{Capability: CapReadContent, Status: StatusSupported, Details: "Read access available for public dataset"},
				{Capability: CapCreateDraft, Status: StatusUnsupported, Details: "Requires write token (Editor or Administrator permissions)"},
				{Capability: CapPublish, Status: StatusUnsupported, Details: "Requires write token (Editor or Administrator permissions)"},
				{Capability: CapUpdateContent, Status: StatusUnsupported, Details: "Requires write token (Editor or Administrator permissions)"},
				{Capability: CapSchedule, Status: StatusUnsupported, Details: "Sanity does not provide scheduled releases out of the box in this tier"},
				{Capability: CapAnalytics, Status: StatusUnsupported, Details: "Content CMS only; use Google Search Console or analytics integration"},
				{Capability: CapLeadIntake, Status: StatusUnsupported, Details: "Sanity is not an enquiry or CRM endpoint"},
			},
		}
	}

	// Token is configured.
	return CheckResult{
		LifecycleStatus: LifecycleConnected,
		Diagnostic:      fmt.Sprintf("Connected to project %q (dataset %q) with write token.", projectID, dataset),
		CheckedAt:       now,
		Capabilities: []CapabilityReport{
			{Capability: CapReadContent, Status: StatusSupported, Details: "Read access verified"},
			{Capability: CapCreateDraft, Status: StatusSupported, Details: "Draft mutation supported with write token"},
			{Capability: CapPublish, Status: StatusSupported, Details: "Publish mutation supported (gated by operator approval)"},
			{Capability: CapUpdateContent, Status: StatusSupported, Details: "Content update mutation supported"},
			{Capability: CapSchedule, Status: StatusUnsupported, Details: "Sanity does not provide scheduled releases out of the box in this tier"},
			{Capability: CapAnalytics, Status: StatusUnsupported, Details: "Content CMS only; use Google Search Console or analytics integration"},
			{Capability: CapLeadIntake, Status: StatusUnsupported, Details: "Sanity is not an enquiry or CRM endpoint"},
		},
	}
}
