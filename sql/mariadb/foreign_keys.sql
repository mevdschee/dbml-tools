-- Named parameters: :schema
SELECT
  rc.constraint_name                                                              AS constraintName,
  rc.table_name                                                                   AS foreignTableName,
  GROUP_CONCAT(kcu.column_name            ORDER BY kcu.ordinal_position SEPARATOR ',') AS foreignColumnNames,
  kcu.referenced_table_name                                                       AS refTableName,
  GROUP_CONCAT(kcu.referenced_column_name ORDER BY kcu.ordinal_position SEPARATOR ',') AS refColumnNames,
  rc.update_rule                                                                  AS onUpdate,
  rc.delete_rule                                                                  AS onDelete
FROM information_schema.referential_constraints rc
JOIN information_schema.key_column_usage kcu
  ON  rc.constraint_name   = kcu.constraint_name
  AND rc.table_name        = kcu.table_name
  AND rc.constraint_schema = kcu.table_schema
WHERE rc.constraint_schema = :schema
GROUP BY
  rc.constraint_name, rc.table_name,
  kcu.referenced_table_name,
  rc.update_rule, rc.delete_rule
ORDER BY rc.table_name
