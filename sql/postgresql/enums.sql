-- Named parameters: :schema
SELECT DISTINCT
  n.nspname        AS schema_name,
  t.typname        AS enum_type,
  e.enumlabel      AS enum_value,
  e.enumsortorder  AS sort_order
FROM pg_enum      e
JOIN pg_type      t ON e.enumtypid   = t.oid
JOIN pg_namespace n ON t.typnamespace = n.oid
WHERE n.nspname = :schema
ORDER BY schema_name, enum_type, sort_order
