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
ORDER BY
 CASE WHEN sqlc.arg(sort)='name_asc' THEN name END ASC,
 CASE WHEN sqlc.arg(sort)='name_desc' THEN name END DESC,
 CASE WHEN sqlc.arg(sort)='price_asc' THEN price_cents END ASC,
 CASE WHEN sqlc.arg(sort)='price_desc' THEN price_cents END DESC,
 CASE WHEN sqlc.arg(sort)='created_asc' THEN created_at END ASC,
 CASE WHEN sqlc.arg(sort)='created_desc' THEN created_at END DESC,
 CASE WHEN (sqlc.arg(sort)='name_asc' OR sqlc.arg(sort)='price_asc' OR sqlc.arg(sort)='created_asc') THEN id END ASC,
 id DESC LIMIT ? OFFSET ?;

-- name: CountProducts :one
SELECT COUNT(*) FROM products WHERE tenant_id=sqlc.arg(tenant_id) AND deleted_at IS NULL
AND (sqlc.arg(status)='' OR status=sqlc.arg(status))
AND (sqlc.arg(search)='' OR name LIKE CONCAT('%',sqlc.arg(search),'%') ESCAPE '=' OR sku LIKE CONCAT('%',sqlc.arg(search),'%') ESCAPE '=');

-- name: UpdateProduct :execrows
UPDATE products SET sku=?,name=?,description=?,price_cents=?,currency=?,status=? WHERE tenant_id=? AND id=? AND deleted_at IS NULL;

-- name: DeleteProduct :execrows
UPDATE products SET deleted_at=? WHERE tenant_id=? AND id=? AND deleted_at IS NULL;
