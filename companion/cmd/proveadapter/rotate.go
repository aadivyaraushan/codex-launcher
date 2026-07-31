package main

import (
	"context"
	"flag"
	"fmt"

	"github.com/codex-launcher/codex-launcher/companion/internal/capability/custody"
)

// ---- rotate -----------------------------------------------------------------

// rotationRows are the fake rows the drill puts into the store. The values
// are obviously not real credentials — there is no code path in this
// function that reads a flag, an environment variable, or stdin, so there
// is no way a real token could end up here even by accident.
var rotationRows = []custody.Record{
	{UserID: "u1", AdapterID: "notion", AccessToken: "fake-token-u1-notion", RefreshToken: "fake-refresh-u1-notion", Scopes: []string{"read"}},
	{UserID: "u2", AdapterID: "notion", AccessToken: "fake-token-u2-notion", RefreshToken: "fake-refresh-u2-notion", Scopes: []string{"read"}},
	{UserID: "u1", AdapterID: "apple-notes", AccessToken: "fake-token-u1-notes", RefreshToken: "fake-refresh-u1-notes", Scopes: []string{"read", "write"}},
}

// unreachableMasterKey stands in for a key service that has gone away
// mid-rotation: it reports a key id distinct from whatever is in service,
// so custody.Rotate treats it as a genuine new key, but wrapping a data key
// under it always fails. It exists to drive the failure half of the drill —
// the plan's exit test wants the dangerous path exercised, not just the
// happy one.
type unreachableMasterKey struct {
	custody.MasterKey
	id string
}

func (k unreachableMasterKey) ID() string { return k.id }

func (k unreachableMasterKey) Wrap([]byte) ([]byte, error) {
	return nil, fmt.Errorf("simulated key service outage: %s is unreachable", k.id)
}

// readAllRows reads every row in rotationRows through b and reports only
// whether each one read back correctly — never the token value itself, in
// the transcript or in an error.
func readAllRows(ctx context.Context, b *custody.Broker) error {
	for _, r := range rotationRows {
		got, err := b.Token(ctx, r.UserID, r.AdapterID)
		if err != nil {
			return fmt.Errorf("%s/%s did not read back: %w", r.UserID, r.AdapterID, err)
		}
		if got.Access != r.AccessToken || got.Refresh != r.RefreshToken {
			return fmt.Errorf("%s/%s read back changed from what was written", r.UserID, r.AdapterID)
		}
		line("%s/%s: reads OK", r.UserID, r.AdapterID)
	}
	return nil
}

// runRotate is the key-rotation drill: it stands up a store, proves it
// reads, rotates it onto a second key, proves the old key is now refused
// and every row still reads under the new key, and then runs the failure
// case — a rotation that cannot finish — and proves the store was left
// untouched by it. This command takes no flag, reads no environment
// variable, and reads no stdin that could carry a credential; every token
// value below is a fake string chosen for this run.
func runRotate(args []string) error {
	fs := flag.NewFlagSet("rotate", flag.ExitOnError)
	if err := fs.Parse(args); err != nil {
		return err
	}
	ctx := context.Background()

	step(1, "Build a store under the first master key and put in several rows")
	keyOne, err := custody.NewLocalMasterKey("key-1")
	if err != nil {
		return fmt.Errorf("build key-1: %w", err)
	}
	store := custody.NewStore(keyOne)
	for _, r := range rotationRows {
		if err := store.Put(ctx, r); err != nil {
			return fmt.Errorf("put %s/%s: %w", r.UserID, r.AdapterID, err)
		}
		line("wrote %s/%s under %s", r.UserID, r.AdapterID, store.MasterKeyID())
	}

	step(2, "Read every row back under the working key")
	broker := custody.NewBroker(store)
	if err := readAllRows(ctx, broker); err != nil {
		return fmt.Errorf("read before rotation: %w", err)
	}

	step(3, "Create a second master key and rotate onto it")
	keyTwo, err := custody.NewLocalMasterKey("key-2")
	if err != nil {
		return fmt.Errorf("build key-2: %w", err)
	}
	proof, err := custody.Rotate(ctx, store, keyTwo)
	if err != nil {
		return fmt.Errorf("rotation was refused: %w", err)
	}

	step(4, "The rotation proof")
	line("from key:   %s", proof.FromKeyID)
	line("to key:     %s", proof.ToKeyID)
	line("rows moved: %d", proof.RowsMoved)
	line("verified:   %t", proof.Verified)
	line("took:       %s", proof.Duration)
	for _, r := range proof.Rows {
		line("  - %s/%s: verified=%t", r.UserID, r.AdapterID, r.Verified)
	}

	step(5, "Prove the retired key can no longer read a row")
	blob, ok := store.Ciphertext(rotationRows[0].UserID, rotationRows[0].AdapterID)
	if !ok {
		return fmt.Errorf("no stored row for %s/%s after rotation", rotationRows[0].UserID, rotationRows[0].AdapterID)
	}
	if _, err := custody.ReadWith(keyOne, blob); err == nil {
		return fmt.Errorf("the retired key %s still reads %s/%s — this is exactly the failure this step exists to catch", keyOne.ID(), rotationRows[0].UserID, rotationRows[0].AdapterID)
	} else {
		line("REFUSED, as it should be: the retired key %s cannot read %s/%s: %v", keyOne.ID(), rotationRows[0].UserID, rotationRows[0].AdapterID, err)
	}

	step(6, "Read every row back under the new key")
	if err := readAllRows(ctx, broker); err != nil {
		return fmt.Errorf("read after rotation: %w", err)
	}

	step(7, "Run the failure case: a rotation that cannot complete")
	broken := unreachableMasterKey{MasterKey: keyOne, id: "key-3-unreachable"}
	line("attempting to rotate %s onto %s, which is about to fail every wrap call", store.MasterKeyID(), broken.ID())
	if _, err := custody.Rotate(ctx, store, broken); err == nil {
		return fmt.Errorf("a rotation onto an unreachable key reported success — exactly the failure this step exists to catch")
	} else {
		line("REFUSED, as it should be: %v", err)
	}
	if store.MasterKeyID() != proof.ToKeyID {
		return fmt.Errorf("the store's key changed after a failed rotation: now %q, want %q", store.MasterKeyID(), proof.ToKeyID)
	}
	line("the store is still on %s, untouched by the failed rotation", store.MasterKeyID())
	if err := readAllRows(ctx, broker); err != nil {
		return fmt.Errorf("read after the failed rotation: %w", err)
	}

	verdict("rotation proven: %d rows moved from %s to %s and verified in %s; the retired key was refused; a rotation that could not finish left the store untouched on %s",
		proof.RowsMoved, proof.FromKeyID, proof.ToKeyID, proof.Duration, store.MasterKeyID())
	return nil
}
