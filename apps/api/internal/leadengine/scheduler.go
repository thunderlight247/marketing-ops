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
	// TryAcquireSchedulerLock returns ok=false when another instance is running a pass.
	TryAcquireSchedulerLock(ctx context.Context) (release func(), ok bool, err error)
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
	// Only one instance runs a pass at a time. With several API replicas, the
	// others skip this tick instead of duplicating discovery and dispatch.
	release, ok, err := s.TryAcquireSchedulerLock(ctx)
	if err != nil {
		log.Printf("[leadengine:scheduler] could not take scheduler lock: %v", err)
		return
	}
	if !ok {
		log.Println("[leadengine:scheduler] another instance holds the scheduler lock; skipping")
		return
	}
	defer release()

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
					PitchTemplate:    cfg.PitchTemplate,
					BusinessOffering: cfg.BusinessOffering,
					TargetAudience:   cfg.TargetAudience,
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
