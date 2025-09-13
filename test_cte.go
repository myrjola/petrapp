package main

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"

	_ "github.com/mattn/go-sqlite3"
	"github.com/myrjola/petrapp/internal/tools"
)

func main() {
	// Create test database
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		panic(err)
	}
	defer db.Close()

	// Create test schema
	schema := `
		CREATE TABLE users (
			id INTEGER PRIMARY KEY,
			name TEXT NOT NULL,
			email TEXT NOT NULL
		);

		CREATE TABLE workouts (
			id INTEGER PRIMARY KEY,
			user_id INTEGER NOT NULL,
			date TEXT NOT NULL,
			FOREIGN KEY (user_id) REFERENCES users(id)
		);
	`

	if _, err := db.Exec(schema); err != nil {
		panic(err)
	}

	// Insert test data
	testData := `
		INSERT INTO users (id, name, email) VALUES 
			(1, 'John Doe', 'john@example.com'),
			(2, 'Jane Smith', 'jane@example.com');

		INSERT INTO workouts (id, user_id, date) VALUES
			(1, 1, '2024-01-01'),
			(2, 1, '2024-01-02'),
			(3, 2, '2024-01-01');
	`

	if _, err := db.Exec(testData); err != nil {
		panic(err)
	}

	// Test CTE query
	logger := slog.Default()
	tool := tools.NewSecureQueryTool(db, logger)
	ctx := context.Background()

	cteQuery := "WITH user_workouts AS (SELECT user_id, COUNT(*) as workout_count FROM workouts GROUP BY user_id) " +
		"SELECT u.name, uw.workout_count FROM users u JOIN user_workouts uw ON u.id = uw.user_id"

	result, err := tool.ExecuteQuery(ctx, cteQuery)
	if err != nil {
		panic(fmt.Sprintf("CTE query failed: %v", err))
	}

	fmt.Printf("CTE Query Results:\n")
	fmt.Printf("Columns: %v\n", result.Columns)
	fmt.Printf("Rows: %v\n", result.Rows)
	fmt.Printf("Row Count: %d\n", result.RowCount)
	fmt.Printf("CTE query executed successfully!\n")
}