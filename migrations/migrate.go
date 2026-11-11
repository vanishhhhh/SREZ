// Package migrations embeds ordered, append-only SQL migrations into the binary.
package migrations

import (
	"context"
	"crypto/sha256"
	"embed"
	"fmt"
	"io/fs"
	"sort"

	"github.com/jmoiron/sqlx"
)

//go:embed *.sql
var files embed.FS

// Apply serializes concurrent starters. SQL and journal entries commit together;
// a failed batch rolls back. A modified applied file fails closed.
func Apply(ctx context.Context, db *sqlx.DB) error {
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(738291401)`); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
 version TEXT PRIMARY KEY, checksum TEXT NOT NULL, applied_at TIMESTAMPTZ NOT NULL DEFAULT now())`); err != nil {
		return err
	}
	names, err := fs.Glob(files, "*.sql")
	if err != nil {
		return err
	}
	sort.Strings(names)
	known := map[string]string{}
	for _, name := range names {
		body, err := files.ReadFile(name)
		if err != nil {
			return err
		}
		known[name] = fmt.Sprintf("%x", sha256.Sum256(body))
	}
	rows, err := tx.QueryxContext(ctx, `SELECT version, checksum FROM schema_migrations`)
	if err != nil {
		return err
	}
	applied := map[string]bool{}
	for rows.Next() {
		var name, checksum string
		if err = rows.Scan(&name, &checksum); err != nil {
			rows.Close()
			return err
		}
		if known[name] != checksum {
			rows.Close()
			return fmt.Errorf("migration %s missing or modified", name)
		}
		applied[name] = true
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, name := range names {
		if applied[name] {
			continue
		}
		// Refuse inserting an older migration behind an already applied version.
		for version := range applied {
			if version > name {
				return fmt.Errorf("out-of-order migration %s", name)
			}
		}
		body, _ := files.ReadFile(name)
		if _, err = tx.ExecContext(ctx, string(body)); err == nil {
			_, err = tx.ExecContext(ctx, `INSERT INTO schema_migrations(version, checksum) VALUES ($1, $2)`, name, known[name])
		}
		if err != nil {
			tx.Rollback()
			return fmt.Errorf("migration %s: %w", name, err)
		}
	}
	return tx.Commit()
}
