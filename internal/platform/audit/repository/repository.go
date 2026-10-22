package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	dbgen "gopanel/internal/db/generated"
	"gopanel/internal/platform/id"
)

type Entry struct {
	ActorID, Action, EntityType, EntityID, Outcome, RequestID string
	Metadata                                                  json.RawMessage
}
type Repository struct{ q *dbgen.Queries }

func New(db dbgen.DBTX) *Repository        { return &Repository{q: dbgen.New(db)} }
func optional(value string) sql.NullString { return sql.NullString{String: value, Valid: value != ""} }

// Append must receive only allowlisted metadata from the service, never a request body.
func (r *Repository) Append(ctx context.Context, tenantID string, e Entry) error {
	if tenantID == "" {
		return fmt.Errorf("tenant is required")
	}
	if len(e.Metadata) > 0 && !json.Valid(e.Metadata) {
		return fmt.Errorf("invalid audit metadata")
	}
	err := r.q.CreateAudit(ctx, dbgen.CreateAuditParams{ID: id.New(), TenantID: optional(tenantID), ActorUserID: optional(e.ActorID), Action: e.Action, EntityType: e.EntityType, EntityID: optional(e.EntityID), Outcome: e.Outcome, RequestID: optional(e.RequestID), Metadata: e.Metadata})
	if err != nil {
		return fmt.Errorf("append audit: %w", err)
	}
	return nil
}
func (r *Repository) List(ctx context.Context, tenantID string, limit, offset int32) ([]dbgen.AuditLog, error) {
	if tenantID == "" || limit < 1 || limit > 50 || offset < 0 {
		return nil, fmt.Errorf("invalid audit list options")
	}
	rows, err := r.q.ListAudit(ctx, dbgen.ListAuditParams{TenantID: optional(tenantID), Limit: limit, Offset: offset})
	if err != nil {
		return nil, fmt.Errorf("list audit: %w", err)
	}
	return rows, nil
}
