package custody

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
)

// The custody gate, in code.
//
// The token store is the most valuable thing in Operator to steal: one
// break-in there is somebody else reading dozens of services for the whole
// user base at once. The security model in
// saved-results/custody-gate-token-store-security-model.md lists five things
// that clear the gate. This file pins the two that are code rather than
// process — encryption at rest with a key we can actually rotate, and
// per-user isolation — and it pins them as properties, not as a description.
//
// The rule the whole file is built around: a rotation path nobody has run is
// a rotation path that does not work.

func record(user, adapter, access string) Record {
	return Record{
		UserID:       user,
		AdapterID:    adapter,
		AccessToken:  access,
		RefreshToken: access + "-refresh",
		Scopes:       []string{"read"},
	}
}

func mustKey(t *testing.T, id string) MasterKey {
	t.Helper()
	k, err := NewLocalMasterKey(id)
	if err != nil {
		t.Fatalf("NewLocalMasterKey(%q): %v", id, err)
	}
	return k
}

func mustStore(t *testing.T, k MasterKey, rows ...Record) *Store {
	t.Helper()
	s := NewStore(k)
	for _, r := range rows {
		if err := s.Put(context.Background(), r); err != nil {
			t.Fatalf("Put(%s/%s): %v", r.UserID, r.AdapterID, err)
		}
	}
	return s
}

// ---- encryption at rest ------------------------------------------------

func TestStoredRowNeverHoldsTheTokenInTheClear(t *testing.T) {
	// The whole point of encrypting at rest. If the token string survives
	// anywhere in the stored bytes, everything below is decoration.
	const secret = "ya29.a0AfB_this-is-the-refresh-token"
	s := mustStore(t, mustKey(t, "key-1"), Record{
		UserID:       "u1",
		AdapterID:    "notion",
		AccessToken:  secret,
		RefreshToken: secret + "-refresh",
		Scopes:       []string{"read", "write"},
	})

	blob, ok := s.Ciphertext("u1", "notion")
	if !ok {
		t.Fatal("no stored row for u1/notion")
	}
	if len(blob) == 0 {
		t.Fatal("the stored row is empty")
	}
	if strings.Contains(string(blob), secret) {
		t.Fatal("the token is sitting in the stored bytes in the clear")
	}
	// The scopes tell an attacker exactly what a stolen token can reach, so
	// they are inside the envelope too, not alongside it.
	if strings.Contains(string(blob), "write") {
		t.Fatal("the granted scopes are readable without the key")
	}
}

func TestTwoUsersWithTheSameTokenDoNotProduceTheSameStoredBytes(t *testing.T) {
	// If identical plaintext encrypted to identical ciphertext, an attacker
	// with only the stored rows could tell which accounts share a token
	// without decrypting anything.
	s := mustStore(t, mustKey(t, "key-1"),
		record("u1", "notion", "same-token"),
		record("u2", "notion", "same-token"),
	)
	a, _ := s.Ciphertext("u1", "notion")
	b, _ := s.Ciphertext("u2", "notion")
	if string(a) == string(b) {
		t.Fatal("two rows with the same token encrypted to the same bytes")
	}
}

func TestARowReadsBackExactlyAsItWasWritten(t *testing.T) {
	want := Record{
		UserID:       "u1",
		AdapterID:    "notion",
		AccessToken:  "access\x00with a null and a — dash",
		RefreshToken: "refresh\nwith a newline",
		Scopes:       []string{"read", "write", "offline_access"},
	}
	s := mustStore(t, mustKey(t, "key-1"), want)

	got, err := NewBroker(s).Token(context.Background(), "u1", "notion")
	if err != nil {
		t.Fatalf("Token: %v", err)
	}
	if got.Access != want.AccessToken || got.Refresh != want.RefreshToken {
		t.Fatalf("token came back changed: %+v", got)
	}
	if !reflect.DeepEqual(got.Scopes, want.Scopes) {
		t.Fatalf("scopes came back as %v, wrote %v", got.Scopes, want.Scopes)
	}
}

func TestTheWrongMasterKeyCannotReadARow(t *testing.T) {
	s := mustStore(t, mustKey(t, "key-1"), record("u1", "notion", "t"))
	blob, _ := s.Ciphertext("u1", "notion")

	if _, err := ReadWith(mustKey(t, "key-2"), blob); err == nil {
		t.Fatal("a different master key read the row")
	}
}

// ---- per-user isolation -------------------------------------------------

func TestABrokerHandsBackOnlyTheRowItWasAskedFor(t *testing.T) {
	s := mustStore(t, mustKey(t, "key-1"),
		record("u1", "notion", "u1-token"),
		record("u2", "notion", "u2-token"),
	)
	got, err := NewBroker(s).Token(context.Background(), "u1", "notion")
	if err != nil {
		t.Fatalf("Token: %v", err)
	}
	if got.Access != "u1-token" {
		t.Fatalf("asked for u1 and got %q", got.Access)
	}
	if got.UserID != "u1" {
		t.Fatalf("the token does not say whose it is: %+v", got)
	}
}

func TestAskingForATokenThatIsNotThereIsAnErrorNotAnEmptyToken(t *testing.T) {
	// An empty token that looks valid is how an adapter ends up making an
	// unauthenticated call and reporting a puzzling vendor error instead of
	// "you are not connected".
	s := mustStore(t, mustKey(t, "key-1"), record("u1", "notion", "t"))
	_, err := NewBroker(s).Token(context.Background(), "u2", "notion")
	if !errors.Is(err, ErrNoToken) {
		t.Fatalf("want ErrNoToken, got %v", err)
	}
}

func TestNothingAnAdapterHoldsCanReachASecondToken(t *testing.T) {
	// The security model's words: adapter code never queries the store at
	// all. It receives the one token it needs and has no way to ask for a
	// second one. That is a structural property, so it is checked
	// structurally rather than by remembering not to add the method.
	for _, f := range reflect.VisibleFields(reflect.TypeOf(Token{})) {
		switch f.Type.Kind() {
		case reflect.String:
		case reflect.Slice:
			if f.Type.Elem().Kind() != reflect.String {
				t.Fatalf("Token.%s carries something other than plain text", f.Name)
			}
		default:
			t.Fatalf("Token.%s is a %s — a token must carry no handles", f.Name, f.Type.Kind())
		}
	}
}

func TestNoExportedMethodHandsBackEveryRowAtOnce(t *testing.T) {
	// A "list all tokens" method is the shape a breach uses. Rotation needs
	// to walk every row, so that walk stays unexported and inside this
	// package.
	for _, typ := range []reflect.Type{reflect.TypeOf(&Store{}), reflect.TypeOf(&Broker{})} {
		for i := 0; i < typ.NumMethod(); i++ {
			m := typ.Method(i)
			for j := 0; j < m.Type.NumOut(); j++ {
				out := m.Type.Out(j)
				if out.Kind() == reflect.Map {
					t.Fatalf("%s.%s hands back a map of rows", typ, m.Name)
				}
				if out.Kind() == reflect.Slice && out.Elem() != reflect.TypeOf(byte(0)) {
					t.Fatalf("%s.%s hands back a slice of rows", typ, m.Name)
				}
			}
		}
	}
}

func TestTheMasterKeyNeverHandsOutItsOwnKeyMaterial(t *testing.T) {
	k := mustKey(t, "key-1")
	typ := reflect.TypeOf(k)
	for i := 0; i < typ.NumMethod(); i++ {
		m := typ.Method(i)
		if m.Type.NumIn() == 1 { // receiver only: an accessor
			for j := 0; j < m.Type.NumOut(); j++ {
				if m.Type.Out(j) == reflect.TypeOf([]byte(nil)) {
					t.Fatalf("MasterKey.%s hands out raw key bytes", m.Name)
				}
			}
		}
	}
}

// ---- the rotation drill, which is the point of the file ------------------

func TestRotationMovesEveryRowAndEveryRowStillReads(t *testing.T) {
	old := mustKey(t, "key-1")
	s := mustStore(t, old,
		record("u1", "notion", "u1-token"),
		record("u2", "notion", "u2-token"),
		record("u1", "apple-notes", "u1-notes-token"),
	)

	proof, err := Rotate(context.Background(), s, mustKey(t, "key-2"))
	if err != nil {
		t.Fatalf("Rotate: %v", err)
	}
	if proof.RowsMoved != 3 {
		t.Fatalf("moved %d rows, store holds 3", proof.RowsMoved)
	}
	if proof.FromKeyID != "key-1" || proof.ToKeyID != "key-2" {
		t.Fatalf("the proof does not name both keys: %+v", proof)
	}
	if !proof.Verified {
		t.Fatal("the rotation did not verify what it wrote")
	}
	if len(proof.Rows) != 3 {
		t.Fatalf("the proof lists %d rows, moved 3", len(proof.Rows))
	}

	b := NewBroker(s)
	for _, want := range []struct{ user, adapter, token string }{
		{"u1", "notion", "u1-token"},
		{"u2", "notion", "u2-token"},
		{"u1", "apple-notes", "u1-notes-token"},
	} {
		got, err := b.Token(context.Background(), want.user, want.adapter)
		if err != nil {
			t.Fatalf("after rotation, %s/%s: %v", want.user, want.adapter, err)
		}
		if got.Access != want.token {
			t.Fatalf("after rotation %s/%s reads %q, wrote %q", want.user, want.adapter, got.Access, want.token)
		}
	}
}

func TestAfterRotationTheOldKeyCanNoLongerReadAnything(t *testing.T) {
	// This is the difference between a rotation and adding a second key. If
	// the old key still opens rows, destroying it is the only thing that
	// finishes the job — and destroying keys is exactly the step people skip.
	old := mustKey(t, "key-1")
	s := mustStore(t, old, record("u1", "notion", "u1-token"))

	if _, err := Rotate(context.Background(), s, mustKey(t, "key-2")); err != nil {
		t.Fatalf("Rotate: %v", err)
	}

	blob, _ := s.Ciphertext("u1", "notion")
	if _, err := ReadWith(old, blob); err == nil {
		t.Fatal("the retired key still reads the row")
	}
	if s.MasterKeyID() != "key-2" {
		t.Fatalf("the store still names %q as its key", s.MasterKeyID())
	}
}

func TestARotationThatFailsPartWayLeavesTheStoreUntouched(t *testing.T) {
	// The dangerous failure is a half-rotated store: some rows under the old
	// key, some under the new, and no single key that opens all of them.
	// Every row is re-encrypted and verified before anything is committed.
	old := mustKey(t, "key-1")
	rows := []Record{
		record("u1", "notion", "u1-token"),
		record("u2", "notion", "u2-token"),
		record("u3", "notion", "u3-token"),
	}
	s := mustStore(t, old, rows...)

	broken := &failingKey{MasterKey: mustKey(t, "key-2"), failAfter: 2}
	if _, err := Rotate(context.Background(), s, broken); err == nil {
		t.Fatal("a rotation that could not encrypt every row reported success")
	}

	if s.MasterKeyID() != "key-1" {
		t.Fatalf("the store switched keys despite the failure: %q", s.MasterKeyID())
	}
	b := NewBroker(s)
	for _, r := range rows {
		got, err := b.Token(context.Background(), r.UserID, r.AdapterID)
		if err != nil {
			t.Fatalf("%s/%s is unreadable after a failed rotation: %v", r.UserID, r.AdapterID, err)
		}
		if got.Access != r.AccessToken {
			t.Fatalf("%s/%s changed during a failed rotation", r.UserID, r.AdapterID)
		}
	}
}

func TestARotationThatCannotVerifyWhatItWroteIsRefused(t *testing.T) {
	// Encrypting successfully is not proof the row can be read back. The
	// rotation reads every new row before it commits, and a row that does
	// not come back as it went in aborts the whole thing.
	s := mustStore(t, mustKey(t, "key-1"), record("u1", "notion", "u1-token"))

	_, err := Rotate(context.Background(), s, &lyingKey{MasterKey: mustKey(t, "key-2")})
	if !errors.Is(err, ErrRotationVerifyFailed) {
		t.Fatalf("want ErrRotationVerifyFailed, got %v", err)
	}
	if s.MasterKeyID() != "key-1" {
		t.Fatal("the store committed to a key whose rows it could not read back")
	}
}

func TestRotatingOntoTheSameKeyIsRefused(t *testing.T) {
	// It would report a clean rotation and leave the key it was meant to
	// retire in service — the one failure mode that looks like success.
	k := mustKey(t, "key-1")
	s := mustStore(t, k, record("u1", "notion", "t"))
	if _, err := Rotate(context.Background(), s, k); !errors.Is(err, ErrSameKey) {
		t.Fatalf("want ErrSameKey, got %v", err)
	}
}

func TestRotatingAnEmptyStoreSucceedsAndSaysItMovedNothing(t *testing.T) {
	// The first rotation drill runs before there are any real users. It has
	// to be runnable then, and it has to be honest that it proved less.
	s := NewStore(mustKey(t, "key-1"))
	proof, err := Rotate(context.Background(), s, mustKey(t, "key-2"))
	if err != nil {
		t.Fatalf("Rotate on an empty store: %v", err)
	}
	if proof.RowsMoved != 0 {
		t.Fatalf("an empty store moved %d rows", proof.RowsMoved)
	}
	if s.MasterKeyID() != "key-2" {
		t.Fatal("an empty store did not take the new key")
	}
}

// ---- deleting, proven by re-reading -------------------------------------

func TestDeleteIsProvenByReadingTheStoreBackNotByReportingSuccess(t *testing.T) {
	// The same rule the consent package's revoke follows: a delete that
	// reports success is not evidence. Reading the store afterwards is.
	s := mustStore(t, mustKey(t, "key-1"), record("u1", "notion", "u1-token"))

	proof, err := s.Delete(context.Background(), "u1", "notion")
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if !proof.Gone {
		t.Fatal("the delete proof does not say the row is gone")
	}
	if _, ok := s.Ciphertext("u1", "notion"); ok {
		t.Fatal("the ciphertext survived the delete")
	}
	if _, err := NewBroker(s).Token(context.Background(), "u1", "notion"); !errors.Is(err, ErrNoToken) {
		t.Fatalf("the token survived the delete: %v", err)
	}
}

func TestDeletingSomethingThatIsNotThereIsNotAnError(t *testing.T) {
	// A mass revoke runs over everyone, including accounts already cleaned
	// up. It must not stop at the first one that is already gone.
	s := NewStore(mustKey(t, "key-1"))
	proof, err := s.Delete(context.Background(), "u1", "notion")
	if err != nil {
		t.Fatalf("Delete of a missing row: %v", err)
	}
	if !proof.Gone {
		t.Fatal("a row that was never there is not reported as gone")
	}
}

// ---- the store is shared, so it has to survive being shared -------------

func TestTheStoreSurvivesBeingUsedFromManyPlacesAtOnce(t *testing.T) {
	s := NewStore(mustKey(t, "key-1"))
	b := NewBroker(s)
	ctx := context.Background()

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			user := fmt.Sprintf("u%d", i)
			if err := s.Put(ctx, record(user, "notion", user+"-token")); err != nil {
				t.Errorf("Put(%s): %v", user, err)
				return
			}
			got, err := b.Token(ctx, user, "notion")
			if err != nil {
				t.Errorf("Token(%s): %v", user, err)
				return
			}
			if got.Access != user+"-token" {
				t.Errorf("Token(%s) returned %q", user, got.Access)
			}
		}(i)
	}
	wg.Wait()
}

// ---- stubs the tests own ------------------------------------------------

// failingKey wraps a real key and stops working after failAfter calls, to
// stand in for a key service that goes away mid-rotation.
type failingKey struct {
	MasterKey
	failAfter int
	mu        sync.Mutex
	calls     int
}

func (k *failingKey) ID() string { return k.MasterKey.ID() }

func (k *failingKey) Wrap(dataKey []byte) ([]byte, error) {
	k.mu.Lock()
	k.calls++
	n := k.calls
	k.mu.Unlock()
	if n > k.failAfter {
		return nil, errors.New("key service unavailable")
	}
	return k.MasterKey.Wrap(dataKey)
}

// lyingKey encrypts happily and then cannot decrypt what it produced — the
// shape of a misconfigured key service that accepts writes and loses reads.
type lyingKey struct {
	MasterKey
}

func (k *lyingKey) ID() string { return k.MasterKey.ID() }

func (k *lyingKey) Unwrap([]byte) ([]byte, error) {
	return nil, errors.New("cannot decrypt")
}
