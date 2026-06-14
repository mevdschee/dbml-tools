package introspect

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

func introspectMariaDB(ctx context.Context, db *sql.DB, schema string) (*DBSchema, error) {
	result := &DBSchema{}
	tableMap := map[string]*Table{}
	var tableOrder []string
	p := map[string]interface{}{"schema": schema}

	// ── 1. Tables & columns ──────────────────────────────────────────────────
	q, err := readSQL("sql/mariadb/tables_columns.sql")
	if err != nil {
		return nil, err
	}
	rows, err := runQuery(ctx, db, EngineMariaDB, q, p)
	if err != nil {
		return nil, fmt.Errorf("tables_columns: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			tableName, tableComment, columnName string
			columnDefault                        sql.NullString
			defaultValueType, columnIsNullable   string
			columnDataType, columnType           string
			columnExtra, columnComment           string
			generationExpression                 string
		)
		if err := rows.Scan(
			&tableName, &tableComment, &columnName,
			&columnDefault, &defaultValueType,
			&columnIsNullable, &columnDataType, &columnType,
			&columnExtra, &columnComment, &generationExpression,
		); err != nil {
			return nil, fmt.Errorf("tables_columns scan: %w", err)
		}

		t, ok := tableMap[tableName]
		if !ok {
			t = &Table{Name: tableName, Comment: tableComment}
			tableMap[tableName] = t
			tableOrder = append(tableOrder, tableName)
		}

		col := &Column{
			Name:        columnName,
			Type:        columnType,
			IsNullable:  columnIsNullable == "YES",
			Comment:     columnComment,
			Generated:   generationExpression,
			IsIncrement: strings.Contains(columnExtra, "auto_increment"),
		}
		if columnDefault.Valid && defaultValueType != "null" {
			col.Default = &columnDefault.String
			col.DefaultType = defaultValueType
		}
		t.Columns = append(t.Columns, col)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for _, name := range tableOrder {
		result.Tables = append(result.Tables, tableMap[name])
	}

	// ── 2. Enums (inline per-column) ─────────────────────────────────────────
	q, err = readSQL("sql/mariadb/enums.sql")
	if err != nil {
		return nil, err
	}
	rows2, err := runQuery(ctx, db, EngineMariaDB, q, p)
	if err != nil {
		return nil, fmt.Errorf("enums: %w", err)
	}
	defer rows2.Close()

	for rows2.Next() {
		var tableName, columnName, rawValues string
		if err := rows2.Scan(&tableName, &columnName, &rawValues); err != nil {
			return nil, fmt.Errorf("enums scan: %w", err)
		}
		enumName := tableName + "_" + columnName
		result.Enums = append(result.Enums, &Enum{
			Name:   enumName,
			Values: parseEnumValues(rawValues),
		})
		// Rewrite the column type to reference the named enum
		if t, ok := tableMap[tableName]; ok {
			for _, col := range t.Columns {
				if col.Name == columnName {
					col.Type = enumName
					break
				}
			}
		}
	}
	if err := rows2.Err(); err != nil {
		return nil, err
	}

	// ── 3. Constraints (PRIMARY KEY, UNIQUE) ─────────────────────────────────
	q, err = readSQL("sql/mariadb/constraints.sql")
	if err != nil {
		return nil, err
	}
	rows3, err := runQuery(ctx, db, EngineMariaDB, q, p)
	if err != nil {
		return nil, fmt.Errorf("constraints: %w", err)
	}
	defer rows3.Close()

	for rows3.Next() {
		var tableName, constraintName, columnNames, constraintType string
		var columnCount int
		if err := rows3.Scan(&tableName, &constraintName, &columnNames, &columnCount, &constraintType); err != nil {
			return nil, fmt.Errorf("constraints scan: %w", err)
		}
		t, ok := tableMap[tableName]
		if !ok {
			continue
		}
		cols := strings.Split(columnNames, ",")
		isPrimary := constraintType == "PRIMARY KEY"

		t.Constraints = append(t.Constraints, &Constraint{
			Name:      constraintName,
			Columns:   cols,
			IsPrimary: isPrimary,
		})
		// Annotate single-column constraints directly on the column
		if len(cols) == 1 {
			for _, col := range t.Columns {
				if col.Name == cols[0] {
					if isPrimary {
						col.IsPrimaryKey = true
					} else {
						col.IsUnique = true
					}
					break
				}
			}
		}
	}
	if err := rows3.Err(); err != nil {
		return nil, err
	}

	// ── 4. Indexes ───────────────────────────────────────────────────────────
	q, err = readSQL("sql/mariadb/indexes.sql")
	if err != nil {
		return nil, err
	}
	rows4, err := runQuery(ctx, db, EngineMariaDB, q, p)
	if err != nil {
		return nil, fmt.Errorf("indexes: %w", err)
	}
	defer rows4.Close()

	type idxKey struct{ table, name string }
	idxMap := map[idxKey]*Index{}
	var idxOrder []idxKey

	for rows4.Next() {
		var (
			tableName, idxName, columnName, idxType string
			isUniqueInt                              int64
			idxSubPart                               sql.NullInt64
			idxExpression                            sql.NullString
		)
		if err := rows4.Scan(&tableName, &isUniqueInt, &idxName, &columnName,
			&idxSubPart, &idxType, &idxExpression); err != nil {
			return nil, fmt.Errorf("indexes scan: %w", err)
		}
		key := idxKey{tableName, idxName}
		idx, ok := idxMap[key]
		if !ok {
			idx = &Index{
				Name:     idxName,
				IsUnique: isUniqueInt != 0,
				Type:     idxType,
			}
			if idxExpression.Valid {
				idx.Expression = idxExpression.String
			}
			idxMap[key] = idx
			idxOrder = append(idxOrder, key)
		}
		if columnName != "" {
			idx.Columns = append(idx.Columns, columnName)
		}
	}
	if err := rows4.Err(); err != nil {
		return nil, err
	}
	for _, key := range idxOrder {
		if t, ok := tableMap[key.table]; ok {
			t.Indexes = append(t.Indexes, idxMap[key])
		}
	}

	// ── 5. Check constraints ─────────────────────────────────────────────────
	q, err = readSQL("sql/mariadb/check_constraints.sql")
	if err != nil {
		return nil, err
	}
	rows5, err := runQuery(ctx, db, EngineMariaDB, q, p)
	if err != nil {
		// check_constraints may not exist on older MariaDB — skip gracefully
		_ = err
	} else {
		defer rows5.Close()
		for rows5.Next() {
			var constraintName, tableName, checkClause string
			if err := rows5.Scan(&constraintName, &tableName, &checkClause); err != nil {
				return nil, fmt.Errorf("check_constraints scan: %w", err)
			}
			if t, ok := tableMap[tableName]; ok {
				t.CheckConstraints = append(t.CheckConstraints, &CheckConstraint{
					Name:       constraintName,
					Expression: checkClause,
				})
			}
		}
		if err := rows5.Err(); err != nil {
			return nil, err
		}
	}

	// ── 6. Foreign keys ──────────────────────────────────────────────────────
	q, err = readSQL("sql/mariadb/foreign_keys.sql")
	if err != nil {
		return nil, err
	}
	rows6, err := runQuery(ctx, db, EngineMariaDB, q, p)
	if err != nil {
		return nil, fmt.Errorf("foreign_keys: %w", err)
	}
	defer rows6.Close()

	for rows6.Next() {
		var constraintName, foreignTableName, foreignColumnNames string
		var refTableName, refColumnNames, onUpdate, onDelete string
		if err := rows6.Scan(
			&constraintName, &foreignTableName, &foreignColumnNames,
			&refTableName, &refColumnNames, &onUpdate, &onDelete,
		); err != nil {
			return nil, fmt.Errorf("foreign_keys scan: %w", err)
		}
		result.FKs = append(result.FKs, &ForeignKey{
			Name:       constraintName,
			TableName:  foreignTableName,
			Columns:    strings.Split(foreignColumnNames, ","),
			RefTable:   refTableName,
			RefColumns: strings.Split(refColumnNames, ","),
			OnDelete:   onDelete,
			OnUpdate:   onUpdate,
		})
	}
	return result, rows6.Err()
}

// parseEnumValues parses MariaDB's column_type enum literal, e.g. ('a','b','c').
func parseEnumValues(raw string) []string {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "(")
	raw = strings.TrimSuffix(raw, ")")
	parts := strings.Split(raw, ",")
	values := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		p = strings.Trim(p, "'")
		if p != "" {
			values = append(values, p)
		}
	}
	return values
}
