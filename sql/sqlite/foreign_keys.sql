-- No parameters — SQLite introspects the local file
SELECT
  m.name          AS table_name,
  fk.id           AS fk_id,
  fk.seq          AS seq,
  fk."table"      AS ref_table,
  fk."from"       AS from_column,
  fk."to"         AS to_column,
  fk.on_update,
  fk.on_delete
FROM sqlite_master m
JOIN pragma_foreign_key_list(m.name) fk
WHERE m.type = 'table'
  AND m.name NOT LIKE 'sqlite_%'
ORDER BY m.name, fk.id, fk.seq
