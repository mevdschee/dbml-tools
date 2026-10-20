-- Named parameters: :schema
--
-- index_keys lists the keys of an index in order, separated by a unit
-- separator (chr(31)). Each key is 'C:<column name>' or 'E:<expression>'.
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
    (
      SELECT string_agg(
        CASE
          WHEN ix.indkey[k - 1] = 0
            THEN 'E:' || pg_get_indexdef(ix.indexrelid, k, true)
          ELSE 'C:' || (
            SELECT a.attname FROM pg_attribute a
            WHERE a.attrelid = ix.indrelid AND a.attnum = ix.indkey[k - 1]
          )
        END,
        chr(31) ORDER BY k)
      FROM generate_series(1, ix.indnkeyatts) AS k
    )                                                        AS index_keys,
    CASE
      WHEN ix.indisprimary THEN 'PRIMARY KEY'
      -- A unique index created with CREATE UNIQUE INDEX backs no constraint.
      WHEN EXISTS (
        SELECT 1 FROM pg_constraint con
        WHERE con.conindid = ix.indexrelid AND con.contype = 'u'
      ) THEN 'UNIQUE'
      ELSE NULL
    END                                                      AS constraint_type
  FROM pg_class t
  JOIN pg_index ix ON t.oid = ix.indrelid
  JOIN pg_class i  ON i.oid = ix.indexrelid
  JOIN pg_am am ON i.relam = am.oid
  WHERE t.relkind = 'r'
)
SELECT
  ut.tableschema   AS table_schema,
  ut.tablename     AS table_name,
  ii.index_name,
  ii.is_unique,
  ii.is_primary,
  ii.index_type,
  ii.index_keys,
  ii.constraint_type
FROM user_tables ut
JOIN index_info ii
  ON ut.tableschema = ii.table_schema AND ut.tablename = ii.table_name
WHERE ii.index_keys IS NOT NULL
ORDER BY ut.tablename, ii.constraint_type NULLS LAST, ii.index_name
