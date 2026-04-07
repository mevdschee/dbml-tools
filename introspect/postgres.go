package introspect

import (
	"database/sql"
	"embed"
	"fmt"
	"strings"
)

func introspectPostgres(db *sql.DB, schema string, sqlFS embed.FS) (*DBSchema, error) {
	result := &DBSchema{}
	tableMap := map[string]*Table{}
	var tableOrder []string
	p := map[string]interface{}{"schema": schema}

	// ── 1. Tables & columns ──────────────────────────────────────────────────
	q, err := readSQL(sqlFS, "sql/postgresql/tables_columns.sql")
	if err != nil {
		return nil, err
	}
	rows, err := runQuery(db, EnginePostgres, q, p)
	if err != nil {
		return nil, fmt.Errorf("tables_columns: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			tableSchema, tableName, columnName string
			dataType                           string
			charMaxLen                         sql.NullInt64
			numPrecision, numScale             sql.NullInt64
			udtSchema, udtName                 string
			identityIncrement                  sql.NullString
			isNullable                         string
			columnDefault                      sql.NullString
			ordinalPosition                    int
			defaultType                        sql.NullString
			tableComment, columnComment        sql.NullString
		)
		if err := rows.Scan(
			&tableSchema, &tableName, &columnName,
			&dataType, &charMaxLen, &numPrecision, &numScale,
			&udtSchema, &udtName,
			&identityIncrement, &isNullable, &columnDefault, &ordinalPosition,
			&defaultType, &tableComment, &columnComment,
		); err != nil {
			return nil, fmt.Errorf("tables_columns scan: %w", err)
		}

		t, ok := tableMap[tableName]
		if !ok {
			comment := ""
			if tableComment.Valid {
				comment = tableComment.String
			}
			t = &Table{Schema: tableSchema, Name: tableName, Comment: comment}
			tableMap[tableName] = t
			tableOrder = append(tableOrder, tableName)
		}

		col := &Column{
			Name:       columnName,
			Type:       pgColumnType(dataType, charMaxLen, numPrecision, numScale, udtSchema, udtName),
			IsNullable: isNullable == "YES",
		}
		if columnComment.Valid {
			col.Comment = columnComment.String
		}
		if columnDefault.Valid {
			col.Default = &columnDefault.String
			if defaultType.Valid {
				col.DefaultType = defaultType.String
				col.IsIncrement = col.DefaultType == "increment"
			}
		}
		if identityIncrement.Valid {
			col.IsIncrement = true
		}
		t.Columns = append(t.Columns, col)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for _, name := range tableOrder {
		result.Tables = append(result.Tables, tableMap[name])
	}

	// ── 2. Indexes (includes PK / UNIQUE constraints) ─────────────────────────
	q, err = readSQL(sqlFS, "sql/postgresql/indexes.sql")
	if err != nil {
		return nil, err
	}
	rows2, err := runQuery(db, EnginePostgres, q, p)
	if err != nil {
		return nil, fmt.Errorf("indexes: %w", err)
	}
	defer rows2.Close()

	for rows2.Next() {
		var (
			tableSchema, tableName, indexName string
			isUnique, isPrimary               bool
			indexType                         string
			indexColumns                      string
			indexExpressions                  sql.NullString
			constraintType                    sql.NullString
		)
		if err := rows2.Scan(
			&tableSchema, &tableName, &indexName,
			&isUnique, &isPrimary, &indexType,
			&indexColumns, &indexExpressions, &constraintType,
		); err != nil {
			return nil, fmt.Errorf("indexes scan: %w", err)
		}

		t, ok := tableMap[tableName]
		if !ok {
			continue
		}
		cols := splitCols(indexColumns)

		if isPrimary || (isUnique && constraintType.Valid) {
			// Treat as a named constraint
			isPK := isPrimary
			c := &Constraint{Name: indexName, Columns: cols, IsPrimary: isPK}
			t.Constraints = append(t.Constraints, c)
			// Annotate single-column constraints on the column itself
			if len(cols) == 1 {
				for _, col := range t.Columns {
					if col.Name == cols[0] {
						if isPK {
							col.IsPrimaryKey = true
						} else {
							col.IsUnique = true
						}
						break
					}
				}
			}
		} else {
			// Regular index
			idx := &Index{
				Name:     indexName,
				Columns:  cols,
				IsUnique: isUnique,
				Type:     indexType,
			}
			if indexExpressions.Valid {
				idx.Expression = indexExpressions.String
			}
			t.Indexes = append(t.Indexes, idx)
		}
	}
	if err := rows2.Err(); err != nil {
		return nil, err
	}

	// ── 3. Check constraints ─────────────────────────────────────────────────
	q, err = readSQL(sqlFS, "sql/postgresql/check_constraints.sql")
	if err != nil {
		return nil, err
	}
	rows3, err := runQuery(db, EnginePostgres, q, p)
	if err != nil {
		return nil, fmt.Errorf("check_constraints: %w", err)
	}
	defer rows3.Close()

	for rows3.Next() {
		var tableSchema, tableName string
		var columnName, checkName, checkDef sql.NullString
		var isColConstraint bool
		if err := rows3.Scan(&tableSchema, &tableName, &columnName, &checkName, &checkDef, &isColConstraint); err != nil {
			return nil, fmt.Errorf("check_constraints scan: %w", err)
		}
		if t, ok := tableMap[tableName]; ok && checkName.Valid && checkDef.Valid {
			cc := &CheckConstraint{Name: checkName.String, Expression: checkDef.String}
			if columnName.Valid {
				cc.ColumnName = columnName.String
			}
			t.CheckConstraints = append(t.CheckConstraints, cc)
		}
	}
	if err := rows3.Err(); err != nil {
		return nil, err
	}

	// ── 4. Enums ─────────────────────────────────────────────────────────────
	q, err = readSQL(sqlFS, "sql/postgresql/enums.sql")
	if err != nil {
		return nil, err
	}
	rows4, err := runQuery(db, EnginePostgres, q, p)
	if err != nil {
		return nil, fmt.Errorf("enums: %w", err)
	}
	defer rows4.Close()

	enumMap := map[string]*Enum{}
	var enumOrder []string

	for rows4.Next() {
		var schemaName, enumType, enumValue string
		var sortOrder float64
		if err := rows4.Scan(&schemaName, &enumType, &enumValue, &sortOrder); err != nil {
			return nil, fmt.Errorf("enums scan: %w", err)
		}
		key := schemaName + "." + enumType
		e, ok := enumMap[key]
		if !ok {
			e = &Enum{Schema: schemaName, Name: enumType}
			enumMap[key] = e
			enumOrder = append(enumOrder, key)
		}
		e.Values = append(e.Values, enumValue)
	}
	if err := rows4.Err(); err != nil {
		return nil, err
	}
	for _, key := range enumOrder {
		result.Enums = append(result.Enums, enumMap[key])
	}

	// ── 5. Foreign keys ──────────────────────────────────────────────────────
	q, err = readSQL(sqlFS, "sql/postgresql/foreign_keys.sql")
	if err != nil {
		return nil, err
	}
	rows5, err := runQuery(db, EnginePostgres, q, p)
	if err != nil {
		return nil, fmt.Errorf("foreign_keys: %w", err)
	}
	defer rows5.Close()

	for rows5.Next() {
		var (
			tableSchema, tableName, fkName string
			columnNames                    string
			foreignTableSchema             string
			foreignTableName               string
			foreignColumnNames             string
			onDelete, onUpdate             sql.NullString
		)
		if err := rows5.Scan(
			&tableSchema, &tableName, &fkName,
			&columnNames, &foreignTableSchema, &foreignTableName, &foreignColumnNames,
			&onDelete, &onUpdate,
		); err != nil {
			return nil, fmt.Errorf("foreign_keys scan: %w", err)
		}
		fk := &ForeignKey{
			Name:       fkName,
			TableName:  tableName,
			Schema:     tableSchema,
			Columns:    splitCols(columnNames),
			RefSchema:  foreignTableSchema,
			RefTable:   foreignTableName,
			RefColumns: splitCols(foreignColumnNames),
		}
		if onDelete.Valid {
			fk.OnDelete = onDelete.String
		}
		if onUpdate.Valid {
			fk.OnUpdate = onUpdate.String
		}
		result.FKs = append(result.FKs, fk)
	}
	return result, rows5.Err()
}

// pgColumnType builds the human-readable PostgreSQL column type.
func pgColumnType(dataType string, charMaxLen, numPrecision, numScale sql.NullInt64, udtSchema, udtName string) string {
	switch strings.ToLower(dataType) {
	case "character varying":
		if charMaxLen.Valid {
			return fmt.Sprintf("varchar(%d)", charMaxLen.Int64)
		}
		return "varchar"
	case "character":
		if charMaxLen.Valid {
			return fmt.Sprintf("char(%d)", charMaxLen.Int64)
		}
		return "char"
	case "numeric", "decimal":
		if numPrecision.Valid && numScale.Valid {
			return fmt.Sprintf("%s(%d,%d)", strings.ToLower(dataType), numPrecision.Int64, numScale.Int64)
		}
		if numPrecision.Valid {
			return fmt.Sprintf("%s(%d)", strings.ToLower(dataType), numPrecision.Int64)
		}
		return strings.ToLower(dataType)
	case "timestamp without time zone":
		return "timestamp"
	case "timestamp with time zone":
		return "timestamptz"
	case "time without time zone":
		return "time"
	case "time with time zone":
		return "timetz"
	case "double precision":
		return "float8"
	case "real":
		return "float4"
	case "user-defined":
		if udtSchema != "" && udtSchema != "pg_catalog" {
			return udtSchema + "." + udtName
		}
		return udtName
	case "array":
		if strings.HasPrefix(udtName, "_") {
			return udtName[1:] + "[]"
		}
		return udtName + "[]"
	default:
		return strings.ToLower(dataType)
	}
}

// splitCols splits a comma-separated column list, trimming spaces.
func splitCols(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
