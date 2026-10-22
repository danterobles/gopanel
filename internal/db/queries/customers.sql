-- name: CreateCustomer :exec
INSERT INTO customers (id,tenant_id,name,email,phone,status,notes) VALUES (?,?,?,?,?,?,?);

-- name: GetCustomer :one
SELECT id,tenant_id,name,email,phone,status,notes,created_at,updated_at,deleted_at
FROM customers WHERE tenant_id=? AND id=? AND deleted_at IS NULL;

-- name: ListCustomers :many
SELECT id,tenant_id,name,email,phone,status,notes,created_at,updated_at,deleted_at
FROM customers WHERE tenant_id=sqlc.arg(tenant_id) AND deleted_at IS NULL
AND (sqlc.arg(status)='' OR status=sqlc.arg(status))
AND (sqlc.arg(search)='' OR name LIKE CONCAT('%',sqlc.arg(search),'%') ESCAPE '=' OR COALESCE(email,'') LIKE CONCAT('%',sqlc.arg(search),'%') ESCAPE '=')
ORDER BY created_at DESC,id DESC LIMIT ? OFFSET ?;

-- name: CountCustomers :one
SELECT COUNT(*) FROM customers WHERE tenant_id=sqlc.arg(tenant_id) AND deleted_at IS NULL
AND (sqlc.arg(status)='' OR status=sqlc.arg(status))
AND (sqlc.arg(search)='' OR name LIKE CONCAT('%',sqlc.arg(search),'%') ESCAPE '=' OR COALESCE(email,'') LIKE CONCAT('%',sqlc.arg(search),'%') ESCAPE '=');

-- name: UpdateCustomer :execrows
UPDATE customers SET name=?,email=?,phone=?,status=?,notes=? WHERE tenant_id=? AND id=? AND deleted_at IS NULL;

-- name: DeleteCustomer :execrows
UPDATE customers SET deleted_at=? WHERE tenant_id=? AND id=? AND deleted_at IS NULL;
