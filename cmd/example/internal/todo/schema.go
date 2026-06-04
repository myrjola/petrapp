package todo

import _ "embed"

// SchemaSQL contains the DDL for the todos table embedded from schema.sql.
//
//go:embed schema.sql
var SchemaSQL string
