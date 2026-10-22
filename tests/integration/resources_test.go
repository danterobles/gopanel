//go:build integration

package integration_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/go-sql-driver/mysql"
	dbgen "gopanel/internal/db/generated"
	customers "gopanel/internal/modules/customers/repository"
	products "gopanel/internal/modules/products/repository"
	"gopanel/internal/platform/id"
)

func mysqlCode(t *testing.T, err error, want uint16) {
	t.Helper()
	var me *mysql.MySQLError
	if !errors.As(err, &me) || me.Number != want {
		t.Fatalf("expected MySQL error %d, got %v", want, err)
	}
}

func TestCustomerIsolationPaginationLiteralSearchAndConstraints(t *testing.T) {
	_, db := scratch(t, true)
	ctx := t.Context()
	north, _, _ := tenantOwner(t, db, "north")
	south, _, _ := tenantOwner(t, db, "south")
	r := customers.New(db)
	input := customers.Input{Name: `Cliente 100%_\=`, Status: "active"}
	value, err := r.Create(ctx, north, input)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = r.Get(ctx, south, value); !errors.Is(err, customers.ErrNotFound) {
		t.Fatal("cross-tenant read was not rejected")
	}
	if err = r.Update(ctx, south, value, customers.Input{Name: "intrusion", Status: "active"}); !errors.Is(err, customers.ErrNotFound) {
		t.Fatal("cross-tenant update was not rejected")
	}
	if err = r.Delete(ctx, south, value); !errors.Is(err, customers.ErrNotFound) {
		t.Fatal("cross-tenant delete was not rejected")
	}
	if err = r.Update(ctx, north, value, input); err != nil {
		t.Fatal("unchanged edit:", err)
	}
	for _, search := range []string{"%", "_", `\`, "=", `100%_\=`} {
		rows, count, err := r.List(ctx, north, customers.ListOptions{Search: search})
		if err != nil || count != 1 || len(rows) != 1 {
			t.Fatalf("literal search %q: rows=%d count=%d err=%v", search, len(rows), count, err)
		}
	}
	_, count, err := r.List(ctx, south, customers.ListOptions{})
	if err != nil || count != 0 {
		t.Fatal("list leaked north tenant")
	}
	_, count, err = r.List(ctx, north, customers.ListOptions{Search: "' OR 1=1 --"})
	if err != nil || count != 0 {
		t.Fatal("search was not literal/parameterized")
	}
	for n := range 12 {
		if _, err = r.Create(ctx, north, customers.Input{Name: fmt.Sprintf("page-item %02d", n), Status: "active"}); err != nil {
			t.Fatal(err)
		}
	}
	page1, count, err := r.List(ctx, north, customers.ListOptions{Search: "page-item", PageSize: 10})
	if err != nil || len(page1) != 10 || count != 12 {
		t.Fatal("invalid first page", err)
	}
	page2, count, err := r.List(ctx, north, customers.ListOptions{Search: "page-item", Page: 2, PageSize: 10})
	if err != nil || len(page2) != 2 || count != 12 {
		t.Fatal("invalid second page", err)
	}
	seen := map[string]bool{}
	for _, row := range page1 {
		seen[row.ID] = true
	}
	for _, row := range page2 {
		if seen[row.ID] {
			t.Fatal("pagination repeats rows")
		}
	}
	for _, options := range []customers.ListOptions{{Page: -1}, {PageSize: 100}, {Status: "bogus"}, {Page: int(^uint(0) >> 1)}} {
		if _, _, err = r.List(ctx, north, options); !errors.Is(err, customers.ErrInvalidOptions) {
			t.Fatal("invalid pagination/filter accepted")
		}
	}
	if _, _, err = r.List(ctx, "", customers.ListOptions{}); !errors.Is(err, customers.ErrTenantRequired) {
		t.Fatal("empty tenant accepted")
	}
	_, err = r.Create(ctx, north, customers.Input{Name: " ", Status: "active"})
	mysqlCode(t, err, 3819)
	_, err = r.Create(ctx, north, customers.Input{Name: "invalid", Status: "bogus"})
	mysqlCode(t, err, 3819)
	_, err = r.Create(ctx, id.New(), input)
	mysqlCode(t, err, 1452)
	if err = r.Delete(ctx, north, value); err != nil {
		t.Fatal(err)
	}
	if _, err = r.Get(ctx, north, value); !errors.Is(err, customers.ErrNotFound) {
		t.Fatal("soft-deleted row visible")
	}
	if err = r.Delete(ctx, north, value); !errors.Is(err, customers.ErrNotFound) {
		t.Fatal("repeated delete accepted")
	}
	if err = r.Update(ctx, north, value, input); !errors.Is(err, customers.ErrNotFound) {
		t.Fatal("deleted row updated")
	}
}

func TestProductIsolationSKUUniquenessSoftDeleteAndMoney(t *testing.T) {
	_, db := scratch(t, true)
	ctx := t.Context()
	north, _, _ := tenantOwner(t, db, "north")
	south, _, _ := tenantOwner(t, db, "south")
	r := products.New(db)
	input := products.Input{Sku: "SKU-SAME", Name: "Producto", PriceCents: 12345, Currency: "MXN", Status: "active"}
	value, err := r.Create(ctx, north, input)
	if err != nil {
		t.Fatal(err)
	}
	_, err = r.Create(ctx, north, input)
	mysqlCode(t, err, 1062)
	lower := input
	lower.Sku = "sku-same"
	_, err = r.Create(ctx, north, lower)
	mysqlCode(t, err, 1062)
	if _, err = r.Create(ctx, south, input); err != nil {
		t.Fatal("same SKU in another tenant:", err)
	}
	if _, err = r.Get(ctx, south, value); !errors.Is(err, products.ErrNotFound) {
		t.Fatal("cross-tenant product read")
	}
	if err = r.Update(ctx, south, value, input); !errors.Is(err, products.ErrNotFound) {
		t.Fatal("cross-tenant product update")
	}
	if err = r.Delete(ctx, south, value); !errors.Is(err, products.ErrNotFound) {
		t.Fatal("cross-tenant product delete")
	}
	if err = r.Update(ctx, north, value, input); err != nil {
		t.Fatal("unchanged product edit:", err)
	}
	got, err := r.Get(ctx, north, value)
	if err != nil || got.PriceCents != 12345 {
		t.Fatal("money did not round-trip in cents")
	}
	rows, count, err := r.List(ctx, north, products.ListOptions{})
	if err != nil || count != 1 || len(rows) != 1 {
		t.Fatal("product list", err)
	}
	if _, _, err = r.List(ctx, north, products.ListOptions{PageSize: 500}); !errors.Is(err, products.ErrInvalidOptions) {
		t.Fatal("unbounded product page")
	}
	bad := input
	bad.Sku = "NEGATIVE"
	bad.PriceCents = -1
	_, err = r.Create(ctx, north, bad)
	mysqlCode(t, err, 3819)
	bad = input
	bad.Sku = "USD"
	bad.Currency = "USD"
	_, err = r.Create(ctx, north, bad)
	mysqlCode(t, err, 3819)
	bad = input
	bad.Sku = " "
	_, err = r.Create(ctx, north, bad)
	mysqlCode(t, err, 3819)
	bad = input
	bad.Sku = "BAD-STATUS"
	bad.Status = "bogus"
	_, err = r.Create(ctx, north, bad)
	mysqlCode(t, err, 3819)
	if err = r.Delete(ctx, north, value); err != nil {
		t.Fatal(err)
	}
	if _, err = r.Get(ctx, north, value); !errors.Is(err, products.ErrNotFound) {
		t.Fatal("deleted product visible")
	}
	if _, err = r.Create(ctx, north, input); err != nil {
		t.Fatal("soft delete did not free SKU:", err)
	}
}

func TestDatabaseForeignKeysEmailAndMembershipChecks(t *testing.T) {
	_, db := scratch(t, true)
	ctx := t.Context()
	tenant, user, _ := tenantOwner(t, db, "north")
	q := dbgen.New(db)
	err := q.CreateUser(ctx, dbgen.CreateUserParams{ID: id.New(), Email: "north@fixture.test", PasswordHash: "$argon2id$fixture", DisplayName: "duplicate"})
	mysqlCode(t, err, 1062)
	err = q.CreateMembership(ctx, dbgen.CreateMembershipParams{ID: id.New(), TenantID: tenant, UserID: user, Role: "owner", IsActive: true})
	mysqlCode(t, err, 1062)
	err = q.CreateMembership(ctx, dbgen.CreateMembershipParams{ID: id.New(), TenantID: tenant, UserID: id.New(), Role: "viewer", IsActive: true})
	mysqlCode(t, err, 1452)
	other := id.New()
	if err = q.CreateUser(ctx, dbgen.CreateUserParams{ID: other, Email: "other@fixture.test", PasswordHash: "$argon2id$fixture", DisplayName: "other"}); err != nil {
		t.Fatal(err)
	}
	err = q.CreateMembership(ctx, dbgen.CreateMembershipParams{ID: id.New(), TenantID: tenant, UserID: other, Role: "superuser", IsActive: true})
	mysqlCode(t, err, 3819)
	_, err = db.ExecContext(ctx, "DELETE FROM tenants WHERE id=?", tenant)
	mysqlCode(t, err, 1451)
	var binaryUUID string
	if err = db.QueryRowContext(ctx, "SELECT collation_name FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name='tenants' AND column_name='id'").Scan(&binaryUUID); err != nil {
		t.Fatal(err)
	}
	if binaryUUID != "ascii_bin" {
		t.Fatal("UUID collation changed")
	}
}

func TestConcurrentProductSKUInsertKeepsUniqueConstraint(t *testing.T) {
	_, db := scratch(t, true)
	tenant, _, _ := tenantOwner(t, db, "concurrent-sku")
	r := products.New(db)
	start := make(chan struct{})
	result := make(chan error, 2)
	for range 2 {
		go func() {
			<-start
			_, err := r.Create(t.Context(), tenant, products.Input{Sku: "CONCURRENT", Name: "Concurrent", Currency: "MXN", Status: "active"})
			result <- err
		}()
	}
	close(start)
	success, duplicate := 0, 0
	for range 2 {
		err := <-result
		if err == nil {
			success++
		} else {
			var me *mysql.MySQLError
			if !errors.As(err, &me) || me.Number != 1062 {
				t.Fatal(err)
			}
			duplicate++
		}
	}
	if success != 1 || duplicate != 1 {
		t.Fatal("SKU uniqueness failed under concurrency")
	}
}
