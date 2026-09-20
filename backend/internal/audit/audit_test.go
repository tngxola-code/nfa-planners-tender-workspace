package audit

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/tngxola-code/nfa-planners-tender-workspace/backend/internal/auth"
)

type memStore struct {
	rows []Row
	err  error
}

func (m *memStore) InsertAudit(_ context.Context, r Row) error {
	if m.err != nil {
		return m.err
	}
	m.rows = append(m.rows, r)
	return nil
}

func fixedNow() time.Time {
	return time.Date(2026, 9, 20, 9, 30, 0, 0, time.UTC)
}

func TestRecordRequiresClaims(t *testing.T) {
	r := New(&memStore{})
	err := r.Record(context.Background(), Event{Action: "search.create"})
	if !errors.Is(err, ErrNoActor) {
		t.Fatalf("want ErrNoActor, got %v", err)
	}
}

// A realm with no sub mapper issues tokens that verify but identify nobody.
// That must not produce an audit row.
func TestRecordRejectsClaimsWithoutSubject(t *testing.T) {
	r := New(&memStore{})
	ctx := auth.WithClaims(context.Background(), &auth.Claims{
		Username: "planner@local",
		AZP:      "nfa-test",
	})
	if err := r.Record(ctx, Event{Action: "search.create"}); !errors.Is(err, ErrNoActor) {
		t.Fatalf("want ErrNoActor for missing subject, got %v", err)
	}
}

func TestRecordRequiresAction(t *testing.T) {
	r := New(&memStore{})
	ctx := auth.WithClaims(context.Background(), &auth.Claims{Subject: "sub-1"})
	if err := r.Record(ctx, Event{}); err == nil {
		t.Fatal("want error for empty action, got nil")
	}
}

func TestRecordAttributesTheCaller(t *testing.T) {
	st := &memStore{}
	r := New(st)
	r.now = fixedNow

	ctx := auth.WithClaims(context.Background(), &auth.Claims{
		Subject:  "fe14a2ea-bf39-4019-aac1-3924767ac0cc",
		Username: "planner@local",
		AZP:      "nfa-test",
	})
	if err := r.Record(ctx, Event{
		Action:     "search.create",
		TargetType: "search",
		TargetID:   "s-123",
	}); err != nil {
		t.Fatalf("Record: %v", err)
	}
	if len(st.rows) != 1 {
		t.Fatalf("want 1 row, got %d", len(st.rows))
	}
	got := st.rows[0]
	if got.ActorSubject != "fe14a2ea-bf39-4019-aac1-3924767ac0cc" {
		t.Errorf("subject: %q", got.ActorSubject)
	}
	if got.ActorUsername != "planner@local" {
		t.Errorf("username: %q", got.ActorUsername)
	}
	if got.ActorClient != "nfa-test" {
		t.Errorf("client: %q", got.ActorClient)
	}
	if !got.At.Equal(fixedNow()) {
		t.Errorf("at: %v, want %v", got.At, fixedNow())
	}
}

func TestRecordPropagatesStoreError(t *testing.T) {
	sentinel := errors.New("db down")
	r := New(&memStore{err: sentinel})
	ctx := auth.WithClaims(context.Background(), &auth.Claims{Subject: "sub-1"})
	if err := r.Record(ctx, Event{Action: "x"}); !errors.Is(err, sentinel) {
		t.Fatalf("want sentinel, got %v", err)
	}
}
