package database

import _ "embed"

//go:embed schema.sql
var schemaSQL string

//go:embed post_summaries.sql
var postSummariesSQL string
