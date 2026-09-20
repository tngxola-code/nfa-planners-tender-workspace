package audit

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/tngxola-code/nfa-planners-tender-workspace/backend/internal/auth"
)

// ErrNoActor is returned when Record is called on a context that carries no
// verified claims. Writing an audit row without an actor is the exact
// failure a missing claims mapper produces, silently, one NULL at a time.
// It must be loud.
var ErrNoActor = errors.New("audit: no actor in context")

// Event is what a caller knows about the thing being audited. The actor is
// deliberately not a field: it is read from the context, where only the
// authentication middleware can have put it, so a handler cannot forge it.
type Event struct {
	Action     string
	TargetType string
	TargetID   string
	Metadata   map[string]any
}

// Row is what gets persisted.
type Row struct {
	At            time.Time
	ActorSubject  string
	ActorUsername string
	ActorClient   string
	Action        string
	TargetType    string
	TargetID      string
	Metadata      map[string]any
}

// Store is the persistence seam. PostgresStore is the production impl;
// tests use an in-memory one.
type Store interface {
	InsertAudit(ctx context.Context, r Row) error
}

// Recorder is the entry point the rest of the app uses.
type Recorder struct {
	store Store
	now   func() time.Time
}

// New returns a Recorder backed by s.
func New(s Store) *Recorder {
	return &Recorder{store: s, now: time.Now}
}

// Record writes an audit row attributed to the verified caller in ctx.
//
// The actor comes from auth.FromContext, the same claims the authentication
// middleware attached and RequireScope authorised against. There is no
// second identity type to keep in step: if a request was authenticated, it
// can be audited.
func (r *Recorder) Record(ctx context.Context, ev Event) error {
	c, ok := auth.FromContext(ctx)
	if !ok {
		return ErrNoActor
	}
	if c.Subject == "" {
		// A token that verified but carries no subject. This is what a
		// realm with no sub mapper produces; refuse rather than write an
		// anonymous row.
		return fmt.Errorf("%w: claims have no subject", ErrNoActor)
	}
	if ev.Action == "" {
		return errors.New("audit: action is required")
	}
	return r.store.InsertAudit(ctx, Row{
		At:            r.now().UTC(),
		ActorSubject:  c.Subject,
		ActorUsername: c.Username,
		ActorClient:   c.AZP,
		Action:        ev.Action,
		TargetType:    ev.TargetType,
		TargetID:      ev.TargetID,
		Metadata:      ev.Metadata,
	})
}
