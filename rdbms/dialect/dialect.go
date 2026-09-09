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

func (d *baseDialect) Name() Name {
	return d.name
}

func (d *baseDialect) HasFeature(f feature.Feature) bool {
	return d.features&f != 0
}

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

func (d *pgDialect) AppendParam(b []byte, idx int) []byte {
	b = append(b, '$')
	return strconv.AppendInt(b, int64(idx), 10)
}

// MySQL dialect
type mySQLDialect struct {
	baseDialect
}

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

func (d *mySQLDialect) AppendParam(b []byte, _ int) []byte {
	return append(b, '?')
}

// SQLite dialect
type sqliteDialect struct {
	baseDialect
}

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

func (d *sqliteDialect) AppendIdent(b []byte, ident string) []byte {
	return (&pgDialect{}).AppendIdent(b, ident)
}

func (d *sqliteDialect) AppendParam(b []byte, _ int) []byte {
	return append(b, '?')
}

// MSSQL dialect
type mssqlDialect struct {
	baseDialect
}

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

func (d *mssqlDialect) AppendParam(b []byte, idx int) []byte {
	b = append(b, "@p"...)
	return strconv.AppendInt(b, int64(idx), 10)
}
