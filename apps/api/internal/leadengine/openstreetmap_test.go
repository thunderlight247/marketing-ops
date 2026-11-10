package leadengine

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"marketingops/api/internal/store"
)

func TestBuildOverpassQuery(t *testing.T) {
	q := buildOverpassQuery("bakeries", "Accra")

	// The city is matched as an administrative-area prefix: OSM calls it
	// "Accra Metropolitan", so an exact match would return nothing.
	if !strings.Contains(q, `area["name"~"^Accra",i]["boundary"="administrative"]`) {
		t.Errorf("query does not scope to Accra: %s", q)
	}
	if !strings.Contains(q, `["shop"~"^(bakery|pastry|confectionery)$"]`) {
		t.Errorf("query does not map 'bakeries' to OSM shop tags: %s", q)
	}
	// Businesses that already have a website are not prospects.
	if strings.Count(q, `[!website][!contact:website]`) != 2 {
		t.Errorf("query should exclude businesses with a website in both clauses: %s", q)
	}
	// A phone is required: the lead engine contacts people over WhatsApp.
	if strings.Count(q, `["phone"]`) != 1 || strings.Count(q, `["contact:phone"]`) != 1 {
		t.Errorf("query should require a phone in both clauses: %s", q)
	}
}

func TestBuildOverpassQuerySanitisesInput(t *testing.T) {
	q := buildOverpassQuery(`salon"];node(1);out;`, `Accra"];out;`)

	if strings.Contains(q, `"];`) {
		t.Fatalf("query kept injected Overpass statements: %s", q)
	}
	// The injected text is neutralised into a single harmless area name, and
	// the statement structure stays exactly what the builder wrote.
	if !strings.Contains(q, `area["name"~"^Accra out",i]`) {
		t.Errorf("city was not degraded to a safe token: %s", q)
	}
	if strings.Count(q, ";") != 6 {
		t.Errorf("unexpected statement count after sanitising: %s", q)
	}
	if !strings.Contains(q, `area.a`) {
		t.Errorf("query lost its area scope: %s", q)
	}
}

func TestBuildOverpassQueryFallsBackToNameSearch(t *testing.T) {
	q := buildOverpassQuery("tutoring", "Kumasi")
	if !strings.Contains(q, `["name"~"tutoring",i]`) {
		t.Errorf("unmapped category should fall back to a name match: %s", q)
	}
}

func TestOSMCategoryStemMatchesPlurals(t *testing.T) {
	cases := map[string]string{
		"bakeries":    `["shop"~"^(bakery|pastry|confectionery)$"]`,
		"pharmacies":  `["amenity"="pharmacy"]`,
		"hair salons": `["shop"~"^(beauty|hairdresser|nail_salon)$"]`,
		"gyms":        `["leisure"="fitness_centre"]`,
		"Restaurants": `["amenity"~"^(restaurant|fast_food|food_court|cafe)$"]`,
	}
	for category, want := range cases {
		if got := osmTagFilterForCategory(category); got != want {
			t.Errorf("osmTagFilterForCategory(%q) = %q, want %q", category, got, want)
		}
	}
}

func TestBuildOverpassQueryWithoutCityStaysInGhana(t *testing.T) {
	q := buildOverpassQuery("pharmacies", "")
	if !strings.Contains(q, `["ISO3166-1"="GH"]`) {
		t.Errorf("a missing city must not widen the search beyond Ghana: %s", q)
	}
}

func TestOSMPlaceFromTags(t *testing.T) {
	p, ok := osmPlaceFromTags("node", 12345, map[string]string{
		"name":             "Adjoa's Bakery",
		"shop":             "bakery",
		"phone":            "+233 24 123 4567",
		"addr:housenumber": "12",
		"addr:street":      "Oxford Street",
		"addr:suburb":      "Osu",
	})
	if !ok {
		t.Fatal("a named bakery with a mobile number should become a lead")
	}
	if p.PlaceID != "osm:node/12345" {
		t.Errorf("PlaceID = %q, want namespaced osm:node/12345", p.PlaceID)
	}
	if p.Source != "openstreetmap" {
		t.Errorf("Source = %q, want openstreetmap", p.Source)
	}
	if p.PhoneIntl != "233241234567" {
		t.Errorf("PhoneIntl = %q, want 233241234567", p.PhoneIntl)
	}
	if p.HasRating {
		t.Error("OpenStreetMap has no ratings; HasRating must be false so the rating is stored NULL")
	}
	if p.Address != "12, Oxford Street, Osu" {
		t.Errorf("Address = %q", p.Address)
	}
}

func TestOSMPlaceFromTagsRejectsUncontactableEntries(t *testing.T) {
	cases := map[string]map[string]string{
		"no name":     {"phone": "+233241234567"},
		"no phone":    {"name": "Anonymous Shop", "shop": "bakery"},
		"landline":    {"name": "Shop", "phone": "+233 30 274 0000"},
		"no digits":   {"name": "Shop", "phone": "call us"},
		"empty phone": {"name": "Shop", "phone": ""},
	}
	for name, tags := range cases {
		if _, ok := osmPlaceFromTags("node", 7, tags); ok {
			t.Errorf("%s: should not become a lead", name)
		}
	}
}

func TestNormalizeGhanaPhone(t *testing.T) {
	cases := []struct{ in, want string }{
		{"+233 24 123 4567", "233241234567"},
		{"0241234567", "233241234567"},
		{"233241234567", "233241234567"},
		{"+233 (0)24 123 4567", "233241234567"},
		{"024 123 4567; 030 274 0000", "233241234567"}, // landline skipped, mobile used
		{"030 274 0000", ""},                           // landline: cannot receive WhatsApp
		{"12345", ""},
		{"", ""},
	}
	for _, c := range cases {
		if got := normalizeGhanaPhone(c.in); got != c.want {
			t.Errorf("normalizeGhanaPhone(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestFilterOSMPlace(t *testing.T) {
	base := DiscoveredPlace{Name: "Kofi Electronics", PhoneIntl: "233241234567"}

	if !filterOSMPlace(base) {
		t.Error("a named business with a mobile number should pass")
	}
	withSite := base
	withSite.WebsiteURL = "https://kofi.example"
	if filterOSMPlace(withSite) {
		t.Error("a business that already has a website is not a prospect")
	}
	noPhone := base
	noPhone.PhoneIntl = ""
	if filterOSMPlace(noPhone) {
		t.Error("a business without a phone cannot be messaged")
	}
}

func TestScrapeOpenStreetMapParsesAndFilters(t *testing.T) {
	body := `{"elements":[
		{"type":"node","id":1,"tags":{"name":"Adjoa's Bakery","shop":"bakery","phone":"0241234567"}},
		{"type":"way","id":2,"tags":{"name":"Website Bakery","shop":"bakery","phone":"0241234568","website":"https://x.example"}},
		{"type":"node","id":3,"tags":{"name":"Landline Bakery","shop":"bakery","phone":"0302740000"}},
		{"type":"node","id":4,"tags":{"name":"Contact Tag Bakery","shop":"bakery","contact:phone":"0551234567"}}
	]}`

	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Errorf("parse form: %v", err)
		}
		gotQuery = r.PostFormValue("data")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	prev := overpassEndpoint
	overpassEndpoint = srv.URL
	defer func() { overpassEndpoint = prev }()

	places, err := ScrapeOpenStreetMap(context.Background(), "bakeries", "Accra")
	if err != nil {
		t.Fatalf("ScrapeOpenStreetMap: %v", err)
	}
	if !strings.Contains(gotQuery, "^Accra") {
		t.Errorf("server did not receive the Accra query: %q", gotQuery)
	}

	// Two of the four are usable: the plain mobile and the contact:phone one.
	if len(places) != 2 {
		var names []string
		for _, p := range places {
			names = append(names, p.Name)
		}
		t.Fatalf("got %d places (%v), want 2", len(places), names)
	}
	if places[0].Name != "Adjoa's Bakery" || places[1].Name != "Contact Tag Bakery" {
		t.Errorf("unexpected places kept: %+v", places)
	}
	if places[0].HasRating {
		t.Error("OSM results must not claim a rating")
	}
}

func TestScrapeOpenStreetMapReportsBusyServer(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte("Too Many Requests"))
	}))
	defer srv.Close()

	prevEndpoint, prevBackoff := overpassEndpoint, overpassBusyBackoff
	overpassEndpoint, overpassBusyBackoff = srv.URL, 0
	defer func() { overpassEndpoint, overpassBusyBackoff = prevEndpoint, prevBackoff }()

	_, err := ScrapeOpenStreetMap(context.Background(), "bakeries", "Accra")
	if err == nil {
		t.Fatal("a 429 from Overpass must surface as an error, not as zero results")
	}
	if !strings.Contains(err.Error(), "busy") {
		t.Errorf("error should say the service is busy: %v", err)
	}
	if attempts != 2 {
		t.Errorf("attempts = %d, want one retry after a busy response", attempts)
	}
}

// A capacity blip must not fail the whole category: the second attempt is made
// after a backoff and its results are used.
func TestScrapeOpenStreetMapRecoversFromBusyOnRetry(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte("This Overpass API server is currently at capacity."))
			return
		}
		_, _ = w.Write([]byte(`{"elements":[{"type":"node","id":5,"tags":{"name":"Adjoa's Bakery","shop":"bakery","phone":"0241234567"}}]}`))
	}))
	defer srv.Close()

	prevEndpoint, prevBackoff := overpassEndpoint, overpassBusyBackoff
	overpassEndpoint, overpassBusyBackoff = srv.URL, 0
	defer func() { overpassEndpoint, overpassBusyBackoff = prevEndpoint, prevBackoff }()

	places, err := ScrapeOpenStreetMap(context.Background(), "bakeries", "Accra")
	if err != nil {
		t.Fatalf("expected recovery on retry, got %v", err)
	}
	if len(places) != 1 || places[0].Name != "Adjoa's Bakery" {
		t.Fatalf("unexpected places after retry: %+v", places)
	}
}

// The saved lead must carry its provenance and a NULL rating, which is the
// whole point of the DiscoveredLead struct.
func TestDiscoverFromOpenStreetMapPersistsProvenance(t *testing.T) {
	body := `{"elements":[{"type":"node","id":9,"tags":{"name":"Adjoa's Bakery","shop":"bakery","phone":"0241234567"}}]}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	prev := overpassEndpoint
	overpassEndpoint = srv.URL
	defer func() { overpassEndpoint = prev }()

	store := &discoveryFakeStore{}
	saved, err := discoverFromOpenStreetMap(context.Background(), "ws", "bakeries", "Accra", 0, store)
	if err != nil {
		t.Fatalf("discoverFromOpenStreetMap: %v", err)
	}
	if saved != 1 {
		t.Fatalf("saved = %d, want 1", saved)
	}
	if len(store.saved) != 1 {
		t.Fatalf("store received %d leads, want 1", len(store.saved))
	}
	in := store.saved[0]
	if in.Source != "openstreetmap" {
		t.Errorf("Source = %q, want openstreetmap", in.Source)
	}
	if in.Rating != nil {
		t.Errorf("Rating = %v, want nil for a source with no ratings", *in.Rating)
	}
	if in.PlaceID != "osm:node/9" {
		t.Errorf("PlaceID = %q", in.PlaceID)
	}
	// discoverFromOpenStreetMap must not consume the Google quota gate.
	if store.recorded != 0 {
		t.Errorf("recorded %d discovery searches; OSM must not write the Google quota table", store.recorded)
	}
}

// ─── fake store ───────────────────────────────────────────────────────────────

// discoveryFakeStore records what discovery persisted. It embeds the dispatch
// fake so the whole LeadEngineStore interface is satisfied; only the two methods
// this test cares about are overridden.
type discoveryFakeStore struct {
	dispatchFakeStore
	saved    []store.DiscoveredLead
	recorded int
}

func (f *discoveryFakeStore) SaveDiscoveredLead(_ context.Context, in store.DiscoveredLead) (store.Lead, error) {
	f.saved = append(f.saved, in)
	return store.Lead{}, nil
}

func (f *discoveryFakeStore) RecordDiscoverySearch(context.Context, store.LeadDiscoverySearch) error {
	f.recorded++
	return nil
}
