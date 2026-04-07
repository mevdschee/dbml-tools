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
  check      [-d driver] <file>                        Check a DBML file for errors
  introspect [--exclude p,...] [--include p,...] <dsn>
                                                       Connect to a database and print its schema as DBML
  dump       [-d driver] <file>                        Generate CREATE TABLE SQL from a DBML file
  migrate    [-d driver] [--apply] [--exclude p,...] [--include p,...] <old> <new>
                                                       Generate migration SQL from schema diff
```

### Type modes

DBML column types flow through the toolchain in one of two modes:

**Generic / normalized** — no `-d` flag given (or `-d normalized`)\
Type names are normalised to canonical DBML equivalents (e.g. `integer` → `int`,
`character varying(255)` → `varchar(255)`, `bytea` → `binary`). Unrecognised or
vendor-specific types (e.g. `public.geometry`) are copied verbatim.

**Dialect-specific** — `-d postgres`, `-d mysql`, or `-d sqlite` given\
Column types are passed through unchanged — the type string from the DBML is
emitted as-is. Only SQL syntax is dialect-specific: identifier quoting, enum
expansion, `SERIAL`/`AUTO_INCREMENT` for `[increment]` columns, `COMMENT ON`,
etc.

The `database_type: 'normalized'` Project setting (emitted automatically by
`introspect`) signals that column types use canonical DBML names. Tools treat
this the same as omitting `-d`.

### introspect

Connects to a live database, reads its schema, and outputs DBML to stdout.

**Supported engines:** MariaDB, PostgreSQL, SQLite.

```sh
# MariaDB / MySQL
dbml-tools introspect mariadb://user:pass@host:3306/mydb

# PostgreSQL (defaults to schema "public"; override with ?schema=)
dbml-tools introspect postgres://user:pass@host:5432/mydb
dbml-tools introspect postgres://user:pass@host:5432/mydb?schema=myschema

# SQLite
dbml-tools introspect sqlite:///path/to/file.db

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

Checks a DBML file for syntax and semantic errors. Without `-d`, column types
are accepted as-is (no type validation). Pass `-d <driver>` to restrict types to
those recognised by the given dialect.

```sh
# Accept any column type (generic/passthrough mode)
dbml-tools check schema.dbml

# Validate types for a specific driver
dbml-tools check -d postgres schema.dbml
```

### dump

Generates a `CREATE TABLE` SQL script from a DBML file.

```sh
# Generic/normalized — type names are mapped to canonical DBML equivalents
dbml-tools dump schema.dbml

# PostgreSQL — types passed through as-is; postgres SQL syntax applied
dbml-tools dump -d postgres schema.dbml

# MySQL / MariaDB — types passed through as-is; mysql SQL syntax applied
dbml-tools dump -d mysql schema.dbml

# SQLite — types passed through as-is; sqlite SQL syntax applied
dbml-tools dump -d sqlite schema.dbml
```

Supported drivers: `postgres`, `mysql`, `sqlite`. Omit `-d` for
generic/normalized mode.

### migrate

Compares two schemas and outputs the SQL statements needed to migrate from the
first to the second. Either argument can be a DBML file path or a live database
connection string. The dialect is auto-detected from any DSN present.

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

# Explicit driver when both sides are files
dbml-tools migrate -d mysql old.dbml new.dbml
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
