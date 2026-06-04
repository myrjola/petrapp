// Package todo is the example app's product domain — a minimal CRUD entity
// proving the shared platform plumbing.
package todo

// A Todo is a single example task item.
//
// Created is the ISO-8601 (STRFTIME '%Y-%m-%dT%H:%M:%fZ') creation timestamp
// stored as TEXT; it is kept as a string because the sqlite driver does not
// scan this TEXT column into a time.Time.
type Todo struct {
	ID      int
	Title   string
	Notes   string
	Done    bool
	Created string
}
