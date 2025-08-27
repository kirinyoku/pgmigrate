// pgmigrate is a small CLI wrapper around golang-migrate that provides
// a simple, batteries-included command-line interface for managing
// Postgres schema migrations stored in a local filesystem directory.
//
// Usage summary (see usage() for full text):
//
//	pgmigrate <command> [flags]
//
// Commands implemented by this binary:
//
//	create <name>          Create timestamped .up/.down.sql pair in migrations dir
//	up [--steps N]         Apply all or N steps to the target database
//	down --steps N|--all   Roll back N steps or all applied migrations
//	to <version>           Migrate to an exact migration version
//	force <version>        Manually set the migration version (useful when dirty)
//	version                Print current version and dirty state
//
// The tool reads defaults from environment variables:
//
//	DATABASE_URL (Postgres DSN) and MIGRATIONS_DIR (defaults to "migrations").
package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

const defaultDir = "migrations"

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	dsn := envOr("DATABASE_URL", "")
	dir := envOr("MIGRATIONS_DIR", defaultDir)

	// We branch on the first CLI argument to select a subcommand. Each
	// subcommand creates its own FlagSet so flags are scoped to that
	// command (e.g. `pgmigrate up --steps 1`).
	switch os.Args[1] {
	case "create":
		fs := flag.NewFlagSet("create", flag.ExitOnError)
		fsDir := fs.String("dir", dir, "migrations dir")
		_ = fs.Parse(os.Args[2:])
		if fs.NArg() < 1 {
			log.Fatal("usage: pgmigrate create <name>")
		}
		name := sanitize(fs.Arg(0))
		if err := createPair(*fsDir, name); err != nil {
			log.Fatal(err)
		}

	case "up":
		fs := flag.NewFlagSet("up", flag.ExitOnError)
		fsDsn := fs.String("dsn", dsn, "database url")
		fsDir := fs.String("dir", dir, "migrations dir")
		steps := fs.Int("steps", 0, "apply N steps (0=all)")
		_ = fs.Parse(os.Args[2:])
		mustDsn(*fsDsn)
		m := mustMigrator(*fsDsn, *fsDir)
		defer closeM(m)
		if *steps > 0 {
			must(m.Steps(*steps))
		} else {
			must(m.Up())
		}

	case "down":
		fs := flag.NewFlagSet("down", flag.ExitOnError)
		fsDsn := fs.String("dsn", dsn, "database url")
		fsDir := fs.String("dir", dir, "migrations dir")
		steps := fs.Int("steps", 0, "rollback N steps")
		all := fs.Bool("all", false, "rollback all")
		_ = fs.Parse(os.Args[2:])
		mustDsn(*fsDsn)
		m := mustMigrator(*fsDsn, *fsDir)
		defer closeM(m)
		if *all {
			must(m.Down())
		} else if *steps > 0 {
			must(m.Steps(-*steps))
		} else {
			log.Fatal("specify --steps or --all")
		}

	case "to":
		fs := flag.NewFlagSet("to", flag.ExitOnError)
		fsDsn := fs.String("dsn", dsn, "database url")
		fsDir := fs.String("dir", dir, "migrations dir")
		_ = fs.Parse(os.Args[2:])
		if fs.NArg() < 1 {
			log.Fatal("usage: pgmigrate to <version>")
		}
		var v uint
		if _, err := fmt.Sscan(fs.Arg(0), &v); err != nil {
			log.Fatalf("invalid version: %v", err)
		}
		mustDsn(*fsDsn)
		m := mustMigrator(*fsDsn, *fsDir)
		defer closeM(m)
		must(m.Migrate(v))

	case "force":
		fs := flag.NewFlagSet("force", flag.ExitOnError)
		fsDsn := fs.String("dsn", dsn, "database url")
		fsDir := fs.String("dir", dir, "migrations dir")
		_ = fs.Parse(os.Args[2:])
		if fs.NArg() < 1 {
			log.Fatal("usage: pgmigrate force <version>")
		}
		var v int
		if _, err := fmt.Sscan(fs.Arg(0), &v); err != nil {
			log.Fatalf("invalid version: %v", err)
		}
		mustDsn(*fsDsn)
		m := mustMigrator(*fsDsn, *fsDir)
		defer closeM(m)
		must(m.Force(v))

	case "version":
		fs := flag.NewFlagSet("version", flag.ExitOnError)
		fsDsn := fs.String("dsn", dsn, "database url")
		fsDir := fs.String("dir", dir, "migrations dir")
		_ = fs.Parse(os.Args[2:])
		mustDsn(*fsDsn)
		m := mustMigrator(*fsDsn, *fsDir)
		defer closeM(m)
		v, dirty, err := m.Version()
		if errors.Is(err, migrate.ErrNilVersion) {
			fmt.Println("version: 0, dirty=false (no migrations applied)")
			return
		}
		must(err)
		fmt.Printf("version: %d, dirty=%v\n", v, dirty)

	default:
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Println(`Usage:
  pgmigrate <command> [flags]

Commands:
  create <name>          Create timestamped .up/.down.sql pair in db/migrations
  up [--steps N]         Apply all or N steps
  down --steps N|--all   Roll back N steps or all
  to <version>           Migrate to exact version
  force <version>        Set version manually (when dirty)
  version                Print current version and dirty

Flags/env:
  --dsn                  Postgres DSN (env DATABASE_URL)
  --dir                  Migrations dir (env MIGRATIONS_DIR, default db/migrations)

Example:
  export DATABASE_URL="postgres://app:app@localhost:5432/app?sslmode=disable"
  pgmigrate create init_schema
  pgmigrate up`)
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func must(err error) {
	if err == nil || errors.Is(err, migrate.ErrNoChange) {
		if errors.Is(err, migrate.ErrNoChange) {
			fmt.Println("no change")
		}
		return
	}
	log.Fatal(err)
}

func mustDsn(dsn string) {
	if dsn == "" {
		log.Fatal("DATABASE_URL/--dsn is required")
	}
}

func closeM(m *migrate.Migrate) { m.Close() }

func mustMigrator(dsn, dir string) *migrate.Migrate {
	if _, err := os.Stat(dir); err != nil {
		log.Fatalf("migrations dir %s not found: %v", dir, err)
	}
	srcURL := "file://" + dir
	m, err := migrate.New(srcURL, dsn)
	must(err)
	return m
}

func createPair(dir, name string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	ts := time.Now().UTC().Format("20060102150405")
	base := fmt.Sprintf("%s_%s", ts, name)
	up := filepath.Join(dir, base+".up.sql")
	down := filepath.Join(dir, base+".down.sql")
	if err := os.WriteFile(up, []byte("-- write UP migration here\n"), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(down, []byte("-- write DOWN migration here\n"), 0o644); err != nil {
		_ = os.Remove(up)
		return err
	}
	fmt.Println("created:")
	fmt.Println(" ", up)
	fmt.Println(" ", down)
	return nil
}

func sanitize(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	s = strings.ReplaceAll(s, " ", "_")
	s = strings.ReplaceAll(s, "-", "_")
	return s
}
