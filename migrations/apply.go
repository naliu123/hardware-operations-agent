package migrations

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	_ "embed"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed 005_identity.sql
var identity string

//go:embed 006_conversations.sql
var conversations string

//go:embed 007_python.sql
var python string

//go:embed 008_attachments.sql
var attachments string

var steps = []string{Core, Devices, ResponseEvents, Diagnosis, identity, conversations, python, attachments}

// Apply adopts the old, unversioned schema only after checking its actual
// columns, constraints and indexes against a disposable reference schema.
// Migrations, checksums and adoption are committed together. No down migration
// can silently discard private data; rollback requires restoring a DB backup.
func Apply(ctx context.Context, pool *pgxpool.Pool) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var target string
	if err = tx.QueryRow(ctx, "SELECT current_schema()").Scan(&target); err != nil {
		return err
	}
	var tracked bool
	if err = tx.QueryRow(ctx, "SELECT to_regclass('hwops_migrations') IS NOT NULL").Scan(&tracked); err != nil {
		return err
	}
	applied := 0
	if tracked {
		rows, err := tx.Query(ctx, "SELECT version, checksum FROM hwops_migrations ORDER BY version")
		if err != nil {
			return err
		}
		for rows.Next() {
			var version int
			var checksum string
			if err = rows.Scan(&version, &checksum); err != nil {
				rows.Close()
				return err
			}
			if version != applied+1 || version > len(steps) || checksum != digest(steps[version-1]) {
				rows.Close()
				return errors.New("migration history or checksum mismatch")
			}
			applied++
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
	}
	reference := "hwops_check_" + strings.ToLower(rand.Text())
	refIdent, targetIdent := pgx.Identifier{reference}.Sanitize(), pgx.Identifier{target}.Sanitize()
	if _, err = tx.Exec(ctx, "CREATE SCHEMA "+refIdent); err != nil {
		return err
	}
	setPath := func(schema string) error {
		_, e := tx.Exec(ctx, "SELECT set_config('search_path',$1,true)", pgx.Identifier{schema}.Sanitize())
		return e
	}
	if err = setPath(reference); err != nil {
		return err
	}
	checkCount := applied
	if !tracked {
		checkCount = 4
	}
	for _, sql := range steps[:checkCount] {
		if _, err = tx.Exec(ctx, sql); err != nil {
			return err
		}
	}
	expected, err := structure(ctx, tx, reference, target)
	if err != nil {
		return err
	}
	actual, err := structure(ctx, tx, target, reference)
	if err != nil {
		return err
	}
	for key, value := range actual {
		if expected[key] != value {
			return fmt.Errorf("schema mismatch at %s; migration stopped without changing business data", key)
		}
	}
	for key := range expected {
		if _, ok := actual[key]; ok {
			continue
		}
		// Legacy installs may be at 001..004. Missing indexes can be created
		// idempotently; missing columns/constraints in an existing table cannot.
		table := strings.Split(key, "/")[0]
		_, exists := actual[table+"/table"]
		if tracked || (exists && !strings.Contains(key, "/index/")) {
			return fmt.Errorf("schema missing %s; migration stopped", key)
		}
	}
	if err = setPath(target); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, "DROP SCHEMA "+refIdent+" CASCADE"); err != nil {
		return err
	}
	if !tracked {
		if _, err = tx.Exec(ctx, `CREATE TABLE `+targetIdent+`.hwops_migrations (
			version integer PRIMARY KEY, checksum text NOT NULL, applied_at timestamptz NOT NULL DEFAULT now())`); err != nil {
			return err
		}
	}
	for i := applied; i < len(steps); i++ {
		if _, err = tx.Exec(ctx, steps[i]); err != nil {
			return fmt.Errorf("migration %03d: %w", i+1, err)
		}
		if _, err = tx.Exec(ctx, "INSERT INTO hwops_migrations(version,checksum) VALUES ($1,$2)", i+1, digest(steps[i])); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func digest(value string) string { return fmt.Sprintf("%x", sha256.Sum256([]byte(value))) }

func structure(ctx context.Context, tx pgx.Tx, schema, other string) (map[string]string, error) {
	rows, err := tx.Query(ctx, `
		WITH tables AS (
			SELECT c.oid,c.relname FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace
			WHERE n.nspname=$1 AND c.relkind='r' AND c.relname <> 'hwops_migrations'
		)
		SELECT relname||'/table', 'table' FROM tables
		UNION ALL
		SELECT t.relname||'/column/'||a.attname,
			format_type(a.atttypid,a.atttypmod)||':'||a.attnotnull||':'||COALESCE(pg_get_expr(d.adbin,d.adrelid),'')
		FROM tables t JOIN pg_attribute a ON a.attrelid=t.oid AND a.attnum>0 AND NOT a.attisdropped
		LEFT JOIN pg_attrdef d ON d.adrelid=t.oid AND d.adnum=a.attnum
		UNION ALL
		SELECT t.relname||'/constraint/'||c.conname, pg_get_constraintdef(c.oid)
		FROM tables t JOIN pg_constraint c ON c.conrelid=t.oid
		UNION ALL
		SELECT t.relname||'/index/'||c.relname, pg_get_indexdef(c.oid)||':'||i.indisvalid
		FROM tables t JOIN pg_index i ON i.indrelid=t.oid JOIN pg_class c ON c.oid=i.indexrelid`, schema)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var key, value string
		if err = rows.Scan(&key, &value); err != nil {
			return nil, err
		}
		for _, prefix := range []string{schema, other} {
			value = strings.ReplaceAll(value, pgx.Identifier{prefix}.Sanitize()+".", "")
			value = strings.ReplaceAll(value, prefix+".", "")
		}
		out[key] = value
	}
	return out, rows.Err()
}
