// Package rdbms provides unified relational database management abstractions,
// query construction, dialect integration, and schema definitions across relational adapters.
//
// File: db.go
// Usage:
//   Exports top-level type aliases and convenience constructor functions (NewDB, NewSelectQuery,
//   NewSelectFromQuery) for building and executing dialect-aware SQL queries.
package rdbms

import (
	"database/sql"

	coreQuery "github.com/SanjayDrop5528/models-go-engine/query"
	"github.com/SanjayDrop5528/models-go-engine/rdbms/dialect"
	"github.com/SanjayDrop5528/models-go-engine/rdbms/query"
	"github.com/SanjayDrop5528/models-go-engine/rdbms/schema"
)

// Type aliases to make rdbms package intuitive and unified.
type (
	DB           = query.DB
	SelectQuery  = query.SelectQuery
	IConn        = query.IConn
	Query        = query.Query
	QueryBuilder = query.QueryBuilder
	Order        = query.Order
	RelationOpts = query.RelationOpts
	Table        = schema.Table
	Field        = schema.Field
	Relation     = schema.Relation
	TableModel   = schema.TableModel
	CoreQuery    = coreQuery.Query
)

const (
	OrderAsc     = query.OrderAsc
	OrderDesc    = query.OrderDesc
	OrderDefault = query.OrderDefault
)

// NewDB creates a new RDBMS database wrapper with the specified dialect.
//
// Purpose:
//   Wraps a standard database/sql connection handle with a dialect implementation
//   to facilitate dialect-specific query generation and execution.
//
// Where it is used:
//   - In relational database adapters (Postgres, MySQL) during initialization.
//
// When can it be used:
//   - Whenever connecting a SQL database handle to the RDBMS query engine.
func NewDB(db *sql.DB, d dialect.Dialect) *DB {
	return query.NewDB(db, d)
}

// NewSelectQuery returns a SelectQuery attached to the provided DB.
//
// Purpose:
//   Initializes an empty fluent SelectQuery builder bound to the provided database handle.
//
// Where it is used:
//   - In query compilation and test suites constructing custom SQL select statements.
//
// When can it be used:
//   - When beginning programmatic construction of a SELECT statement.
func NewSelectQuery(db *DB) *SelectQuery {
	return query.NewSelectQuery(db)
}

// NewSelectFromQuery returns a SelectQuery initialized from a unified query.Query.
//
// Purpose:
//   Translates an engine-level unified Query specification into a dialect-aware SelectQuery.
//
// Where it is used:
//   - In relational adapter Query implementations to compile and execute unified queries.
//
// When can it be used:
//   - When bridging engine Query ASTs directly into executable SQL SELECT queries.
func NewSelectFromQuery(db *DB, uq coreQuery.Query) *SelectQuery {
	return query.NewSelectQuery(db).ApplyUnifiedQuery(uq)
}

