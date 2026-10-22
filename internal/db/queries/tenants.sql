-- name: CreateTenant :exec
INSERT INTO tenants (id,name,slug) VALUES (?,?,?);

-- name: GetTenantBySlug :one
SELECT id,name,slug,is_active,created_at,updated_at FROM tenants WHERE slug=?;

-- name: GetTenant :one
SELECT id,name,slug,is_active,created_at,updated_at FROM tenants WHERE id=?;

-- name: LockTenant :one
SELECT id FROM tenants WHERE id=? AND is_active=TRUE FOR UPDATE;

-- name: CreateMembership :exec
INSERT INTO tenant_memberships (id,tenant_id,user_id,role,is_active) VALUES (?,?,?,?,?);

-- name: GetMembership :one
SELECT id,tenant_id,user_id,role,is_active,created_at,updated_at FROM tenant_memberships WHERE tenant_id=? AND id=?;

-- name: ListMemberships :many
SELECT id,tenant_id,user_id,role,is_active,created_at,updated_at FROM tenant_memberships WHERE tenant_id=? ORDER BY created_at,id;

-- name: CountActiveOwners :one
SELECT COUNT(*) FROM tenant_memberships m JOIN users u ON u.id=m.user_id
WHERE m.tenant_id=? AND m.role='owner' AND m.is_active=TRUE AND u.is_active=TRUE;

-- name: UpdateMembership :execrows
UPDATE tenant_memberships SET role=?,is_active=? WHERE tenant_id=? AND id=?;
