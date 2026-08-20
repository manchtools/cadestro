-- Durable delivery rows.
--
-- A terminal result advances PENDING exactly once, so duplicate results cannot
-- move a delivery backwards.

-- name: InsertDelivery :one
-- Commits the complete manifest before any send is attempted.
INSERT INTO deliveries (
    delivery_id, device_id, manifest_id, manifest,
    state, operation_id, available_at
) VALUES (?, ?, ?, ?, 'PENDING', ?, ?)
RETURNING *;

-- name: GetDelivery :one
SELECT * FROM deliveries WHERE delivery_id = ?;

-- name: MarkDeliveryResult :execrows
-- A per-action and manifest result. Sync is the transport boundary.
UPDATE deliveries
SET state = ?,
    terminal_at = ?,
    result_code = ?
WHERE delivery_id = ?
  AND state = 'PENDING';

-- name: ListDueDeliveriesForDevice :many
-- Due one-shot manifests returned by the authenticated device's Sync.
SELECT * FROM deliveries
WHERE device_id = ?
  AND state = 'PENDING'
  AND available_at <= ?
ORDER BY available_at, created_at
LIMIT ?;
