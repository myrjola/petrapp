package todo

import (
	"context"
	"fmt"

	"github.com/myrjola/petrapp/internal/platform/sqlitekit"
)

// Repository persists Todo items in a sqlitekit-backed database.
type Repository struct {
	db *sqlitekit.Database
}

// NewRepository returns a Repository backed by the given database.
func NewRepository(db *sqlitekit.Database) *Repository {
	return &Repository{db: db}
}

// Create inserts a new todo and returns its generated id.
func (r *Repository) Create(ctx context.Context, title, notes string) (int, error) {
	res, err := r.db.ReadWrite.ExecContext(ctx,
		"INSERT INTO todos (title, notes) VALUES (?, ?)", title, notes)
	if err != nil {
		return 0, fmt.Errorf("insert todo: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("last insert id: %w", err)
	}
	return int(id), nil
}

// List returns all todos ordered newest first.
func (r *Repository) List(ctx context.Context) ([]Todo, error) {
	rows, err := r.db.ReadOnly.QueryContext(ctx,
		"SELECT id, title, notes, done, created FROM todos ORDER BY id DESC")
	if err != nil {
		return nil, fmt.Errorf("query todos: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var todos []Todo
	for rows.Next() {
		var t Todo
		if err = rows.Scan(&t.ID, &t.Title, &t.Notes, &t.Done, &t.Created); err != nil {
			return nil, fmt.Errorf("scan todo: %w", err)
		}
		todos = append(todos, t)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate todos: %w", err)
	}
	return todos, nil
}

// Get returns the todo with the given id.
func (r *Repository) Get(ctx context.Context, id int) (Todo, error) {
	var t Todo
	err := r.db.ReadOnly.QueryRowContext(ctx,
		"SELECT id, title, notes, done, created FROM todos WHERE id = ?", id).
		Scan(&t.ID, &t.Title, &t.Notes, &t.Done, &t.Created)
	if err != nil {
		return Todo{}, fmt.Errorf("get todo %d: %w", id, err)
	}
	return t, nil
}

// Toggle flips the done flag of the todo with the given id.
func (r *Repository) Toggle(ctx context.Context, id int) error {
	if _, err := r.db.ReadWrite.ExecContext(ctx,
		"UPDATE todos SET done = NOT done WHERE id = ?", id); err != nil {
		return fmt.Errorf("toggle todo %d: %w", id, err)
	}
	return nil
}

// Delete removes the todo with the given id.
func (r *Repository) Delete(ctx context.Context, id int) error {
	if _, err := r.db.ReadWrite.ExecContext(ctx,
		"DELETE FROM todos WHERE id = ?", id); err != nil {
		return fmt.Errorf("delete todo %d: %w", id, err)
	}
	return nil
}
