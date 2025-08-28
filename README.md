# pgmigrate

A lightweight, repo-agnostic CLI for managing PostgreSQL schema migrations powered by [golang-migrate](https://github.com/golang-migrate/migrate). Install once with `go install` and reuse across projects. Migrations live in your project folder (no embed required).

[![Go Reference](https://pkg.go.dev/badge/github.com/kirinyoku/pgmigrate.svg)](https://pkg.go.dev/github.com/kirinyoku/pgmigrate)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

## Table of Contents
- [Install](#install)
- [Quick start](#quick-start)
- [Commands](#commands)
- [Configuration](#configuration)
- [Migration file format](#migration-file-format)
- [Postgres best practices](#postgres-best-practices)
- [Dirty state](#dirty-state)
- [License](#license)

## Install

```bash
go install github.com/kirinyoku/pgmigrate@latest
```

## Quick start

In your project root:

```bash
# 1) Set DSN
export DATABASE_URL="postgres://user:pass@localhost:5432/dbname?sslmode=disable"

# 2) Create a new migration pair
pgmigrate create init_schema
# Creates: db/migrations/20250827150000_init_schema.up.sql
#          db/migrations/20250827150000_init_schema.down.sql

# 3) Edit the .sql files, then apply
pgmigrate up

# Roll back one step
pgmigrate down --steps 1

# Show current version and dirty state
pgmigrate version
```

## Commands

```text
pgmigrate <command> [flags]

Commands:
  create <name>          Create timestamped .up/.down.sql pair in db/migrations
  up [--steps N]         Apply all or N steps
  down --steps N|--all   Roll back N steps or all
  to <version>           Migrate to exact version
  force <version>        Set version manually (when dirty)
  version                Print current version and dirty

Flags / Env:
  --dsn                  Postgres DSN (env DATABASE_URL)
  --dir                  Migrations dir (env MIGRATIONS_DIR, default db/migrations)
```

Examples:

```bash
# Apply all
pgmigrate up

# Apply N steps
pgmigrate up --steps 2

# Roll back N steps
pgmigrate down --steps 1

# Roll back everything
pgmigrate down --all

# Show current version and dirty state
pgmigrate version

# Migrate to a specific version
pgmigrate to 20250827153000

# Fix dirty state (after a failed migration you manually corrected)
pgmigrate force 20250827153000
```

## Configuration

- DATABASE_URL or `--dsn`:
  - Example: `postgres://user:pass@host:5432/dbname?sslmode=disable`
- MIGRATIONS_DIR or `--dir`:
  - Default: `migrations`

Tip: Keep a Makefile or Taskfile in each project:

```make
DB_URL=postgres://user:pass@localhost:5432/dbname?sslmode=disable

create:
	pgmigrate create $(name)

up:
	DATABASE_URL=$(DB_URL) pgmigrate up

down:
	DATABASE_URL=$(DB_URL) pgmigrate down --steps $(steps)
```

## Migration file format

Files follow golang-migrate’s convention:

- `<version>_<name>.up.sql`
- `<version>_<name>.down.sql`

Version is a monotonically increasing number, typically a UTC timestamp:
`20060102150405` (YYYYMMDDHHMMSS).

Example:

```text
migrations/
  20250827153000_init_schema.up.sql
  20250827153000_init_schema.down.sql
```

## Postgres best practices

- Prefer one statement per file for clarity and safety.
- Use `RETURNING` to fetch IDs after INSERT/UPDATE.
- Use `INSERT ... ON CONFLICT ...` for upserts.
- Operations that cannot run inside a transaction must be isolated:
  - CREATE INDEX CONCURRENTLY / DROP INDEX CONCURRENTLY
  - ALTER TYPE ... ADD VALUE
  - REINDEX CONCURRENTLY, VACUUM, CREATE/DROP DATABASE, etc.
- For big tables:
  - Create indexes with CONCURRENTLY
  - Add NOT NULL in stages (add column nullable, backfill in batches, then SET NOT NULL)
- Set timeouts during migrations to avoid hanging:
  - Add to your DSN:
    - `?x-statement-timeout=60000` (60s)
    - Optionally `?x-lock-timeout=3000` (3s)

Example DSN with timeouts:

```text
postgres://user:pass@localhost:5432/dbname?sslmode=disable&x-statement-timeout=60000&x-lock-timeout=3000
```

## Dirty state

If a migration fails mid-way, the database can become “dirty” and further migrations won’t run until fixed.

Steps:
1. Investigate and manually fix the schema/data as needed.
2. Mark the database at the correct version:
   ```bash
   pgmigrate force <version>
   ```
3. Re-run `pgmigrate up`.

Always test migrations on staging or a copy of production data before production rollout.

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.