package integration

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// GenericAPIAdapter implements Adapter for a client's custom HTTP REST content API.
type GenericAPIAdapter struct{}

func (a *GenericAPIAdapter) Kind() string {
	return "generic_api"
}

func (a *GenericAPIAdapter) DisplayName() string {
	return "Custom REST API"
}

func (a *GenericAPIAdapter) Description() string {
	return "Client-provided HTTP API with token or key authorization"
}

func (a *GenericAPIAdapter) DefaultCapabilities() []CapabilityReport {
	return []CapabilityReport{
		{Capability: CapReadContent, Status: StatusUnverified, Details: "Endpoint read unverified"},
		{Capability: CapCreateDraft, Status: StatusUnverified, Details: "Endpoint draft mutation unverified"},
		{Capability: CapPublish, Status: StatusUnverified, Details: "Endpoint publish mutation unverified"},
		{Capability: CapUpdateContent, Status: StatusUnverified, Details: "Endpoint update mutation unverified"},
		{Capability: CapSchedule, Status: StatusUnsupported, Details: "Server-side scheduling not supported by standard REST API"},
		{Capability: CapAnalytics, Status: StatusUnsupported, Details: "Custom API does not report analytics"},
		{Capability: CapLeadIntake, Status: StatusUnsupported, Details: "Custom API does not handle lead intake"},
	}
}

func (a *GenericAPIAdapter) Check(ctx context.Context, config map[string]any, creds map[string]string) CheckResult {
	now := time.Now().UTC()

	rawURL, _ := config["baseUrl"].(string)
	rawURL = strings.TrimSpace(rawURL)

	if rawURL == "" {
		return CheckResult{
			LifecycleStatus: LifecycleError,
			Diagnostic:      "Configuration error: baseUrl is required",
			CheckedAt:       now,
			Capabilities: []CapabilityReport{
				{Capability: CapReadContent, Status: StatusFailed, Details: "Missing baseUrl"},
				{Capability: CapCreateDraft, Status: StatusFailed, Details: "Missing baseUrl"},
				{Capability: CapPublish, Status: StatusFailed, Details: "Missing baseUrl"},
				{Capability: CapUpdateContent, Status: StatusFailed, Details: "Missing baseUrl"},
				{Capability: CapSchedule, Status: StatusUnsupported, Details: "Not supported"},
				{Capability: CapAnalytics, Status: StatusUnsupported, Details: "Not supported"},
				{Capability: CapLeadIntake, Status: StatusUnsupported, Details: "Not supported"},
			},
		}
	}

	u, err := url.Parse(rawURL)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return CheckResult{
			LifecycleStatus: LifecycleError,
			Diagnostic:      "Configuration error: baseUrl must be a valid HTTP or HTTPS URL",
			CheckedAt:       now,
			Capabilities:    a.DefaultCapabilities(),
		}
	}

	apiKey := strings.TrimSpace(creds["apiKey"])
	token := strings.TrimSpace(creds["token"])
	hasAuth := apiKey != "" || token != ""

	if !hasAuth {
		return CheckResult{
			LifecycleStatus: LifecycleLimited,
			Diagnostic:      fmt.Sprintf("API endpoint configured at %s without authorization credentials; mutations disabled.", u.Host),
			CheckedAt:       now,
			Capabilities: []CapabilityReport{
				{Capability: CapReadContent, Status: StatusSupported, Details: "Public endpoint read configured"},
				{Capability: CapCreateDraft, Status: StatusUnsupported, Details: "Requires authorization credentials (apiKey or token)"},
				{Capability: CapPublish, Status: StatusUnsupported, Details: "Requires authorization credentials (apiKey or token)"},
				{Capability: CapUpdateContent, Status: StatusUnsupported, Details: "Requires authorization credentials (apiKey or token)"},
				{Capability: CapSchedule, Status: StatusUnsupported, Details: "Server-side scheduling not supported"},
				{Capability: CapAnalytics, Status: StatusUnsupported, Details: "Not supported"},
				{Capability: CapLeadIntake, Status: StatusUnsupported, Details: "Not supported"},
			},
		}
	}

	return CheckResult{
		LifecycleStatus: LifecycleConnected,
		Diagnostic:      fmt.Sprintf("Connected to API endpoint %s with authorization credentials.", u.Host),
		CheckedAt:       now,
		Capabilities: []CapabilityReport{
			{Capability: CapReadContent, Status: StatusSupported, Details: "API read endpoint configured"},
			{Capability: CapCreateDraft, Status: StatusSupported, Details: "API draft creation supported"},
			{Capability: CapPublish, Status: StatusSupported, Details: "API publish mutation supported (gated by operator approval)"},
			{Capability: CapUpdateContent, Status: StatusSupported, Details: "API update mutation supported"},
			{Capability: CapSchedule, Status: StatusUnsupported, Details: "Server-side scheduling not supported"},
			{Capability: CapAnalytics, Status: StatusUnsupported, Details: "Not supported"},
			{Capability: CapLeadIntake, Status: StatusUnsupported, Details: "Not supported"},
		},
	}
}
