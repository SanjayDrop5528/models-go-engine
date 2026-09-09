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
func NewDB(db *sql.DB, d dialect.Dialect) *DB {
	return query.NewDB(db, d)
}

// NewSelectQuery returns a SelectQuery attached to the provided DB.
func NewSelectQuery(db *DB) *SelectQuery {
	return query.NewSelectQuery(db)
}

// NewSelectFromQuery returns a SelectQuery initialized from a unified query.Query.
func NewSelectFromQuery(db *DB, uq coreQuery.Query) *SelectQuery {
	return query.NewSelectQuery(db).ApplyUnifiedQuery(uq)
}

