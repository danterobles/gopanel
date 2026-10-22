package dashboard

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"gopanel/internal/modules/dashboard/repository"
	"gopanel/internal/platform/rbac"
	"gopanel/internal/platform/tenancy"
)

type Service struct {
	db  *sql.DB
	now func() time.Time
}

func New(db *sql.DB, now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{db: db, now: now}
}
func (s *Service) Read(ctx context.Context, actor tenancy.ActorContext) (repository.Snapshot, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return repository.Snapshot{}, fmt.Errorf("begin dashboard: %w", err)
	}
	defer tx.Rollback()
	if err = rbac.Require(ctx, tx, actor, rbac.DashboardRead); err != nil {
		return repository.Snapshot{}, fmt.Errorf("authorize dashboard: %w", err)
	}
	result, err := repository.New(tx).Read(ctx, actor.TenantID, s.now())
	if err != nil {
		return result, err
	}
	if err = tx.Commit(); err != nil {
		return result, fmt.Errorf("commit dashboard read: %w", err)
	}
	return result, nil
}
