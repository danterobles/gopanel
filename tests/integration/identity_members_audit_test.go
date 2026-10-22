//go:build integration

package integration_test

import (
	"crypto/sha256"
	"database/sql"
	"errors"
	"sync"
	"testing"
	"time"

	dbgen "gopanel/internal/db/generated"
	customers "gopanel/internal/modules/customers/repository"
	members "gopanel/internal/modules/members/repository"
	tenants "gopanel/internal/modules/tenants/repository"
	audit "gopanel/internal/platform/audit/repository"
	identity "gopanel/internal/platform/auth/repository"
	"gopanel/internal/platform/database"
	"gopanel/internal/platform/id"
)

func TestLastOwnerIsProtectedAcrossConcurrentMembershipChanges(t *testing.T) {
	_, db := scratch(t, true)
	ctx := t.Context()
	north, _, first := tenantOwner(t, db, "north")
	south, _, foreign := tenantOwner(t, db, "south")
	q := dbgen.New(db)
	r := members.New(db)
	if err := r.Change(ctx, north, first, "viewer", true); !errors.Is(err, members.ErrLastOwner) {
		t.Fatal("last owner demoted", err)
	}
	if err := r.Change(ctx, north, first, "owner", false); !errors.Is(err, members.ErrLastOwner) {
		t.Fatal("last owner disabled", err)
	}
	if err := r.Change(ctx, north, foreign, "viewer", true); !errors.Is(err, members.ErrNotFound) {
		t.Fatal("cross-tenant member changed", err)
	}
	secondUser := id.New()
	if err := q.CreateUser(ctx, dbgen.CreateUserParams{ID: secondUser, Email: "second@fixture.test", PasswordHash: "$argon2id$fixture", DisplayName: "second"}); err != nil {
		t.Fatal(err)
	}
	second, err := r.Add(ctx, north, secondUser, "owner")
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for _, member := range []string{first, second} {
		wg.Add(1)
		go func() { defer wg.Done(); <-start; results <- r.Change(ctx, north, member, "viewer", true) }()
	}
	close(start)
	wg.Wait()
	close(results)
	success, rejected := 0, 0
	for err := range results {
		if err == nil {
			success++
		} else if errors.Is(err, members.ErrLastOwner) {
			rejected++
		} else {
			t.Fatal(err)
		}
	}
	if success != 1 || rejected != 1 {
		t.Fatalf("concurrent owner changes: success=%d rejected=%d", success, rejected)
	}
	count, err := q.CountActiveOwners(ctx, north)
	if err != nil || count != 1 {
		t.Fatal("owner invariant violated", err)
	}
	count, err = q.CountActiveOwners(ctx, south)
	if err != nil || count != 1 {
		t.Fatal("foreign tenant was changed", err)
	}
}

func TestMembershipSelectionAndSessionPersistence(t *testing.T) {
	cfg, db := scratch(t, true)
	ctx := t.Context()
	north, _, _ := tenantOwner(t, db, "north")
	south, _, _ := tenantOwner(t, db, "south")
	q := dbgen.New(db)
	shared, platform := id.New(), id.New()
	for _, u := range []struct {
		id, email string
		platform  bool
	}{{shared, "shared@fixture.test", false}, {platform, "platform@fixture.test", true}} {
		if err := q.CreateUser(ctx, dbgen.CreateUserParams{ID: u.id, Email: u.email, PasswordHash: "$argon2id$fixture", DisplayName: "identity", IsPlatformAdmin: u.platform}); err != nil {
			t.Fatal(err)
		}
	}
	r := members.New(db)
	if _, err := r.Add(ctx, north, shared, "admin"); err != nil {
		t.Fatal(err)
	}
	sur, err := r.Add(ctx, south, shared, "viewer")
	if err != nil {
		t.Fatal(err)
	}
	selector := tenants.New(db)
	rows, err := selector.ActiveForUser(ctx, shared)
	if err != nil || len(rows) != 2 {
		t.Fatal("shared user's tenants", err)
	}
	roles := map[string]string{}
	for _, row := range rows {
		roles[row.ID] = row.Role
	}
	if roles[north] != "admin" || roles[south] != "viewer" {
		t.Fatal("membership roles lost")
	}
	rows, err = selector.ActiveForUser(ctx, platform)
	if err != nil || len(rows) != 0 {
		t.Fatal("platform admin acquired implicit membership")
	}
	if err = r.Change(ctx, south, sur, "viewer", false); err != nil {
		t.Fatal(err)
	}
	rows, err = selector.ActiveForUser(ctx, shared)
	if err != nil || len(rows) != 1 {
		t.Fatal("inactive membership listed")
	}
	if _, err = db.ExecContext(ctx, "UPDATE tenants SET is_active=FALSE WHERE id=?", north); err != nil {
		t.Fatal(err)
	}
	rows, err = selector.ActiveForUser(ctx, shared)
	if err != nil || len(rows) != 0 {
		t.Fatal("inactive tenant listed")
	}
	auth := identity.New(db)
	user, err := auth.UserByEmail(ctx, " SHARED@FIXTURE.TEST ")
	if err != nil || user.ID != shared {
		t.Fatal("email normalization failed", err)
	}
	hash := sha256.Sum256([]byte("test-token-not-for-production"))
	if err = auth.CreateSession(ctx, hash, shared, sql.NullString{}, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	other, err := database.Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	reopened := identity.New(other)
	session, err := reopened.Session(ctx, hash)
	if err != nil || session.UserID != shared {
		t.Fatal("session did not persist between pools", err)
	}
	if err = reopened.RevokeSession(ctx, hash); err != nil {
		t.Fatal(err)
	}
	if _, err = auth.Session(ctx, hash); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal("revoked session returned", err)
	}
	if err = auth.CreateSession(ctx, hash, shared, sql.NullString{}, time.Now().Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, err = auth.Session(ctx, hash); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal("expired session returned", err)
	}
	if err = auth.RevokeSession(ctx, hash); err != nil {
		t.Fatal(err)
	}
	if err = auth.CreateSession(ctx, hash, shared, sql.NullString{}, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err = db.ExecContext(ctx, "UPDATE users SET is_active=FALSE WHERE id=?", shared); err != nil {
		t.Fatal(err)
	}
	if _, err = auth.Session(ctx, hash); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal("inactive identity session returned", err)
	}
}

func TestResourceAndAuditCommitOrRollbackTogether(t *testing.T) {
	_, db := scratch(t, true)
	ctx := t.Context()
	north, owner, _ := tenantOwner(t, db, "north")
	south, _, _ := tenantOwner(t, db, "south")
	for _, commit := range []bool{false, true} {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		value, err := customers.New(tx).Create(ctx, north, customers.Input{Name: "Transactional", Status: "active"})
		if err != nil {
			tx.Rollback()
			t.Fatal(err)
		}
		if err = audit.New(tx).Append(ctx, north, audit.Entry{ActorID: owner, Action: "customers.create", EntityType: "customer", EntityID: value, Outcome: "success"}); err != nil {
			tx.Rollback()
			t.Fatal(err)
		}
		if commit {
			err = tx.Commit()
		} else {
			err = tx.Rollback()
		}
		if err != nil {
			t.Fatal(err)
		}
		_, err = customers.New(db).Get(ctx, north, value)
		if commit && err != nil {
			t.Fatal(err)
		}
		if !commit && !errors.Is(err, customers.ErrNotFound) {
			t.Fatal("rollback retained resource")
		}
		rows, err := audit.New(db).List(ctx, north, 25, 0)
		if err != nil {
			t.Fatal(err)
		}
		if (!commit && len(rows) != 0) || (commit && len(rows) != 1) {
			t.Fatal("audit not atomic with business mutation")
		}
		rows, err = audit.New(db).List(ctx, south, 25, 0)
		if err != nil || len(rows) != 0 {
			t.Fatal("audit leaked between tenants")
		}
	}
}
