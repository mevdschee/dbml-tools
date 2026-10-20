-- No parameters — SQLite introspects the local file
-- origin: 'c' = CREATE INDEX, 'u' = UNIQUE constraint, 'pk' = primary key (excluded)
-- column_name is NULL for an expression key; its text is only in index_sql.
SELECT
  m.name          AS table_name,
  il.name         AS index_name,
  il."unique"     AS is_unique,
  il.origin       AS origin,
  ii.seqno        AS column_position,
  ii.name         AS column_name,
  (SELECT s.sql FROM sqlite_master s
   WHERE s.type = 'index' AND s.name = il.name) AS index_sql
FROM sqlite_master m
JOIN pragma_index_list(m.name) il
JOIN pragma_index_info(il.name) ii
WHERE m.type = 'table'
  AND m.name NOT LIKE 'sqlite_%'
  AND il.origin != 'pk'
ORDER BY m.name, il.name, ii.seqno
