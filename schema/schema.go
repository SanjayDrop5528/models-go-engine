package schema

import (
	"github.com/SanjayDrop5528/models-go-engine/model"
)

// SchemaAttribute defines an attribute in a normalized database schema.
type SchemaAttribute struct {
	Name          string         `json:"name"`
	Type          model.DataType `json:"type"`
	Length        int            `json:"length,omitempty"`
	Precision     int            `json:"precision,omitempty"`
	Scale         int            `json:"scale,omitempty"`
	Nullable      bool           `json:"nullable"`
	Default       any            `json:"default,omitempty"`
	PrimaryKey    bool           `json:"primary_key"`
	Unique        bool           `json:"unique"`
	AutoIncrement bool           `json:"auto_increment"`
	Comment       string         `json:"comment,omitempty"`
	Extra         map[string]any `json:"extra,omitempty"`
}

// SchemaKey defines a primary or unique key constraint.
type SchemaKey struct {
	Name    string   `json:"name,omitempty"`
	Columns []string `json:"columns"`
}

// SchemaIndex defines an index on the table/collection.
type SchemaIndex struct {
	Name    string          `json:"name"`
	Columns []string        `json:"columns"`
	Unique  bool            `json:"unique"`
	Type    model.IndexType `json:"type,omitempty"`
}

// SchemaRelation defines a foreign key or collection reference.
type SchemaRelation struct {
	Name          string `json:"name"`
	Column        string `json:"column"`
	ForeignTable  string `json:"foreign_table"`
	ForeignColumn string `json:"foreign_column"`
	OnDelete      string `json:"on_delete,omitempty"`
	OnUpdate      string `json:"on_update,omitempty"`
}

// Schema represents the database-independent schema of a table or collection.
type Schema struct {
	Name        string            `json:"name"`
	StorageType model.StorageType `json:"storage_type"`
	Attributes  []SchemaAttribute `json:"attributes"`
	PrimaryKey  *SchemaKey        `json:"primary_key,omitempty"`
	Indexes     []SchemaIndex     `json:"indexes,omitempty"`
	Relations   []SchemaRelation  `json:"relations,omitempty"`
	RawMetadata map[string]any    `json:"raw_metadata,omitempty"`
}

// GetAttribute finds an attribute by name.
func (s *Schema) GetAttribute(name string) *SchemaAttribute {
	for i := range s.Attributes {
		if s.Attributes[i].Name == name {
			return &s.Attributes[i]
		}
	}
	return nil
}

// GetIndex finds an index by name.
func (s *Schema) GetIndex(name string) *SchemaIndex {
	for i := range s.Indexes {
		if s.Indexes[i].Name == name {
			return &s.Indexes[i]
		}
	}
	return nil
}

// FromModel converts a Model metadata definition to a Schema representation.
func FromModel(m *model.Model) *Schema {
	if m == nil {
		return nil
	}

	storageName := m.StorageName
	if storageName == "" {
		storageName = m.Name
	}

	s := &Schema{
		Name:        storageName,
		StorageType: m.StorageType,
		Attributes:  make([]SchemaAttribute, 0, len(m.Attributes)),
		Indexes:     make([]SchemaIndex, 0, len(m.Indexes)),
		Relations:   make([]SchemaRelation, 0, len(m.Relations)),
	}

	var pkCols []string
	if m.PrimaryKey != nil {
		pkCols = append(pkCols, m.PrimaryKey.Columns...)
	}

	for _, attr := range m.Attributes {
		isPK := m.IsPrimaryKey(attr.Name)
		sAttr := SchemaAttribute{
			Name:          attr.Name,
			Type:          attr.Type,
			Length:        attr.Length,
			Precision:     attr.Precision,
			Scale:         attr.Scale,
			Nullable:      attr.Nullable,
			Default:       attr.Default,
			PrimaryKey:    isPK,
			Unique:        attr.Unique,
			AutoIncrement: attr.AutoIncrement,
			Comment:       attr.Comment,
		}
		if isPK && len(pkCols) == 0 {
			pkCols = append(pkCols, attr.Name)
		}
		s.Attributes = append(s.Attributes, sAttr)
	}

	if len(pkCols) > 0 {
		s.PrimaryKey = &SchemaKey{
			Columns: pkCols,
		}
	}

	for _, idx := range m.Indexes {
		s.Indexes = append(s.Indexes, SchemaIndex{
			Name:    idx.Name,
			Columns: idx.Columns,
			Unique:  idx.Unique,
			Type:    idx.Type,
		})
	}

	for _, rel := range m.Relations {
		s.Relations = append(s.Relations, SchemaRelation{
			Name:          rel.Name,
			Column:        rel.ForeignKey,
			ForeignTable:  rel.TargetModel,
			ForeignColumn: rel.TargetKey,
			OnDelete:      rel.OnDelete,
			OnUpdate:      rel.OnUpdate,
		})
	}

	return s
}
