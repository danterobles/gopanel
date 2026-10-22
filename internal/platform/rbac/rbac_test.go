package rbac

import "testing"

func TestPermissionMatrix(t *testing.T) {
	cases := []struct {
		action Permission
		roles  []string
	}{
		{DashboardRead, []string{"owner", "admin", "viewer"}}, {CustomersRead, []string{"owner", "admin", "viewer"}}, {ProductsRead, []string{"owner", "admin", "viewer"}},
		{CustomersCreate, []string{"owner", "admin"}}, {CustomersUpdate, []string{"owner", "admin"}}, {CustomersDelete, []string{"owner", "admin"}},
		{ProductsCreate, []string{"owner", "admin"}}, {ProductsUpdate, []string{"owner", "admin"}}, {ProductsDelete, []string{"owner", "admin"}}, {MembersRead, []string{"owner", "admin"}},
		{MembersManage, []string{"owner"}}, {TenantSettings, []string{"owner"}}, {Permission("unknown"), nil},
	}
	for _, c := range cases {
		for _, role := range []string{"owner", "admin", "viewer", "platform_admin", "", "OWNER"} {
			want := false
			for _, allowed := range c.roles {
				want = want || role == allowed
			}
			if got := Can(role, c.action); got != want {
				t.Errorf("%s %s = %v want %v", role, c.action, got, want)
			}
		}
	}
}
