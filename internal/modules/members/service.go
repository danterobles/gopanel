package members

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/mail"
	"strings"

	"github.com/go-sql-driver/mysql"
	dbgen "gopanel/internal/db/generated"
	repo "gopanel/internal/modules/members/repository"
	audit "gopanel/internal/platform/audit/repository"
	"gopanel/internal/platform/rbac"
	"gopanel/internal/platform/tenancy"
)

type Service struct{ db *sql.DB }

func New(db *sql.DB) *Service { return &Service{db: db} }
func Email(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	address, err := mail.ParseAddress(value)
	if err != nil || len(value) > 254 || address.Address != value {
		return "", rbac.ErrValidation
	}
	return value, nil
}
func assignable(role string) bool { return role == "admin" || role == "viewer" }
func (s *Service) List(ctx context.Context, actor tenancy.ActorContext, offset int32) (rows []dbgen.ListMemberDetailsRow, err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("list tenant members: %w", err)
		}
	}()
	if err := rbac.Require(ctx, s.db, actor, rbac.MembersRead); err != nil {
		return nil, err
	}
	return repo.NewStore(s.db).Details(ctx, actor.TenantID, offset)
}

// Mutations lock the tenant before resolving actor permissions and touching targets.
func (s *Service) Add(ctx context.Context, actor tenancy.ActorContext, email, role, requestID string) (err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("add tenant member: %w", err)
		}
	}()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	store := repo.NewStore(tx)
	if err = store.Lock(ctx, actor.TenantID); err != nil {
		return err
	}
	if err = rbac.Require(ctx, tx, actor, rbac.MembersManage); err != nil {
		return err
	}
	email, err = Email(email)
	if err != nil || !assignable(role) {
		return rbac.ErrValidation
	}
	user, err := store.ExistingUser(ctx, email)
	if err != nil {
		return err
	}
	member, err := store.Insert(ctx, actor.TenantID, user, role)
	var duplicate *mysql.MySQLError
	if errors.As(err, &duplicate) && duplicate.Number == 1062 {
		return rbac.ErrValidation
	}
	if err != nil {
		return fmt.Errorf("add member: %w", err)
	}
	if err = store.Owners(ctx, actor.TenantID); err != nil {
		return err
	}
	if err = store.Revoke(ctx, user); err != nil {
		return err
	}
	if err = audit.New(tx).Append(ctx, actor.TenantID, audit.Entry{ActorID: actor.UserID, Action: "memberships.create", EntityType: "membership", EntityID: member, Outcome: "success", RequestID: requestID}); err != nil {
		return err
	}
	return tx.Commit()
}
func (s *Service) Change(ctx context.Context, actor tenancy.ActorContext, member, role string, active bool, requestID string) (selfRevoked bool, err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("change tenant member: %w", err)
		}
	}()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	store := repo.NewStore(tx)
	if err = store.Lock(ctx, actor.TenantID); err != nil {
		return false, err
	}
	if err = rbac.Require(ctx, tx, actor, rbac.MembersManage); err != nil {
		return false, err
	}
	target, err := store.Get(ctx, actor.TenantID, member)
	if err != nil {
		return false, err
	}
	// Existing Owners can retain their role while changing state; promotion is forbidden.
	if !assignable(role) && !(target.Role == "owner" && role == "owner") {
		return false, rbac.ErrValidation
	}
	if err = store.Update(ctx, actor.TenantID, member, role, active); err != nil {
		return false, err
	}
	if err = store.Owners(ctx, actor.TenantID); err != nil {
		return false, err
	}
	changed := target.Role != role || target.IsActive != active
	if changed {
		if err = store.Revoke(ctx, target.UserID); err != nil {
			return false, err
		}
	}
	if err = audit.New(tx).Append(ctx, actor.TenantID, audit.Entry{ActorID: actor.UserID, Action: "memberships.update", EntityType: "membership", EntityID: member, Outcome: "success", RequestID: requestID}); err != nil {
		return false, err
	}
	if err = tx.Commit(); err != nil {
		return false, err
	}
	return changed && target.UserID == actor.UserID, nil
}
func (s *Service) Rejected(ctx context.Context, actor tenancy.ActorContext, action, requestID string) error {
	return audit.New(s.db).Append(ctx, actor.TenantID, audit.Entry{ActorID: actor.UserID, Action: action, EntityType: "membership", Outcome: "denied", RequestID: requestID})
}
