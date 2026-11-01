package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

var (
	sanityProjectIDPattern = regexp.MustCompile(`^[a-z0-9_-]{4,30}$`)
	sanityDatasetPattern   = regexp.MustCompile(`^[a-z0-9_~-]+$`)
)

// SanityAdapter verifies dataset access without making any external writes.
// BaseURL and Client are injectable for controlled tests, never workspace config.
type SanityAdapter struct {
	BaseURL string
	Client  *http.Client
}

func (a *SanityAdapter) Kind() string        { return "sanity" }
func (a *SanityAdapter) DisplayName() string { return "Sanity CMS" }
func (a *SanityAdapter) Description() string {
	return "Structured content platform (Next.js + Sanity blog pilot)"
}

func (a *SanityAdapter) DefaultCapabilities() []CapabilityReport {
	return []CapabilityReport{
		{Capability: CapReadContent, Status: StatusUnverified, Details: "Dataset read access has not been verified"},
		{Capability: CapCreateDraft, Status: StatusUnverified, Details: "Write permission has not been verified"},
		{Capability: CapPublish, Status: StatusUnverified, Details: "Write permission has not been verified"},
		{Capability: CapUpdateContent, Status: StatusUnverified, Details: "Write permission has not been verified"},
		{Capability: CapSchedule, Status: StatusUnsupported, Details: "Scheduling uses the platform worker, not Sanity scheduled releases"},
		{Capability: CapAnalytics, Status: StatusUnsupported, Details: "Content CMS only; use a separate analytics integration"},
		{Capability: CapLeadIntake, Status: StatusUnsupported, Details: "Sanity is not an enquiry or CRM endpoint"},
	}
}

func (a *SanityAdapter) Check(ctx context.Context, config map[string]any, creds map[string]string) CheckResult {
	result := CheckResult{LifecycleStatus: LifecycleError, CheckedAt: time.Now().UTC(), Capabilities: a.DefaultCapabilities()}
	projectID, _ := config["projectId"].(string)
	projectID = strings.TrimSpace(projectID)
	dataset, _ := config["dataset"].(string)
	dataset = strings.TrimSpace(dataset)
	switch {
	case projectID == "":
		result.Diagnostic = "Configuration error: projectId is required"
		return result
	case !sanityProjectIDPattern.MatchString(projectID):
		result.Diagnostic = "Configuration error: projectId has invalid format"
		return result
	case dataset == "":
		result.Diagnostic = "Configuration error: dataset is required"
		return result
	case !sanityDatasetPattern.MatchString(dataset):
		result.Diagnostic = "Configuration error: dataset has invalid format"
		return result
	}
	token := strings.TrimSpace(creds["token"])
	if token == "" {
		result.LifecycleStatus = LifecycleLimited
		result.Diagnostic = "Add a project token to verify dataset access; access has not been checked."
		return result
	}
	base := a.BaseURL
	if base == "" {
		base = "https://" + projectID + ".api.sanity.io"
	}
	u, err := url.Parse(strings.TrimRight(base, "/") + "/v2024-10-14/data/query/" + url.PathEscape(dataset))
	if err != nil {
		result.Diagnostic = "Sanity connection configuration is invalid."
		return result
	}
	q := u.Query()
	q.Set("query", "count(*[_id in path('drafts.**')])")
	q.Set("perspective", "raw")
	u.RawQuery = q.Encode()
	checkCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(checkCtx, http.MethodGet, u.String(), nil)
	if err != nil {
		result.Diagnostic = "Sanity connection configuration is invalid."
		return result
	}
	req.Header.Set("Authorization", "Bearer "+token)
	client := a.Client
	if client == nil {
		client = http.DefaultClient
	}
	// Never forward credentials to a redirected host.
	copyClient := *client
	copyClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := copyClient.Do(req)
	if err != nil {
		result.Diagnostic = "Could not reach Sanity; check the network and try again."
		return result
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		result.Diagnostic = fmt.Sprintf("Sanity refused the dataset check (HTTP %d); verify the project, dataset and token permissions.", resp.StatusCode)
		return result
	}
	var body struct {
		Result *int `json:"result"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 65536)).Decode(&body); err != nil || body.Result == nil {
		result.Diagnostic = "Sanity returned an unexpected response; dataset access could not be verified."
		return result
	}
	result.LifecycleStatus = LifecycleConnected
	result.Diagnostic = "Sanity accepted the token and dataset query. Write permissions and the website schema are not verified by this read-only check."
	result.Capabilities[0] = CapabilityReport{Capability: CapReadContent, Status: StatusSupported, Details: "Authenticated dataset query succeeded"}
	return result
}
