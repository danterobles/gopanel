//go:build e2e

package e2e_test

import (
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"gopanel/internal/config"
	"gopanel/internal/platform/backup"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"
)

type tableDigest struct {
	Rows   int    `json:"rows"`
	SHA256 string `json:"sha256"`
}

func digest(t *testing.T, db *sql.DB) map[string]tableDigest {
	t.Helper()
	result := map[string]tableDigest{}
	for _, table := range []string{"schema_migrations", "users", "tenants", "tenant_memberships", "customers", "products", "audit_logs", "sessions", "login_rate_limits"} {
		rows, err := db.QueryContext(t.Context(), "SELECT * FROM `"+table+"`")
		if err != nil {
			t.Fatal(err)
		}
		columns, _ := rows.Columns()
		var serialized []string
		for rows.Next() {
			raw := make([]sql.RawBytes, len(columns))
			pointers := make([]any, len(columns))
			for i := range raw {
				pointers[i] = &raw[i]
			}
			if rows.Scan(pointers...) != nil {
				t.Fatal("digest scan")
			}
			value := make([]any, len(raw))
			for i, v := range raw {
				if v != nil {
					value[i] = base64.RawStdEncoding.EncodeToString(v)
				}
			}
			encoded, _ := json.Marshal(value)
			serialized = append(serialized, string(encoded))
		}
		if rows.Err() != nil {
			t.Fatal("digest rows")
		}
		rows.Close()
		sort.Strings(serialized)
		data, _ := json.Marshal(serialized)
		sum := sha256.Sum256(data)
		result[table] = tableDigest{len(serialized), hex.EncodeToString(sum[:])}
	}
	return result
}
func TestNativeBackupRestoreAndAuthentication(t *testing.T) {
	f := setup(t, 30)
	path := filepath.Join(f.dir, "backup.sql")
	start := time.Now()
	if err := backup.Export(t.Context(), f.cfg, path); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0600 || info.Size() == 0 {
		t.Fatal("backup permissions/size", err)
	}
	before := digest(t, f.db)
	restoreCfg, restored := scratch(t, false)
	if err = backup.RestoreTemporary(t.Context(), restoreCfg, path); err != nil {
		t.Fatal(err)
	}
	after := digest(t, restored)
	a, _ := json.Marshal(before)
	b, _ := json.Marshal(after)
	if string(a) != string(b) {
		t.Fatal("restored table digests differ")
	}
	if backup.RestoreTemporary(t.Context(), config.Config{DBName: "goconcept"}, path) == nil {
		t.Fatal("work database accepted as restore target")
	}
	if backup.RestoreTemporary(t.Context(), restoreCfg, path) == nil {
		t.Fatal("nonempty restore target accepted")
	}
	if backup.Export(t.Context(), f.cfg, path) == nil {
		t.Fatal("existing backup overwritten")
	}
	f.cfg = restoreCfg
	f.start(t)
	c := f.client()
	f.login(t, c, "shared@gopanel.test")
	f.selectTenant(t, c, "empresa-norte")
	f.stop(t)
	writeReport(t, "backup-verification.json", map[string]any{"passed": true, "tables": after, "duration_seconds": time.Since(start).Seconds(), "backup_bytes": info.Size(), "mode": "0600", "login_on_restored_database": true})
}
