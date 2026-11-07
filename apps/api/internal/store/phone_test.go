package store

import "testing"

// The legacy engine normalised every number before storing it, so one business
// was one row. These cases pin that behaviour: the same Ghana number written
// five ways must collapse to a single identity.
func TestNormalizePhone(t *testing.T) {
	want := "233542635494"
	for _, in := range []string{
		"+233 54 263 5494",
		"0542635494",
		"233542635494",
		"(054) 263-5494",
	} {
		if got := NormalizePhone(in); got != want {
			t.Errorf("NormalizePhone(%q) = %q, want %q", in, got, want)
		}
	}
	if got := NormalizePhone("   "); got != "" {
		t.Errorf("blank phone must normalise to empty, got %q", got)
	}
}

func TestPhoneKey(t *testing.T) {
	want := "542635494"
	for _, in := range []string{
		"+233 54 263 5494",
		"0542635494",
		"233542635494",
		"542635494",
	} {
		if got := PhoneKey(in); got != want {
			t.Errorf("PhoneKey(%q) = %q, want %q", in, got, want)
		}
	}
	if got := PhoneKey("0244"); got != "0244" {
		t.Errorf("short phone must stay comparable, got %q", got)
	}
}
