// Package rbac defines deny-by-default permissions, independent of UI visibility.
package rbac

import "errors"

type Permission string

const (
	DashboardRead   Permission = "dashboard.read"
	CustomersRead   Permission = "customers.read"
	CustomersCreate Permission = "customers.create"
	CustomersUpdate Permission = "customers.update"
	CustomersDelete Permission = "customers.delete"
	ProductsRead    Permission = "products.read"
	ProductsCreate  Permission = "products.create"
	ProductsUpdate  Permission = "products.update"
	ProductsDelete  Permission = "products.delete"
	MembersRead     Permission = "memberships.read"
	MembersManage   Permission = "memberships.manage"
	TenantSettings  Permission = "tenant.settings"
)

var ErrForbidden = errors.New("action forbidden")
var ErrValidation = errors.New("invalid input")
var ErrNotFound = errors.New("resource unavailable")

func Can(role string, action Permission) bool {
	switch action {
	case DashboardRead, CustomersRead, ProductsRead:
		return role == "owner" || role == "admin" || role == "viewer"
	case CustomersCreate, CustomersUpdate, CustomersDelete, ProductsCreate, ProductsUpdate, ProductsDelete, MembersRead:
		return role == "owner" || role == "admin"
	case MembersManage, TenantSettings:
		return role == "owner"
	default:
		return false
	}
}
