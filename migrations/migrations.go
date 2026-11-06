// Package migrations embeds the SQL schema files so the binaries can apply them.
package migrations

import _ "embed"

//go:embed 001_initial_schema.sql
var InitialSchema string
