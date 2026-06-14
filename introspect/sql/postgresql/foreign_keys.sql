-- Named parameters: :schema
SELECT
  tc.table_schema,
  tc.table_name,
  tc.constraint_name                                     AS fk_constraint_name,
  STRING_AGG(kcu.column_name, ',' ORDER BY kcu.ordinal_position) AS column_names,
  ccu.table_schema                                       AS foreign_table_schema,
  ccu.table_name                                         AS foreign_table_name,
  STRING_AGG(ccu.column_name, ',' ORDER BY kcu.ordinal_position) AS foreign_column_names,
  rc.delete_rule                                         AS on_delete,
  rc.update_rule                                         AS on_update
FROM information_schema.table_constraints AS tc
JOIN information_schema.key_column_usage AS kcu
  ON  tc.constraint_name = kcu.constraint_name
  AND tc.table_schema    = kcu.table_schema
JOIN information_schema.constraint_column_usage AS ccu
  ON ccu.constraint_name = tc.constraint_name
JOIN information_schema.referential_constraints AS rc
  ON  tc.constraint_name  = rc.constraint_name
  AND tc.table_schema     = rc.constraint_schema
WHERE tc.constraint_type = 'FOREIGN KEY'
  AND tc.table_schema = :schema
GROUP BY
  tc.table_schema, tc.table_name, tc.constraint_name,
  ccu.table_schema, ccu.table_name,
  rc.delete_rule, rc.update_rule
ORDER BY tc.table_schema, tc.table_name
