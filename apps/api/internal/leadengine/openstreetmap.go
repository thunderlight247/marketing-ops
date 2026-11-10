// openstreetmap.go adds OpenStreetMap as a second discovery source next to
// Google Places.
//
// Two reasons this exists:
//
//  1. Cost and quota. Google Places bills per request and caps the workspace's
//     daily searches, so a business with no card on file discovers nothing. OSM
//     is free, needs no key, and can be queried for the same categories.
//  2. Coverage. Small Ghanaian businesses that were never added to Google Maps
//     are often mapped in OSM by local contributors instead.
//
// What OSM does NOT have is ratings or review counts. DiscoveredPlace carries
// HasRating so the rest of the engine stores a NULL rating rather than a 0,
// which would read as "rated zero stars" instead of "not rated anywhere". The
// Google-specific rating filter is deliberately not applied here: the engine
// only targets businesses without a website, and applying "at least 4 stars" to
// a source that has no stars would reject every OSM result.
package leadengine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"marketingops/api/internal/store"
)

// DefaultOverpassEndpoint is the public Overpass API. Tests override
// overpassEndpoint to point at an httptest server.
const DefaultOverpassEndpoint = "https://overpass-api.de/api/interpreter"

var overpassEndpoint = DefaultOverpassEndpoint

// overpassClient bounds a discovery call. Overpass is a shared, donation-funded
// service: a short timeout keeps a slow query from stalling the discovery run.
var overpassClient = &http.Client{Timeout: 45 * time.Second}

// errOverpassBusy marks the responses Overpass uses to say "queue full, try
// again" rather than "your query is wrong".
var errOverpassBusy = errors.New("overpass busy")

// overpassBusyBackoff is how long to wait before the single retry of a busy
// request. Overpass asks callers to back off; one retry keeps discovery from
// failing a whole run on a momentary capacity spike without hammering a
// donation-funded service. Tests set it to 0.
var overpassBusyBackoff = 2 * time.Second

// ─── category → OSM tag mapping ───────────────────────────────────────────────

// osmCategoryFilters maps the free-text categories a workspace types in ("hair
// salons", "bakeries", "phone repair") onto the OSM tags those businesses are
// actually filed under. Order matters: the first keyword match wins, so
// specific phrases are listed before the general ones they contain.
var osmCategoryFilters = []struct {
	keywords []string
	filter   string // fragment appended after nwr["name"]
}{
	{[]string{"phone repair", "mobile repair", "phone accessories", "gadget"},
		`["shop"~"^(mobile_phone|electronics)$"]`},
	{[]string{"electronic", "computer", "phone", "laptop"},
		`["shop"~"^(electronics|computer|mobile_phone)$"]`},
	{[]string{"salon", "beauty", "spa", "cosmetic", "makeup", "make-up", "nails", "barber", "hair"},
		`["shop"~"^(beauty|hairdresser|nail_salon)$"]`},
	{[]string{"bakery", "bread", "pastry", "cake"},
		`["shop"~"^(bakery|pastry|confectionery)$"]`},
	{[]string{"restaurant", "eatery", "food", "cater", "chop bar", "chopbar"},
		`["amenity"~"^(restaurant|fast_food|food_court|cafe)$"]`},
	{[]string{"cafe", "coffee"},
		`["amenity"="cafe"]`},
	{[]string{"bar", "pub", "drink"},
		`["amenity"~"^(bar|pub)$"]`},
	{[]string{"hotel", "guest", "lodge", "hostel", "accommodation"},
		`["tourism"~"^(hotel|guest_house|hostel)$"]`},
	{[]string{"gym", "fitness", "workout"},
		`["leisure"="fitness_centre"]`},
	{[]string{"pharmac", "chemist", "drug store", "drugstore"},
		`["amenity"="pharmacy"]`},
	{[]string{"laundry", "dry clean", "dryclean"},
		`["shop"~"^(laundry|dry_cleaning)$"]`},
	{[]string{"tailor", "fashion", "cloth", "seam", "dressmak"},
		`["shop"~"^(clothes|tailor|fabric)$"]`},
	{[]string{"mechanic", "car repair", "auto", "garage", "spare part"},
		`["shop"~"^(car_repair|car_parts|tyres)$"]`},
	{[]string{"print", "copy", "stationer", "cyber"},
		`["shop"~"^(copyshop|stationery|printing)$"]`},
	{[]string{"supermarket", "grocery", "provision", "market"},
		`["shop"~"^(supermarket|convenience|greengrocer|general)$"]`},
	{[]string{"gift", "craft", "souvenir", "art"},
		`["shop"~"^(gift|art|craft)$"]`},
}

// osmTagFilterForCategory returns the Overpass filter for a category, falling
// back to a case-insensitive name match. The fallback keeps arbitrary
// workspace-typed categories useful ("catering services" still finds businesses
// that call themselves that) instead of silently discovering nothing.
func osmTagFilterForCategory(category string) string {
	lower := osmCategoryStem(category)
	for _, m := range osmCategoryFilters {
		for _, kw := range m.keywords {
			if strings.Contains(lower, kw) {
				return m.filter
			}
		}
	}
	name := regexp.QuoteMeta(osmSanitizeToken(lower))
	if name == "" {
		return ""
	}
	return fmt.Sprintf(`["name"~"%s",i]`, name)
}

// osmCategoryStem singularises each word so workspace-typed plurals match the
// keyword table: "bakeries" → "bakery", "pharmacies" → "pharmacy",
// "salons" → "salon". Without this, every category an operator is likely to
// type fell through to the weaker name-regex fallback.
func osmCategoryStem(category string) string {
	words := strings.Fields(strings.ToLower(strings.TrimSpace(category)))
	for i, w := range words {
		switch {
		case strings.HasSuffix(w, "ies") && len(w) > 4:
			words[i] = w[:len(w)-3] + "y"
		case strings.HasSuffix(w, "ss"), len(w) <= 3:
			// "glass", "spa": leave untouched.
		case strings.HasSuffix(w, "s"):
			words[i] = w[:len(w)-1]
		}
	}
	return strings.Join(words, " ")
}

// ─── query ────────────────────────────────────────────────────────────────────

// osmSanitizeToken strips characters that would change an Overpass query's
// meaning (quotes, brackets, backslashes, semicolons). City and category come
// from workspace configuration, so they must never be interpolated raw.
// Disallowed characters become spaces rather than being deleted, so that
// `Accra"];out;` degrades to the harmless token "Accra out" instead of the
// fused "Accraout" — deletion could otherwise splice two tokens into one that
// accidentally matches something.
func osmSanitizeToken(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == ' ', r == '-', r == '\'', r == '&', r == '/':
			b.WriteRune(r)
		default:
			b.WriteRune(' ')
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

// buildOverpassQuery builds the discovery query for one category+city pair.
//
// The city is resolved through its administrative area with a prefix match
// because OSM names city relations "Accra Metropolitan", "Kumasi Metropolitan"
// and so on — an exact match on "Accra" would silently return nothing.
//
// Businesses that already have a website are excluded at the query level, with
// the same rule the Google filter applies: an existing website means the owner
// does not need one built. A phone is required because the engine contacts
// people over WhatsApp, and OSM records carry no rating to filter on.
func buildOverpassQuery(category, city string) string {
	cityToken := osmSanitizeToken(city)
	filter := osmTagFilterForCategory(category)

	var b strings.Builder
	b.WriteString("[out:json][timeout:30];")
	if cityToken != "" {
		fmt.Fprintf(&b, `area["name"~"^%s",i]["boundary"="administrative"]->.a;`, cityToken)
	} else {
		// No city configured: anywhere in Ghana.
		b.WriteString(`area["ISO3166-1"="GH"][admin_level=2]->.a;`)
	}

	scope := "(area.a)"
	if filter == "" {
		// No usable category at all: any named, phone-numbered business.
		filter = ""
	}
	b.WriteString("(")
	fmt.Fprintf(&b, `nwr["name"]%s["phone"][!website][!contact:website]%s;`, filter, scope)
	fmt.Fprintf(&b, `nwr["name"]%s["contact:phone"][!website][!contact:website]%s;`, filter, scope)
	b.WriteString(");")
	b.WriteString("out body center 60;")
	return b.String()
}

// ─── HTTP ─────────────────────────────────────────────────────────────────────

type overpassResponse struct {
	Elements []struct {
		Type string            `json:"type"`
		ID   int64             `json:"id"`
		Tags map[string]string `json:"tags"`
	} `json:"elements"`
}

// ScrapeOpenStreetMap fetches and filters businesses from OpenStreetMap for one
// category+city pair. It needs no API key.
func ScrapeOpenStreetMap(ctx context.Context, category, city string) ([]DiscoveredPlace, error) {
	query := buildOverpassQuery(category, city)

	// Two attempts: the second only after a busy response. Observed in practice
	// as HTTP 502 with "this Overpass API server is currently at capacity" and
	// HTTP 429/504 on the same condition.
	var or *overpassResponse
	var err error
	for attempt := 0; attempt < 2; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(overpassBusyBackoff):
			}
		}
		or, err = queryOverpass(ctx, query)
		if err == nil {
			break
		}
		if !errors.Is(err, errOverpassBusy) {
			return nil, err
		}
	}
	if err != nil {
		return nil, fmt.Errorf("%s/%s: %w", category, city, err)
	}

	var out []DiscoveredPlace
	for _, el := range or.Elements {
		dp, ok := osmPlaceFromTags(el.Type, el.ID, el.Tags)
		if !ok {
			continue
		}
		if filterOSMPlace(dp) {
			out = append(out, dp)
		}
	}
	return out, nil
}

// queryOverpass performs one request and classifies the response.
func queryOverpass(ctx context.Context, query string) (*overpassResponse, error) {
	form := url.Values{"data": {query}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		overpassEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	// Overpass's front end rejects requests without a descriptive agent (406).
	req.Header.Set("User-Agent", "marketing-ops-lead-discovery/1.0")

	resp, err := overpassClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusTooManyRequests, http.StatusBadGateway, http.StatusGatewayTimeout:
		return nil, fmt.Errorf("%w (%d)", errOverpassBusy, resp.StatusCode)
	default:
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("overpass error %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var or overpassResponse
	if err := json.NewDecoder(resp.Body).Decode(&or); err != nil {
		return nil, fmt.Errorf("decode overpass response: %w", err)
	}
	return &or, nil
}

// osmPlaceFromTags converts one Overpass element into a DiscoveredPlace.
// ok=false when the element cannot become a contactable lead.
func osmPlaceFromTags(kind string, id int64, tags map[string]string) (DiscoveredPlace, bool) {
	name := strings.TrimSpace(tags["name"])
	if name == "" || id == 0 {
		return DiscoveredPlace{}, false
	}
	phone := normalizeGhanaPhone(firstNonEmpty(
		tags["phone"],
		tags["contact:phone"],
		tags["phone:mobile"],
		tags["contact:mobile"],
	))
	if phone == "" {
		// A postal-only or half-recorded entry: without a usable Ghana mobile
		// number there is nobody to message.
		return DiscoveredPlace{}, false
	}
	return DiscoveredPlace{
		// Namespaced so an OSM id can never collide with a Google place_id in
		// the same workspace's unique (workspace_id, place_id) index.
		PlaceID:    fmt.Sprintf("osm:%s/%d", kind, id),
		Name:       name,
		PhoneIntl:  phone,
		WebsiteURL: strings.TrimSpace(firstNonEmpty(tags["website"], tags["contact:website"])),
		Address:    osmAddress(tags),
		Category:   firstNonEmpty(tags["shop"], tags["amenity"], tags["tourism"], tags["leisure"]),
		Source:     "openstreetmap",
		// No rating and no reviews: OSM has neither. HasRating=false makes the
		// store write NULL instead of a zero that would be read as a real score.
		HasRating:   false,
		Rating:      0,
		ReviewCount: 0,
	}, true
}

// filterOSMPlace is the OSM counterpart of filterPlace. It intentionally has no
// rating or review threshold, because that data does not exist here.
func filterOSMPlace(p DiscoveredPlace) bool {
	if strings.TrimSpace(p.Name) == "" {
		return false
	}
	if strings.TrimSpace(p.WebsiteURL) != "" {
		return false // already has a website — not a prospect
	}
	return strings.TrimSpace(p.PhoneIntl) != ""
}

func osmAddress(tags map[string]string) string {
	if full := strings.TrimSpace(tags["addr:full"]); full != "" {
		return full
	}
	var parts []string
	if hn := strings.TrimSpace(tags["addr:housenumber"]); hn != "" {
		parts = append(parts, hn)
	}
	if st := strings.TrimSpace(tags["addr:street"]); st != "" {
		parts = append(parts, st)
	}
	if len(parts) > 0 {
		if area := firstNonEmpty(tags["addr:suburb"], tags["addr:city"], tags["addr:district"]); area != "" {
			parts = append(parts, area)
		}
		return strings.Join(parts, ", ")
	}
	return firstNonEmpty(tags["addr:place"], tags["addr:city"])
}

// ─── phone normalisation ──────────────────────────────────────────────────────

// ghanaMobilePrefixes are the two digits that follow 233 on a Ghanaian mobile
// number (020, 024, 054 …). Landlines (030 / 059-adjacent blocks) cannot
// receive WhatsApp, so a number outside this set is not a usable lead — queuing
// it only produces a failed send, which is what the pilot workspace's 52
// failures are made of.
var ghanaMobilePrefixes = map[string]bool{
	"20": true, "23": true, "24": true, "25": true, "26": true, "27": true,
	"28": true, "29": true, "50": true, "53": true, "54": true, "55": true,
	"56": true, "57": true, "58": true, "59": true,
}

// normalizeGhanaPhone converts an OSM phone value to the 233XXXXXXXXX form the
// dispatcher expects, or "" when the value is not a Ghanaian mobile number.
// OSM stores phone numbers as free text, often several separated by ';'.
func normalizeGhanaPhone(raw string) string {
	for _, part := range strings.FieldsFunc(raw, func(r rune) bool {
		return r == ';' || r == ',' || r == '/' || r == '|'
	}) {
		var digits strings.Builder
		for _, r := range part {
			if r >= '0' && r <= '9' {
				digits.WriteRune(r)
			}
		}
		d := digits.String()
		switch {
		case len(d) >= 13 && strings.HasPrefix(d, "2330"):
			// "+233 (0)24 123 4567" — the bracketed trunk 0 is not part of the
			// international number.
			d = "233" + d[4:13]
		case len(d) >= 12 && strings.HasPrefix(d, "233"):
			d = d[:12]
		case len(d) == 10 && d[0] == '0':
			d = "233" + d[1:]
		case len(d) == 9:
			d = "233" + d
		default:
			continue
		}
		if len(d) == 12 && ghanaMobilePrefixes[d[3:5]] {
			return d
		}
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if s := strings.TrimSpace(v); s != "" {
			return s
		}
	}
	return ""
}

// ─── engine integration ───────────────────────────────────────────────────────

// discoverFromOpenStreetMap runs one OSM category+city query and persists the
// new leads. It does not touch lead_discovery_searches: that table records the
// Google quota consumed and gates "already searched today", and an OSM row
// there would make the next Google pass skip a pair it never queried.
func discoverFromOpenStreetMap(ctx context.Context, workspaceID, category, city string, remaining int, s LeadEngineStore) (int, error) {
	places, err := ScrapeOpenStreetMap(ctx, category, city)
	if err != nil {
		return 0, err
	}

	saved := 0
	for _, pl := range places {
		if remaining > 0 && saved >= remaining {
			break
		}
		phone := formatPhone(pl)
		bl, _ := s.IsBlacklisted(ctx, workspaceID, phone)
		if bl {
			continue
		}
		_, err := s.SaveDiscoveredLead(ctx, store.DiscoveredLead{
			WorkspaceID: workspaceID,
			PlaceID:     pl.PlaceID,
			Source:      pl.Source,
			Name:        pl.Name,
			Phone:       phone,
			Category:    category,
			City:        city,
			Address:     pl.Address,
			Rating:      nil, // unknown, not zero
			ReviewCount: 0,
			WebsiteURL:  pl.WebsiteURL,
		})
		if err == nil {
			saved++
		}
	}
	// Log the counts even when they are zero: an unresolvable city area and a
	// city whose businesses are not mapped both produce no results, and the
	// operator needs a record to tell those apart.
	slog.Info("openstreetmap discovery",
		"workspace_id", workspaceID, "category", category, "city", city,
		"found", len(places), "saved", saved)
	return saved, nil
}
