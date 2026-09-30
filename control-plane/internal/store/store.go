// Package store manages the bordod state database.
package store

import (
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/hasangenc0/bordo/control-plane/internal/config"
	_ "modernc.org/sqlite" // register "sqlite" driver
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Open opens (and creates if necessary) the database described by cfg.
// For SQLite, the parent directory is created automatically.
func Open(cfg config.StoreConfig) (*sql.DB, error) {
	switch strings.ToLower(cfg.Driver) {
	case "sqlite", "":
		if err := os.MkdirAll(filepath.Dir(cfg.Path), 0o700); err != nil {
			return nil, fmt.Errorf("creating db directory: %w", err)
		}
		db, err := sql.Open("sqlite", cfg.Path)
		if err != nil {
			return nil, fmt.Errorf("opening sqlite db %s: %w", cfg.Path, err)
		}
		// Enable WAL mode, limit page cache to 4 MB, disable mmap to
		// reduce virtual-memory footprint on memory-constrained servers.
		if _, err := db.Exec(`PRAGMA journal_mode=WAL; PRAGMA foreign_keys=ON; PRAGMA cache_size=-4096; PRAGMA mmap_size=0;`); err != nil {
			db.Close()
			return nil, fmt.Errorf("configuring sqlite pragmas: %w", err)
		}
		return db, nil
	case "postgres":
		db, err := sql.Open("pgx", cfg.Path)
		if err != nil {
			return nil, fmt.Errorf("opening postgres db: %w", err)
		}
		return db, nil
	default:
		return nil, fmt.Errorf("unsupported store driver %q (use \"sqlite\" or \"postgres\")", cfg.Driver)
	}
}

// Migrate runs all pending SQL migrations embedded in migrations/*.sql.
// Migrations are applied in alphabetical order and tracked in schema_migrations.
func Migrate(db *sql.DB) error {
	// Ensure the tracking table exists first (bootstrap).
	bootstrapSQL, err := fs.ReadFile(migrationsFS, "migrations/001_initial.sql")
	if err != nil {
		return fmt.Errorf("reading bootstrap migration: %w", err)
	}
	if _, err := db.Exec(string(bootstrapSQL)); err != nil {
		return fmt.Errorf("applying bootstrap migration: %w", err)
	}

	// Collect all migration files.
	entries, err := fs.ReadDir(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("reading migrations dir: %w", err)
	}
	files := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			files = append(files, e.Name())
		}
	}
	sort.Strings(files)

	for _, name := range files {
		version := strings.TrimSuffix(name, ".sql")

		// Check if already applied.
		var count int
		if err := db.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE version = ?`, version).Scan(&count); err != nil {
			return fmt.Errorf("checking migration %s: %w", version, err)
		}
		if count > 0 {
			continue // already applied
		}

		// Apply the migration.
		content, err := fs.ReadFile(migrationsFS, "migrations/"+name)
		if err != nil {
			return fmt.Errorf("reading migration %s: %w", name, err)
		}
		if _, err := db.Exec(string(content)); err != nil {
			return fmt.Errorf("applying migration %s: %w", name, err)
		}

		// Record it.
		if _, err := db.Exec(`INSERT OR IGNORE INTO schema_migrations (version) VALUES (?)`, version); err != nil {
			return fmt.Errorf("recording migration %s: %w", name, err)
		}
	}
	return nil
}

// Ping verifies the database connection is alive.
func Ping(db *sql.DB) error {
	return db.Ping()
}
