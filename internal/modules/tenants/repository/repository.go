package repository

import (
	"context"
	"fmt"
	dbgen "gopanel/internal/db/generated"
)

type Repository struct{ q *dbgen.Queries }

func New(db dbgen.DBTX) *Repository { return &Repository{q: dbgen.New(db)} }
func (r *Repository) Get(ctx context.Context, tenantID string) (dbgen.Tenant, error) {
	if tenantID == "" {
		return dbgen.Tenant{}, fmt.Errorf("tenant is required")
	}
	value, err := r.q.GetTenant(ctx, tenantID)
	if err != nil {
		return value, fmt.Errorf("get tenant: %w", err)
	}
	return value, nil
}

// ActiveForUser is an identity lookup used before the user has selected a tenant.
func (r *Repository) ActiveForUser(ctx context.Context, userID string) ([]dbgen.ListActiveTenantsForUserRow, error) {
	rows, err := r.q.ListActiveTenantsForUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list memberships for user: %w", err)
	}
	return rows, nil
}
