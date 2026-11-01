package leadengine

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"unicode/utf8"
)

// ─── category rules (port of gh-lead-engine/src/ai.js CATEGORY_RULES) ────────

type categoryRule struct {
	Matches   []string
	Recipient string
	Subject   string
	Value     string
}

var categoryRules = []categoryRule{
	{
		Matches:   []string{"school", "academy", "college"},
		Recipient: "administration",
		Subject:   "school",
		Value:     "parents easily find information about your programs, admissions, fees, location, and contact details",
	},
	{
		Matches:   []string{"clinic", "hospital", "medical"},
		Recipient: "management",
		Subject:   "clinic",
		Value:     "patients easily find your services, doctors, opening hours, location, and appointment details",
	},
	{
		Matches:   []string{"hotel", "guesthouse", "guest house", "lodge"},
		Recipient: "management",
		Subject:   "hotel",
		Value:     "guests easily find your rooms, rates, amenities, location, and reservation details",
	},
	{
		Matches:   []string{"restaurant", "chopbar", "cafe", "café", "food"},
		Recipient: "team",
		Subject:   "restaurant",
		Value:     "customers easily find your menu, prices, location, opening hours, and reservation details",
	},
	{
		Matches:   []string{"salon", "barbershop", "barber"},
		Recipient: "team",
		Subject:   "salon",
		Value:     "clients easily find your services, prices, location, opening hours, and appointment details",
	},
	{
		Matches:   []string{"professional services", "accounting", "law firm", "consulting"},
		Recipient: "team",
		Subject:   "professional services firm",
		Value:     "clients easily find your services, expertise, location, contact details, and consultation options",
	},
	{
		Matches:   []string{"small business", "shop", "store", "boutique"},
		Recipient: "team",
		Subject:   "business",
		Value:     "customers easily find your products, prices, location, opening hours, and WhatsApp contact",
	},
	{
		Matches:   []string{"local organization", "ngo", "foundation", "church", "association"},
		Recipient: "team",
		Subject:   "organization",
		Value:     "community members easily find your programs, activities, events, location, and contact details",
	},
}

func getCategoryRule(category string) categoryRule {
	norm := strings.ToLower(strings.TrimSpace(category))
	for _, r := range categoryRules {
		for _, m := range r.Matches {
			if strings.Contains(norm, m) {
				return r
			}
		}
	}
	subj := norm
	if subj == "" {
		subj = "business"
	}
	return categoryRule{
		Recipient: "team",
		Subject:   subj,
		Value:     "customers easily find your services, products, prices, location, opening hours, and contact details",
	}
}

func getOpening(businessName, category, senderName string) string {
	rule := getCategoryRule(category)
	if rule.Recipient == "administration" {
		return fmt.Sprintf("Good day to the administration of %s; my name is %s.", businessName, senderName)
	}
	return fmt.Sprintf("Good day, %s %s; my name is %s.", businessName, rule.Recipient, senderName)
}

// ─── pitch context helpers ─────────────────────────────────────────────────────

type pitchContext struct {
	BusinessName    string
	Category        string
	City            string
	Rating          float64
	ReviewCount     int
	WebsiteVerified bool // true means they have a website (we shouldn't message those — but included for completeness)
}

func getRecognition(ctx pitchContext) string {
	if ctx.Rating >= minRating && ctx.ReviewCount >= minReviewCount {
		return fmt.Sprintf("Congratulations on your strong Google reviews in %s.", ctx.City)
	}
	rule := getCategoryRule(ctx.Category)
	return fmt.Sprintf("I came across %s while researching %ss in %s.", ctx.BusinessName, rule.Subject, ctx.City)
}

func getWebsiteObservation(ctx pitchContext, category string) string {
	if !ctx.WebsiteVerified {
		rule := getCategoryRule(category)
		return fmt.Sprintf("I noticed that no dedicated website appears to be listed for your %s.", rule.Subject)
	}
	return "I believe a dedicated website could make it easier for people to learn about your business."
}

// ─── template fallback ────────────────────────────────────────────────────────

func getTemplatePitch(businessName, category, city, senderName, portfolioURL string, ctx pitchContext) string {
	rule := getCategoryRule(category)
	sentences := []string{
		getOpening(businessName, category, senderName),
		getRecognition(ctx),
		getWebsiteObservation(ctx, category),
		fmt.Sprintf("A professional, mobile-friendly website can help %s.", rule.Value),
		fmt.Sprintf("At Great Light Group, we have experience building high-performance, modern websites and software solutions across Ghana; you can view recent web and software projects here: %s.", portfolioURL),
		fmt.Sprintf("Would you be open to receiving a 1-minute video demo of a website designed specifically for %s?", businessName),
		"Thank you.",
	}
	return strings.Join(sentences, " ")
}

// ─── validation (port of validatePitch from ai.js) ────────────────────────────

var sentenceSplitter = regexp.MustCompile(`[.!?]+`)

func validatePitch(pitch, businessName, category, portfolioURL string) bool {
	// Replace portfolio URL before counting sentences (it contains dots).
	withoutURL := strings.ReplaceAll(pitch, portfolioURL, "PORTFOLIO_LINK")
	parts := sentenceSplitter.Split(withoutURL, -1)
	count := 0
	for _, p := range parts {
		if strings.TrimSpace(p) != "" {
			count++
		}
	}
	rule := getCategoryRule(category)
	return count == 7 &&
		strings.Contains(pitch, businessName) &&
		strings.Contains(strings.ToLower(pitch), strings.Split(rule.Value, " ")[0]) &&
		strings.Contains(pitch, portfolioURL) &&
		strings.HasSuffix(pitch, "Thank you.") &&
		!hasEmoji(pitch) &&
		utf8.RuneCountInString(pitch) <= 900
}

func hasEmoji(s string) bool {
	for _, r := range s {
		if r >= 0x1F600 && r <= 0x1F64F { // emoticons
			return true
		}
		if r >= 0x1F300 && r <= 0x1F5FF { // misc symbols
			return true
		}
	}
	return false
}

// ─── AI pitch generation ──────────────────────────────────────────────────────

type openRouterRequest struct {
	Model    string              `json:"model"`
	Messages []openRouterMessage `json:"messages"`
}

type openRouterMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openRouterResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

func buildPrompt(businessName, category, city, senderName, portfolioURL string, ctx pitchContext) string {
	rule := getCategoryRule(category)
	opening := getOpening(businessName, category, senderName)
	recognition := getRecognition(ctx)
	websiteObs := getWebsiteObservation(ctx, category)
	return fmt.Sprintf(`You are an expert local digital agency representative in Ghana.
Write a professional, friendly WhatsApp introduction to %s of "%s", a %s in %s, Ghana.

Use exactly 7 short sentences:
1. Start exactly with: "%s"
2. %s
3. %s
4. Explain that a professional, mobile-friendly website can help %s.
5. In one short sentence, introduce Great Light Group and our experience building high-performance, modern websites and software solutions across Ghana, and invite them to view recent web and software projects here: %s.
6. Ask whether they would be open to receiving a 1-minute video demo of a website designed specifically for %s.
7. End the message with exactly: "Thank you."

Use plain professional English and keep the portfolio reference conversational, not like a generic link drop. Do not use emojis, quotation marks, buzzwords, unsupported claims beyond the Great Light Group experience provided above, pricing, multiple questions, or mention AI. Output only the message body.`,
		rule.Recipient, businessName, category, city,
		opening, recognition, websiteObs, rule.Value,
		portfolioURL, businessName)
}

// GeneratePitch generates a WhatsApp pitch for a lead using the OpenRouter API,
// falling back to the template pitch if the AI response fails validation.
func GeneratePitch(goCtx context.Context, params PitchParams) (string, error) {
	ctx := pitchContext{
		BusinessName: params.BusinessName,
		Category:     params.Category,
		City:         params.City,
		Rating:       params.Rating,
		ReviewCount:  params.ReviewCount,
	}
	fallback := getTemplatePitch(params.BusinessName, params.Category, params.City,
		params.SenderName, params.PortfolioURL, ctx)

	if params.AIKey == "" {
		return fallback, nil
	}

	prompt := buildPrompt(params.BusinessName, params.Category, params.City,
		params.SenderName, params.PortfolioURL, ctx)

	model := params.AIModel
	if model == "" {
		model = "meta-llama/llama-3.3-70b-instruct:free"
	}

	body, err := json.Marshal(openRouterRequest{
		Model:    model,
		Messages: []openRouterMessage{{Role: "user", Content: prompt}},
	})
	if err != nil {
		return fallback, nil
	}

	req, err := http.NewRequestWithContext(goCtx, http.MethodPost,
		"https://openrouter.ai/api/v1/chat/completions",
		bytes.NewReader(body))
	if err != nil {
		return fallback, nil
	}
	req.Header.Set("Authorization", "Bearer "+params.AIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fallback, nil
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	var or openRouterResponse
	if err := json.Unmarshal(raw, &or); err != nil {
		return fallback, nil
	}
	if len(or.Choices) == 0 {
		return fallback, nil
	}
	aiPitch := strings.TrimSpace(or.Choices[0].Message.Content)
	if validatePitch(aiPitch, params.BusinessName, params.Category, params.PortfolioURL) {
		return aiPitch, nil
	}
	return fallback, nil
}

// PitchParams is the input to GeneratePitch.
type PitchParams struct {
	BusinessName string
	Category     string
	City         string
	SenderName   string
	PortfolioURL string
	Rating       float64
	ReviewCount  int
	AIKey        string
	AIModel      string
}
