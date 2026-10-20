package introspect

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

func introspectSQLite(ctx context.Context, db *sql.DB) (*DBSchema, error) {
	result := &DBSchema{}
	tableMap := map[string]*Table{}
	var tableOrder []string
	// pkCols holds each table's primary key columns, indexed by pk_order - 1.
	pkCols := map[string][]string{}

	// ── 1. Tables & columns ──────────────────────────────────────────────────
	q, err := readSQL("sql/sqlite/tables_columns.sql")
	if err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("tables_columns: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			tableName, columnName, columnType string
			columnID, notNull, pkOrder        int
			columnDefault                     sql.NullString
		)
		if err := rows.Scan(&tableName, &columnID, &columnName, &columnType,
			&notNull, &columnDefault, &pkOrder); err != nil {
			return nil, fmt.Errorf("tables_columns scan: %w", err)
		}

		t, ok := tableMap[tableName]
		if !ok {
			t = &Table{Name: tableName}
			tableMap[tableName] = t
			tableOrder = append(tableOrder, tableName)
		}

		col := &Column{
			Name:       columnName,
			Type:       columnType,
			IsNullable: notNull == 0,
		}
		if columnDefault.Valid {
			col.Default = &columnDefault.String
			col.DefaultType = inferSQLiteDefault(columnDefault.String)
		}
		if pkOrder > 0 {
			cols := pkCols[tableName]
			for len(cols) < pkOrder {
				cols = append(cols, "")
			}
			cols[pkOrder-1] = columnName
			pkCols[tableName] = cols
		}
		// Single-column PK: pkOrder == 1 and it's the only PK column.
		// We handle multi-col PKs in the constraints step below.
		if pkOrder == 1 {
			col.IsPrimaryKey = true
			// INTEGER PRIMARY KEY is SQLite's rowid alias → auto-increment
			if strings.ToUpper(columnType) == "INTEGER" {
				col.IsIncrement = true
			}
		}
		t.Columns = append(t.Columns, col)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Fix multi-column PKs: if any table has columns with pkOrder > 1,
	// the PK is composite — add a Constraint instead of per-column [pk].
	for name, t := range tableMap {
		if cols := pkCols[name]; len(cols) > 1 {
			// Remove IsPrimaryKey from individual columns and use a Constraint.
			for _, col := range t.Columns {
				col.IsPrimaryKey = false
				col.IsIncrement = false
			}
			t.Constraints = append(t.Constraints, &Constraint{
				Name:      "",
				Columns:   cols,
				IsPrimary: true,
			})
		}
	}

	for _, name := range tableOrder {
		result.Tables = append(result.Tables, tableMap[name])
	}

	// ── 2. Indexes ───────────────────────────────────────────────────────────
	q, err = readSQL("sql/sqlite/indexes.sql")
	if err != nil {
		return nil, err
	}
	rows2, err := db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("indexes: %w", err)
	}
	defer rows2.Close()

	type idxKey struct{ table, name string }
	idxMap := map[idxKey]*Index{}
	idxOrigin := map[idxKey]string{}
	// The keys parsed from an index's SQL, read once per expression index.
	idxSQLKeys := map[idxKey][]string{}
	var idxOrder []idxKey

	for rows2.Next() {
		var (
			tableName, indexName, origin string
			isUniqueInt, columnPosition  int
			columnName, indexSQL         sql.NullString
		)
		if err := rows2.Scan(&tableName, &indexName, &isUniqueInt, &origin,
			&columnPosition, &columnName, &indexSQL); err != nil {
			return nil, fmt.Errorf("indexes scan: %w", err)
		}

		key := idxKey{tableName, indexName}
		idx, ok := idxMap[key]
		if !ok {
			isUnique := isUniqueInt != 0
			idx = &Index{Name: indexName, IsUnique: isUnique, Type: "btree"}
			idxMap[key] = idx
			idxOrigin[key] = origin
			idxOrder = append(idxOrder, key)
		}
		if columnName.Valid {
			idx.addKey(columnName.String, false)
		} else {
			keys, parsed := idxSQLKeys[key]
			if !parsed {
				keys = sqliteIndexKeys(indexSQL.String)
				idxSQLKeys[key] = keys
			}
			if columnPosition < len(keys) {
				idx.addKey(keys[columnPosition], true)
			}
		}

		// origin='u' is a UNIQUE constraint — reflect that on the column if single-col
		if origin == "u" && isUniqueInt != 0 {
			if t, ok := tableMap[tableName]; ok {
				for _, col := range t.Columns {
					if col.Name == columnName.String {
						col.IsUnique = true
					}
				}
			}
		}
	}
	if err := rows2.Err(); err != nil {
		return nil, err
	}

	// Add indexes to tables; single-column unique ones are already on the column,
	// but we still emit them in the index block for multi-col uniques.
	for _, key := range idxOrder {
		if t, ok := tableMap[key.table]; ok {
			idx := idxMap[key]
			// Skip single-column UNIQUE constraints — already reflected on the
			// column. A unique index created with CREATE INDEX is kept.
			if idxOrigin[key] == "u" && len(idx.Columns) == 1 {
				continue
			}
			t.Indexes = append(t.Indexes, idx)
		}
	}

	// ── 3. Foreign keys ──────────────────────────────────────────────────────
	q, err = readSQL("sql/sqlite/foreign_keys.sql")
	if err != nil {
		return nil, err
	}
	rows3, err := db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("foreign_keys: %w", err)
	}
	defer rows3.Close()

	type fkKey struct {
		table string
		id    int
	}
	fkMap := map[fkKey]*ForeignKey{}
	var fkOrder []fkKey

	for rows3.Next() {
		var (
			tableName, refTable, fromCol string
			toCol                        sql.NullString
			onUpdate, onDelete           string
			fkID, seq                    int
		)
		if err := rows3.Scan(&tableName, &fkID, &seq, &refTable, &fromCol, &toCol,
			&onUpdate, &onDelete); err != nil {
			return nil, fmt.Errorf("foreign_keys scan: %w", err)
		}

		key := fkKey{tableName, fkID}
		fk, ok := fkMap[key]
		if !ok {
			fk = &ForeignKey{
				TableName: tableName,
				RefTable:  refTable,
				OnUpdate:  onUpdate,
				OnDelete:  onDelete,
			}
			fkMap[key] = fk
			fkOrder = append(fkOrder, key)
		}
		fk.Columns = append(fk.Columns, fromCol)
		if toCol.Valid && toCol.String != "" {
			fk.RefColumns = append(fk.RefColumns, toCol.String)
		}
	}
	if err := rows3.Err(); err != nil {
		return nil, err
	}
	for _, key := range fkOrder {
		result.FKs = append(result.FKs, fkMap[key])
	}

	return result, nil
}

// inferSQLiteDefault guesses the DBML default type for a SQLite default value string.
func inferSQLiteDefault(val string) string {
	if val == "NULL" {
		return "null"
	}
	if val == "TRUE" || val == "FALSE" || val == "true" || val == "false" {
		return "boolean"
	}
	if strings.HasPrefix(val, "'") {
		return "string"
	}
	// numeric literal
	trimmed := strings.TrimSpace(val)
	isNum := true
	for i, c := range trimmed {
		if c == '-' && i == 0 {
			continue
		}
		if c == '.' {
			continue
		}
		if c < '0' || c > '9' {
			isNum = false
			break
		}
	}
	if isNum && trimmed != "" {
		return "number"
	}
	return "expression"
}

// sqliteIndexKeys splits the key list of a CREATE INDEX statement into its
// keys, with an expression's wrapping parentheses and any ASC/DESC removed.
// SQLite reports no name for an expression key, so this is the only place
// its text can be read from.
func sqliteIndexKeys(stmt string) []string {
	var keys []string
	depth, start := 0, -1
	var quote byte
	for i := 0; i < len(stmt); i++ {
		c := stmt[i]
		switch {
		case quote != 0:
			if c == quote {
				quote = 0
			}
		case c == '"' || c == '\'' || c == '`' || c == '[':
			quote = c
			if c == '[' {
				quote = ']'
			}
		case c == '(':
			depth++
			if depth == 1 && start < 0 {
				start = i + 1
			}
		case c == ',' && depth == 1:
			keys = append(keys, stmt[start:i])
			start = i + 1
		case c == ')':
			depth--
			if depth == 0 && start >= 0 {
				keys = append(keys, stmt[start:i])
				for j, k := range keys {
					keys[j] = trimIndexKey(k)
				}
				return keys
			}
		}
	}
	return nil
}

func trimIndexKey(k string) string {
	k = strings.TrimSpace(k)
	upper := strings.ToUpper(k)
	for _, dir := range []string{" ASC", " DESC"} {
		if strings.HasSuffix(upper, dir) {
			k = strings.TrimSpace(k[:len(k)-len(dir)])
			break
		}
	}
	// Strip one pair of parentheses only when they wrap the whole key.
	if strings.HasPrefix(k, "(") && strings.HasSuffix(k, ")") {
		depth := 0
		for i := 0; i < len(k); i++ {
			switch k[i] {
			case '(':
				depth++
			case ')':
				depth--
				if depth == 0 && i < len(k)-1 {
					return k
				}
			}
		}
		k = strings.TrimSpace(k[1 : len(k)-1])
	}
	return k
}
