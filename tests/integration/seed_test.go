//go:build integration

package integration_test

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/crypto/argon2"
	dbgen "gopanel/internal/db/generated"
	"gopanel/internal/platform/seed"
)

func TestSeedIsIdempotentAndKeepsCredentialsPrivate(t *testing.T) {
	_, db := scratch(t, true)
	ctx := t.Context()
	path := filepath.Join(t.TempDir(), "private", "credentials.json")
	options := seed.Options{Count: 100, CredentialsPath: path}
	result, err := seed.Run(ctx, db, options)
	if err != nil {
		t.Fatal(err)
	}
	if result.Tenants != 2 || result.Users != 8 || result.Memberships != 8 || result.Customers != 200 || result.Products != 200 {
		t.Fatalf("unexpected seed counts: %+v", result)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0077 != 0 {
		t.Fatal("credentials readable by other users")
	}
	var credentials []seed.Credential
	if err = json.Unmarshal(data, &credentials); err != nil || len(credentials) != 8 {
		t.Fatal("invalid private credentials file")
	}
	q := dbgen.New(db)
	hashes := map[string]bool{}
	for _, credential := range credentials {
		user, err := q.GetUserByEmail(ctx, credential.Email)
		if err != nil {
			t.Fatal(err)
		}
		if len(credential.Password) < 24 || !strings.HasPrefix(user.PasswordHash, "$argon2id$v=19$") || strings.Contains(user.PasswordHash, credential.Password) || hashes[user.PasswordHash] {
			t.Fatal("invalid credential hashing")
		}
		hashes[user.PasswordHash] = true
	}
	user, err := q.GetUserByEmail(ctx, credentials[0].Email)
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(user.PasswordHash, "$")
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		t.Fatal(err)
	}
	key, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(key, argon2.IDKey([]byte(credentials[0].Password), salt, 3, 65536, 2, 32)) {
		t.Fatal("seed hash does not match generated password")
	}
	again, err := seed.Run(ctx, db, options)
	if err != nil || !again.AlreadySeeded {
		t.Fatal("repeat seed was not idempotent", err)
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(data, after) {
		t.Fatal("repeat seed replaced credentials")
	}
	for table, want := range map[string]int{"tenants": 2, "users": 8, "tenant_memberships": 8, "customers": 200, "products": 200, "audit_logs": 2, "sessions": 0} {
		var count int
		if err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table).Scan(&count); err != nil || count != want {
			t.Fatalf("%s count %d want %d: %v", table, count, want, err)
		}
	}
	if _, err = seed.Run(ctx, db, seed.Options{Count: 10000, CredentialsPath: path}); err == nil {
		t.Fatal("dataset change silently accepted")
	}
}

func TestSeedCredentialWriteFailureRollsBackEverything(t *testing.T) {
	_, db := scratch(t, true)
	path := filepath.Join(t.TempDir(), "already-exists")
	if err := os.WriteFile(path, []byte("preserve"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := seed.Run(t.Context(), db, seed.Options{Count: 1, CredentialsPath: path}); err == nil {
		t.Fatal("credential overwrite accepted")
	}
	for _, table := range []string{"users", "tenants", "tenant_memberships", "customers", "products", "audit_logs"} {
		var count int
		if err := db.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM "+table).Scan(&count); err != nil || count != 0 {
			t.Fatalf("failed seed retained %s", table)
		}
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "preserve" {
		t.Fatal("existing credential file changed")
	}
}
