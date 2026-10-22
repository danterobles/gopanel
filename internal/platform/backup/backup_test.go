package backup

import (
	"context"
	"gopanel/internal/config"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFailedDumpCleansItsPartialFileButPreservesExistingBackup(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	path := filepath.Join(t.TempDir(), "snapshot.sql")
	cfg := config.Config{DBName: "fixture", DBUser: "fixture", DBPassword: "private-fixture", DBHost: "127.0.0.1", DBPort: "3306"}
	if Export(context.Background(), cfg, path) == nil {
		t.Fatal("dump should fail without executable")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("partial output retained")
	}
	if os.WriteFile(path, []byte("existing backup"), 0600) != nil {
		t.Fatal("write fixture")
	}
	if Export(context.Background(), cfg, path) == nil {
		t.Fatal("overwrote existing file")
	}
	data, _ := os.ReadFile(path)
	if string(data) != "existing backup" {
		t.Fatal("existing backup changed")
	}
}
func TestPrivateOptionsCannotInjectConfiguration(t *testing.T) {
	path, cleanup, err := defaults(config.Config{DBUser: "fixture", DBPassword: "quote\"\n[malicious]\npassword=other\\", DBHost: "127.0.0.1", DBPort: "3306"})
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0600 {
		t.Fatal("options permissions")
	}
	data, _ := os.ReadFile(path)
	if strings.Count(string(data), "\n") != 7 || strings.Contains(string(data), "\n[malicious]") {
		t.Fatal("credential injected configuration")
	}
	cleanup()
	if _, err = os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("options not cleaned")
	}
}
