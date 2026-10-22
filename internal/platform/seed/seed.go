// Package seed installs an explicit development fixture, never production data.
package seed

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/crypto/argon2"
	dbgen "gopanel/internal/db/generated"
	"gopanel/internal/platform/id"
)

type Options struct {
	Count           int
	CredentialsPath string
}
type Result struct {
	AlreadySeeded                                    bool
	Tenants, Users, Memberships, Customers, Products int
}
type Credential struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func Run(ctx context.Context, db *sql.DB, options Options) (Result, error) {
	if options.Count == 0 {
		options.Count = 100
	}
	if options.Count < 1 || options.Count > 10000 || options.CredentialsPath == "" {
		return Result{}, fmt.Errorf("invalid seed options")
	}
	conn, err := db.Conn(ctx)
	if err != nil {
		return Result{}, fmt.Errorf("seed connection: %w", err)
	}
	defer conn.Close()
	var locked int
	if err = conn.QueryRowContext(ctx, "SELECT GET_LOCK(CONCAT(DATABASE(), ':gopanel_seed'),5)").Scan(&locked); err != nil || locked != 1 {
		return Result{}, fmt.Errorf("cannot acquire seed lock")
	}
	defer func() {
		_, _ = conn.ExecContext(context.Background(), "SELECT RELEASE_LOCK(CONCAT(DATABASE(), ':gopanel_seed'))")
	}()
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return Result{}, fmt.Errorf("begin seed: %w", err)
	}
	defer tx.Rollback()
	q := dbgen.New(tx)
	existing := 0
	for _, slug := range []string{"empresa-norte", "empresa-sur"} {
		tenant, findErr := q.GetTenantBySlug(ctx, slug)
		if errors.Is(findErr, sql.ErrNoRows) {
			continue
		}
		if findErr != nil {
			return Result{}, fmt.Errorf("inspect seed fixture: %w", findErr)
		}
		metadata, markErr := q.GetSeedMetadata(ctx, sql.NullString{String: tenant.ID, Valid: true})
		if markErr != nil {
			return Result{}, fmt.Errorf("seed tenant exists without completion mark")
		}
		var mark struct {
			Rows int `json:"rows_per_resource"`
		}
		if json.Unmarshal(metadata, &mark) != nil || mark.Rows != options.Count {
			return Result{}, fmt.Errorf("existing seed uses a different dataset; use an empty development database")
		}
		existing++
	}
	if existing == 2 {
		return Result{AlreadySeeded: true}, nil
	}
	if existing != 0 {
		return Result{}, fmt.Errorf("partial seed fixture; refusing to overwrite")
	}

	var credentials []Credential
	userIDs := map[string]string{}
	for _, name := range []string{"owner.norte", "owner.sur", "admin.norte", "admin.sur", "viewer.norte", "viewer.sur", "shared", "platform"} {
		password := randomText(18)
		hash := hashPassword(password)
		userID := id.New()
		userIDs[name] = userID
		email := name + "@gopanel.test"
		if err = q.CreateUser(ctx, dbgen.CreateUserParams{ID: userID, Email: email, PasswordHash: hash, DisplayName: name, IsPlatformAdmin: name == "platform"}); err != nil {
			return Result{}, fmt.Errorf("create seed identity: %w", err)
		}
		credentials = append(credentials, Credential{Email: email, Password: password})
	}
	for _, company := range []string{"norte", "sur"} {
		tenantID := id.New()
		if err = q.CreateTenant(ctx, dbgen.CreateTenantParams{ID: tenantID, Name: "Empresa " + company, Slug: "empresa-" + company}); err != nil {
			return Result{}, fmt.Errorf("create seed tenant: %w", err)
		}
		for _, role := range []string{"owner", "admin", "viewer", "shared"} {
			userName, memberRole := role+"."+company, role
			if role == "shared" {
				userName = "shared"
				memberRole = "admin"
				if company == "sur" {
					memberRole = "viewer"
				}
			}
			if err = q.CreateMembership(ctx, dbgen.CreateMembershipParams{ID: id.New(), TenantID: tenantID, UserID: userIDs[userName], Role: memberRole, IsActive: true}); err != nil {
				return Result{}, fmt.Errorf("create seed membership: %w", err)
			}
		}
		for n := 1; n <= options.Count; n++ {
			if err = q.CreateCustomer(ctx, dbgen.CreateCustomerParams{ID: id.New(), TenantID: tenantID, Name: fmt.Sprintf("Cliente %05d", n), Email: sql.NullString{String: fmt.Sprintf("cliente%05d@%s.test", n, company), Valid: true}, Status: "active"}); err != nil {
				return Result{}, fmt.Errorf("create seed customer: %w", err)
			}
			if err = q.CreateProduct(ctx, dbgen.CreateProductParams{ID: id.New(), TenantID: tenantID, Sku: fmt.Sprintf("SKU-%05d", n), Name: fmt.Sprintf("Producto %05d", n), PriceCents: int64(n) * 100, Currency: "MXN", Status: "active"}); err != nil {
				return Result{}, fmt.Errorf("create seed product: %w", err)
			}
		}
		metadata, _ := json.Marshal(map[string]int{"rows_per_resource": options.Count})
		if err = q.CreateAudit(ctx, dbgen.CreateAuditParams{ID: id.New(), TenantID: sql.NullString{String: tenantID, Valid: true}, ActorUserID: sql.NullString{String: userIDs["owner."+company], Valid: true}, Action: "seed.initialized", EntityType: "tenant", EntityID: sql.NullString{String: tenantID, Valid: true}, Outcome: "success", Metadata: metadata}); err != nil {
			return Result{}, fmt.Errorf("create seed audit: %w", err)
		}
	}
	if err = writeCredentials(options.CredentialsPath, credentials); err != nil {
		return Result{}, fmt.Errorf("write private seed credentials: %w", err)
	}
	if err = tx.Commit(); err != nil {
		// A lost acknowledgement can leave a committed fixture. Preserve its only
		// credential copy until the operator has checked the database state.
		return Result{}, fmt.Errorf("commit seed uncertain; credentials preserved for inspection: %w", err)
	}
	return Result{Tenants: 2, Users: 8, Memberships: 8, Customers: 2 * options.Count, Products: 2 * options.Count}, nil
}

func randomText(size int) string {
	data := make([]byte, size)
	if _, err := rand.Read(data); err != nil {
		panic("secure random source unavailable")
	}
	return base64.RawURLEncoding.EncodeToString(data)
}
func hashPassword(password string) string {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		panic("secure random source unavailable")
	}
	hash := argon2.IDKey([]byte(password), salt, 3, 64*1024, 2, 32)
	return fmt.Sprintf("$argon2id$v=19$m=65536,t=3,p=2$%s$%s", base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(hash))
}
func writeCredentials(path string, credentials []Credential) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	keep := false
	defer func() {
		_ = file.Close()
		if !keep {
			_ = os.Remove(path)
		}
	}()
	if err = json.NewEncoder(file).Encode(credentials); err != nil {
		return err
	}
	if err = file.Sync(); err != nil {
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	keep = true
	return nil
}
