// Package gates implements the hard-gate policy for the agent tool bridge:
// the set of outbound tool calls that must stop and wait for the owner's
// explicit OK before they run, regardless of what the agent or any rules
// file says.
package gates

import "sync"

// Kind names why a call was gated.
type Kind string

const (
	// KindFirstContact gates the first outbound message to a recipient the
	// store has never seen messaged before.
	KindFirstContact Kind = "first_contact"
	// KindRevoke gates any call that revokes or disconnects something.
	KindRevoke Kind = "revoke"
	// KindIrreversible gates an irreversible action that isn't on the
	// rules-file allow-list.
	KindIrreversible Kind = "irreversible"
	// KindExfiltration gates an outbound call in a turn that already read
	// from a different adapter — the prompt-injection path.
	KindExfiltration Kind = "exfiltration"
)

// CallFacts describes one tool-bridge call the policy must judge.
type CallFacts struct {
	Adapter      string
	Verb         string
	Recipient    string
	TurnKey      string
	Irreversible bool
	AllowListed  bool
	Revoke       bool
}

// Gate is a call that stopped for the owner's OK.
type Gate struct {
	ID        string
	Kind      Kind
	Adapter   string
	Verb      string
	Recipient string
}

// Decision is the result of evaluating a call against the policy.
type Decision struct {
	Allow bool
	Gate  *Gate
}

// Store is the durable backing for recipient history and denials. The
// policy never marks a recipient known itself — that happens after a
// successful execute, outside this package.
type Store interface {
	KnownRecipient(adapter, recipient string) (bool, error)
	MarkRecipientMessaged(adapter, recipient string) error
	RecordDenial(gateID string) error
	WasDenied(gateID string) (bool, error)
}

// Policy is the hard-gate policy. It is safe for concurrent use: bridge
// handlers run per-request goroutines.
type Policy struct {
	store Store
	newID func() string

	mu      sync.Mutex
	pending map[string]Gate
	reads   map[string]map[string]bool // turn key -> set of adapters read this turn
}

// New builds a Policy backed by store, minting gate ids with newID.
func New(store Store, newID func() string) *Policy {
	return &Policy{
		store:   store,
		newID:   newID,
		pending: make(map[string]Gate),
		reads:   make(map[string]map[string]bool),
	}
}

// Evaluate judges one call against the hard-gate policy.
func (p *Policy) Evaluate(facts CallFacts) (Decision, error) {
	if facts.Verb == "read" {
		p.mu.Lock()
		bucket := p.reads[facts.TurnKey]
		if bucket == nil {
			bucket = make(map[string]bool)
			p.reads[facts.TurnKey] = bucket
		}
		bucket[facts.Adapter] = true
		p.mu.Unlock()
		return Decision{Allow: true}, nil
	}

	var kind Kind
	gated := false
	switch {
	case facts.Revoke:
		kind, gated = KindRevoke, true
	case facts.Irreversible && !facts.AllowListed:
		kind, gated = KindIrreversible, true
	default:
		p.mu.Lock()
		crossAdapterRead := false
		for adapter := range p.reads[facts.TurnKey] {
			if adapter != facts.Adapter {
				crossAdapterRead = true
				break
			}
		}
		p.mu.Unlock()
		if crossAdapterRead {
			kind, gated = KindExfiltration, true
		} else {
			known, err := p.store.KnownRecipient(facts.Adapter, facts.Recipient)
			if err != nil {
				return Decision{}, err
			}
			if !known {
				kind, gated = KindFirstContact, true
			}
		}
	}

	if !gated {
		return Decision{Allow: true}, nil
	}

	gate := Gate{
		ID:        p.newID(),
		Kind:      kind,
		Adapter:   facts.Adapter,
		Verb:      facts.Verb,
		Recipient: facts.Recipient,
	}
	p.mu.Lock()
	p.pending[gate.ID] = gate
	p.mu.Unlock()
	return Decision{Allow: false, Gate: &gate}, nil
}

// Approve releases a pending gate exactly once: the gate is removed before
// it is returned, so a second call with the same id never releases again.
func (p *Policy) Approve(gateID string) (Gate, bool) {
	p.mu.Lock()
	gate, exists := p.pending[gateID]
	if exists {
		delete(p.pending, gateID)
	}
	p.mu.Unlock()
	if !exists {
		return Gate{}, false
	}
	return gate, true
}

// Deny removes a pending gate and durably records the denial so it can
// never release, even across restarts.
func (p *Policy) Deny(gateID string) error {
	p.mu.Lock()
	delete(p.pending, gateID)
	p.mu.Unlock()
	return p.store.RecordDenial(gateID)
}
