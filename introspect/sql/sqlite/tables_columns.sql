-- No parameters — SQLite introspects the local file
SELECT
  m.name         AS table_name,
  p.cid          AS column_id,
  p.name         AS column_name,
  p.type         AS column_type,
  p."notnull"    AS not_null,
  p.dflt_value   AS column_default,
  p.pk           AS pk_order
FROM sqlite_master m
JOIN pragma_table_info(m.name) p
WHERE m.type = 'table'
  AND m.name NOT LIKE 'sqlite_%'
ORDER BY m.name, p.cid
