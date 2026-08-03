package contract

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// The sibling guard in schema_matches_the_wire_test.go compares the message
// type *enum* against validateBody's switch, and it passes. This one compares
// the same switch against the schema's per-type *branches*, and it does not.
//
// envelope.schema.json is a top-level object with a `type` enum and an `allOf`
// of branches shaped `if type == X then { sender: ..., body: {...} }`. Every
// rule about what a frame of a given type may contain — including which side
// is allowed to send it — lives inside one of those branches. A type that is
// in the enum with no branch has no rules at all: the only thing the schema
// then asks of it is that a `body` key exists, because `required` and
// `additionalProperties: false` sit at the top level. The body may be `{}`, or
// junk, or an array, and the sender may be whichever side you like.
//
// Two types are in that state — task_read and task_page. Both ship.
//
// This is the third time this repository has produced the same shape of dead
// guard, and the second time it has produced it *inside a fix for the previous
// one*: the guard added for action kinds closed by saying "enum parity is not
// body parity, and nothing here will catch it". This is what that sentence
// meant, measured.
//
// It is also the failure mode that reads as success. An unconstrained body
// accepts every fixture anybody writes for it, so the fixture check goes
// greener the harder somebody tries.
//
// Inputs: the type switch in validateBody, read out of this package's own
// source; and the `allOf` branch conditions in the checked-in envelope schema,
// read off disk.
//
// Output: the types that have a switch case and no branch, or a branch and no
// switch case.
//
// Neither side is hand-written here. If this fails, do not add a list to this
// file — there is no list in this file. Add a branch to
// protocol/schema/envelope.schema.json, or take the type out of validateBody.
//
// What this still does NOT guard: whether a branch that exists says the right
// thing. A branch can require the wrong fields and both these tests pass.

// typesWithABodyBranch reads the checked-in envelope schema and returns the
// message types named by each `allOf` branch that actually says something about
// the body.
//
// The "actually says something about the body" part is the whole test. Naming a
// type in a branch is not the same as constraining it: the branch at
// envelope.schema.json:26-29 names five types and only says whether they carry
// a `seq`. Counting that as coverage would let a type be listed there, get no
// body rules at all, and still satisfy this guard — which is the exact bug this
// file exists to catch, rebuilt inside the check for it. So a branch only
// counts when its `then` defines `body`.
//
// Unlike the action-kind guard next door, this one can reuse
// messageTypesInValidateBody as-is: validateBody switches on `message.Type`,
// which is the selector named "Type" that helper already matches.
func typesWithABodyBranch(t *testing.T) []string {
	t.Helper()

	path := filepath.Join("..", "..", "..", "..", "protocol", "schema", "envelope.schema.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("could not read the wire schema at %s: %v", path, err)
	}

	type typeCondition struct {
		Const string   `json:"const"`
		Enum  []string `json:"enum"`
	}
	var schema struct {
		AllOf []struct {
			If struct {
				Properties struct {
					Type typeCondition `json:"type"`
				} `json:"properties"`
			} `json:"if"`
			Then struct {
				Properties struct {
					Body json.RawMessage `json:"body"`
				} `json:"properties"`
			} `json:"then"`
		} `json:"allOf"`
	}
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatalf("could not parse the wire schema: %v", err)
	}

	var out []string
	for _, branch := range schema.AllOf {
		if len(branch.Then.Properties.Body) == 0 {
			// A branch that does not mention `body` constrains something else
			// about the envelope. It is not body coverage.
			continue
		}
		named := branch.If.Properties.Type
		if named.Const != "" {
			out = append(out, named.Const)
		}
		out = append(out, named.Enum...)
	}

	if len(out) == 0 {
		t.Fatal("the wire schema has no per-type body branches at all — if the allOf moved or changed shape, this test is no longer guarding anything and would pass forever")
	}
	sort.Strings(out)
	return out
}

func TestEveryMessageTypeTheWireAcceptsHasABodyShape(t *testing.T) {
	accepted := messageTypesInValidateBody(t)
	branched := typesWithABodyBranch(t)

	unconstrained := missingFrom(accepted, branched)
	if len(unconstrained) > 0 {
		t.Fatalf(
			"these message types are in the schema's enum but have no body branch, so the published contract accepts any body and any sender for them: %s\n"+
				"add an `if type == X then { sender, body }` branch to protocol/schema/envelope.schema.json for each",
			strings.Join(unconstrained, ", "),
		)
	}
}

// The other direction. A branch for a type validateBody has never heard of is
// dead weight that reads as coverage: it makes the schema look like it
// describes more of the wire than it does, and nothing would ever exercise it.
func TestTheSchemaDescribesNoMessageTypeTheWireWouldRefuse(t *testing.T) {
	accepted := messageTypesInValidateBody(t)
	branched := typesWithABodyBranch(t)

	orphaned := missingFrom(branched, accepted)
	if len(orphaned) > 0 {
		t.Fatalf(
			"the schema has body branches for these message types but validateBody would refuse them outright: %s\n"+
				"remove each branch from protocol/schema/envelope.schema.json, or teach validateBody about the type",
			strings.Join(orphaned, ", "),
		)
	}
}
