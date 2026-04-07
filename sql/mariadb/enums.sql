-- Named parameters: :schema
SELECT
  t.table_name                                 AS tableName,
  c.column_name                                AS columnName,
  TRIM(LEADING 'enum' FROM c.column_type)      AS rawValues
FROM information_schema.tables t
JOIN information_schema.columns c
  ON t.table_schema = c.table_schema AND t.table_name = c.table_name
WHERE t.table_schema = :schema
  AND t.table_type = 'BASE TABLE'
  AND c.data_type = 'enum'
ORDER BY t.table_name, c.ordinal_position
