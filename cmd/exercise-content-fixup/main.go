// Package main implements a one-shot tool that cleans dead links and
// redundant rep-guidance text from exercises.description_markdown in a
// SQLite snapshot of the production database. It is read-only against the
// input database; the deliverable is a SQL UPDATE file that a human
// reviews before applying to production via `make fly-sql-write`.
package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"

	_ "github.com/mattn/go-sqlite3"
)

const exitUsage = 2

func main() {
	var (
		dbPath  = flag.String("db", "", "Path to the SQLite snapshot to read (required)")
		outPath = flag.String("out", "", "Path to write the generated UPDATE SQL (required)")
	)
	flag.Parse()

	if *dbPath == "" || *outPath == "" {
		flag.Usage()
		os.Exit(exitUsage)
	}

	if err := run(*dbPath, *outPath); err != nil {
		log.Fatalf("exercise-content-fixup: %v", err)
	}
}

func run(dbPath, outPath string) error {
	db, err := sql.Open("sqlite3", "file:"+dbPath+"?mode=ro")
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer func() { _ = db.Close() }()

	ctx := context.Background()
	rows, err := db.QueryContext(ctx, "SELECT id, name, description_markdown FROM exercises ORDER BY id")
	if err != nil {
		return fmt.Errorf("select exercises: %w", err)
	}
	defer func() { _ = rows.Close() }()

	count := 0
	for rows.Next() {
		var (
			id   int
			name string
			desc string
		)
		if err = rows.Scan(&id, &name, &desc); err != nil {
			return fmt.Errorf("scan row: %w", err)
		}
		count++
	}
	if err = rows.Err(); err != nil {
		return fmt.Errorf("iterate rows: %w", err)
	}

	fmt.Printf("scanned %d exercises\n", count) //nolint:forbidigo // human-facing progress output.

	// outPath is unused until task 9 adds UPDATE emission.
	_ = outPath

	return nil
}
