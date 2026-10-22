// Package auth implements MySQL-backed identity/session workflows, without a cache.
package auth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"gopanel/internal/config"
	dbgen "gopanel/internal/db/generated"
	"gopanel/internal/platform/auth/password"
	"gopanel/internal/platform/id"
)

var ErrCredentials = errors.New("invalid credentials")
var ErrUnauthorized = errors.New("session unavailable")
var ErrTenant = errors.New("tenant unavailable")
var ErrLimited = errors.New("login attempt limit reached")
var ErrBusy = errors.New("password verifier busy")

type Principal struct {
	session dbgen.Session
	hash    [32]byte
	token   string
}

func (p Principal) UserID() string   { return p.session.UserID }
func (p Principal) TenantID() string { return p.session.ActiveTenantID.String }
func (p Principal) CSRF() string {
	mac := hmac.New(sha256.New, []byte(p.token))
	_, _ = mac.Write([]byte("gopanel.csrf.v1"))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

type Issued struct {
	Token     string
	ExpiresAt time.Time
}
type Service struct {
	db        *sql.DB
	cfg       config.Config
	now       func() time.Time
	dummy     string
	verifiers chan struct{}
}

func New(db *sql.DB, cfg config.Config, clock func() time.Time) *Service {
	if clock == nil {
		clock = time.Now
	}
	return &Service{db: db, cfg: cfg, now: clock, dummy: password.Hash(rand.Text()), verifiers: make(chan struct{}, 2)}
}
func newToken() (string, [32]byte) {
	raw := make([]byte, 32)
	_, _ = rand.Read(raw)
	return base64.RawURLEncoding.EncodeToString(raw), sha256.Sum256(raw)
}
func parseToken(value string) ([32]byte, error) {
	if len(value) != 43 {
		return [32]byte{}, ErrUnauthorized
	}
	raw, err := base64.RawURLEncoding.Strict().DecodeString(value)
	if err != nil || len(raw) != 32 {
		return [32]byte{}, ErrUnauthorized
	}
	return sha256.Sum256(raw), nil
}
func optional(value string) sql.NullString { return sql.NullString{String: value, Valid: value != ""} }

func writeAudit(ctx context.Context, q *dbgen.Queries, user, tenant, action, outcome, requestID string) error {
	return q.CreateAudit(ctx, dbgen.CreateAuditParams{ID: id.New(), TenantID: optional(tenant), ActorUserID: optional(user), Action: action, EntityType: "session", Outcome: outcome, RequestID: optional(requestID)})
}
func (s *Service) Denied(ctx context.Context, action, requestID string, p *Principal) error {
	user, tenant := "", ""
	if p != nil {
		user, tenant = p.UserID(), p.TenantID()
	}
	if strings.HasPrefix(action, "platform.") {
		tenant = ""
	}
	if err := writeAudit(ctx, dbgen.New(s.db), user, tenant, action, "denied", requestID); err != nil {
		return fmt.Errorf("audit denied authentication action: %w", err)
	}
	return nil
}

func (s *Service) consumeAttempt(ctx context.Context, ip, requestID string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin login admission: %w", err)
	}
	defer tx.Rollback()
	q := dbgen.New(tx)
	now := s.now().UTC()
	key := sha256.Sum256([]byte(ip))
	if err = q.EnsureLoginLimit(ctx, dbgen.EnsureLoginLimitParams{KeyHash: key[:], WindowStart: now}); err != nil {
		return fmt.Errorf("initialize login admission: %w", err)
	}
	row, err := q.LockLoginLimit(ctx, key[:])
	if err != nil {
		return fmt.Errorf("lock login admission: %w", err)
	}
	if !now.Before(row.WindowStart.Add(time.Minute)) {
		row.WindowStart = now
		row.Attempts = 0
	}
	limited := int(row.Attempts) >= s.cfg.LoginAttemptsPerMinute
	if !limited {
		row.Attempts++
	}
	if err = q.UpdateLoginLimit(ctx, dbgen.UpdateLoginLimitParams{WindowStart: row.WindowStart, Attempts: row.Attempts, KeyHash: key[:]}); err != nil {
		return fmt.Errorf("record login admission: %w", err)
	}
	if limited {
		if err = writeAudit(ctx, q, "", "", "auth.login", "denied", requestID); err != nil {
			return fmt.Errorf("audit login limit: %w", err)
		}
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit login admission: %w", err)
	}
	if limited {
		return ErrLimited
	}
	return nil
}

func (s *Service) Login(ctx context.Context, email, value, oldToken, ip, requestID string) (Issued, error) {
	if err := s.consumeAttempt(ctx, ip, requestID); err != nil {
		return Issued{}, err
	}
	select {
	case s.verifiers <- struct{}{}:
		defer func() { <-s.verifiers }()
	default:
		if err := s.Denied(ctx, "auth.login", requestID, nil); err != nil {
			return Issued{}, err
		}
		return Issued{}, ErrBusy
	}
	q := dbgen.New(s.db)
	user, err := q.GetUserByEmail(ctx, strings.ToLower(strings.TrimSpace(email)))
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return Issued{}, fmt.Errorf("load login identity: %w", err)
	}
	exists := err == nil && user.IsActive
	hash := s.dummy
	if exists {
		hash = user.PasswordHash
	}
	valid, hashErr := password.Verify(value, hash)
	if hashErr != nil {
		_, _ = password.Verify(value, s.dummy)
		valid = false
	}
	if !exists || !valid {
		if err = s.Denied(ctx, "auth.login", requestID, nil); err != nil {
			return Issued{}, err
		}
		return Issued{}, ErrCredentials
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Issued{}, fmt.Errorf("begin login: %w", err)
	}
	defer tx.Rollback()
	q = q.WithTx(tx)
	current, err := q.LockIdentity(ctx, user.ID)
	if err != nil {
		return Issued{}, fmt.Errorf("revalidate login identity: %w", err)
	}
	if !current.IsActive || current.PasswordHash != user.PasswordHash {
		_ = tx.Rollback()
		if err = s.Denied(ctx, "auth.login", requestID, nil); err != nil {
			return Issued{}, err
		}
		return Issued{}, ErrCredentials
	}
	if oldHash, parseErr := parseToken(oldToken); parseErr == nil {
		if err = q.DeleteSession(ctx, oldHash[:]); err != nil {
			return Issued{}, fmt.Errorf("rotate prior login: %w", err)
		}
	}
	token, hashValue := newToken()
	now := s.now().UTC()
	expires := now.Add(s.cfg.SessionAbsoluteTTL)
	if err = q.CreateSession(ctx, dbgen.CreateSessionParams{TokenHash: hashValue[:], UserID: user.ID, CreatedAt: now, LastSeenAt: now, ExpiresAt: expires}); err != nil {
		return Issued{}, fmt.Errorf("create login session: %w", err)
	}
	if err = writeAudit(ctx, q, user.ID, "", "auth.login", "success", requestID); err != nil {
		return Issued{}, fmt.Errorf("audit login: %w", err)
	}
	if err = tx.Commit(); err != nil {
		return Issued{}, fmt.Errorf("commit login: %w", err)
	}
	return Issued{Token: token, ExpiresAt: expires}, nil
}

func (s *Service) Authenticate(ctx context.Context, token string) (Principal, error) {
	return s.authenticate(ctx, token, true)
}

// AuthenticatePassive checks a session without renewing user activity.
func (s *Service) AuthenticatePassive(ctx context.Context, token string) (Principal, error) {
	return s.authenticate(ctx, token, false)
}
func (s *Service) authenticate(ctx context.Context, token string, touch bool) (Principal, error) {
	hash, err := parseToken(token)
	if err != nil {
		return Principal{}, ErrUnauthorized
	}
	now := s.now().UTC()
	q := dbgen.New(s.db)
	session, err := q.GetSession(ctx, dbgen.GetSessionParams{TokenHash: hash[:], Now: now})
	if errors.Is(err, sql.ErrNoRows) {
		return Principal{}, ErrUnauthorized
	}
	if err != nil {
		return Principal{}, fmt.Errorf("load session: %w", err)
	}
	if !session.LastSeenAt.After(now.Add(-s.cfg.SessionIdleTTL)) {
		return Principal{}, ErrUnauthorized
	}
	if touch && !now.Before(session.LastSeenAt.Add(s.cfg.SessionTouchInterval)) {
		if _, err = q.TouchSession(ctx, dbgen.TouchSessionParams{TokenHash: hash[:], Now: now, TouchCutoff: now.Add(-s.cfg.SessionTouchInterval)}); err != nil {
			return Principal{}, fmt.Errorf("touch session: %w", err)
		}
	}
	return Principal{session: session, hash: hash, token: token}, nil
}

func (s *Service) Tenants(ctx context.Context, p Principal) ([]dbgen.ListActiveTenantsForUserRow, error) {
	rows, err := dbgen.New(s.db).ListActiveTenantsForUser(ctx, p.UserID())
	if err != nil {
		return nil, fmt.Errorf("list selectable tenants: %w", err)
	}
	return rows, nil
}
func (s *Service) Membership(ctx context.Context, p Principal) (dbgen.ActiveMembershipRow, error) {
	if p.TenantID() == "" {
		return dbgen.ActiveMembershipRow{}, ErrTenant
	}
	row, err := dbgen.New(s.db).ActiveMembership(ctx, dbgen.ActiveMembershipParams{TenantID: p.TenantID(), UserID: p.UserID()})
	if errors.Is(err, sql.ErrNoRows) {
		return row, ErrTenant
	}
	if err != nil {
		return row, fmt.Errorf("resolve active tenant: %w", err)
	}
	return row, nil
}

func (s *Service) Activate(ctx context.Context, p Principal, tenantID, requestID string) (Issued, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Issued{}, fmt.Errorf("begin tenant activation: %w", err)
	}
	defer tx.Rollback()
	q := dbgen.New(tx)
	if _, err = q.LockTenant(ctx, tenantID); errors.Is(err, sql.ErrNoRows) {
		return Issued{}, ErrTenant
	} else if err != nil {
		return Issued{}, fmt.Errorf("lock activation tenant: %w", err)
	}
	now := s.now().UTC()
	current, err := q.LockSession(ctx, dbgen.LockSessionParams{TokenHash: p.hash[:], Now: now, IdleCutoff: now.Add(-s.cfg.SessionIdleTTL)})
	if errors.Is(err, sql.ErrNoRows) {
		return Issued{}, ErrUnauthorized
	}
	if err != nil {
		return Issued{}, fmt.Errorf("revalidate activation session: %w", err)
	}
	if _, err = q.ActiveMembership(ctx, dbgen.ActiveMembershipParams{TenantID: tenantID, UserID: current.UserID}); errors.Is(err, sql.ErrNoRows) {
		return Issued{}, ErrTenant
	} else if err != nil {
		return Issued{}, fmt.Errorf("verify activation membership: %w", err)
	}
	token, hash := newToken()
	if err = q.DeleteSession(ctx, p.hash[:]); err != nil {
		return Issued{}, fmt.Errorf("rotate tenant session: %w", err)
	}
	if err = q.CreateSession(ctx, dbgen.CreateSessionParams{TokenHash: hash[:], UserID: current.UserID, ActiveTenantID: optional(tenantID), CreatedAt: current.CreatedAt, LastSeenAt: now, ExpiresAt: current.ExpiresAt}); err != nil {
		return Issued{}, fmt.Errorf("create tenant session: %w", err)
	}
	if err = writeAudit(ctx, q, current.UserID, tenantID, "tenants.activate", "success", requestID); err != nil {
		return Issued{}, fmt.Errorf("audit activation: %w", err)
	}
	if err = tx.Commit(); err != nil {
		return Issued{}, fmt.Errorf("commit activation: %w", err)
	}
	return Issued{Token: token, ExpiresAt: current.ExpiresAt}, nil
}
func (s *Service) Logout(ctx context.Context, p Principal, requestID string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin logout: %w", err)
	}
	defer tx.Rollback()
	q := dbgen.New(tx)
	now := s.now().UTC()
	current, err := q.LockSession(ctx, dbgen.LockSessionParams{TokenHash: p.hash[:], Now: now, IdleCutoff: now.Add(-s.cfg.SessionIdleTTL)})
	if errors.Is(err, sql.ErrNoRows) {
		return ErrUnauthorized
	}
	if err != nil {
		return fmt.Errorf("revalidate logout: %w", err)
	}
	if err = q.DeleteSession(ctx, p.hash[:]); err != nil {
		return fmt.Errorf("revoke logout session: %w", err)
	}
	if err = writeAudit(ctx, q, current.UserID, current.ActiveTenantID.String, "auth.logout", "success", requestID); err != nil {
		return fmt.Errorf("audit logout: %w", err)
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit logout: %w", err)
	}
	return nil
}
func (s *Service) Cleanup(ctx context.Context) error {
	q := dbgen.New(s.db)
	now := s.now().UTC()
	if _, err := q.CleanupSessions(ctx, dbgen.CleanupSessionsParams{Now: now, IdleCutoff: now.Add(-s.cfg.SessionIdleTTL)}); err != nil {
		return fmt.Errorf("clean sessions: %w", err)
	}
	if _, err := q.CleanupLoginLimits(ctx, now.Add(-24*time.Hour)); err != nil {
		return fmt.Errorf("clean login limits: %w", err)
	}
	return nil
}
func (s *Service) Maintain(ctx context.Context, logger *slog.Logger) {
	ticker := time.NewTicker(s.cfg.AuthCleanupInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			cleanup, cancel := context.WithTimeout(ctx, 10*time.Second)
			err := s.Cleanup(cleanup)
			cancel()
			if err != nil && ctx.Err() == nil {
				logger.Error("auth_cleanup_failed")
			}
		}
	}
}

// PlatformAdmin reads the current global flag; it never implies a tenant membership.
func (s *Service) PlatformAdmin(ctx context.Context, p Principal) (bool, error) {
	value, err := dbgen.New(s.db).IsPlatformAdmin(ctx, p.UserID())
	if errors.Is(err, sql.ErrNoRows) {
		return false, ErrUnauthorized
	}
	return value, err
}

// StreamAuthorized checks session and membership together, without a touch or cache.
func (s *Service) StreamAuthorized(ctx context.Context, p Principal) error {
	now := s.now().UTC()
	count, err := dbgen.New(s.db).StreamAuthorization(ctx, dbgen.StreamAuthorizationParams{TokenHash: p.hash[:], UserID: p.UserID(), TenantID: optional(p.TenantID()), Now: now, IdleCutoff: now.Add(-s.cfg.SessionIdleTTL)})
	if err != nil {
		return fmt.Errorf("revalidate stream: %w", err)
	}
	if count != 1 {
		return ErrUnauthorized
	}
	return nil
}
