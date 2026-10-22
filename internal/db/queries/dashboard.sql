-- name: DashboardActiveCustomers :one
SELECT COUNT(*) FROM customers WHERE tenant_id = ? AND status = 'active' AND deleted_at IS NULL;

-- name: DashboardActiveProducts :one
SELECT COUNT(*) FROM products WHERE tenant_id = ? AND status = 'active' AND deleted_at IS NULL;

-- name: DashboardCustomerDays :many
SELECT DATE(created_at) AS day, COUNT(*) AS total FROM customers
WHERE tenant_id = sqlc.arg(tenant_id) AND deleted_at IS NULL
AND created_at >= sqlc.arg(window_start) AND created_at < sqlc.arg(window_end)
GROUP BY DATE(created_at) ORDER BY DATE(created_at);
