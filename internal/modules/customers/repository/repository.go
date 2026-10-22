package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	dbgen "gopanel/internal/db/generated"
	"gopanel/internal/platform/id"
)

var ErrNotFound = errors.New("resource not found")
var ErrInvalidOptions = errors.New("invalid list options")
var ErrTenantRequired = errors.New("tenant is required")

type Input struct {
	Name   string
	Email  sql.NullString
	Phone  sql.NullString
	Status string
	Notes  sql.NullString
}
type ListOptions struct {
	Search, Status, Sort string
	Page, PageSize       int
}
type Repository struct{ q *dbgen.Queries }

func New(db dbgen.DBTX) *Repository { return &Repository{q: dbgen.New(db)} }

func (r *Repository) Create(ctx context.Context, tenantID string, input Input) (string, error) {
	if tenantID == "" {
		return "", ErrTenantRequired
	}
	value := id.New()
	err := r.q.CreateCustomer(ctx, dbgen.CreateCustomerParams{ID: value, TenantID: tenantID, Name: input.Name, Email: input.Email, Phone: input.Phone, Status: input.Status, Notes: input.Notes})
	if err != nil {
		return "", fmt.Errorf("create customer: %w", err)
	}
	return value, nil
}
func (r *Repository) Get(ctx context.Context, tenantID, resourceID string) (dbgen.Customer, error) {
	if tenantID == "" {
		return dbgen.Customer{}, ErrTenantRequired
	}
	if !id.Valid(resourceID) {
		return dbgen.Customer{}, ErrNotFound
	}
	value, err := r.q.GetCustomer(ctx, dbgen.GetCustomerParams{TenantID: tenantID, ID: resourceID})
	if errors.Is(err, sql.ErrNoRows) {
		return value, ErrNotFound
	}
	if err != nil {
		return value, fmt.Errorf("get customer: %w", err)
	}
	return value, nil
}
func (r *Repository) Update(ctx context.Context, tenantID, resourceID string, input Input) error {
	if tenantID == "" {
		return ErrTenantRequired
	}
	if !id.Valid(resourceID) {
		return ErrNotFound
	}
	n, err := r.q.UpdateCustomer(ctx, dbgen.UpdateCustomerParams{TenantID: tenantID, ID: resourceID, Name: input.Name, Email: input.Email, Phone: input.Phone, Status: input.Status, Notes: input.Notes})
	if err != nil {
		return fmt.Errorf("update customer: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
func (r *Repository) Delete(ctx context.Context, tenantID, resourceID string) error {
	if tenantID == "" {
		return ErrTenantRequired
	}
	if !id.Valid(resourceID) {
		return ErrNotFound
	}
	n, err := r.q.DeleteCustomer(ctx, dbgen.DeleteCustomerParams{TenantID: tenantID, ID: resourceID, DeletedAt: sql.NullTime{Time: time.Now().UTC(), Valid: true}})
	if err != nil {
		return fmt.Errorf("delete customer: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
func (r *Repository) List(ctx context.Context, tenantID string, options ListOptions) ([]dbgen.Customer, int64, error) {
	if tenantID == "" {
		return nil, 0, ErrTenantRequired
	}
	if options.Page == 0 {
		options.Page = 1
	}
	if options.PageSize == 0 {
		options.PageSize = 25
	}
	if options.Page < 1 || (options.PageSize != 10 && options.PageSize != 25 && options.PageSize != 50) || options.Page-1 > math.MaxInt32/options.PageSize || (options.Status != "" && options.Status != "active" && options.Status != "inactive") {
		return nil, 0, ErrInvalidOptions
	}
	if options.Sort == "" {
		options.Sort = "created_desc"
	}
	if options.Sort != "created_desc" && options.Sort != "created_asc" && options.Sort != "name_asc" && options.Sort != "name_desc" {
		return nil, 0, ErrInvalidOptions
	}
	if (options.Page-1)*options.PageSize > 100000 {
		return nil, 0, ErrInvalidOptions
	}
	search := strings.NewReplacer("=", "==", "%", "=%", "_", "=_").Replace(options.Search)
	items, err := r.q.ListCustomers(ctx, dbgen.ListCustomersParams{TenantID: tenantID, Status: options.Status, Search: search, Sort: options.Sort, Limit: int32(options.PageSize), Offset: int32((options.Page - 1) * options.PageSize)})
	if err != nil {
		return nil, 0, fmt.Errorf("list customers: %w", err)
	}
	count, err := r.q.CountCustomers(ctx, dbgen.CountCustomersParams{TenantID: tenantID, Status: options.Status, Search: search})
	if err != nil {
		return nil, 0, fmt.Errorf("count customers: %w", err)
	}
	return items, count, nil
}
