package tenants

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/go-sql-driver/mysql"

	dbgen "gopanel/internal/db/generated"
	"gopanel/internal/modules/members"
	memberrepo "gopanel/internal/modules/members/repository"
	repo "gopanel/internal/modules/tenants/repository"
	audit "gopanel/internal/platform/audit/repository"
	"gopanel/internal/platform/id"
	"gopanel/internal/platform/rbac"
	"gopanel/internal/platform/tenancy"
)

type Service struct{ db *sql.DB }

func New(db *sql.DB) *Service { return &Service{db: db} }
func nameValid(value string) bool {
	return utf8.ValidString(value) && utf8.RuneCountInString(value) > 0 && utf8.RuneCountInString(value) <= 180
}

var slugPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,98}[a-z0-9]$`)

func (s *Service) Settings(ctx context.Context, actor tenancy.ActorContext) (tenant dbgen.Tenant, err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("read tenant settings: %w", err)
		}
	}()
	if err := rbac.Require(ctx, s.db, actor, rbac.TenantSettings); err != nil {
		return dbgen.Tenant{}, err
	}
	return repo.New(s.db).Get(ctx, actor.TenantID)
}
func (s *Service) Rename(ctx context.Context, actor tenancy.ActorContext, name, requestID string) (err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("rename tenant: %w", err)
		}
	}()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	store := repo.New(tx)
	if err = store.Lock(ctx, actor.TenantID, false); err != nil {
		return err
	}
	if err = rbac.Require(ctx, tx, actor, rbac.TenantSettings); err != nil {
		return err
	}
	name = strings.TrimSpace(name)
	if !nameValid(name) {
		return rbac.ErrValidation
	}
	if err = store.Rename(ctx, actor.TenantID, name); err != nil {
		return err
	}
	if err = audit.New(tx).Append(ctx, actor.TenantID, audit.Entry{ActorID: actor.UserID, Action: "tenant.settings", EntityType: "tenant", EntityID: actor.TenantID, Outcome: "success", RequestID: requestID}); err != nil {
		return err
	}
	return tx.Commit()
}
func (s *Service) Platform(ctx context.Context, user string, offset int32) (tenants []dbgen.Tenant, stats sql.DBStats, err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("read platform tenants: %w", err)
		}
	}()
	if err := rbac.RequirePlatform(ctx, s.db, user); err != nil {
		return nil, sql.DBStats{}, err
	}
	rows, err := repo.New(s.db).PlatformList(ctx, offset)
	return rows, s.db.Stats(), err
}
func (s *Service) Create(ctx context.Context, user, name, slug, email, requestID string) (err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("create platform tenant: %w", err)
		}
	}()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = rbac.RequirePlatform(ctx, tx, user); err != nil {
		return err
	}
	name = strings.TrimSpace(name)
	slug = strings.TrimSpace(slug)
	if !nameValid(name) || !slugPattern.MatchString(slug) {
		return rbac.ErrValidation
	}
	email, err = members.Email(email)
	if err != nil {
		return err
	}
	ms := memberrepo.NewStore(tx)
	owner, err := ms.ExistingUser(ctx, email)
	if err != nil {
		return err
	}
	tenant := id.New()
	err = repo.New(tx).Create(ctx, tenant, name, slug)
	var duplicate *mysql.MySQLError
	if errors.As(err, &duplicate) && duplicate.Number == 1062 {
		return rbac.ErrValidation
	}
	if err != nil {
		return err
	}
	if _, err = ms.Insert(ctx, tenant, owner, "owner"); err != nil {
		return err
	}
	if err = ms.Revoke(ctx, owner); err != nil {
		return err
	}
	if err = audit.New(tx).AppendGlobal(ctx, audit.Entry{ActorID: user, Action: "platform.tenants.create", EntityType: "tenant", EntityID: tenant, Outcome: "success", RequestID: requestID}); err != nil {
		return err
	}
	return tx.Commit()
}
func (s *Service) Update(ctx context.Context, user, tenant, name string, active bool, requestID string) (err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("update platform tenant: %w", err)
		}
	}()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = rbac.RequirePlatform(ctx, tx, user); err != nil {
		return err
	}
	store := repo.New(tx)
	if err = store.Lock(ctx, tenant, true); err != nil {
		return err
	}
	name = strings.TrimSpace(name)
	if !nameValid(name) {
		return rbac.ErrValidation
	}
	if err = store.Update(ctx, tenant, name, active); err != nil {
		return err
	}
	if err = audit.New(tx).AppendGlobal(ctx, audit.Entry{ActorID: user, Action: "platform.tenants.update", EntityType: "tenant", EntityID: tenant, Outcome: "success", RequestID: requestID}); err != nil {
		return err
	}
	return tx.Commit()
}
func (s *Service) Rejected(ctx context.Context, user, tenant, action, requestID string) error {
	e := audit.Entry{ActorID: user, Action: action, EntityType: "tenant", Outcome: "denied", RequestID: requestID}
	if tenant == "" {
		return audit.New(s.db).AppendGlobal(ctx, e)
	}
	return audit.New(s.db).Append(ctx, tenant, e)
}
