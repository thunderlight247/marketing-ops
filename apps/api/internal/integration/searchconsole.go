package integration

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// SearchConsoleAdapter implements Adapter for Google Search Console
// (Milestone 10). Read-only by contract: the adapter reports search analytics
// only and never claims content, schedule, or lead-intake capabilities, so no
// content-plan tier special-casing applies here; its account summary is based
// purely on what the service account can read.
type SearchConsoleAdapter struct{}

// gscOpsConstructor builds the ops client used by Check. Package-level so
// tests can point it at a fake server instead of Google's endpoints.
var gscOpsConstructor = func(serviceJSON string) *GSCOps {
	return &GSCOps{ServiceAccountJSON: serviceJSON}
}

func (a *SearchConsoleAdapter) Kind() string {
	return "search_console"
}

func (a *SearchConsoleAdapter) DisplayName() string {
	return "Google Search Console"
}

func (a *SearchConsoleAdapter) Description() string {
	return "Authorized read-only search analytics: impressions, clicks, and position per property"
}

func (a *SearchConsoleAdapter) DefaultCapabilities() []CapabilityReport {
	return []CapabilityReport{
		{Capability: CapReadContent, Status: StatusUnsupported, Details: "Search Console exposes analytics, not content documents"},
		{Capability: CapCreateDraft, Status: StatusUnsupported, Details: "Search Console is read-only and does not create content"},
		{Capability: CapPublish, Status: StatusUnsupported, Details: "Search Console is read-only and does not publish content"},
		{Capability: CapUpdateContent, Status: StatusUnsupported, Details: "Search Console is read-only and does not update content"},
		{Capability: CapSchedule, Status: StatusUnsupported, Details: "Search Console does not schedule content"},
		{Capability: CapAnalytics, Status: StatusUnverified, Details: "Search analytics available once the service account is connected and the property is verified"},
		{Capability: CapLeadIntake, Status: StatusUnsupported, Details: "Search Console does not receive submissions"},
	}
}

// gscAnalyticsFailed builds the analytics row with the given brief detail.
func gscAnalyticsFailed(details string) CapabilityReport {
	return CapabilityReport{Capability: CapAnalytics, Status: StatusUnsupported, Details: details}
}

// gscPropertyLooksLikeURL accepts URL-prefix and domain (sc-domain:) notation.
func gscPropertyLooksLikeURL(property string) bool {
	if strings.HasPrefix(property, "sc-domain:") {
		return strings.TrimPrefix(property, "sc-domain:") != ""
	}
	u, err := url.Parse(property)
	return err == nil && (u.Scheme == "https" || u.Scheme == "http") && u.Host != ""
}

func (a *SearchConsoleAdapter) Check(ctx context.Context, config map[string]any, creds map[string]string) CheckResult {
	now := time.Now().UTC()

	property, _ := config["property"].(string)
	property = strings.TrimSpace(property)

	if property == "" {
		result := CheckResult{
			LifecycleStatus: LifecycleError,
			Diagnostic:      "Configuration error: property is required (for example https://example.com/ or sc-domain:example.com)",
			CheckedAt:       now,
		}
		base := a.DefaultCapabilities()
		base[5] = gscAnalyticsFailed("Missing Search Console property")
		result.Capabilities = base
		return result
	}

	if !gscPropertyLooksLikeURL(property) {
		result := CheckResult{
			LifecycleStatus: LifecycleError,
			Diagnostic:      "Configuration error: property must be a URL-prefix property (https://example.com/) or a domain property (sc-domain:example.com)",
			CheckedAt:       now,
		}
		result.Capabilities = a.DefaultCapabilities()
		return result
	}

	serviceJSON := strings.TrimSpace(creds["serviceAccount"])
	if serviceJSON == "" {
		result := CheckResult{
			LifecycleStatus: LifecycleLimited,
			Diagnostic:      fmt.Sprintf("Property %s is configured, but no service-account key has been added, so property access could not be verified.", property),
			CheckedAt:       now,
		}
		base := a.DefaultCapabilities()
		base[5] = gscAnalyticsFailed("Waiting for the service-account key")
		result.Capabilities = base
		return result
	}

	if _, keyErr := ParseServiceAccount(serviceJSON); keyErr != nil {
		result := CheckResult{
			LifecycleStatus: LifecycleError,
			Diagnostic:      fmt.Sprintf("Credential error: the service-account key could not be read"),
			CheckedAt:       now,
		}
		base := a.DefaultCapabilities()
		base[5] = gscAnalyticsFailed("The service-account key is not parseable")
		result.Capabilities = base
		return result
	}

	ops := gscOpsConstructor(serviceJSON)
	status, verifyErr := ops.VerifySite(ctx, now, property)
	if verifyErr != nil {
		var gerr *GSCError
		diag := "the Google API could not be reached"
		if errors.As(verifyErr, &gerr) {
			diag = gerr.SafeMessage()
		}
		result := CheckResult{
			LifecycleStatus: LifecycleError,
			Diagnostic:      fmt.Sprintf("Check failed against the Google API: %s.", diag),
			CheckedAt:       now,
		}
		base := a.DefaultCapabilities()
		base[5] = gscAnalyticsFailed(diag)
		result.Capabilities = base
		return result
	}

	analytics := gscAnalyticsFailed("the service account does not have access to the property yet")
	if status.Found {
		analytics = CapabilityReport{
			Capability: CapAnalytics,
			Status:     StatusSupported,
			Details:    fmt.Sprintf("Read access confirmed with %q permission", status.Level),
		}
	} else {
		analytics = CapabilityReport{
			Capability: CapAnalytics,
			Status:     StatusUnverified,
			Details:    "The property was not found in the service account's Search Console portal; grant the service account access to it first",
		}
	}

	diagnostic := fmt.Sprintf("Search Console check for property %s: the property was not found in the service account's portal.", property)
	lifecycle := LifecycleLimited
	if status.Found {
		lifecycle = LifecycleConnected
		diagnostic = fmt.Sprintf("Search Console read-only check passed for property %s with %q permission.", property, status.Level)
	}
	return CheckResult{
		LifecycleStatus: lifecycle,
		Diagnostic:      diagnostic,
		CheckedAt:       now,
		Capabilities: func() []CapabilityReport {
			base := a.DefaultCapabilities()
			base[5] = analytics
			return base
		}(),
	}
}
