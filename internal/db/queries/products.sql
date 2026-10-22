-- name: CreateProduct :exec
INSERT INTO products (id,tenant_id,sku,name,description,price_cents,currency,status) VALUES (?,?,?,?,?,?,?,?);

-- name: GetProduct :one
SELECT id,tenant_id,sku,name,description,price_cents,currency,status,created_at,updated_at,deleted_at,active_sku
FROM products WHERE tenant_id=? AND id=? AND deleted_at IS NULL;

-- name: ListProducts :many
SELECT id,tenant_id,sku,name,description,price_cents,currency,status,created_at,updated_at,deleted_at,active_sku
FROM products WHERE tenant_id=sqlc.arg(tenant_id) AND deleted_at IS NULL
AND (sqlc.arg(status)='' OR status=sqlc.arg(status))
AND (sqlc.arg(search)='' OR name LIKE CONCAT('%',sqlc.arg(search),'%') ESCAPE '=' OR sku LIKE CONCAT('%',sqlc.arg(search),'%') ESCAPE '=')
ORDER BY created_at DESC,id DESC LIMIT ? OFFSET ?;

-- name: CountProducts :one
SELECT COUNT(*) FROM products WHERE tenant_id=sqlc.arg(tenant_id) AND deleted_at IS NULL
AND (sqlc.arg(status)='' OR status=sqlc.arg(status))
AND (sqlc.arg(search)='' OR name LIKE CONCAT('%',sqlc.arg(search),'%') ESCAPE '=' OR sku LIKE CONCAT('%',sqlc.arg(search),'%') ESCAPE '=');

-- name: UpdateProduct :execrows
UPDATE products SET sku=?,name=?,description=?,price_cents=?,currency=?,status=? WHERE tenant_id=? AND id=? AND deleted_at IS NULL;

-- name: DeleteProduct :execrows
UPDATE products SET deleted_at=? WHERE tenant_id=? AND id=? AND deleted_at IS NULL;
