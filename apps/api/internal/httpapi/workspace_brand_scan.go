// Brand website scan. POST /api/v1/workspaces/{id}/brand-scan reads the
// workspace's own stored website through Firecrawl and returns what the page
// says about the business so the operator can compare it with the brand
// profile.
//
// Honesty rules:
//   - Read-only. The scan never writes the brand profile or any other record;
//     the operator copies what they approve through the normal brand form.
//   - Scraped text is labelled as "scraped" evidence, never as a verified fact.
//   - The target is the workspace's stored website URL, not a URL from the
//     request body, so a caller cannot point the scanner at arbitrary hosts.
package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"marketingops/api/internal/firecrawl"
)

// BrandScanner reads one public page. *firecrawl.Client satisfies it; tests
// use a fake.
type BrandScanner interface {
	Scrape(ctx context.Context, target string) (firecrawl.Page, error)
}

// brandScanExcerptLimit bounds the markdown returned to the browser.
const brandScanExcerptLimit = 4000

// POST /api/v1/workspaces/{id}/brand-scan
func postBrandScanHandler(ws WorkspaceStore, scanner BrandScanner) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if scanner == nil {
			writeJSONError(w, http.StatusNotImplemented, "brand website scan is not configured (set FIRECRAWL_API_KEY)")
			return
		}
		workspaceID, ok := workspaceScope(w, r, ws)
		if !ok {
			return
		}
		workspace, err := ws.GetWorkspaceByID(r.Context(), workspaceID)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "workspace lookup failed")
			return
		}
		if workspace.WebsiteURL == nil || strings.TrimSpace(*workspace.WebsiteURL) == "" {
			writeJSONError(w, http.StatusBadRequest, "the workspace has no website URL to scan")
			return
		}
		target, err := firecrawl.ValidateSiteURL(*workspace.WebsiteURL)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "the workspace website URL is not a valid http or https URL")
			return
		}

		page, err := scanner.Scrape(r.Context(), target)
		if err != nil {
			var se *firecrawl.ScrapeError
			if errors.As(err, &se) && se.Kind == firecrawl.KindBlocked {
				writeJSONError(w, http.StatusBadGateway, "the website could not be read")
				return
			}
			writeJSONError(w, http.StatusBadGateway, "brand website scan failed")
			return
		}

		excerpt := page.Markdown
		truncated := false
		if runes := []rune(excerpt); len(runes) > brandScanExcerptLimit {
			excerpt = string(runes[:brandScanExcerptLimit])
			truncated = true
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"workspaceId": workspaceID,
			"source":      "scraped",
			"url":         target,
			"title":       page.Title,
			"description": page.Description,
			"excerpt":     excerpt,
			"truncated":   truncated,
		})
	}
}
