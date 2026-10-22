//go:build integration

package integration_test

import (
	"fmt"
	dbgen "gopanel/internal/db/generated"
	customerrepo "gopanel/internal/modules/customers/repository"
	productrepo "gopanel/internal/modules/products/repository"
	"strings"
	"testing"
	"time"
)

func TestRecentQueryParityWithExistingOrdersAndScope(t *testing.T) {
	f := newAuthFixture(t, false)
	q := dbgen.New(f.db)
	at := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	for _, tenant := range []string{f.north, f.south} {
		for i := 0; i < 27; i++ {
			state := "active"
			if i%2 == 0 {
				state = "inactive"
			}
			customer := createCustomer(t, f, tenant, fmt.Sprintf("A%%_=%02d", i), state)
			product := createProduct(t, f, tenant, fmt.Sprint(i), fmt.Sprintf("A%%_=%02d", i), state, int64(i))
			if _, err := f.db.ExecContext(t.Context(), "UPDATE customers SET created_at=? WHERE tenant_id=? AND id=?", at, tenant, customer); err != nil {
				t.Fatal(err)
			}
			if _, err := f.db.ExecContext(t.Context(), "UPDATE products SET created_at=? WHERE tenant_id=? AND id=?", at, tenant, product); err != nil {
				t.Fatal(err)
			}
			if i%7 == 0 {
				if err := customerrepo.New(f.db).Delete(t.Context(), tenant, customer); err != nil {
					t.Fatal(err)
				}
				if err := productrepo.New(f.db).Delete(t.Context(), tenant, product); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
	for _, tenant := range []string{f.north, f.south} {
		for _, state := range []string{"", "active", "inactive"} {
			for _, search := range []string{"", "%_=", "not-present"} {
				for _, page := range []int{1, 2, 3} {
					escaped := strings.NewReplacer("=", "==", "%", "=%", "_", "=_").Replace(search)
					actual, count, err := customerrepo.New(f.db).List(t.Context(), tenant, customerrepo.ListOptions{Search: search, Status: state, Sort: "created_desc", Page: page, PageSize: 10})
					if err != nil {
						t.Fatal(err)
					}
					old, err := q.ListCustomers(t.Context(), dbgen.ListCustomersParams{TenantID: tenant, Status: state, Search: escaped, Sort: "created_desc", Limit: 10, Offset: int32((page - 1) * 10)})
					if err != nil || len(actual) != len(old) {
						t.Fatal("customer recent mismatch", err)
					}
					for i := range old {
						if actual[i].ID != old[i].ID || actual[i].TenantID != tenant {
							t.Fatal("customer ordering/scope changed")
						}
					}
					oldCount, err := q.CountCustomers(t.Context(), dbgen.CountCustomersParams{TenantID: tenant, Status: state, Search: escaped})
					if err != nil || count != oldCount {
						t.Fatal("customer count changed")
					}
					products, pc, err := productrepo.New(f.db).List(t.Context(), tenant, productrepo.ListOptions{Search: search, Status: state, Sort: "created_desc", Page: page, PageSize: 10})
					if err != nil {
						t.Fatal(err)
					}
					previous, err := q.ListProducts(t.Context(), dbgen.ListProductsParams{TenantID: tenant, Status: state, Search: escaped, Sort: "created_desc", Limit: 10, Offset: int32((page - 1) * 10)})
					if err != nil || len(products) != len(previous) {
						t.Fatal("product recent mismatch", err)
					}
					for i := range previous {
						if products[i].ID != previous[i].ID || products[i].TenantID != tenant {
							t.Fatal("product ordering/scope changed")
						}
					}
					previousCount, err := q.CountProducts(t.Context(), dbgen.CountProductsParams{TenantID: tenant, Status: state, Search: escaped})
					if err != nil || pc != previousCount {
						t.Fatal("product count changed")
					}
				}
			}
		}
	}
}
