


-- name: NextAuditEventSeq :one
SELECT CAST(COALESCE(MAX(chain_seq), 0) + 1 AS INTEGER)
FROM (
    SELECT chain_seq FROM audit_operations WHERE audit_operations.stream = sqlc.arg(stream)
    UNION ALL
    SELECT chain_seq FROM audit_effects WHERE audit_effects.stream = sqlc.arg(stream)
);

-- name: InsertAuditOperation :one
INSERT INTO audit_operations (
    operation_id, stream, chain_seq,
    operation_class, actor_type, actor_id, actor_fingerprint,
    origin, origin_fingerprint, request_descriptor,
    authorization_outcome, authorization_detail,
    result, result_code, occurred_at,
    sealed_detail, sealed_detail_subject
) VALUES (
    ?, ?, ?,
    ?, ?, ?, ?,
    ?, ?, ?,
    ?, ?,
    ?, ?, ?,
    ?, ?
)
RETURNING *;

-- name: InsertAuditEffect :one
INSERT INTO audit_effects (
    effect_id, operation_id, stream, chain_seq, effect_seq,
    resource_type, resource_id, action, outcome,
    changed_fields,
    before_ref, after_ref, before_flag, after_flag, before_count, after_count,
    evidence_kind, evidence_fingerprint,
    sealed_detail, sealed_detail_subject, occurred_at
) VALUES (
    ?, ?, ?, ?, ?,
    ?, ?, ?, ?,
    ?,
    ?, ?, ?, ?, ?, ?,
    ?, ?,
    ?, ?, ?
)
RETURNING *;

-- name: GetAuditOperation :one
SELECT * FROM audit_operations WHERE operation_id = ?;

-- name: CountAuditOperations :one
SELECT COUNT(*) FROM audit_operations WHERE stream = ?;

-- name: NextAuditEffectSeq :one
SELECT CAST(COALESCE(MAX(effect_seq) + 1, 0) AS INTEGER)
FROM audit_effects WHERE operation_id = ?;

-- name: ListAuditEffectsForOperation :many
SELECT * FROM audit_effects WHERE operation_id = ? ORDER BY effect_seq;

-- name: ListAuditEventRows :many
SELECT * FROM audit_event_rows
WHERE (sqlc.arg(actor_id) = '' OR actor_id = sqlc.arg(actor_id))
  AND (sqlc.arg(event_type) = '' OR instr(lower(event_type), lower(sqlc.arg(event_type))) > 0)
  AND occurred_at >= sqlc.arg(filter_from_time)
  AND occurred_at <= sqlc.arg(filter_to_time)
  AND (sqlc.arg(before_seq) = 0 OR chain_seq < sqlc.arg(before_seq))
  AND (json_array_length(sqlc.arg(stream_types_json)) = 0 OR stream_type IN (
      SELECT CAST(value AS TEXT) FROM json_each(sqlc.arg(stream_types_json))))
ORDER BY chain_seq DESC
LIMIT sqlc.arg(row_limit);

-- name: CountAuditEventRows :one
SELECT COUNT(*) FROM audit_event_rows
WHERE (sqlc.arg(actor_id) = '' OR actor_id = sqlc.arg(actor_id))
  AND (sqlc.arg(event_type) = '' OR instr(lower(event_type), lower(sqlc.arg(event_type))) > 0)
  AND (json_array_length(sqlc.arg(stream_types_json)) = 0 OR stream_type IN (
      SELECT CAST(value AS TEXT) FROM json_each(sqlc.arg(stream_types_json))));
