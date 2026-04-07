# dbml-tools

A CLI toolset for working with [DBML](https://dbml.dbdiagram.io) (Database
Markup Language) files and live databases.

Goals:

- A database visualiser (via open source dot files)
- A database documentation builder (with an open source hugo output)

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

```
dbml-tools <command> [args]

Commands:
  lex        <file>                                    Tokenize a DBML file and output lexer JSON
  parse      <file>                                    Parse a DBML file and output AST JSON
  interpret  <file>                                    Interpret a DBML file and output database schema JSON
  check      <file>                                    Check a DBML file for errors
  introspect [--normalize] [--exclude p,...] [--include p,...] <dsn>
                                                       Connect to a database and print its schema as DBML
  dump       <file>                                    Generate CREATE TABLE SQL from a DBML file
  migrate    [--apply] [--exclude p,...] [--include p,...] <old> <new>
                                                       Generate migration SQL from schema diff
```

### How the SQL dialect is determined

The SQL dialect used by `dump` and `migrate` is determined automatically from
the `database_type` Project setting inside the DBML file:

```dbml
Project {
  database_type: 'PostgreSQL'   // or 'MySQL', 'SQLite'
}
```

`introspect` writes this setting automatically based on the database engine it
connects to. When `database_type` is absent or set to `'normalized'`, a generic
dialect is used.

### Type modes

DBML column types flow through the toolchain in one of two modes:

**Native** (default for `introspect`)\
Column types are preserved exactly as they appear in the source database
(e.g. `character varying(255)`, `bytea`, `mediumtext`). The `database_type`
Project setting records which engine they came from, so downstream tools
(`dump`, `migrate`) emit the correct SQL syntax.

**Normalized** (`introspect --normalize`)\
Type names are mapped to canonical DBML equivalents (e.g. `integer` → `int`,
`character varying(255)` → `varchar(255)`, `bytea` → `binary`). The
`database_type` is set to `'normalized'`. Unrecognised or vendor-specific types
(e.g. `public.geometry`) are copied verbatim. This mode produces
database-agnostic DBML.

### introspect

Connects to a live database, reads its schema, and outputs DBML to stdout.
By default column types are preserved as-is from the database and a
`database_type` Project setting is written (e.g. `'PostgreSQL'`).

Pass `--normalize` to map types to database-agnostic canonical DBML equivalents
(e.g. `character varying` → `varchar`, `bytea` → `binary`).

**Supported engines:** MariaDB, PostgreSQL, SQLite.

```sh
# MariaDB / MySQL (native types)
dbml-tools introspect mariadb://user:pass@host:3306/mydb

# PostgreSQL (defaults to schema "public"; override with ?schema=)
dbml-tools introspect postgres://user:pass@host:5432/mydb
dbml-tools introspect postgres://user:pass@host:5432/mydb?schema=myschema

# SQLite
dbml-tools introspect sqlite:///path/to/file.db

# Normalized / database-agnostic output
dbml-tools introspect --normalize postgres://user:pass@host:5432/mydb

# Save to a file
dbml-tools introspect sqlite:///path/to/file.db > schema.dbml

# Exclude specific tables (exact name or glob pattern, comma-separated)
dbml-tools introspect --exclude 'spatial_ref_sys,geography_columns' postgres://...
dbml-tools introspect --exclude 'tmp_*,_*' mariadb://...

# Include only specific tables
dbml-tools introspect --include 'orders,order_*' postgres://...
```

> **Note:** flags must be placed before the DSN argument.

**Filter patterns** support `*` (any sequence of characters) and `?` (any single
character), matched case-insensitively. Comma-separate multiple patterns.

The `public` schema prefix is omitted from PostgreSQL output since it is the
default schema.

### check

Checks a DBML file for syntax and semantic errors.

```sh
dbml-tools check schema.dbml
```

### dump

Generates a `CREATE TABLE` SQL script from a DBML file. The SQL dialect is
determined by the `database_type` Project setting in the file.

```sh
# Dialect determined by database_type in the file (e.g. 'PostgreSQL')
dbml-tools dump schema.dbml
```

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

# Remove the dry-run header (output is ready to pipe into psql / mysql)
dbml-tools migrate --apply old.dbml new.dbml | psql "$DATABASE_URL"

# Exclude system or extension tables from the comparison
dbml-tools migrate --exclude 'spatial_ref_sys,geography_columns' old.dbml postgres://...
```

By default `migrate` is a **dry run**: it prints a `-- DRY RUN` header and the
SQL statements, but executes nothing. Pass `--apply` to suppress the header so
the output can be piped directly into a database client.

The `--exclude` / `--include` filter flags work the same as for `introspect` and
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
