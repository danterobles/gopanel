// Package repository provides identity persistence; it does not authenticate HTTP requests.
package repository

import (
	"context"
	"database/sql"
	"fmt"
	dbgen "gopanel/internal/db/generated"
	"strings"
	"time"
)

type Repository struct{ q *dbgen.Queries }

func New(db dbgen.DBTX) *Repository { return &Repository{q: dbgen.New(db)} }
func (r *Repository) UserByEmail(ctx context.Context, email string) (dbgen.User, error) {
	user, err := r.q.GetUserByEmail(ctx, strings.ToLower(strings.TrimSpace(email)))
	if err != nil {
		return user, fmt.Errorf("find identity: %w", err)
	}
	return user, nil
}
func (r *Repository) CreateSession(ctx context.Context, hash [32]byte, userID string, tenant sql.NullString, expires time.Time) error {
	err := r.q.CreateSession(ctx, dbgen.CreateSessionParams{TokenHash: hash[:], UserID: userID, ActiveTenantID: tenant, ExpiresAt: expires.UTC()})
	if err != nil {
		return fmt.Errorf("persist session: %w", err)
	}
	return nil
}
func (r *Repository) Session(ctx context.Context, hash [32]byte) (dbgen.Session, error) {
	session, err := r.q.GetSession(ctx, dbgen.GetSessionParams{TokenHash: hash[:], Now: time.Now().UTC()})
	if err != nil {
		return session, fmt.Errorf("find session: %w", err)
	}
	return session, nil
}
func (r *Repository) RevokeSession(ctx context.Context, hash [32]byte) error {
	if err := r.q.DeleteSession(ctx, hash[:]); err != nil {
		return fmt.Errorf("revoke session: %w", err)
	}
	return nil
}
