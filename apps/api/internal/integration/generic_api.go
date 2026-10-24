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
		{Capability: CapReadContent, Status: StatusUnverified, Details: "Custom API read contract is not configured or verified"},
		{Capability: CapCreateDraft, Status: StatusUnsupported, Details: "No custom draft endpoint contract has been implemented"},
		{Capability: CapPublish, Status: StatusUnsupported, Details: "No custom publish endpoint contract has been implemented"},
		{Capability: CapUpdateContent, Status: StatusUnsupported, Details: "No custom update endpoint contract has been implemented"},
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
			Diagnostic:      fmt.Sprintf("API endpoint %s is configured without authorization; no custom endpoint contract has been verified.", u.Host),
			CheckedAt:       now,
			Capabilities: []CapabilityReport{
				{Capability: CapReadContent, Status: StatusUnverified, Details: "Custom API read contract is not configured or verified"},
				{Capability: CapCreateDraft, Status: StatusUnsupported, Details: "No custom draft endpoint contract has been implemented"},
				{Capability: CapPublish, Status: StatusUnsupported, Details: "No custom publish endpoint contract has been implemented"},
				{Capability: CapUpdateContent, Status: StatusUnsupported, Details: "No custom update endpoint contract has been implemented"},
				{Capability: CapSchedule, Status: StatusUnsupported, Details: "Server-side scheduling not supported"},
				{Capability: CapAnalytics, Status: StatusUnsupported, Details: "Not supported"},
				{Capability: CapLeadIntake, Status: StatusUnsupported, Details: "Not supported"},
			},
		}
	}

	return CheckResult{
		LifecycleStatus: LifecycleLimited,
		Diagnostic:      fmt.Sprintf("API endpoint %s and authorization are configured, but the custom request contract has not been verified.", u.Host),
		CheckedAt:       now,
		Capabilities: []CapabilityReport{
			{Capability: CapReadContent, Status: StatusUnverified, Details: "Custom API read contract is not configured or verified"},
			{Capability: CapCreateDraft, Status: StatusUnsupported, Details: "No custom draft endpoint contract has been implemented"},
			{Capability: CapPublish, Status: StatusUnsupported, Details: "No custom publish endpoint contract has been implemented"},
			{Capability: CapUpdateContent, Status: StatusUnsupported, Details: "No custom update endpoint contract has been implemented"},
			{Capability: CapSchedule, Status: StatusUnsupported, Details: "Server-side scheduling not supported"},
			{Capability: CapAnalytics, Status: StatusUnsupported, Details: "Not supported"},
			{Capability: CapLeadIntake, Status: StatusUnsupported, Details: "Not supported"},
		},
	}
}
