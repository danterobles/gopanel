package products

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/go-sql-driver/mysql"

	dbgen "gopanel/internal/db/generated"
	repo "gopanel/internal/modules/products/repository"
	tenantrepo "gopanel/internal/modules/tenants/repository"
	audit "gopanel/internal/platform/audit/repository"
	"gopanel/internal/platform/events"
	"gopanel/internal/platform/rbac"
	"gopanel/internal/platform/tenancy"
)

type Service struct {
	db     *sql.DB
	events events.Publisher
}

func New(db *sql.DB, publishers ...events.Publisher) *Service {
	s := &Service{db: db}
	if len(publishers) > 0 {
		s.events = publishers[0]
	}
	return s
}
func (s *Service) List(ctx context.Context, actor tenancy.ActorContext, options repo.ListOptions) (rows []dbgen.Product, count int64, err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("list authorized products: %w", err)
		}
	}()
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return nil, 0, err
	}
	defer tx.Rollback()
	if err = rbac.Require(ctx, tx, actor, rbac.ProductsRead); err != nil {
		return nil, 0, err
	}
	rows, count, err = repo.New(tx).List(ctx, actor.TenantID, options)
	if err != nil {
		return nil, 0, err
	}
	err = tx.Commit()
	return
}
func (s *Service) Get(ctx context.Context, actor tenancy.ActorContext, resource string, action rbac.Permission) (row dbgen.Product, err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("get authorized product: %w", err)
		}
	}()
	switch action {
	case rbac.ProductsRead, rbac.ProductsUpdate, rbac.ProductsDelete:
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
	return rbac.Require(ctx, s.db, actor, rbac.ProductsCreate)
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
			err = fmt.Errorf("save authorized product: %w", err)
		}
	}()
	normalized = input
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", normalized, err
	}
	defer tx.Rollback()
	permission, action := rbac.ProductsCreate, "products.create"
	if resource != "" {
		permission, action = rbac.ProductsUpdate, "products.update"
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
	var duplicate *mysql.MySQLError
	if errors.As(err, &duplicate) && duplicate.Number == 1062 {
		return "", normalized, &ValidationError{Fields: map[string]string{"sku": "El SKU ya está en uso en esta empresa."}}
	}
	if err != nil {
		return "", normalized, err
	}
	if err = audit.New(tx).Append(ctx, actor.TenantID, audit.Entry{ActorID: actor.UserID, Action: action, EntityType: "product", EntityID: value, Outcome: "success", RequestID: requestID}); err != nil {
		return "", normalized, err
	}
	err = tx.Commit()
	if err == nil && s.events != nil {
		s.events.Publish(actor.TenantID, "products")
	}
	return
}
func (s *Service) Delete(ctx context.Context, actor tenancy.ActorContext, resource, confirmation, requestID string) (err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("delete authorized product: %w", err)
		}
	}()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = lock(ctx, tx, actor, rbac.ProductsDelete); err != nil {
		return err
	}
	repository := repo.New(tx)
	if _, err = repository.Get(ctx, actor.TenantID, resource); err != nil {
		return err
	}
	if confirmation != "yes" {
		return &ValidationError{Fields: map[string]string{"confirm": "Confirma la baja del producto."}}
	}
	if err = repository.Delete(ctx, actor.TenantID, resource); err != nil {
		return err
	}
	if err = audit.New(tx).Append(ctx, actor.TenantID, audit.Entry{ActorID: actor.UserID, Action: "products.delete", EntityType: "product", EntityID: resource, Outcome: "success", RequestID: requestID}); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	if s.events != nil {
		s.events.Publish(actor.TenantID, "products")
	}
	return nil
}
func (s *Service) Rejected(ctx context.Context, actor tenancy.ActorContext, action, requestID string) error {
	return audit.New(s.db).Append(ctx, actor.TenantID, audit.Entry{ActorID: actor.UserID, Action: action, EntityType: "product", Outcome: "denied", RequestID: requestID})
}
