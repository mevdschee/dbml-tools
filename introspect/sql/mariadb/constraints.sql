-- Named parameters: :schema
SELECT
  tc.table_name                                                          AS tableName,
  tc.constraint_name                                                     AS constraintName,
  GROUP_CONCAT(kcu.column_name ORDER BY kcu.ordinal_position SEPARATOR ',') AS columnNames,
  COUNT(kcu.column_name)                                                 AS columnCount,
  tc.constraint_type                                                     AS constraintType
FROM information_schema.table_constraints tc
JOIN information_schema.key_column_usage kcu
  ON  tc.constraint_schema = kcu.constraint_schema
  AND tc.constraint_name   = kcu.constraint_name
  AND tc.table_name        = kcu.table_name
WHERE (tc.constraint_type = 'PRIMARY KEY' OR tc.constraint_type = 'UNIQUE')
  AND tc.table_schema = :schema
GROUP BY tc.table_name, tc.constraint_name, tc.constraint_type
ORDER BY tc.table_name, tc.constraint_name
