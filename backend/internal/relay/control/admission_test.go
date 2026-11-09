package control

import (
	"context"
	"testing"
)

func TestAdmissionCannotRegisterAfterRevocation(t *testing.T) {
	store := NewStore(nil)
	old := &State{ControlRevision: 1}
	current := &State{ControlRevision: 2}
	store.current.Store(old)
	allowed := func(s *State) bool { return s == old }
	ctx, done, err := store.Register(context.Background(), allowed)
	if err != nil {
		t.Fatal(err)
	}
	defer done()
	store.mu.Lock()
	store.current.Store(current)
	store.cancelInvalid(current)
	store.mu.Unlock()
	if ctx.Err() == nil {
		t.Fatal("revocation left registered request running")
	}
	if _, _, err = store.Register(context.Background(), allowed); err == nil {
		t.Fatal("late registration escaped revocation")
	}
}
