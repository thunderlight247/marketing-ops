// Package leadengine contains the Google Places scraper, AI pitch generator,
// and the orchestration logic for the per-workspace lead outreach engine.
package leadengine

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// ─── types ────────────────────────────────────────────────────────────────────

// DiscoveredPlace is a single raw result from the Places API before filtering.
type DiscoveredPlace struct {
	PlaceID     string
	Name        string
	Rating      float64
	ReviewCount int
	WebsiteURL  string
	PhoneIntl   string // international format e.g. +233 24 123 4567
	PhoneLocal  string
	Address     string
}

// ─── filter criteria (matches gh-lead-engine/src/filter.js) ──────────────────

const (
	minRating      = 4.0
	minReviewCount = 5
)

func filterPlace(p DiscoveredPlace) bool {
	if p.Rating < minRating {
		return false
	}
	if p.ReviewCount < minReviewCount {
		return false
	}
	if p.WebsiteURL != "" {
		return false // already has a website — skip
	}
	phone := strings.TrimSpace(p.PhoneIntl)
	if phone == "" {
		phone = strings.TrimSpace(p.PhoneLocal)
	}
	if phone == "" {
		return false
	}
	return true
}

func formatPhone(p DiscoveredPlace) string {
	if strings.TrimSpace(p.PhoneIntl) != "" {
		// Strip spaces/dashes for WhatsApp.
		raw := strings.ReplaceAll(p.PhoneIntl, " ", "")
		raw = strings.ReplaceAll(raw, "-", "")
		return raw
	}
	raw := strings.ReplaceAll(p.PhoneLocal, " ", "")
	raw = strings.ReplaceAll(raw, "-", "")
	return raw
}

// ─── Google Places API client ─────────────────────────────────────────────────

type placesResponse struct {
	Places []struct {
		ID          string `json:"id"`
		DisplayName struct {
			Text string `json:"text"`
		} `json:"displayName"`
		Rating          float64 `json:"rating"`
		UserRatingCount int     `json:"userRatingCount"`
		WebsiteURI      string  `json:"websiteUri"`
		IntlPhone       string  `json:"internationalPhoneNumber"`
		NationalPhone   string  `json:"nationalPhoneNumber"`
		FormattedAddr   string  `json:"formattedAddress"`
	} `json:"places"`
}

// ScrapeCategoryInCity fetches and filters businesses from Google Places for
// one category+city pair. googleKey is the plaintext API key.
func ScrapeCategoryInCity(ctx context.Context, googleKey, category, city string) ([]DiscoveredPlace, error) {
	query := fmt.Sprintf("%s in %s, Ghana", category, city)
	reqBody := fmt.Sprintf(`{"textQuery":%q,"languageCode":"en","maxResultCount":20}`, query)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://places.googleapis.com/v1/places:searchText",
		strings.NewReader(reqBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Goog-Api-Key", googleKey)
	req.Header.Set("X-Goog-FieldMask", strings.Join([]string{
		"places.id",
		"places.displayName",
		"places.rating",
		"places.userRatingCount",
		"places.websiteUri",
		"places.internationalPhoneNumber",
		"places.nationalPhoneNumber",
		"places.formattedAddress",
	}, ","))
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == 429 {
		return nil, fmt.Errorf("google Places quota exhausted for %q", query)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("places API error %d: %s", resp.StatusCode, body)
	}

	var pr placesResponse
	if err := json.NewDecoder(resp.Body).Decode(&pr); err != nil {
		return nil, fmt.Errorf("decode places response: %w", err)
	}

	var out []DiscoveredPlace
	for _, p := range pr.Places {
		dp := DiscoveredPlace{
			PlaceID:     p.ID,
			Name:        p.DisplayName.Text,
			Rating:      p.Rating,
			ReviewCount: p.UserRatingCount,
			WebsiteURL:  p.WebsiteURI,
			PhoneIntl:   p.IntlPhone,
			PhoneLocal:  p.NationalPhone,
			Address:     p.FormattedAddr,
		}
		if filterPlace(dp) {
			out = append(out, dp)
		}
	}
	return out, nil
}
