package audit

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
)

// PostgresStore persists audit rows to the audit_log table. See
// migrations/0001_audit_log.sql for the schema.
type PostgresStore struct {
	DB *sql.DB
}

func (p *PostgresStore) InsertAudit(ctx context.Context, r Row) error {
	var meta []byte
	if r.Metadata != nil {
		b, err := json.Marshal(r.Metadata)
		if err != nil {
			return fmt.Errorf("audit: marshal metadata: %w", err)
		}
		meta = b
	}
	_, err := p.DB.ExecContext(ctx, `
		INSERT INTO audit_log
			(at, actor_sub, actor_username, actor_client, action, target_type, target_id, metadata)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
		r.At, r.ActorSubject, r.ActorUsername, r.ActorClient,
		r.Action, r.TargetType, r.TargetID, meta,
	)
	if err != nil {
		return fmt.Errorf("audit: insert: %w", err)
	}
	return nil
}
