-- Named parameters: :schema
SELECT
  n.nspname                        AS table_schema,
  c.relname                        AS table_name,
  MAX(a.attname)                   AS column_name,
  con.conname                      AS check_name,
  pg_get_constraintdef(con.oid)    AS check_definition,
  CASE WHEN COUNT(*) = 1 THEN TRUE ELSE FALSE END AS is_column_constraint
FROM pg_catalog.pg_constraint AS con
JOIN pg_catalog.pg_class      AS c  ON con.conrelid = c.oid
LEFT JOIN pg_catalog.pg_namespace AS n ON c.relnamespace = n.oid
LEFT JOIN pg_catalog.pg_attribute AS a
  ON a.attrelid = c.oid AND a.attnum = ANY(con.conkey)
WHERE con.contype = 'c'
  AND n.nspname = :schema
GROUP BY con.conname, n.nspname, c.relname, con.oid
ORDER BY table_name, check_name
