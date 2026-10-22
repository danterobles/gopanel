package customers

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	dbgen "gopanel/internal/db/generated"
	repo "gopanel/internal/modules/customers/repository"
	tenantrepo "gopanel/internal/modules/tenants/repository"
	audit "gopanel/internal/platform/audit/repository"
	"gopanel/internal/platform/rbac"
	"gopanel/internal/platform/tenancy"
)

type Service struct{ db *sql.DB }

func New(db *sql.DB) *Service { return &Service{db: db} }
func (s *Service) List(ctx context.Context, actor tenancy.ActorContext, options repo.ListOptions) (rows []dbgen.Customer, count int64, err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("list authorized customers: %w", err)
		}
	}()
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return nil, 0, err
	}
	defer tx.Rollback()
	if err = rbac.Require(ctx, tx, actor, rbac.CustomersRead); err != nil {
		return nil, 0, err
	}
	rows, count, err = repo.New(tx).List(ctx, actor.TenantID, options)
	if err != nil {
		return nil, 0, err
	}
	err = tx.Commit()
	return
}
func (s *Service) Get(ctx context.Context, actor tenancy.ActorContext, resource string, action rbac.Permission) (row dbgen.Customer, err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("get authorized customer: %w", err)
		}
	}()
	switch action {
	case rbac.CustomersRead, rbac.CustomersUpdate, rbac.CustomersDelete:
	default:
		return row, rbac.ErrForbidden
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return row, err
	}
	defer tx.Rollback()
	if err = rbac.Require(ctx, tx, actor, action); err != nil {
		return row, err
	}
	row, err = repo.New(tx).Get(ctx, actor.TenantID, resource)
	if err != nil {
		return row, err
	}
	err = tx.Commit()
	return
}
func (s *Service) CanCreate(ctx context.Context, actor tenancy.ActorContext) error {
	return rbac.Require(ctx, s.db, actor, rbac.CustomersCreate)
}
func lock(ctx context.Context, tx *sql.Tx, actor tenancy.ActorContext, action rbac.Permission) error {
	if actor.TenantID == "" || actor.UserID == "" {
		return rbac.ErrForbidden
	}
	err := tenantrepo.New(tx).Lock(ctx, actor.TenantID, false)
	if errors.Is(err, rbac.ErrNotFound) {
		return rbac.ErrForbidden
	}
	if err != nil {
		return err
	}
	return rbac.Require(ctx, tx, actor, action)
}
func (s *Service) Save(ctx context.Context, actor tenancy.ActorContext, resource string, input Input, requestID string) (value string, normalized Input, err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("save authorized customer: %w", err)
		}
	}()
	normalized = input
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", normalized, err
	}
	defer tx.Rollback()
	permission, action := rbac.CustomersCreate, "customers.create"
	if resource != "" {
		permission, action = rbac.CustomersUpdate, "customers.update"
	}
	if err = lock(ctx, tx, actor, permission); err != nil {
		return "", normalized, err
	}
	repository := repo.New(tx)
	if resource != "" {
		if _, err = repository.Get(ctx, actor.TenantID, resource); err != nil {
			return "", normalized, err
		}
	}
	normalized, err = Validate(input)
	if err != nil {
		return "", normalized, err
	}
	value = resource
	if resource == "" {
		value, err = repository.Create(ctx, actor.TenantID, normalized.record())
	} else {
		err = repository.Update(ctx, actor.TenantID, resource, normalized.record())
	}
	if err != nil {
		return "", normalized, err
	}
	if err = audit.New(tx).Append(ctx, actor.TenantID, audit.Entry{ActorID: actor.UserID, Action: action, EntityType: "customer", EntityID: value, Outcome: "success", RequestID: requestID}); err != nil {
		return "", normalized, err
	}
	err = tx.Commit()
	return
}
func (s *Service) Delete(ctx context.Context, actor tenancy.ActorContext, resource, confirmation, requestID string) (err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("delete authorized customer: %w", err)
		}
	}()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = lock(ctx, tx, actor, rbac.CustomersDelete); err != nil {
		return err
	}
	repository := repo.New(tx)
	if _, err = repository.Get(ctx, actor.TenantID, resource); err != nil {
		return err
	}
	if confirmation != "yes" {
		return &ValidationError{Fields: map[string]string{"confirm": "Confirma la baja del cliente."}}
	}
	if err = repository.Delete(ctx, actor.TenantID, resource); err != nil {
		return err
	}
	if err = audit.New(tx).Append(ctx, actor.TenantID, audit.Entry{ActorID: actor.UserID, Action: "customers.delete", EntityType: "customer", EntityID: resource, Outcome: "success", RequestID: requestID}); err != nil {
		return err
	}
	return tx.Commit()
}
func (s *Service) Rejected(ctx context.Context, actor tenancy.ActorContext, action, requestID string) error {
	return audit.New(s.db).Append(ctx, actor.TenantID, audit.Entry{ActorID: actor.UserID, Action: action, EntityType: "customer", Outcome: "denied", RequestID: requestID})
}
