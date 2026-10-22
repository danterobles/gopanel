-- name: CreateUser :exec
INSERT INTO users (id,email,password_hash,display_name,is_platform_admin) VALUES (?,?,?,?,?);

-- name: GetUserByEmail :one
SELECT id,email,password_hash,display_name,is_platform_admin,is_active,created_at,updated_at FROM users WHERE email=?;

-- name: CreateSession :exec
INSERT INTO sessions (token_hash,user_id,active_tenant_id,created_at,last_seen_at,expires_at) VALUES (?,?,?,?,?,?);

-- name: GetSession :one
SELECT s.token_hash,s.user_id,s.active_tenant_id,s.created_at,s.last_seen_at,s.expires_at
FROM sessions s JOIN users u ON u.id=s.user_id
WHERE s.token_hash=sqlc.arg(token_hash) AND s.expires_at>sqlc.arg(now) AND u.is_active=TRUE;

-- name: DeleteSession :exec
DELETE FROM sessions WHERE token_hash=?;

-- name: ListActiveTenantsForUser :many
SELECT t.id,t.name,t.slug,m.role FROM tenant_memberships m
JOIN tenants t ON t.id=m.tenant_id JOIN users u ON u.id=m.user_id
WHERE m.user_id=? AND m.is_active=TRUE AND t.is_active=TRUE AND u.is_active=TRUE
ORDER BY t.name,t.id;

-- name: LockSession :one
SELECT s.token_hash,s.user_id,s.active_tenant_id,s.created_at,s.last_seen_at,s.expires_at
FROM sessions s JOIN users u ON u.id=s.user_id
WHERE s.token_hash=sqlc.arg(token_hash) AND s.expires_at>sqlc.arg(now) AND s.last_seen_at>sqlc.arg(idle_cutoff) AND u.is_active=TRUE FOR UPDATE;

-- name: DeleteSessionRows :execrows
DELETE FROM sessions WHERE token_hash=?;

-- name: TouchSession :execrows
UPDATE sessions SET last_seen_at=sqlc.arg(now)
WHERE token_hash=sqlc.arg(token_hash) AND last_seen_at<=sqlc.arg(touch_cutoff) AND expires_at>sqlc.arg(now);

-- name: ActiveMembership :one
SELECT t.id AS tenant_id,t.name AS tenant_name,m.role
FROM tenant_memberships m JOIN tenants t ON t.id=m.tenant_id JOIN users u ON u.id=m.user_id
WHERE m.tenant_id=? AND m.user_id=? AND m.is_active=TRUE AND t.is_active=TRUE AND u.is_active=TRUE;

-- name: CleanupSessions :execrows
DELETE FROM sessions WHERE expires_at<=sqlc.arg(now) OR last_seen_at<=sqlc.arg(idle_cutoff);

-- name: EnsureLoginLimit :exec
INSERT INTO login_rate_limits (key_hash,window_start,attempts) VALUES (?,?,0)
ON DUPLICATE KEY UPDATE key_hash=key_hash;

-- name: LockLoginLimit :one
SELECT key_hash,window_start,attempts FROM login_rate_limits WHERE key_hash=? FOR UPDATE;

-- name: UpdateLoginLimit :exec
UPDATE login_rate_limits SET window_start=?,attempts=? WHERE key_hash=?;

-- name: CleanupLoginLimits :execrows
DELETE FROM login_rate_limits WHERE window_start<?;

-- name: LockIdentity :one
SELECT id,password_hash,is_active FROM users WHERE id=? FOR UPDATE;

-- name: IsPlatformAdmin :one
SELECT is_platform_admin FROM users WHERE id=? AND is_active=TRUE;

-- name: RevokeUserSessions :exec
DELETE FROM sessions WHERE user_id=?;

-- name: RevokeTenantSessions :exec
DELETE FROM sessions WHERE active_tenant_id=?;

-- name: ActiveUserIDByEmail :one
SELECT id FROM users WHERE email=sqlc.arg(email) AND CAST(email AS BINARY)=CAST(sqlc.arg(email) AS BINARY) AND is_active=TRUE;

-- name: StreamAuthorization :one
SELECT COUNT(*) FROM sessions s
JOIN users u ON u.id=s.user_id
JOIN tenants t ON t.id=s.active_tenant_id
JOIN tenant_memberships m ON m.tenant_id=t.id AND m.user_id=u.id
WHERE s.token_hash=sqlc.arg(token_hash) AND s.user_id=sqlc.arg(user_id) AND s.active_tenant_id=sqlc.arg(tenant_id)
AND s.expires_at>sqlc.arg(now) AND s.last_seen_at>sqlc.arg(idle_cutoff)
AND u.is_active=TRUE AND t.is_active=TRUE AND m.is_active=TRUE AND m.role IN ('owner','admin','viewer');
