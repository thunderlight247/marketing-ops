package agents

import (
	"context"
	"strings"
	"testing"
)

// stubSpecialist lets tests declare arbitrary metadata without a working run.
type stubSpecialist struct {
	id, version, contract string
	actions, wants        []string
}

func (s stubSpecialist) ID() string               { return s.id }
func (s stubSpecialist) Version() string          { return s.version }
func (s stubSpecialist) DisplayName() string      { return "Stub" }
func (s stubSpecialist) Description() string      { return "A test stub." }
func (s stubSpecialist) OutputContract() string   { return s.contract }
func (s stubSpecialist) AllowedActions() []string { return s.actions }
func (s stubSpecialist) WantsData() []string      { return s.wants }
func (s stubSpecialist) Run(ctx context.Context, run PromptRun, input RunInput) (RunOutput, error) {
	return RunOutput{}, nil
}

func TestRegistryValidation(t *testing.T) {
	for _, tc := range []struct {
		name  string
		fails string
		s     Specialist
	}{{"unknown contract fails", "output contract", stubSpecialist{id: "a", version: "1", contract: "chat"}},
		{"empty id fails", "non-empty ID", stubSpecialist{version: "1", contract: ContractAdvisory}},
		{"advisory with actions fails", "takes no actions", stubSpecialist{
			id: "b", version: "1", contract: ContractAdvisory, actions: []string{ActionFileDraft}}},
		{"drafting must license file_draft", "file_draft", stubSpecialist{
			id: "c", version: "1", contract: ContractDrafting}},
		{"unknown data need fails", "campaigns", stubSpecialist{
			id: "d", version: "1", contract: ContractAdvisory, wants: []string{"campaigns"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := NewRegistry()
			if err := r.Register(tc.s); err == nil || !strings.Contains(err.Error(), tc.fails) {
				t.Fatalf("expected a gating error mentioning %q, got %v", tc.fails, err)
			}
		})
	}
}

func TestRegistryDuplicateIDRejected(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(stubSpecialist{id: "same", version: "1", contract: ContractAdvisory}); err != nil {
		t.Fatal(err)
	}
	if err := r.Register(stubSpecialist{id: "same", version: "1", contract: ContractAdvisory}); err == nil {
		t.Fatal("duplicate registration must fail, not silently replace")
	}
	got, ok := r.Get("same")
	if !ok || got.Version() != "1" {
		t.Fatalf("original registration must stay: %#v", got)
	}
}

func TestRegistryGetUnknown(t *testing.T) {
	if _, ok := NewRegistry().Get("nope"); ok {
		t.Fatal("unknown agent must not resolve")
	}
}

func TestRegistryListDescriptorsSorted(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(stubSpecialist{id: "zz", version: "1", contract: ContractAdvisory, wants: []string{DataNeedSearchConsole}}); err != nil {
		t.Fatal(err)
	}
	if err := r.Register(stubSpecialist{id: "aa", version: "3", contract: ContractDrafting, actions: []string{ActionFileDraft}}); err != nil {
		t.Fatal(err)
	}
	out := r.ListDescriptors()
	if len(out) != 2 || out[0].ID != "aa" || out[1].ID != "zz" {
		t.Fatalf("descriptors must be ID-sorted: %#v", out)
	}
	if out[0].Actions[0] != ActionFileDraft || len(out[0].DataNeeds) != 0 {
		t.Fatalf("descriptor must mirror the specialist's declared capabilities: %+v", out[0])
	}
}

// TestDefaultRegistryContracts checks the built-in specialists declare the
// governance facts the routes rely on.
func TestDefaultRegistryContracts(t *testing.T) {
	r := NewDefaultRegistry()
	byID := map[string]Descriptor{}
	for _, d := range r.ListDescriptors() {
		byID[d.ID] = d
	}
	seo, ok := byID[SEOAdvisorID]
	if !ok || seo.OutputContract != ContractAdvisory || seo.Version != SEOAdvisorVersion ||
		len(seo.DataNeeds) != 1 || seo.DataNeeds[0] != DataNeedSearchConsole || len(seo.Actions) != 0 {
		t.Fatalf("unexpected seo descriptor: %#v", seo)
	}
	creator, ok := byID[ContentCreatorID]
	if !ok || creator.OutputContract != ContractDrafting || creator.Version != ContentCreatorVersion ||
		len(creator.DataNeeds) != 0 ||
		len(creator.Actions) != 1 || creator.Actions[0] != ActionFileDraft {
		t.Fatalf("unexpected creator descriptor: %#v", creator)
	}
	gap, ok := byID[SearchGapWriterID]
	if !ok || gap.OutputContract != ContractDrafting || gap.Version != SearchGapWriterVersion ||
		len(gap.DataNeeds) != 1 || gap.DataNeeds[0] != DataNeedSearchConsole ||
		len(gap.Actions) != 1 || gap.Actions[0] != ActionFileDraft {
		t.Fatalf("unexpected gap-writer descriptor: %#v", gap)
	}
}
