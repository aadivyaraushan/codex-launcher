package contract

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// The wire schema drifted for months and every check stayed green.
//
// protocol/schema/envelope.schema.json declares the message types the wire is
// allowed to carry, and release/checks/protocol/schema_test.py runs it in CI.
// What that check actually does is validate the fixtures in protocol/fixtures
// against the schema. Both of those are hand-written. **Neither of them is the
// implementation.** Add a message type to validateBody below and to the
// Kotlin codec, ship it, and the schema never learns about it, because nobody
// wrote a fixture for it — so there is nothing for the check to reject.
//
// That is exactly the defect already found and fixed once in this repository,
// in runtime/contract_every_adapter_test.go: a hand-written list compared
// against a hand-written count, where both sides were the expectation. A guard
// whose two sides are both written by the same person at the same moment
// cannot fail, and this one did not. The schema is currently missing more than
// a dozen types the wire genuinely carries, including every capability and
// device-action message — the whole Direct Reply path.
//
// Inputs: the type switch in validateBody, read out of this package's own
// source; and the enum in the checked-in schema file, read off disk.
//
// Output: the names that appear on one side and not the other.
//
// Neither side is written by hand here. The switch is the implementation — it
// is what actually decides whether a frame is accepted — and the schema file
// is the artifact under test. Reading the source is deliberate and is not the
// same mistake as a test discovering its own expectation: the thing being
// compared is two independent artifacts that are supposed to agree, and the
// only way to keep them agreeing without a human remembering is to derive
// both.
//
// If this test fails, do not edit the list in this file. There is no list in
// this file. Add the missing type to the schema, or take it out of the switch.

// messageTypesInValidateBody reads this package's own validation.go, finds the
// type switch inside validateBody, and returns every message type it accepts.
func messageTypesInValidateBody(t *testing.T) []string {
	t.Helper()

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "validation.go", nil, 0)
	if err != nil {
		t.Fatalf("could not read validation.go to find out which types the wire accepts: %v", err)
	}

	var found []string
	ast.Inspect(file, func(n ast.Node) bool {
		decl, ok := n.(*ast.FuncDecl)
		if !ok || decl.Name.Name != "validateBody" {
			return true
		}
		// The first switch inside validateBody is the one on message.Type.
		// Every case in it is a type the wire is willing to accept.
		ast.Inspect(decl.Body, func(inner ast.Node) bool {
			sw, ok := inner.(*ast.SwitchStmt)
			if !ok {
				return true
			}
			sel, ok := sw.Tag.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "Type" {
				return true
			}
			for _, stmt := range sw.Body.List {
				clause, ok := stmt.(*ast.CaseClause)
				if !ok {
					continue
				}
				for _, expr := range clause.List {
					lit, ok := expr.(*ast.BasicLit)
					if !ok || lit.Kind != token.STRING {
						continue
					}
					name, err := strconv.Unquote(lit.Value)
					if err == nil {
						found = append(found, name)
					}
				}
			}
			return false
		})
		return false
	})

	if len(found) == 0 {
		t.Fatal("found no message types in validateBody's switch — this test can no longer see the thing it guards, which is worse than a drifting schema because it fails silently")
	}
	sort.Strings(found)
	return found
}

// messageTypesInSchema reads the checked-in envelope schema and returns the
// enum of message types it declares.
func messageTypesInSchema(t *testing.T) []string {
	t.Helper()

	path := filepath.Join("..", "..", "..", "..", "protocol", "schema", "envelope.schema.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("could not read the wire schema at %s: %v", path, err)
	}

	var schema struct {
		Properties struct {
			Type struct {
				Enum []string `json:"enum"`
			} `json:"type"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatalf("could not parse the wire schema: %v", err)
	}
	if len(schema.Properties.Type.Enum) == 0 {
		t.Fatal("the wire schema declares no message types at all — if the enum moved, this test is no longer guarding anything")
	}

	out := append([]string(nil), schema.Properties.Type.Enum...)
	sort.Strings(out)
	return out
}

func missingFrom(want, have []string) []string {
	present := make(map[string]bool, len(have))
	for _, name := range have {
		present[name] = true
	}
	var out []string
	for _, name := range want {
		if !present[name] {
			out = append(out, name)
		}
	}
	return out
}

func TestTheSchemaDeclaresEveryMessageTypeTheWireAccepts(t *testing.T) {
	accepted := messageTypesInValidateBody(t)
	declared := messageTypesInSchema(t)

	undeclared := missingFrom(accepted, declared)
	if len(undeclared) > 0 {
		t.Fatalf(
			"the wire accepts these message types but the schema does not declare them, so nothing in CI would notice if their shape changed: %s\n"+
				"add each to the type enum in protocol/schema/envelope.schema.json",
			strings.Join(undeclared, ", "),
		)
	}
}

func TestTheSchemaDeclaresNoMessageTypeTheWireWouldReject(t *testing.T) {
	accepted := messageTypesInValidateBody(t)
	declared := messageTypesInSchema(t)

	phantom := missingFrom(declared, accepted)
	if len(phantom) > 0 {
		t.Fatalf(
			"the schema declares these message types but the wire rejects them, so anything built against the schema would be dropped on arrival: %s\n"+
				"remove each from protocol/schema/envelope.schema.json, or add it to validateBody",
			strings.Join(phantom, ", "),
		)
	}
}

// A guard that has only ever been seen passing is not a guard. This one holds
// that the reading half still works: if validation.go is restructured so the
// switch can no longer be found, the tests above would compare an empty list
// against an empty list and pass while guarding nothing.
func TestTheReadingHalfOfThisGuardStillFindsRealTypes(t *testing.T) {
	accepted := messageTypesInValidateBody(t)

	for _, mustHave := range []string{"hello", "welcome", "ack", "device_action_result"} {
		found := false
		for _, name := range accepted {
			if name == mustHave {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("validateBody plainly handles %q, but this test could not see it — the switch reader is broken, not the schema", mustHave)
		}
	}
}
