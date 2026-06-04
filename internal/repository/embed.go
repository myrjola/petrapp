package repository

import _ "embed"

// SchemaSQL is petra's product schema (workouts, exercises, preferences, etc.).
// It is concatenated after auth.SchemaSQL when constructing the database, so
// petra tables may reference auth tables (e.g. users) via foreign keys.
//
//go:embed schema.sql
var SchemaSQL string

// FixturesSQL is petra's idempotent seed data.
//
//go:embed fixtures.sql
var FixturesSQL string
