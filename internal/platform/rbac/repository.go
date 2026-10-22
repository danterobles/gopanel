package rbac

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	dbgen "gopanel/internal/db/generated"
	"gopanel/internal/platform/tenancy"
)

// Require resolves current membership; actor.Role is deliberately ignored.
func Require(ctx context.Context, db dbgen.DBTX, actor tenancy.ActorContext, action Permission) error {
	if actor.UserID == "" || actor.TenantID == "" {
		return ErrForbidden
	}
	row, err := dbgen.New(db).ActiveMembership(ctx, dbgen.ActiveMembershipParams{TenantID: actor.TenantID, UserID: actor.UserID})
	if errors.Is(err, sql.ErrNoRows) {
		return ErrForbidden
	}
	if err != nil {
		return fmt.Errorf("resolve permission: %w", err)
	}
	if !Can(row.Role, action) {
		return ErrForbidden
	}
	return nil
}
func RequirePlatform(ctx context.Context, db dbgen.DBTX, userID string) error {
	if userID == "" {
		return ErrForbidden
	}
	allowed, err := dbgen.New(db).IsPlatformAdmin(ctx, userID)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrForbidden
	}
	if err != nil {
		return fmt.Errorf("resolve platform permission: %w", err)
	}
	if !allowed {
		return ErrForbidden
	}
	return nil
}
