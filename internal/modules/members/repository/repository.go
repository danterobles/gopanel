package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	dbgen "gopanel/internal/db/generated"
	"gopanel/internal/platform/id"
)

var ErrLastOwner = errors.New("tenant must retain an active owner")
var ErrNotFound = errors.New("membership not found")
var ErrInvalidRole = errors.New("invalid membership role")

type Repository struct{ db *sql.DB }

func New(db *sql.DB) *Repository { return &Repository{db: db} }
func validRole(role string) bool { return role == "owner" || role == "admin" || role == "viewer" }

func (r *Repository) List(ctx context.Context, tenantID string) ([]dbgen.TenantMembership, error) {
	if tenantID == "" {
		return nil, fmt.Errorf("tenant is required")
	}
	rows, err := dbgen.New(r.db).ListMemberships(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list memberships: %w", err)
	}
	return rows, nil
}

// Change enforces persistence invariants. The caller must authorize the actor first.
func (r *Repository) Change(ctx context.Context, tenantID, membershipID, role string, active bool) error {
	if !validRole(role) {
		return ErrInvalidRole
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin membership transaction: %w", err)
	}
	defer tx.Rollback()
	q := dbgen.New(tx)
	if _, err = q.LockTenant(ctx, tenantID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return fmt.Errorf("lock tenant: %w", err)
	}
	n, err := q.UpdateMembership(ctx, dbgen.UpdateMembershipParams{TenantID: tenantID, ID: membershipID, Role: role, IsActive: active})
	if err != nil {
		return fmt.Errorf("update membership: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	owners, err := q.CountActiveOwners(ctx, tenantID)
	if err != nil {
		return fmt.Errorf("count owners: %w", err)
	}
	if owners < 1 {
		return ErrLastOwner
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit membership: %w", err)
	}
	return nil
}

func (r *Repository) Add(ctx context.Context, tenantID, userID, role string) (string, error) {
	if !validRole(role) {
		return "", ErrInvalidRole
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("begin membership transaction: %w", err)
	}
	defer tx.Rollback()
	q := dbgen.New(tx)
	if _, err = q.LockTenant(ctx, tenantID); err != nil {
		return "", fmt.Errorf("lock tenant: %w", err)
	}
	value := id.New()
	if err = q.CreateMembership(ctx, dbgen.CreateMembershipParams{ID: value, TenantID: tenantID, UserID: userID, Role: role, IsActive: true}); err != nil {
		return "", fmt.Errorf("create membership: %w", err)
	}
	owners, err := q.CountActiveOwners(ctx, tenantID)
	if err != nil {
		return "", fmt.Errorf("count owners: %w", err)
	}
	if owners < 1 {
		return "", ErrLastOwner
	}
	if err = tx.Commit(); err != nil {
		return "", fmt.Errorf("commit membership: %w", err)
	}
	return value, nil
}
