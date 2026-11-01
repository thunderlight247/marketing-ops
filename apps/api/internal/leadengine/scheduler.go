// Package leadengine provides automated lead discovery, AI generation, and outreach dispatch.
package leadengine

import (
	"context"
	"log"
	"time"

	"marketingops/api/internal/secretcipher"
	"marketingops/api/internal/store"
)

// SchedulerStore is the subset of store methods needed by the automated scheduler.
type SchedulerStore interface {
	LeadEngineStore
	ListEnabledLeadConfigs(ctx context.Context) ([]store.WorkspaceLeadConfig, error)
}

// StartScheduler starts the automated background loop for discovery and outreach dispatch.
// It checks each enabled workspace on the given interval and terminates when ctx is cancelled.
func StartScheduler(ctx context.Context, s SchedulerStore, credKey []byte, dispatchURL, dispatcherSecret string, interval time.Duration) {
	if interval <= 0 {
		interval = 6 * time.Hour
	}

	log.Printf("[leadengine:scheduler] starting background scheduler with interval %v", interval)

	ticker := time.NewTicker(interval)
	go func() {
		defer ticker.Stop()

		// Run an initial check shortly after startup (after 30 seconds)
		select {
		case <-ctx.Done():
			return
		case <-time.After(30 * time.Second):
			runSchedulerPass(ctx, s, credKey, dispatchURL, dispatcherSecret)
		}

		for {
			select {
			case <-ctx.Done():
				log.Println("[leadengine:scheduler] stopped")
				return
			case <-ticker.C:
				runSchedulerPass(ctx, s, credKey, dispatchURL, dispatcherSecret)
			}
		}
	}()
}

func runSchedulerPass(ctx context.Context, s SchedulerStore, credKey []byte, dispatchURL, dispatcherSecret string) {
	log.Println("[leadengine:scheduler] checking enabled workspaces for automated outreach...")

	configs, err := s.ListEnabledLeadConfigs(ctx)
	if err != nil {
		log.Printf("[leadengine:scheduler] failed to list enabled lead configs: %v", err)
		return
	}

	for _, cfg := range configs {
		if ctx.Err() != nil {
			return
		}

		workspaceID := cfg.WorkspaceID

		// 1. Automated Discovery (if Google Places API key is configured)
		if len(cfg.GooglePlacesAPIKeyCiphertext) > 0 {
			googleKey, err := secretcipher.Decrypt(credKey, cfg.GooglePlacesAPIKeyCiphertext)
			if err == nil && len(googleKey) > 0 {
				log.Printf("[leadengine:scheduler:%s] running automated discovery", workspaceID)
				discovered, errs := RunDiscovery(ctx, DiscoveryParams{
					WorkspaceID:     workspaceID,
					GooglePlacesKey: string(googleKey),
					Categories:      cfg.TargetCategories,
					Cities:          cfg.TargetCities,
					DailyLimit:      cfg.DailyLimit,
				}, s)
				log.Printf("[leadengine:scheduler:%s] discovery complete: %d new leads found (%d errors)", workspaceID, discovered, len(errs))
			}
		}

		// 2. Automated Dispatch (if WhatsApp is connected and AI provider key is configured)
		if cfg.WhatsAppStatus == "connected" && len(cfg.AIProviderKeyCiphertext) > 0 {
			aiKey, err := secretcipher.Decrypt(credKey, cfg.AIProviderKeyCiphertext)
			if err == nil && len(aiKey) > 0 {
				log.Printf("[leadengine:scheduler:%s] running automated dispatch", workspaceID)
				if err := RunDispatch(ctx, DispatchParams{
					WorkspaceID:      workspaceID,
					AIKey:            string(aiKey),
					AIModel:          cfg.AIModel,
					SenderName:       cfg.SenderName,
					PortfolioURL:     cfg.PortfolioURL,
					MinDelayMs:       cfg.MinDelayMs,
					MaxDelayMs:       cfg.MaxDelayMs,
					DailyLimit:       cfg.DailyLimit,
					DispatchURL:      dispatchURL,
					DispatcherSecret: dispatcherSecret,
				}, s); err != nil {
					log.Printf("[leadengine:scheduler:%s] dispatch error: %v", workspaceID, err)
				}
			}
		}
	}
}
