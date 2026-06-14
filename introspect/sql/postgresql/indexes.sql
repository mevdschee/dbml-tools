-- Named parameters: :schema
WITH user_tables AS (
  SELECT schemaname AS tableschema, tablename
  FROM pg_tables
  WHERE schemaname = :schema
),
index_info AS (
  SELECT
    t.relnamespace::regnamespace::text                       AS table_schema,
    t.relname                                                AS table_name,
    i.relname                                                AS index_name,
    ix.indisunique                                           AS is_unique,
    ix.indisprimary                                          AS is_primary,
    am.amname                                                AS index_type,
    array_to_string(array_agg(a.attname ORDER BY x.n), ',') AS columns,
    pg_get_expr(ix.indexprs, ix.indrelid)                   AS expressions,
    CASE
      WHEN ix.indisprimary THEN 'PRIMARY KEY'
      WHEN ix.indisunique  THEN 'UNIQUE'
      ELSE NULL
    END                                                      AS constraint_type
  FROM pg_class t
  JOIN pg_index ix ON t.oid = ix.indrelid
  JOIN pg_class i  ON i.oid = ix.indexrelid
  LEFT JOIN pg_attribute a
    ON a.attrelid = t.oid AND a.attnum = ANY(ix.indkey)
  JOIN pg_am am ON i.relam = am.oid
  LEFT JOIN generate_subscripts(ix.indkey, 1) AS x(n)
    ON a.attnum = ix.indkey[x.n]
  WHERE t.relkind = 'r'
  GROUP BY
    t.relnamespace, t.relname, i.relname,
    ix.indisunique, ix.indisprimary, am.amname,
    ix.indexprs, ix.indrelid
)
SELECT
  ut.tableschema   AS table_schema,
  ut.tablename     AS table_name,
  ii.index_name,
  ii.is_unique,
  ii.is_primary,
  ii.index_type,
  ii.columns       AS index_columns,
  ii.expressions   AS index_expressions,
  ii.constraint_type
FROM user_tables ut
LEFT JOIN index_info ii
  ON ut.tableschema = ii.table_schema AND ut.tablename = ii.table_name
WHERE ii.columns IS NOT NULL
ORDER BY ut.tablename, ii.constraint_type NULLS LAST, ii.index_name
