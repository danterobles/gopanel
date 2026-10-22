package repository

import (
	"context"
	"database/sql"
	"errors"

	dbgen "gopanel/internal/db/generated"
	"gopanel/internal/platform/rbac"
)

func (r *Repository) Lock(ctx context.Context, tenant string, platform bool) error {
	var err error
	if platform {
		_, err = r.q.LockAnyTenant(ctx, tenant)
	} else {
		_, err = r.q.LockTenant(ctx, tenant)
	}
	if errors.Is(err, sql.ErrNoRows) {
		return rbac.ErrNotFound
	}
	return err
}
func (r *Repository) Rename(ctx context.Context, tenant, name string) error {
	n, err := r.q.UpdateTenantName(ctx, dbgen.UpdateTenantNameParams{ID: tenant, Name: name})
	if err != nil {
		return err
	}
	if n == 0 {
		return rbac.ErrNotFound
	}
	return nil
}
func (r *Repository) PlatformList(ctx context.Context, offset int32) ([]dbgen.Tenant, error) {
	if offset < 0 || offset > 100000 {
		return nil, rbac.ErrValidation
	}
	return r.q.ListPlatformTenants(ctx, dbgen.ListPlatformTenantsParams{Limit: 25, Offset: offset})
}
func (r *Repository) Create(ctx context.Context, tenant, name, slug string) error {
	return r.q.CreateTenant(ctx, dbgen.CreateTenantParams{ID: tenant, Name: name, Slug: slug})
}
func (r *Repository) Update(ctx context.Context, tenant, name string, active bool) error {
	n, err := r.q.UpdatePlatformTenant(ctx, dbgen.UpdatePlatformTenantParams{ID: tenant, Name: name, IsActive: active})
	if err != nil {
		return err
	}
	if n == 0 {
		return rbac.ErrNotFound
	}
	if !active {
		return r.q.RevokeTenantSessions(ctx, sql.NullString{String: tenant, Valid: true})
	}
	return nil
}
