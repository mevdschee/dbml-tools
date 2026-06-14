-- Named parameters: :schema
-- Requires MariaDB 10.2.1+ / MySQL 8.0.16+
SELECT
  cc.constraint_name  AS constraintName,
  tc.table_name       AS tableName,
  cc.check_clause     AS checkClause
FROM information_schema.check_constraints cc
JOIN information_schema.table_constraints tc
  ON cc.constraint_name   = tc.constraint_name
  AND cc.constraint_schema = tc.constraint_schema
WHERE cc.constraint_schema = :schema
ORDER BY tc.table_name, cc.constraint_name
