// Package rdbms provides unified relational database management abstractions,
// query construction, dialect integration, and schema definitions across relational adapters.
//
// File: table_model.go
// Usage:
//   Implements DynamicTableModel to adapt generic engine model.Model definitions into
//   the schema.TableModel interface required by relational query builders.
package rdbms

import (
	"strings"

	"github.com/SanjayDrop5528/models-go-engine/model"
	"github.com/SanjayDrop5528/models-go-engine/rdbms/schema"
)

// DynamicTableModel implements schema.TableModel for dynamic models generated from metadata.
type DynamicTableModel struct {
	table *schema.Table
	joins []*schema.RelationJoin
}

// Table returns the table metadata.
//
// Purpose:
//   Provides the schema.Table description including fields, primary keys, and aliases.
//
// Where it is used:
//   - In query builders when determining column names, table names, and FROM clauses.
//
// When can it be used:
//   - Whenever inspecting the underlying schema table definition of a dynamic model.
func (d *DynamicTableModel) Table() *schema.Table {
	return d.table
}

// Clone creates a shallow copy of the DynamicTableModel.
//
// Purpose:
//   Clones the DynamicTableModel and its relation joins slice for isolated query state.
//
// Where it is used:
//   - In query execution and relation joining when building query trees.
//
// When can it be used:
//   - When duplicating a table model to avoid mutating shared join state.
func (d *DynamicTableModel) Clone() schema.TableModel {
	joinsCopy := make([]*schema.RelationJoin, len(d.joins))
	copy(joinsCopy, d.joins)
	return &DynamicTableModel{
		table: d.table,
		joins: joinsCopy,
	}
}

// GetJoins returns all relation joins.
//
// Purpose:
//   Returns the slice of defined relation joins associated with this table model.
//
// Where it is used:
//   - In SelectQuery when evaluating eager loads or implicit JOIN conditions.
//
// When can it be used:
//   - Whenever traversing or rendering relational joins for the table.
func (d *DynamicTableModel) GetJoins() []*schema.RelationJoin {
	return d.joins
}

// Join returns a RelationJoin by name (case-insensitive).
//
// Purpose:
//   Finds a specific relation join by relation name matching case-insensitively.
//
// Where it is used:
//   - In SelectQuery when joining a specific named relationship.
//
// When can it be used:
//   - When resolving relation joins requested by name in query builders.
func (d *DynamicTableModel) Join(name string) *schema.RelationJoin {
	for _, j := range d.joins {
		if j != nil && j.Relation != nil && strings.EqualFold(j.Relation.Name, name) {
			return j
		}
	}
	return nil
}

// NewTableModelFromModel converts a model.Model into a schema.TableModel with orbital reference relations.
//
// Purpose:
//   Converts engine metadata model definitions (attributes, primary keys, relations) into
//   relational schema.Table and schema.Relation structures.
//
// Where it is used:
//   - In relational adapters when converting catalog models into queryable table models.
//
// When can it be used:
//   - Whenever bridging an engine model.Model definition into the RDBMS query layer.
func NewTableModelFromModel(m *model.Model) schema.TableModel {
	if m == nil {
		return nil
	}

	tblName := m.Table
	if tblName == "" {
		tblName = m.StorageName
	}
	if tblName == "" {
		tblName = m.Name
	}

	tbl := schema.NewTable(tblName)
	if m.Name != "" {
		tbl.SQLAlias = strings.ToLower(m.Name)
	}

	pkSet := make(map[string]bool)
	if m.PrimaryKey != nil {
		for _, col := range m.PrimaryKey.Columns {
			pkSet[col] = true
		}
	}

	for _, attr := range m.Attributes {
		colName := attr.ColumnName
		if colName == "" {
			colName = attr.Name
		}
		isPK := attr.IsPrimaryKey || pkSet[colName]
		f := &schema.Field{
			Name:     attr.Name,
			SQLName:  colName,
			DataType: string(attr.Type),
			IsPK:     isPK,
		}
		tbl.AddField(f)
	}

	var joins []*schema.RelationJoin
	for _, rel := range m.Relations {
		var relField *schema.Field
		for _, f := range tbl.Fields {
			if f.SQLName == rel.ForeignKey || f.Name == rel.ForeignKey {
				relField = f
				break
			}
		}

		targetTbl := schema.NewTable(rel.TargetModel)

		joinRel := &schema.Relation{
			Name:       rel.Name,
			Type:       schema.BelongsToRelation,
			Field:      relField,
			JoinTable:  targetTbl,
			ForeignKey: rel.ForeignKey,
			TargetKey:  rel.TargetKey,
		}

		targetModel := &DynamicTableModel{
			table: targetTbl,
		}

		joins = append(joins, schema.NewRelationJoin(joinRel, targetModel))
	}

	return &DynamicTableModel{
		table: tbl,
		joins: joins,
	}
}
