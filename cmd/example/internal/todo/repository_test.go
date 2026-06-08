package todo_test

import (
	"testing"

	"github.com/myrjola/petrapp/cmd/example/internal/todo"
	"github.com/myrjola/petrapp/internal/platform/auth"
	"github.com/myrjola/petrapp/internal/platform/sqlitekit"
	"github.com/myrjola/petrapp/internal/platform/testkit"
)

func newTestRepo(t *testing.T) *todo.Repository {
	t.Helper()
	logger := testkit.NewLogger(testkit.NewWriter(t))
	db, err := sqlitekit.NewDatabase(t.Context(), sqlitekit.Config{
		URL:          ":memory:",
		Schema:       auth.SchemaSQL + "\n" + todo.SchemaSQL,
		Fixtures:     "",
		Logger:       logger,
		Premigration: nil,
	})
	if err != nil {
		t.Fatalf("NewDatabase: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return todo.NewRepository(db)
}

func TestRepository_CreateListGetToggle(t *testing.T) {
	t.Parallel()
	repo := newTestRepo(t)
	ctx := t.Context()

	id, err := repo.Create(ctx, "buy milk", "2%")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	items, err := repo.List(ctx)
	if err != nil || len(items) != 1 {
		t.Fatalf("List: items=%d err=%v", len(items), err)
	}
	got, err := repo.Get(ctx, id)
	if err != nil || got.Title != "buy milk" || got.Done {
		t.Fatalf("Get: %+v err=%v", got, err)
	}
	if err = repo.Toggle(ctx, id); err != nil {
		t.Fatalf("Toggle: %v", err)
	}
	got, _ = repo.Get(ctx, id)
	if !got.Done {
		t.Fatal("expected Done after Toggle")
	}
}
