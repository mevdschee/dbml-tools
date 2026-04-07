-- Named parameters: :schema
SELECT
  t.table_name                                              AS tableName,
  COALESCE(t.table_comment, '')                            AS tableComment,
  c.column_name                                             AS columnName,
  c.column_default                                          AS columnDefault,
  CASE
    WHEN c.column_default IS NULL                          THEN 'null'
    WHEN c.column_default = 'NULL'                         THEN 'null'
    WHEN c.data_type = 'enum'                              THEN 'string'
    WHEN c.column_default REGEXP '^-?[0-9]+(\\.[0-9]+)?$' THEN 'number'
    WHEN c.extra LIKE '%DEFAULT_GENERATED%'                THEN 'expression'
    ELSE 'string'
  END                                                       AS defaultValueType,
  c.is_nullable                                             AS columnIsNullable,
  c.data_type                                               AS columnDataType,
  c.column_type                                             AS columnType,
  COALESCE(c.extra, '')                                    AS columnExtra,
  COALESCE(c.column_comment, '')                           AS columnComment,
  COALESCE(c.generation_expression, '')                    AS generationExpression
FROM information_schema.tables t
JOIN information_schema.columns c
  ON t.table_schema = c.table_schema AND t.table_name = c.table_name
WHERE t.table_schema = :schema
  AND t.table_type = 'BASE TABLE'
ORDER BY t.table_name, c.ordinal_position
