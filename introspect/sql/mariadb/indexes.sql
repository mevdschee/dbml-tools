-- Named parameters: :schema (used twice — once in CTE, once in WHERE)
WITH pk_fk_uniques AS (
  SELECT constraint_name, table_name
  FROM information_schema.table_constraints
  WHERE table_schema = :schema
)
SELECT
  st.table_name                                              AS tableName,
  CASE WHEN st.non_unique = 0 THEN TRUE ELSE FALSE END      AS isIdxUnique,
  st.index_name                                              AS idxName,
  st.column_name                                             AS columnName,
  st.sub_part                                                AS idxSubPart,
  st.index_type                                              AS idxType,
  NULL                                                       AS idxExpression
FROM information_schema.statistics st
WHERE st.table_schema = :schema
  AND st.index_name NOT IN (
    SELECT constraint_name
    FROM pk_fk_uniques pfu
    WHERE pfu.table_name = st.table_name
  )
  AND st.index_type IN ('BTREE', 'HASH')
GROUP BY
  st.table_name, st.non_unique, st.index_name, st.column_name,
  st.sub_part, st.index_type
ORDER BY st.table_name, st.index_name
