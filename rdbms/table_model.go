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
func (d *DynamicTableModel) Table() *schema.Table {
	return d.table
}

// Clone creates a shallow copy of the DynamicTableModel.
func (d *DynamicTableModel) Clone() schema.TableModel {
	joinsCopy := make([]*schema.RelationJoin, len(d.joins))
	copy(joinsCopy, d.joins)
	return &DynamicTableModel{
		table: d.table,
		joins: joinsCopy,
	}
}

// GetJoins returns all relation joins.
func (d *DynamicTableModel) GetJoins() []*schema.RelationJoin {
	return d.joins
}

// Join returns a RelationJoin by name (case-insensitive).
func (d *DynamicTableModel) Join(name string) *schema.RelationJoin {
	for _, j := range d.joins {
		if j != nil && j.Relation != nil && strings.EqualFold(j.Relation.Name, name) {
			return j
		}
	}
	return nil
}

// NewTableModelFromModel converts a model.Model into a schema.TableModel with orbital reference relations.
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
