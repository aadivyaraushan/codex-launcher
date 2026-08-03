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

// The sibling guard in schema_matches_the_wire_test.go compares message *type*
// names and passes. It says nothing about what is inside an `action` body, and
// inside that body the same drift happened again: the wire accepts thirteen
// action kinds and the checked-in schema has heard of six.
//
// The seven it has never heard of include every capability action — the whole
// "ask my computer to do a thing in an app" path — plus archive, fork and
// rename.
//
// Nothing caught it, and could not have, because the only thing that ever
// reads the schema is release/checks/protocol/schema_test.py, and all that
// does is validate the hand-written examples in protocol/fixtures. A kind
// nobody wrote an example for is a kind the schema is never asked about. Both
// sides of that check are written by the same person in the same sitting,
// which is the third time this repository has produced that exact shape of
// dead guard.
//
// Inputs: the `switch kind` inside validateAction, read out of this package's
// own source; and the `kind` values in the checked-in action schema, read off
// disk.
//
// Output: the kinds that appear on one side and not the other.
//
// Neither side is hand-written here, which is the whole point. If this test
// fails, do not add a list to this file — there is no list in this file. Add
// the kind to protocol/schema/action.schema.json, or take it out of the
// validator.
//
// What this does NOT guard: the *shape* of each branch. The schema can list a
// kind and still require the wrong fields for it, and this test will pass. See
// the start_turn new-task shape, which was wrong the entire time the kind was
// "known".

// actionKindsInValidateAction reads this package's own validation.go, finds the
// switch inside validateAction, and returns every action kind it accepts.
//
// It cannot reuse the sibling walk. That one matches a switch whose tag is a
// selector expression named "Type" (message.Type). validateAction switches on
// a bare local variable — `switch kind {` — so the tag is an identifier and
// the sibling matcher would silently find nothing. This one keys on the
// enclosing function name instead and takes the first switch inside it.
func actionKindsInValidateAction(t *testing.T) []string {
	t.Helper()

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "validation.go", nil, 0)
	if err != nil {
		t.Fatalf("could not read validation.go to find out which action kinds the wire accepts: %v", err)
	}

	var found []string
	var done bool
	ast.Inspect(file, func(n ast.Node) bool {
		decl, ok := n.(*ast.FuncDecl)
		if !ok || decl.Name.Name != "validateAction" {
			return true
		}
		ast.Inspect(decl.Body, func(inner ast.Node) bool {
			if done {
				return false
			}
			sw, ok := inner.(*ast.SwitchStmt)
			if !ok {
				return true
			}
			// The first switch inside validateAction is the one on the action
			// kind. Guarded by name so a later refactor that switches on
			// something else first fails loudly rather than quietly reading
			// the wrong list.
			ident, ok := sw.Tag.(*ast.Ident)
			if !ok || ident.Name != "kind" {
				return true
			}
			for _, stmt := range sw.Body.List {
				clause, ok := stmt.(*ast.CaseClause)
				if !ok {
					continue
				}
				// One clause can name several kinds
				// (`case "archive_task", "fork_task":`), so every string
				// literal in the clause counts.
				for _, expr := range clause.List {
					lit, ok := expr.(*ast.BasicLit)
					if !ok || lit.Kind != token.STRING {
						continue
					}
					if name, err := strconv.Unquote(lit.Value); err == nil {
						found = append(found, name)
					}
				}
			}
			done = true
			return false
		})
		return false
	})

	if len(found) == 0 {
		t.Fatal("found no action kinds in validateAction's switch — this test can no longer see the thing it guards, which is worse than a drifting schema because it fails silently")
	}
	sort.Strings(found)
	return found
}

// actionKindsInSchema reads the checked-in action schema and returns every kind
// its branches name, whether by `const` or by `enum`.
func actionKindsInSchema(t *testing.T) []string {
	t.Helper()

	path := filepath.Join("..", "..", "..", "..", "protocol", "schema", "action.schema.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("could not read the action schema at %s: %v", path, err)
	}

	var schema struct {
		OneOf []struct {
			Properties struct {
				Kind struct {
					Const string   `json:"const"`
					Enum  []string `json:"enum"`
				} `json:"kind"`
			} `json:"properties"`
		} `json:"oneOf"`
	}
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatalf("could not parse the action schema: %v", err)
	}

	var out []string
	for _, branch := range schema.OneOf {
		kind := branch.Properties.Kind
		if kind.Const != "" {
			out = append(out, kind.Const)
		}
		out = append(out, kind.Enum...)
	}
	if len(out) == 0 {
		t.Fatal("the action schema names no kinds at all — if its shape moved, this test is no longer guarding anything")
	}
	sort.Strings(out)
	return out
}

func TestTheSchemaDeclaresEveryActionKindTheWireAccepts(t *testing.T) {
	accepted := actionKindsInValidateAction(t)
	declared := actionKindsInSchema(t)

	undeclared := missingFrom(accepted, declared)
	if len(undeclared) > 0 {
		t.Fatalf(
			"the wire accepts these action kinds but the schema does not declare them, so a fixture carrying one cannot be validated and nothing in CI would notice if its shape changed: %s\n"+
				"add a branch for each to protocol/schema/action.schema.json",
			strings.Join(undeclared, ", "),
		)
	}
}

// The other direction. A kind the schema declares and the validator would
// refuse is a promise the wire does not keep, and it is the more dangerous of
// the two: a frame that validates against the published contract and is then
// rejected at runtime looks like a bug in the sender.
func TestTheSchemaDeclaresNoActionKindTheWireWouldRefuse(t *testing.T) {
	accepted := actionKindsInValidateAction(t)
	declared := actionKindsInSchema(t)

	unaccepted := missingFrom(declared, accepted)
	if len(unaccepted) > 0 {
		t.Fatalf(
			"the schema declares these action kinds but the wire would refuse them: %s\n"+
				"remove each from protocol/schema/action.schema.json, or teach validateAction about it",
			strings.Join(unaccepted, ", "),
		)
	}
}
