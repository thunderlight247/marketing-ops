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
		if senderName == "" {
			return fmt.Sprintf("Good day to the administration of %s.", businessName)
		}
		return fmt.Sprintf("Good day to the administration of %s; my name is %s.", businessName, senderName)
	}
	if senderName == "" {
		return fmt.Sprintf("Good day, %s %s.", businessName, rule.Recipient)
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
} // ─── template fallback ────────────────────────────────────────────────────────

// getTemplatePitch builds a pitch from what the workspace configured: sender
// name, business offering, target audience, and portfolio URL. Nothing about
// the sending business is hardcoded here.
func getTemplatePitch(ctx pitchContext, senderName, offering, audience, portfolioURL string) string {
	sentences := []string{
		getOpening(ctx.BusinessName, ctx.Category, senderName),
		getRecognition(ctx),
	}
	if offering != "" {
		sentences = append(sentences, fmt.Sprintf("We support businesses across Ghana with %s.", offering))
	} else {
		rule := getCategoryRule(ctx.Category)
		sentences = append(sentences, fmt.Sprintf("We help %ss in Ghana reach more customers and run more efficiently.", rule.Subject))
	}
	if audience != "" {
		sentences = append(sentences, fmt.Sprintf("We work with %s.", audience))
	}
	if portfolioURL != "" {
		sentences = append(sentences, fmt.Sprintf("You can learn more about us here: %s.", portfolioURL))
	}
	sentences = append(sentences,
		fmt.Sprintf("Would you be open to a brief conversation about how we could assist %s?", ctx.BusinessName),
		"Thank you.")
	return strings.Join(sentences, " ")
}

// ─── validation ───────────────────────────────────────────────────────────────

var sentenceSplitter = regexp.MustCompile(`[.!?]+`)

// validatePitch checks a generated pitch is usable: a sane sentence range,
// addressed to the right business, ending properly, no emojis, and short
// enough for WhatsApp. When a portfolio URL is configured it must appear.
func validatePitch(pitch, businessName, portfolioURL string) bool {
	withoutURL := strings.ReplaceAll(pitch, portfolioURL, "PORTFOLIO_LINK")
	parts := sentenceSplitter.Split(withoutURL, -1)
	count := 0
	for _, p := range parts {
		if strings.TrimSpace(p) != "" {
			count++
		}
	}
	pitch = strings.TrimSpace(pitch)
	ok := count >= 4 && count <= 9 &&
		businessName != "" && strings.Contains(pitch, businessName) &&
		strings.HasSuffix(pitch, "Thank you.") &&
		!hasEmoji(pitch) &&
		utf8.RuneCountInString(pitch) <= 900
	if portfolioURL != "" {
		ok = ok && strings.Contains(pitch, portfolioURL)
	}
	return ok
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

func buildPrompt(ctx pitchContext, senderName, offering, audience, portfolioURL string) string {
	rule := getCategoryRule(ctx.Category)
	opening := getOpening(ctx.BusinessName, ctx.Category, senderName)
	recognition := getRecognition(ctx)
	promptLines := []string{
		"You are the sender of this outreach, between representatives of the sending business and the prospect in Ghana.",
		fmt.Sprintf("Write a professional, friendly WhatsApp introduction to the %s of the business named %q, a %s in %s, Ghana.", rule.Recipient, ctx.BusinessName, ctx.Category, ctx.City),
		"",
		"Use 5 to 7 short sentences:",
		fmt.Sprintf("1. Start exactly with: %q", opening),
		fmt.Sprintf("2. Follow with: %s", recognition),
	}
	if offering != "" {
		promptLines = append(promptLines, fmt.Sprintf("3. Briefly present our services and how they can help this prospect. Our services: %s", offering))
	}
	if audience != "" {
		promptLines = append(promptLines, fmt.Sprintf("4. Show we know who we serve. Who we work with: %s", audience))
	}
	i := len(promptLines) + 1
	if portfolioURL != "" {
		promptLines = append(promptLines, fmt.Sprintf("%d. In one short sentence, point to our recent work here: %s, keeping the reference conversational, not like a generic link drop.", i, portfolioURL))
		i++
	}
	promptLines = append(promptLines,
		fmt.Sprintf("%d. Ask whether they would be open to a brief conversation about how we could assist their %s.", i, rule.Subject),
		fmt.Sprintf("%d. End the message with exactly: Thank you.", i+1))
	return fmt.Sprintf("%s\n\nUse plain professional English. Do not use emojis, quotation marks, buzzwords, unsupported claims beyond the experience provided above, pricing, multiple questions, or mention AI. Output only the message body.", strings.Join(promptLines, "\n"))
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
	fallback := getTemplatePitch(ctx, params.SenderName, params.BusinessOffering, params.TargetAudience, params.PortfolioURL)

	if params.AIKey == "" {
		return fallback, nil
	}

	prompt := buildPrompt(ctx, params.SenderName, params.BusinessOffering, params.TargetAudience, params.PortfolioURL)

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
	if validatePitch(aiPitch, params.BusinessName, params.PortfolioURL) {
		return aiPitch, nil
	}
	return fallback, nil
}

// PitchParams is the input to GeneratePitch.
type PitchParams struct {
	BusinessName     string
	Category         string
	City             string
	SenderName       string
	BusinessOffering string
	TargetAudience   string
	PortfolioURL     string
	Rating           float64
	ReviewCount      int
	AIKey            string
	AIModel          string
}

// ReplyParams is the input to GenerateConversationalReply.
type ReplyParams struct {
	SenderName       string
	BusinessName     string
	BusinessOffering string
	PortfolioURL     string
	InitialPitch     string
	InboundMessage   string
	AIKey            string
	AIModel          string
}

// GenerateConversationalReply uses the LLM to draft a helpful, professional, concise reply to an inbound WhatsApp response.
func GenerateConversationalReply(goCtx context.Context, params ReplyParams) (string, error) {
	if params.AIKey == "" {
		msg := "Thank you for reaching out! We received your message."
		if params.PortfolioURL != "" {
			msg += " You can view our recent projects here: " + params.PortfolioURL + "."
		}
		msg += " We will follow up with you shortly."
		return msg, nil
	}

	model := params.AIModel
	if model == "" {
		model = "meta-llama/llama-3.3-70b-instruct:free"
	}

	prompt := fmt.Sprintf(`You are %s, a representative of the business that sent the pitch below.
Our offering: %s

Earlier, we sent this WhatsApp pitch to "%s":
"%s"

The prospect just replied with:
"%s"

Write a professional, warm, and concise WhatsApp reply (maximum 3 short sentences):
1. Acknowledge their response directly and answer their question or interest politely.
2. Do not invent services beyond our offering; if they ask for something outside it, say we can discuss it on a call.
3. Keep it natural, human, and under 50 words. Do not use buzzwords, multiple exclamation points, or robotic phrases. Output only the reply message body.`,
		params.SenderName, params.BusinessOffering,
		params.BusinessName, params.InitialPitch, params.InboundMessage)

	body, err := json.Marshal(openRouterRequest{
		Model:    model,
		Messages: []openRouterMessage{{Role: "user", Content: prompt}},
	})
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(goCtx, http.MethodPost,
		"https://openrouter.ai/api/v1/chat/completions",
		bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+params.AIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	var or openRouterResponse
	if err := json.Unmarshal(raw, &or); err != nil {
		return "", err
	}
	if len(or.Choices) == 0 {
		return "", fmt.Errorf("no response choices from AI provider")
	}
	return strings.Trim(strings.TrimSpace(or.Choices[0].Message.Content), `"`), nil
}
