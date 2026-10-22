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
	Sku         string
	Name        string
	Description sql.NullString
	PriceCents  int64
	Currency    string
	Status      string
}
type ListOptions struct {
	Search, Status string
	Page, PageSize int
}
type Repository struct{ q *dbgen.Queries }

func New(db dbgen.DBTX) *Repository { return &Repository{q: dbgen.New(db)} }

func (r *Repository) Create(ctx context.Context, tenantID string, input Input) (string, error) {
	if tenantID == "" {
		return "", ErrTenantRequired
	}
	value := id.New()
	err := r.q.CreateProduct(ctx, dbgen.CreateProductParams{ID: value, TenantID: tenantID, Sku: input.Sku, Name: input.Name, Description: input.Description, PriceCents: input.PriceCents, Currency: input.Currency, Status: input.Status})
	if err != nil {
		return "", fmt.Errorf("create product: %w", err)
	}
	return value, nil
}
func (r *Repository) Get(ctx context.Context, tenantID, resourceID string) (dbgen.Product, error) {
	if tenantID == "" {
		return dbgen.Product{}, ErrTenantRequired
	}
	value, err := r.q.GetProduct(ctx, dbgen.GetProductParams{TenantID: tenantID, ID: resourceID})
	if errors.Is(err, sql.ErrNoRows) {
		return value, ErrNotFound
	}
	if err != nil {
		return value, fmt.Errorf("get product: %w", err)
	}
	return value, nil
}
func (r *Repository) Update(ctx context.Context, tenantID, resourceID string, input Input) error {
	if tenantID == "" {
		return ErrTenantRequired
	}
	n, err := r.q.UpdateProduct(ctx, dbgen.UpdateProductParams{TenantID: tenantID, ID: resourceID, Sku: input.Sku, Name: input.Name, Description: input.Description, PriceCents: input.PriceCents, Currency: input.Currency, Status: input.Status})
	if err != nil {
		return fmt.Errorf("update product: %w", err)
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
	n, err := r.q.DeleteProduct(ctx, dbgen.DeleteProductParams{TenantID: tenantID, ID: resourceID, DeletedAt: sql.NullTime{Time: time.Now().UTC(), Valid: true}})
	if err != nil {
		return fmt.Errorf("delete product: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
func (r *Repository) List(ctx context.Context, tenantID string, options ListOptions) ([]dbgen.Product, int64, error) {
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
	search := strings.NewReplacer("=", "==", "%", "=%", "_", "=_").Replace(options.Search)
	items, err := r.q.ListProducts(ctx, dbgen.ListProductsParams{TenantID: tenantID, Status: options.Status, Search: search, Limit: int32(options.PageSize), Offset: int32((options.Page - 1) * options.PageSize)})
	if err != nil {
		return nil, 0, fmt.Errorf("list products: %w", err)
	}
	count, err := r.q.CountProducts(ctx, dbgen.CountProductsParams{TenantID: tenantID, Status: options.Status, Search: search})
	if err != nil {
		return nil, 0, fmt.Errorf("count products: %w", err)
	}
	return items, count, nil
}
