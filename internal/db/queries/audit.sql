-- name: CreateAudit :exec
INSERT INTO audit_logs (id,tenant_id,actor_user_id,action,entity_type,entity_id,outcome,request_id,metadata) VALUES (?,?,?,?,?,?,?,?,?);

-- name: ListAudit :many
SELECT id,tenant_id,actor_user_id,action,entity_type,entity_id,outcome,request_id,metadata,created_at FROM audit_logs
WHERE tenant_id=? ORDER BY created_at DESC,id DESC LIMIT ? OFFSET ?;

-- name: CountSeedMarks :one
SELECT COUNT(*) FROM audit_logs WHERE tenant_id=? AND action='seed.initialized' AND outcome='success';

-- name: GetSeedMetadata :one
SELECT metadata FROM audit_logs WHERE tenant_id=? AND action='seed.initialized' AND outcome='success' ORDER BY created_at,id LIMIT 1;
