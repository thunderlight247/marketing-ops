package agents

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"

	"marketingops/api/internal/aiprovider"
)

// This file generalizes the two curated specialists (SEOAdvisor,
// ContentCreator) into pluggable agency agents. The governance rule the
// registry encodes: a specialist is a curated, versioned prompt profile with
// a documented output contract, declared data needs, and a closed set of
// allowed action classes. Adding the third agent must not require new
// handlers, stores, or credentials — only a new profile registered here.

// Output contracts are a closed set. A new contract must be designed and
// validated before it exists (see docs/decisions.md); the run route refuses
// anything it does not know how to render.
const (
	// ContractAdvisory: the run returns a validated AdvisorReport. Advisory
	// only — it never edits or publishes anything.
	ContractAdvisory = "advisory"
	// ContractDrafting: the run returns a validated ContentPlan that the
	// platform files as a draft in the existing approval workflow. It never
	// publishes or schedules.
	ContractDrafting = "drafting"
	// ContractProposal: the run returns a validated CampaignProposal that the
	// platform files as one planned campaign for operator review. The
	// proposal arrives inactive (status "planned") and every activation,
	// edit, or deletion stays an operator action in the ordinary flows.
	ContractProposal = "proposal"
)

// Action classes are a closed set: each one is a reviewed executor that maps
// a validated output onto existing, human-gated flows. Unknown classes must
// never be declared or executed.
const ActionFileDraft = "file_draft"

// ActionCreateCampaign files one proposed campaign into the campaigns store
// as a reviewable "planned" record; it never activates, edits, or deletes.
const ActionCreateCampaign = "create_campaign"

// Data needs are a closed set: one key per registered snapshot loader.
// Undeclared data never reaches a prompt.
const DataNeedSearchConsole = "search_console"

// RunInput is the bounded context of one specialist run: workspace facts the
// operator already shares, the operator's focus, and the snapshots for the
// specialist's declared data needs (nil value = no connected data this run).
// No stores, credentials, or plumbing ever reach a specialist.
type RunInput struct {
	Workspace aiprovider.WorkspaceContext
	Focus     string
	Data      map[string]any
}

// RunOutput is the union of the closed output contracts. Exactly one field
// is set, matching the specialist's declared contract.
type RunOutput struct {
	Report   *AdvisorReport
	Plan     *ContentPlan
	Proposal *CampaignProposal
}

// Specialist is one pluggable agency agent. Implementations validate their
// own replies and cap every field; the run route plus registry provide
// dispatch, caps, timeouts, rate limiting, audit, and data loading.
type Specialist interface {
	ID() string
	Version() string
	// DisplayName and Description are shown on the dashboard's agents panel.
	DisplayName() string
	Description() string
	// OutputContract is one of the Contract* constants.
	OutputContract() string
	// AllowedActions lists the Action* classes this agent may execute.
	AllowedActions() []string
	// WantsData lists the data-needs keys this agent reads.
	WantsData() []string
	// Run performs one bounded pass over the declared inputs only.
	Run(ctx context.Context, run PromptRun, input RunInput) (RunOutput, error)
}

// capabilities caps what a specialist may declare today. Governance is made
// executable here instead of by convention: the advisory contract takes no
// actions, the drafting contract files drafts, and only registered loaders
// may answer a data need.
func capabilities(contract string, actions []string, wants []string) error {
	if !knownContracts[contract] {
		return fmt.Errorf("unknown output contract %q", contract)
	}
	switch contract {
	case ContractAdvisory:
		if len(actions) != 0 {
			return fmt.Errorf("the %s contract takes no actions, got %v", contract, actions)
		}
	case ContractDrafting:
		if len(actions) != 1 || actions[0] != ActionFileDraft {
			return fmt.Errorf("the %s contract must declare exactly the %q action, got %v",
				contract, ActionFileDraft, actions)
		}
	case ContractProposal:
		if len(actions) != 1 || actions[0] != ActionCreateCampaign {
			return fmt.Errorf("the %s contract must declare exactly the %q action, got %v",
				contract, ActionCreateCampaign, actions)
		}
	}
	for _, need := range wants {
		if !knownDataNeeds[need] {
			return fmt.Errorf("unknown data need %q", need)
		}
	}
	return nil
}

var knownContracts = map[string]bool{
	ContractAdvisory: true,
	ContractDrafting: true,
	ContractProposal: true,
}

var knownDataNeeds = map[string]bool{
	DataNeedSearchConsole: true,
}

// Descriptor is the registry's sorted, dashboard-facing metadata for one
// specialist. It carries nothing secret.
type Descriptor struct {
	ID             string   `json:"id"`
	Version        string   `json:"version"`
	DisplayName    string   `json:"displayName"`
	Description    string   `json:"description"`
	OutputContract string   `json:"outputContract"`
	DataNeeds      []string `json:"dataNeeds"`
	Actions        []string `json:"actions"`
}

// Registry keeps the enabled specialists. Same shape as the integration
// Registry: duplicate registration is an error, lists are sorted, and lookup
// is by stable ID.
type Registry struct {
	mu     sync.RWMutex
	agents map[string]Specialist
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{agents: make(map[string]Specialist)}
}

// Register adds a specialist; a duplicate ID is an error because a silent
// replacement would rewrite system-level history under the same audit ID.
func (r *Registry) Register(s Specialist) error {
	if s == nil {
		return fmt.Errorf("cannot register a nil specialist")
	}
	if strings.TrimSpace(s.ID()) == "" || strings.TrimSpace(s.Version()) == "" {
		return fmt.Errorf("specialist %T must declare a non-empty ID and version", s)
	}
	if err := capabilities(s.OutputContract(), s.AllowedActions(), s.WantsData()); err != nil {
		return fmt.Errorf("specialist %q: %w", s.ID(), err)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.agents[s.ID()]; exists {
		return fmt.Errorf("agent %q is already registered; bump the version or use a new ID", s.ID())
	}
	r.agents[s.ID()] = s
	return nil
}

// Get returns the specialist for id, or false if none is registered.
func (r *Registry) Get(id string) (Specialist, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	s, ok := r.agents[id]
	return s, ok
}

// ListDescriptors returns sorted metadata for every registered specialist.
func (r *Registry) ListDescriptors() []Descriptor {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Descriptor, 0, len(r.agents))
	for _, a := range r.agents {
		out = append(out, descriptorOf(a))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// descriptorOf copies the specialist's declared strings into the descriptor
// so later mutations of the specialist's metadata cannot retroactively
// change what the dashboard showed.
func descriptorOf(s Specialist) Descriptor {
	return Descriptor{
		ID:             s.ID(),
		Version:        s.Version(),
		DisplayName:    s.DisplayName(),
		Description:    s.Description(),
		OutputContract: s.OutputContract(),
		DataNeeds:      append([]string{}, s.WantsData()...),
		Actions:        append([]string{}, s.AllowedActions()...),
	}
}

// NewDefaultRegistry registers the curated specialists. The built-ins are
// valid by construction, so a registration failure here is a programming
// error, not a configuration problem.
func NewDefaultRegistry() *Registry {
	r := NewRegistry()
	for _, s := range []Specialist{SEOAdvisor{}, ContentCreator{}, SearchGapWriter{}, CampaignStrategist{}} {
		if err := r.Register(s); err != nil {
			panic(fmt.Sprintf("agents built-in registration failed: %v", err))
		}
	}
	return r
}
