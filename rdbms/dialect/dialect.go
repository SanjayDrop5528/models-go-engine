// Package dialect provides SQL dialect formatting, identifier quoting, parameter placeholder
// syntax, and capability feature checks for PostgreSQL, MySQL, SQLite, and MSSQL.
//
// File: dialect.go
// Usage:
//   Defines the Dialect interface and dialect instances (NewPostgreSQL, NewMySQL,
//   NewSQLite, NewMSSQL) that specialize SQL statement rendering for relational engines.
package dialect

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/SanjayDrop5528/models-go-engine/rdbms/dialect/feature"
)

// Name identifies a SQL dialect.
type Name string

const (
	MySQL      Name = "mysql"
	PostgreSQL Name = "pg"
	SQLite     Name = "sqlite"
	MSSQL      Name = "mssql"
)

// Dialect defines formatting, quoting, and feature rules for a SQL dialect.
type Dialect interface {
	Name() Name
	HasFeature(f feature.Feature) bool
	AppendIdent(b []byte, ident string) []byte
	AppendString(b []byte, s string) []byte
	AppendParam(b []byte, idx int) []byte
}

type baseDialect struct {
	name     Name
	features feature.Feature
}

// Name returns the dialect Name identifier.
//
// Purpose:
//   Identifies the target database dialect (pg, mysql, sqlite, mssql).
//
// Where it is used:
//   - In query builders and driver matching logic.
//
// When can it be used:
//   - When checking the name of the active SQL dialect.
func (d *baseDialect) Name() Name {
	return d.name
}

// HasFeature returns whether the dialect supports a specific feature flag.
//
// Purpose:
//   Checks the bitmask for supported syntax features like CTEs, Returning, or Lock statements.
//
// Where it is used:
//   - In query compilation when choosing whether to emit RETURNING or separate SELECT queries.
//
// When can it be used:
//   - Before emitting syntax that requires specific engine capabilities.
func (d *baseDialect) HasFeature(f feature.Feature) bool {
	return d.features&f != 0
}

// AppendString escapes and appends a single-quoted string literal to the buffer.
//
// Purpose:
//   Appends SQL string literals with standard single-quote escaping (' -> '').
//
// Where it is used:
//   - In dialect query generation for raw literal string formatting.
//
// When can it be used:
//   - When appending string literals directly into SQL buffers.
func (d *baseDialect) AppendString(b []byte, s string) []byte {
	b = append(b, '\'')
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '\'' {
			b = append(b, '\'', '\'')
		} else {
			b = append(b, c)
		}
	}
	b = append(b, '\'')
	return b
}

// PostgreSQL dialect
type pgDialect struct {
	baseDialect
}

// NewPostgreSQL creates a Dialect instance configured for PostgreSQL.
//
// Purpose:
//   Initializes the PostgreSQL dialect with double-quote identifier quoting and $1 positional parameters.
//
// Where it is used:
//   - In models-go-postgres adapter initialization.
//
// When can it be used:
//   - When targeting PostgreSQL database instances.
func NewPostgreSQL() Dialect {
	return &pgDialect{
		baseDialect: baseDialect{
			name: PostgreSQL,
			features: feature.SelectExists |
				feature.Returning |
				feature.CTE |
				feature.RecursiveCTE |
				feature.DistinctOn |
				feature.TableLocking,
		},
	}
}

// AppendIdent quotes an identifier using PostgreSQL double quotes.
//
// Purpose:
//   Escapes identifiers and dots in table/column names using double quotes ("schema"."table").
//
// Where it is used:
//   - In query generation when rendering column, table, and schema references.
//
// When can it be used:
//   - Whenever formatting PostgreSQL identifiers safely.
func (d *pgDialect) AppendIdent(b []byte, ident string) []byte {
	if strings.Contains(ident, ".") {
		parts := strings.Split(ident, ".")
		for i, p := range parts {
			if i > 0 {
				b = append(b, '.')
			}
			b = d.appendQuote(b, p)
		}
		return b
	}
	return d.appendQuote(b, ident)
}

// appendQuote quotes a single string token in double quotes.
//
// Purpose:
//   Surrounds a token in double quotes, escaping embedded quotes.
//
// Where it is used:
//   - Internally in AppendIdent.
//
// When can it be used:
//   - When quoting single identifier segments.
func (d *pgDialect) appendQuote(b []byte, s string) []byte {
	b = append(b, '"')
	for i := 0; i < len(s); i++ {
		if s[i] == '"' {
			b = append(b, '"', '"')
		} else {
			b = append(b, s[i])
		}
	}
	b = append(b, '"')
	return b
}

// AppendParam formats a 1-based parameter placeholder ($1, $2, ...).
//
// Purpose:
//   Emits PostgreSQL positional parameter placeholders.
//
// Where it is used:
//   - In QueryGen when binding arguments into SQL statements.
//
// When can it be used:
//   - When generating parameterized PostgreSQL queries.
func (d *pgDialect) AppendParam(b []byte, idx int) []byte {
	b = append(b, '$')
	return strconv.AppendInt(b, int64(idx), 10)
}

// MySQL dialect
type mySQLDialect struct {
	baseDialect
}

// NewMySQL creates a Dialect instance configured for MySQL.
//
// Purpose:
//   Initializes the MySQL dialect with backtick identifier quoting and ? positional parameters.
//
// Where it is used:
//   - In models-go-mysql adapter initialization.
//
// When can it be used:
//   - When targeting MySQL or MariaDB database instances.
func NewMySQL() Dialect {
	return &mySQLDialect{
		baseDialect: baseDialect{
			name: MySQL,
			features: feature.SelectExists |
				feature.CTE |
				feature.RecursiveCTE |
				feature.IndexHints |
				feature.TableLocking,
		},
	}
}

// AppendIdent quotes an identifier using MySQL backticks.
//
// Purpose:
//   Quotes table and column identifiers with backticks (`schema`.`table`).
//
// Where it is used:
//   - In query compilation for MySQL targets.
//
// When can it be used:
//   - When formatting MySQL identifiers.
func (d *mySQLDialect) AppendIdent(b []byte, ident string) []byte {
	if strings.Contains(ident, ".") {
		parts := strings.Split(ident, ".")
		for i, p := range parts {
			if i > 0 {
				b = append(b, '.')
			}
			b = d.appendQuote(b, p)
		}
		return b
	}
	return d.appendQuote(b, ident)
}

// appendQuote quotes a token with backticks.
//
// Purpose:
//   Encloses a token in backticks and escapes interior backticks.
//
// Where it is used:
//   - Internally in MySQL AppendIdent.
//
// When can it be used:
//   - When quoting individual identifier segments for MySQL.
func (d *mySQLDialect) appendQuote(b []byte, s string) []byte {
	b = append(b, '`')
	for i := 0; i < len(s); i++ {
		if s[i] == '`' {
			b = append(b, '`', '`')
		} else {
			b = append(b, s[i])
		}
	}
	b = append(b, '`')
	return b
}

// AppendParam appends a ? positional placeholder.
//
// Purpose:
//   Emits MySQL question-mark parameter placeholders.
//
// Where it is used:
//   - In QueryGen when binding arguments for MySQL.
//
// When can it be used:
//   - When generating parameterized MySQL queries.
func (d *mySQLDialect) AppendParam(b []byte, _ int) []byte {
	return append(b, '?')
}

// SQLite dialect
type sqliteDialect struct {
	baseDialect
}

// NewSQLite creates a Dialect instance configured for SQLite.
//
// Purpose:
//   Initializes SQLite dialect with standard SQL quoting and ? parameter placeholders.
//
// Where it is used:
//   - In embedded SQLite test fixtures and adapters.
//
// When can it be used:
//   - When targeting SQLite database files or in-memory SQLite instances.
func NewSQLite() Dialect {
	return &sqliteDialect{
		baseDialect: baseDialect{
			name: SQLite,
			features: feature.SelectExists |
				feature.Returning |
				feature.CTE |
				feature.RecursiveCTE,
		},
	}
}

// AppendIdent quotes an identifier using SQLite standard quotes.
//
// Purpose:
//   Quotes SQLite identifiers using standard double quotes.
//
// Where it is used:
//   - In query compilation for SQLite targets.
//
// When can it be used:
//   - When formatting SQLite identifiers.
func (d *sqliteDialect) AppendIdent(b []byte, ident string) []byte {
	return (&pgDialect{}).AppendIdent(b, ident)
}

// AppendParam appends a ? positional placeholder for SQLite.
//
// Purpose:
//   Emits SQLite parameter placeholders.
//
// Where it is used:
//   - In QueryGen when binding arguments for SQLite.
//
// When can it be used:
//   - When generating parameterized SQLite queries.
func (d *sqliteDialect) AppendParam(b []byte, _ int) []byte {
	return append(b, '?')
}

// MSSQL dialect
type mssqlDialect struct {
	baseDialect
}

// NewMSSQL creates a Dialect instance configured for Microsoft SQL Server.
//
// Purpose:
//   Initializes MSSQL dialect with square bracket identifier quoting and @p parameter placeholders.
//
// Where it is used:
//   - When configuring Microsoft SQL Server connections.
//
// When can it be used:
//   - When generating SQL queries targeting Microsoft SQL Server.
func NewMSSQL() Dialect {
	return &mssqlDialect{
		baseDialect: baseDialect{
			name: MSSQL,
			features: feature.SelectExists |
				feature.CTE |
				feature.RecursiveCTE,
		},
	}
}

// AppendIdent quotes an identifier using MSSQL square brackets ([schema].[table]).
//
// Purpose:
//   Quotes table and column identifiers with brackets for SQL Server.
//
// Where it is used:
//   - In query compilation for MSSQL targets.
//
// When can it be used:
//   - When formatting SQL Server identifiers.
func (d *mssqlDialect) AppendIdent(b []byte, ident string) []byte {
	if strings.Contains(ident, ".") {
		parts := strings.Split(ident, ".")
		for i, p := range parts {
			if i > 0 {
				b = append(b, '.')
			}
			b = append(b, fmt.Sprintf("[%s]", p)...)
		}
		return b
	}
	return append(b, fmt.Sprintf("[%s]", ident)...)
}

// AppendParam appends a 1-based parameter placeholder (@p1, @p2, ...).
//
// Purpose:
//   Emits MSSQL named parameter placeholders.
//
// Where it is used:
//   - In QueryGen when binding arguments for MSSQL.
//
// When can it be used:
//   - When generating parameterized SQL Server queries.
func (d *mssqlDialect) AppendParam(b []byte, idx int) []byte {
	b = append(b, "@p"...)
	return strconv.AppendInt(b, int64(idx), 10)
}
