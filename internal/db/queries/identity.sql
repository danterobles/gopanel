-- name: CreateUser :exec
INSERT INTO users (id,email,password_hash,display_name,is_platform_admin) VALUES (?,?,?,?,?);

-- name: GetUserByEmail :one
SELECT id,email,password_hash,display_name,is_platform_admin,is_active,created_at,updated_at FROM users WHERE email=?;

-- name: CreateSession :exec
INSERT INTO sessions (token_hash,user_id,active_tenant_id,expires_at) VALUES (?,?,?,?);

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
