-- Named parameters: :schema
WITH comments AS (
  SELECT DISTINCT ON (pc.relname, pn.nspname, pa.attname)
    pc.relname  AS table_name,
    pn.nspname  AS table_schema,
    pa.attname  AS column_name,
    pd.description
  FROM pg_description pd
  JOIN pg_class     pc ON pd.objoid = pc.oid
  JOIN pg_namespace pn ON pc.relnamespace = pn.oid
  LEFT JOIN pg_attribute pa
    ON pd.objoid = pa.attrelid AND pd.objsubid = pa.attnum
  WHERE pc.relkind = 'r'
    AND pn.nspname NOT IN ('pg_catalog', 'information_schema')
),
table_positions AS (
  SELECT
    pc.oid      AS table_ordinal_position,
    pc.relname  AS table_name,
    pn.nspname  AS table_schema
  FROM pg_class     pc
  JOIN pg_namespace pn ON pc.relnamespace = pn.oid
  WHERE pc.relkind = 'r'
    AND pn.nspname = :schema
)
SELECT
  t.table_schema,
  t.table_name,
  c.column_name,
  c.data_type,
  c.character_maximum_length,
  c.numeric_precision,
  c.numeric_scale,
  c.udt_schema,
  c.udt_name,
  c.identity_increment,
  c.is_nullable,
  c.column_default,
  c.ordinal_position,
  CASE
    WHEN c.column_default IS NULL                              THEN NULL
    WHEN c.column_default LIKE 'nextval(%'                    THEN 'increment'
    WHEN c.column_default LIKE '''%'                          THEN 'string'
    WHEN c.column_default = 'true' OR c.column_default = 'false' THEN 'boolean'
    WHEN c.column_default ~ '^-?[0-9]+(.[0-9]+)?$'           THEN 'number'
    ELSE 'expression'
  END AS default_type,
  (SELECT description FROM comments
   WHERE table_name = t.table_name AND table_schema = t.table_schema
     AND column_name IS NULL LIMIT 1)                         AS table_comment,
  (SELECT description FROM comments
   WHERE table_name = t.table_name AND table_schema = t.table_schema
     AND column_name = c.column_name LIMIT 1)                AS column_comment
FROM information_schema.columns c
JOIN information_schema.tables  t
  ON c.table_name = t.table_name AND c.table_schema = t.table_schema
JOIN table_positions tp
  ON tp.table_name = t.table_name AND tp.table_schema = t.table_schema
WHERE t.table_type = 'BASE TABLE'
  AND t.table_schema = :schema
ORDER BY t.table_schema, tp.table_ordinal_position, c.ordinal_position
