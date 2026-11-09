package leadengine

import (
	"strings"
	"testing"

	"marketingops/api/internal/store"
)

func TestParseSpintax(t *testing.T) {
	// Simple variation
	input := "{Hi|Hello|Hey} there!"
	out := ParseSpintax(input)
	valid := map[string]bool{
		"Hi there!":    true,
		"Hello there!": true,
		"Hey there!":   true,
	}
	if !valid[out] {
		t.Errorf("unexpected output from ParseSpintax: %q", out)
	}

	// Multiple variations in one template
	inputMulti := "{Hi|Hello} {{business_name}}, {I saw|I noticed} your business."
	leadName := "Acme Corp"
	lead := store.Lead{
		ContactName: &leadName,
		Category:    "Dental Clinic",
		City:        "Accra",
	}
	res := applyPitchTemplate(inputMulti, lead, "Sarah", "https://portfolio.example")
	if !strings.Contains(res, "Acme Corp") {
		t.Errorf("expected merged business name in %q", res)
	}
	if !(strings.HasPrefix(res, "Hi") || strings.HasPrefix(res, "Hello")) {
		t.Errorf("expected spintax greeting in %q", res)
	}
	if !(strings.Contains(res, "I saw") || strings.Contains(res, "I noticed")) {
		t.Errorf("expected spintax variation in %q", res)
	}

	// Nested spintax
	nestedInput := "{{Great|Good} morning|Hello} everyone!"
	nestedOut := ParseSpintax(nestedInput)
	validNested := map[string]bool{
		"Great morning everyone!": true,
		"Good morning everyone!":  true,
		"Hello everyone!":         true,
	}
	if !validNested[nestedOut] {
		t.Errorf("unexpected nested spintax output: %q", nestedOut)
	}
}
