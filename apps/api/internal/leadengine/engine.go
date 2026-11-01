// engine.go orchestrates discovery (Google Places scrape + save) and dispatch
// (AI pitch generation + WhatsApp send) for the lead generation engine.
package leadengine

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"time"

	"marketingops/api/internal/store"
)

// LeadEngineStore is the subset of store.Store the engine needs.
// It is satisfied by *store.Store and by httpapi.WorkspaceStore.
type LeadEngineStore interface {
	WasSearchedToday(ctx context.Context, workspaceID, dayKey, category, city string) (bool, error)
	RecordDiscoverySearch(ctx context.Context, s store.LeadDiscoverySearch) error
	IsBlacklisted(ctx context.Context, workspaceID, phone string) (bool, error)
	SaveDiscoveredLead(ctx context.Context, workspaceID, placeID, name, phone, category, city, address string, rating float64, reviewCount int, websiteURL string) (store.Lead, error)
	ListLeadEngineLeads(ctx context.Context, workspaceID string) ([]store.Lead, error)
	UpdateLeadPitch(ctx context.Context, workspaceID, leadID, pitch string) error
	UpdateLeadStatus(ctx context.Context, workspaceID, leadID, status string) error
}

// ─── discovery ─────────────────────────────────────────────────────────────────

// DiscoveryParams holds everything RunDiscovery needs.
type DiscoveryParams struct {
	WorkspaceID     string
	GooglePlacesKey string
	Categories      []string
	Cities          []string
	DailyLimit      int
}

// RunDiscovery scrapes Google Places for each category+city pair, filters,
// checks blacklist, and persists new leads. It is safe to run in a goroutine.
// Returns the count of newly saved leads and any non-fatal errors.
func RunDiscovery(ctx context.Context, p DiscoveryParams, s LeadEngineStore) (int, []error) {
	dayKey := time.Now().UTC().Format("2006-01-02")
	var errs []error
	total := 0

	cats := p.Categories
	if len(cats) == 0 {
		cats = defaultCategories()
	}
	cities := p.Cities
	if len(cities) == 0 {
		cities = defaultCities()
	}

	for _, cat := range cats {
		for _, city := range cities {
			if total >= p.DailyLimit && p.DailyLimit > 0 {
				break
			}
			already, err := s.WasSearchedToday(ctx, p.WorkspaceID, dayKey, cat, city)
			if err != nil || already {
				continue
			}

			places, err := ScrapeCategoryInCity(ctx, p.GooglePlacesKey, cat, city)
			if err != nil {
				errs = append(errs, fmt.Errorf("scrape %s/%s: %w", cat, city, err))
				_ = s.RecordDiscoverySearch(ctx, store.LeadDiscoverySearch{
					WorkspaceID: p.WorkspaceID,
					DayKey:      dayKey,
					Category:    cat,
					City:        city,
					ResultCount: 0,
					Error:       err.Error(),
				})
				continue
			}

			saved := 0
			for _, pl := range places {
				phone := formatPhone(pl)
				bl, _ := s.IsBlacklisted(ctx, p.WorkspaceID, phone)
				if bl {
					continue
				}
				_, err := s.SaveDiscoveredLead(ctx, p.WorkspaceID,
					pl.PlaceID, pl.Name, phone, cat, city, pl.Address,
					pl.Rating, pl.ReviewCount, pl.WebsiteURL)
				if err == nil {
					saved++
					total++
				}
			}
			_ = s.RecordDiscoverySearch(ctx, store.LeadDiscoverySearch{
				WorkspaceID: p.WorkspaceID,
				DayKey:      dayKey,
				Category:    cat,
				City:        city,
				ResultCount: saved,
			})
		}
	}
	return total, errs
}

// ─── dispatch ─────────────────────────────────────────────────────────────────

// DispatchParams holds everything RunDispatch needs.
type DispatchParams struct {
	WorkspaceID  string
	AIKey        string
	AIModel      string
	SenderName   string
	PortfolioURL string
	MinDelayMs   int
	MaxDelayMs   int
	DailyLimit   int
	DispatchURL  string // WhatsApp dispatcher base URL e.g. http://localhost:8083
}

// RunDispatch generates pitches for discovered leads and sends them via WhatsApp.
// It respects the daily limit and the configured send delays.
func RunDispatch(ctx context.Context, p DispatchParams, s LeadEngineStore) error {
	leads, err := s.ListLeadEngineLeads(ctx, p.WorkspaceID)
	if err != nil {
		return err
	}
	portfolioURL := p.PortfolioURL
	if portfolioURL == "" {
		portfolioURL = "https://greatlightgroup.com/software/"
	}
	senderName := p.SenderName
	if senderName == "" {
		senderName = "John"
	}

	sent := 0
	for _, lead := range leads {
		if p.DailyLimit > 0 && sent >= p.DailyLimit {
			break
		}
		if lead.Status != "discovered" && lead.Status != "pending" {
			continue
		}

		// Generate pitch if not already generated.
		pitch := lead.Pitch
		name := ""
		if lead.ContactName != nil {
			name = *lead.ContactName
		}
		phone := ""
		if lead.ContactPhone != nil {
			phone = *lead.ContactPhone
		}
		if pitch == "" {
			rating := 0.0
			if lead.Rating != nil {
				rating = *lead.Rating
			}
			generated, err := GeneratePitch(ctx, PitchParams{
				BusinessName: name,
				Category:     lead.Category,
				City:         lead.City,
				SenderName:   senderName,
				PortfolioURL: portfolioURL,
				Rating:       rating,
				ReviewCount:  lead.ReviewCount,
				AIKey:        p.AIKey,
				AIModel:      p.AIModel,
			})
			if err != nil {
				continue
			}
			pitch = generated
			_ = s.UpdateLeadPitch(ctx, p.WorkspaceID, lead.ID, pitch)
		}

		_ = s.UpdateLeadStatus(ctx, p.WorkspaceID, lead.ID, "processing")

		// Send via WhatsApp dispatcher.
		sendURL := p.DispatchURL + "/sessions/" + p.WorkspaceID + "/send"
		payload, _ := json.Marshal(map[string]string{
			"phone":   phone,
			"message": pitch,
		})
		resp, err := http.Post(sendURL, "application/json", bytes.NewReader(payload)) //nolint:noctx
		if err != nil || resp.StatusCode >= 400 {
			_ = s.UpdateLeadStatus(ctx, p.WorkspaceID, lead.ID, "failed")
			if resp != nil {
				resp.Body.Close()
			}
			continue
		}
		resp.Body.Close()
		_ = s.UpdateLeadStatus(ctx, p.WorkspaceID, lead.ID, "sent")
		sent++

		// Rate-limit delay.
		delayMs := p.MinDelayMs
		if p.MaxDelayMs > p.MinDelayMs {
			delayMs = p.MinDelayMs + rand.Intn(p.MaxDelayMs-p.MinDelayMs)
		}
		if delayMs <= 0 {
			delayMs = 25000
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Duration(delayMs) * time.Millisecond):
		}
	}
	return nil
}

// ─── Ghana defaults (mirror of gh-lead-engine/config/index.js) ────────────────

func defaultCategories() []string {
	return []string{
		"school", "academy", "clinic", "hospital", "hotel",
		"guesthouse", "restaurant", "salon", "barbershop",
		"shop", "boutique", "store", "church", "ngo",
		"law firm", "accounting", "consulting",
	}
}

func defaultCities() []string {
	return []string{
		"Accra", "Kumasi", "Tamale", "Cape Coast", "Takoradi",
		"Sunyani", "Koforidua", "Ho", "Bolgatanga", "Wa",
		"Tema", "Kasoa", "Ashaiman",
	}
}
