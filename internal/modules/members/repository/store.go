package repository

import (
	"context"
	"database/sql"
	"errors"

	dbgen "gopanel/internal/db/generated"
	"gopanel/internal/platform/id"
	"gopanel/internal/platform/rbac"
)

// Store participates in a service-owned transaction; it never commits it.
type Store struct{ q *dbgen.Queries }

func NewStore(db dbgen.DBTX) *Store { return &Store{q: dbgen.New(db)} }
func (s *Store) Lock(ctx context.Context, tenant string) error {
	_, err := s.q.LockTenant(ctx, tenant)
	if errors.Is(err, sql.ErrNoRows) {
		return rbac.ErrForbidden
	}
	return err
}
func (s *Store) Details(ctx context.Context, tenant string, offset int32) ([]dbgen.ListMemberDetailsRow, error) {
	if tenant == "" || offset < 0 || offset > 100000 {
		return nil, rbac.ErrValidation
	}
	return s.q.ListMemberDetails(ctx, dbgen.ListMemberDetailsParams{TenantID: tenant, Limit: 25, Offset: offset})
}
func (s *Store) Get(ctx context.Context, tenant, member string) (dbgen.TenantMembership, error) {
	row, err := s.q.GetMembership(ctx, dbgen.GetMembershipParams{TenantID: tenant, ID: member})
	if errors.Is(err, sql.ErrNoRows) {
		err = rbac.ErrNotFound
	}
	return row, err
}
func (s *Store) ExistingUser(ctx context.Context, email string) (string, error) {
	user, err := s.q.ActiveUserIDByEmail(ctx, dbgen.ActiveUserIDByEmailParams{Email: email})
	if errors.Is(err, sql.ErrNoRows) {
		return "", rbac.ErrValidation
	}
	return user, err
}

func (s *Store) Insert(ctx context.Context, tenant, user, role string) (string, error) {
	value := id.New()
	err := s.q.CreateMembership(ctx, dbgen.CreateMembershipParams{ID: value, TenantID: tenant, UserID: user, Role: role, IsActive: true})
	return value, err
}
func (s *Store) Update(ctx context.Context, tenant, member, role string, active bool) error {
	n, err := s.q.UpdateMembership(ctx, dbgen.UpdateMembershipParams{TenantID: tenant, ID: member, Role: role, IsActive: active})
	if err != nil {
		return err
	}
	if n == 0 {
		return rbac.ErrNotFound
	}
	return nil
}
func (s *Store) Owners(ctx context.Context, tenant string) error {
	n, err := s.q.CountActiveOwners(ctx, tenant)
	if err != nil {
		return err
	}
	if n < 1 {
		return ErrLastOwner
	}
	return nil
}
func (s *Store) Revoke(ctx context.Context, user string) error {
	return s.q.RevokeUserSessions(ctx, user)
}
