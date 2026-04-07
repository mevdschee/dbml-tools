# dbml-tools

A CLI toolset for working with [DBML](https://dbml.dbdiagram.io) (Database
Markup Language) files and live databases.

Functionality

- Convert DSN to DBML
- Convert DBML to SQL
- SQL migration from DBML/DSN
- Generate Graphviz DOT diagrams

## Installation

```sh
go install dbml-tools@latest
```

Or build from source:

```sh
git clone ...
cd dbml-tools
go build -o dbml-tools .
```

## Commands

| Command                         | Description                                        |
| ------------------------------- | -------------------------------------------------- |
| `check <file>`                  | Check a DBML file for parse/semantic errors        |
| `todbml [options] <dsn>`        | Connect to a database and print its schema as DBML |
| `tosql <file>`                  | Generate CREATE TABLE SQL from a DBML file         |
| `todot <file>`                  | Generate Graphviz DOT diagram from a DBML file     |
| `migrate [options] <old> <new>` | Generate migration SQL from schema diff            |

### todbml options

| Flag              | Description                                                          |
| ----------------- | -------------------------------------------------------------------- |
| `--normalize`     | Map column types to database-agnostic DBML equivalents               |
| `--data`          | Also export row data as DBML `records` blocks                        |
| `--exclude p,...` | Comma-separated table name patterns to exclude (supports `*` glob)   |
| `--include p,...` | Comma-separated table name patterns to include (all others excluded) |

### migrate options

| Flag              | Description                                    |
| ----------------- | ---------------------------------------------- |
| `--exclude p,...` | Comma-separated table name patterns to exclude |
| `--include p,...` | Comma-separated table name patterns to include |

### Extra debugging commands

| Command            | Description                                           |
| ------------------ | ----------------------------------------------------- |
| `lex <file>`       | Tokenize a DBML file and output lexer JSON            |
| `parse <file>`     | Parse a DBML file and output AST JSON                 |
| `interpret <file>` | Interpret a DBML file and output database schema JSON |

### How the SQL dialect is determined

The SQL dialect used by `tosql` and `migrate` is determined in this order:

1. **DSN auto-detect** (`migrate` only) — inferred from the connection string
   scheme
2. **`database_type` Project setting** — read from the DBML file
3. **Default** — MariaDB when none of the above yields a specific dialect

```dbml
Project {
  database_type: 'PostgreSQL'   // or 'MariaDB', 'SQLite'
  // todbml --normalize produces e.g. 'MariaDB normalized'
}
```

`todbml` writes the `database_type` setting automatically based on the database
engine it connects to, even when `--normalize` is used. This means `tosql` and
`migrate` can always auto-detect the dialect from the DBML file or the DSN.

### Type modes

DBML column types flow through the toolchain in one of two modes:

**Native** (default for `todbml`)\
Column types are preserved exactly as they appear in the source database (e.g.
`character varying(255)`, `bytea`, `mediumtext`). The `database_type` Project
setting records which engine they came from, so downstream tools (`tosql`,
`migrate`) emit the correct SQL syntax.

**Normalized** (`todbml --normalize`)\
Type names are mapped to canonical DBML equivalents (e.g. `integer` → `int`,
`character varying(255)` → `varchar(255)`, `bytea` → `binary`). The
`database_type` is set to `'<engine> normalized'` (e.g. `'MariaDB normalized'`)
so that downstream tools know both the source engine and that types are
normalized. Unrecognised or vendor-specific types (e.g. `geometry`) are copied
verbatim. This mode produces database-agnostic DBML.

### todbml

Connects to a live database, reads its schema, and outputs DBML to stdout. By
default column types are preserved as-is from the database and a `database_type`
Project setting is written (e.g. `'PostgreSQL'`).

Pass `--normalize` to map types to database-agnostic canonical DBML equivalents
(e.g. `character varying` → `varchar`, `bytea` → `binary`).

**Supported engines:** MariaDB, PostgreSQL, SQLite.

```sh
# MariaDB (native types)
dbml-tools todbml mariadb://user:pass@host:3306/mydb

# PostgreSQL (defaults to schema "public"; override with ?schema=)
dbml-tools todbml postgres://user:pass@host:5432/mydb
dbml-tools todbml postgres://user:pass@host:5432/mydb?schema=myschema

# SQLite
dbml-tools todbml sqlite:///path/to/file.db

# Normalized / database-agnostic output
dbml-tools todbml --normalize postgres://user:pass@host:5432/mydb

# Save to a file
dbml-tools todbml sqlite:///path/to/file.db > schema.dbml

# Exclude specific tables (exact name or glob pattern, comma-separated)
dbml-tools todbml --exclude 'spatial_ref_sys,geography_columns' postgres://...
dbml-tools todbml --exclude 'tmp_*,_*' mariadb://...

# Include only specific tables
dbml-tools todbml --include 'orders,order_*' postgres://...

# Export schema together with row data
dbml-tools todbml --data mariadb://user:pass@host:3306/mydb
```

> **Note:** flags must be placed before the DSN argument.

**`--data`** also fetches all rows from each table and emits them as DBML
`records` blocks. Binary column types (`binary`, `varbinary`, `blob`, `bytea`,
`bit`, `geometry`, `point`, etc.) are hex-encoded using `X'...'` notation.

**Filter patterns** support `*` (any sequence of characters) and `?` (any single
character), matched case-insensitively. Comma-separate multiple patterns.

The `public` schema prefix is omitted from PostgreSQL output since it is the
default schema.

### check

Checks a DBML file for syntax and semantic errors.

```sh
dbml-tools check schema.dbml
```

### tosql

Generates a `CREATE TABLE` SQL script from a DBML file. The SQL dialect is
determined by the `database_type` Project setting in the file. If the DBML
contains `records` blocks (from `todbml --data`), matching `INSERT INTO`
statements are generated after each table.

```sh
dbml-tools tosql schema.dbml
```

### todot

Generates a Graphviz DOT diagram of the database schema. Tables are rendered
with columns, types, PK/FK markers, and constraint badges. Foreign key
relationships are drawn as orthogonal edges with crow's foot notation.

```sh
# Output DOT to stdout
dbml-tools todot schema.dbml

# Generate a PDF
dbml-tools todot schema.dbml | dot -Tpdf -o schema.pdf

# Generate a PNG
dbml-tools todot schema.dbml | dot -Tpng -o schema.png

# Generate an SVG
dbml-tools todot schema.dbml | dot -Tsvg -o schema.svg
```

Requires [Graphviz](https://graphviz.org/) (`dot`) to be installed for
rendering.

### migrate

Compares two schemas and outputs the SQL statements needed to migrate from the
first to the second. Either argument can be a DBML file path or a live database
connection string. The dialect is auto-detected from any DSN present or from the
`database_type` Project setting in DBML files.

```sh
# Diff two DBML files (dry-run by default)
dbml-tools migrate old.dbml new.dbml

# Compare a DBML file against a live database
dbml-tools migrate schema.dbml postgres://user:pass@host/db

# Compare two live databases
dbml-tools migrate mariadb://host/db1 mariadb://host/db2

# Exclude system or extension tables from the comparison
dbml-tools migrate --exclude 'spatial_ref_sys,geography_columns' old.dbml postgres://...
```

**Normalized DBML support:** when either schema has a `database_type` ending in
`" normalized"` (as produced by `todbml --normalize`), type names are normalized
before comparison. This means equivalent types with different names (e.g.
`binary` vs `blob`, `text` vs `longtext`) are correctly recognized as identical
and do not produce spurious ALTER statements.

The `--exclude` / `--include` filter flags work the same as for `todbml` and
apply to both schema sides.

**What is introspected:**

| Feature               | MariaDB    | PostgreSQL | SQLite |
| --------------------- | ---------- | ---------- | ------ |
| Tables & columns      | ✓          | ✓          | ✓      |
| Column types          | ✓          | ✓          | ✓      |
| NOT NULL              | ✓          | ✓          | ✓      |
| Default values        | ✓          | ✓          | ✓      |
| Primary keys          | ✓          | ✓          | ✓      |
| Unique constraints    | ✓          | ✓          | ✓      |
| Indexes               | ✓          | ✓          | ✓      |
| Foreign keys          | ✓          | ✓          | ✓      |
| Check constraints     | ✓          | ✓          | —      |
| Enums                 | ✓ (inline) | ✓ (types)  | —      |
| Table/column comments | ✓          | ✓          | —      |

The SQL queries used for introspection live in [sql/](sql/) and use `:schema` as
a named parameter so they can be run independently against any compatible
client.
