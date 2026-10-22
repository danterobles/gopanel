// Package backup exports private MySQL snapshots and rehearses restoration only in empty scratch databases.
package backup

import (
	"context"
	"fmt"
	"gopanel/internal/config"
	"gopanel/internal/platform/database"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

func option(value string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`, "\r", `\r`).Replace(value) + `"`
}
func defaults(c config.Config) (string, func(), error) {
	f, err := os.CreateTemp("", "gopanel-mysql-*.cnf")
	if err != nil {
		return "", nil, fmt.Errorf("create private MySQL options: %w", err)
	}
	cleanup := func() { os.Remove(f.Name()) }
	mode := "DISABLED"
	if c.DBTLS {
		mode = "VERIFY_IDENTITY"
	}
	_, err = fmt.Fprintf(f, "[client]\nuser=%s\npassword=%s\nhost=%s\nport=%s\nprotocol=TCP\nssl-mode=%s\n", option(c.DBUser), option(c.DBPassword), option(c.DBHost), option(c.DBPort), mode)
	closeErr := f.Close()
	if err != nil || closeErr != nil {
		cleanup()
		return "", nil, fmt.Errorf("write private MySQL options")
	}
	return f.Name(), cleanup, nil
}
func Export(ctx context.Context, c config.Config, path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("create backup directory: %w", err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("create exclusive backup: %w", err)
	}
	keep := false
	defer func() {
		f.Close()
		if !keep {
			os.Remove(path)
		}
	}()
	options, cleanup, err := defaults(c)
	if err != nil {
		return err
	}
	defer cleanup()
	command := exec.CommandContext(ctx, "mysqldump", "--defaults-file="+options, "--single-transaction", "--quick", "--no-tablespaces", "--set-gtid-purged=OFF", "--column-statistics=0", "--skip-comments", "--hex-blob", "--", c.DBName)
	command.Stdout = f
	if err = command.Run(); err != nil {
		return fmt.Errorf("mysqldump failed (diagnostics suppressed): %w", err)
	}
	if err = f.Sync(); err != nil {
		return fmt.Errorf("sync backup: %w", err)
	}
	if err = f.Close(); err != nil {
		return fmt.Errorf("close backup: %w", err)
	}
	keep = true
	return nil
}

var scratchName = regexp.MustCompile(`^gopanel_test_[0-9a-f]{32}$`)

func RestoreTemporary(ctx context.Context, c config.Config, path string) error {
	if !scratchName.MatchString(c.DBName) {
		return fmt.Errorf("restore requires a disposable gopanel_test UUID database")
	}
	db, err := database.Open(c)
	if err != nil {
		return err
	}
	defer db.Close()
	var count int
	if err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=?", c.DBName).Scan(&count); err != nil {
		return fmt.Errorf("inspect restoration target: %w", err)
	}
	if count != 0 {
		return fmt.Errorf("restoration target must be empty")
	}
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open backup: %w", err)
	}
	defer file.Close()
	options, cleanup, err := defaults(c)
	if err != nil {
		return err
	}
	defer cleanup()
	command := exec.CommandContext(ctx, "mysql", "--defaults-file="+options, "--", c.DBName)
	command.Stdin = file
	if err = command.Run(); err != nil {
		return fmt.Errorf("mysql restore failed (diagnostics suppressed): %w", err)
	}
	return nil
}
