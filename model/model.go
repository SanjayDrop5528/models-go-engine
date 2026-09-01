package model

import (
	"fmt"
	"strings"
	"time"
)

// ModelRef uniquely identifies a model reference for adapter CRUD and execution calls.
//
// Example Usage:
//
//	ref := model.ModelRef{
//	    ID:          "employee",
//	    Name:        "Employee",
//	    StorageName: "public.employees", // Schema-qualified table name
//	    Database:    "meet_kriyatec_spark",
//	    PrimaryKey:  "id",
//	}
type ModelRef struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	StorageName string `json:"storage_name"`
	Database    string `json:"database"`
	PrimaryKey  string `json:"primary_key,omitempty"`
}

// NewModelRef creates a ModelRef with standard default values.
func NewModelRef(id, name, storageName, primaryKey string) ModelRef {
	if storageName == "" {
		storageName = id
	}
	if primaryKey == "" {
		primaryKey = "id"
	}
	return ModelRef{
		ID:          id,
		Name:        name,
		StorageName: storageName,
		PrimaryKey:  primaryKey,
	}
}

// OrbitalRefSpec defines complete reference parameters for foreign and orbital references.
type OrbitalRefSpec struct {
	Schema    string `json:"schema,omitempty"`
	Model     string `json:"model"`
	Attribute string `json:"attribute"`
	OnDelete  string `json:"on_delete,omitempty"`
	OnUpdate  string `json:"on_update,omitempty"`
}

// ItemRule defines validation rules for array item elements.
type ItemRule struct {
	Type      DataType `json:"type,omitempty"`
	MinLength *int     `json:"min_length,omitempty"`
	MaxLength *int     `json:"max_length,omitempty"`
	Pattern   string   `json:"pattern,omitempty"`
}

// Attribute defines a single field/column in a dynamic model.
type Attribute struct {
	Name          string          `json:"name"`
	RefName       string          `json:"ref_name,omitempty"`
	ColumnName    string          `json:"column_name,omitempty"`
	JSONField     string          `json:"json_field,omitempty"`
	Type          DataType        `json:"type"`
	CustomType    string          `json:"custom_type,omitempty"`
	Length        int             `json:"length,omitempty"`
	Precision     int             `json:"precision,omitempty"`
	Scale         int             `json:"scale,omitempty"`
	Nullable      bool            `json:"nullable"`
	Default       any             `json:"default,omitempty"`
	Unique        bool            `json:"unique"`
	IsPrimaryKey  bool            `json:"is_primary_key,omitempty"`
	AutoIncrement bool            `json:"auto_increment"`
	Comment       string          `json:"comment,omitempty"`
	Validation    *RuleSet        `json:"validation,omitempty"`
	Reference     *OrbitalRefSpec `json:"reference,omitempty"`
}

// RuleSet defines field-level validation constraints.
type RuleSet struct {
	Required  bool      `json:"required,omitempty"`
	Min       *float64  `json:"min,omitempty"`
	Max       *float64  `json:"max,omitempty"`
	MinLength *int      `json:"min_length,omitempty"`
	MaxLength *int      `json:"max_length,omitempty"`
	Pattern   string    `json:"pattern,omitempty"`
	Enum      []any     `json:"enum,omitempty"`
	Precision *int      `json:"precision,omitempty"`
	Scale     *int      `json:"scale,omitempty"`
	Items     *ItemRule `json:"items,omitempty"`
}

// Index defines an index on a model.
type Index struct {
	Name    string    `json:"name"`
	Columns []string  `json:"columns"`
	Unique  bool      `json:"unique"`
	Type    IndexType `json:"type,omitempty"`
}

// RelationType defines the multiplicity of model relationships.
type RelationType string

const (
	RelOneToOne   RelationType = "ONE_TO_ONE"
	RelOneToMany  RelationType = "ONE_TO_MANY"
	RelManyToOne  RelationType = "MANY_TO_ONE"
	RelManyToMany RelationType = "MANY_TO_MANY"
)

// Relation defines a relationship to another model.
type Relation struct {
	Name        string       `json:"name"`
	Type        RelationType `json:"type"`
	TargetModel string       `json:"target_model"`
	ForeignKey  string       `json:"foreign_key"`
	TargetKey   string       `json:"target_key"`
	OnDelete    string       `json:"on_delete,omitempty"`
	OnUpdate    string       `json:"on_update,omitempty"`
}

// PrimaryKey defines the primary key constraint for a model.
type PrimaryKey struct {
	Name    string   `json:"name,omitempty"`
	Columns []string `json:"columns"`
}

// Model defines a dynamic entity metadata schema.
type Model struct {
	ID          string         `json:"id"`
	Schema      string         `json:"schema,omitempty"`
	Name        string         `json:"name"`
	Table       string         `json:"table,omitempty"`
	StorageName string         `json:"storage_name"`
	Database    string         `json:"database"`
	StorageType StorageType    `json:"storage_type"`
	Version     int            `json:"version"`
	Status      ModelStatus    `json:"status"`
	Description string         `json:"description,omitempty"`
	Attributes  []Attribute    `json:"attributes"`
	PrimaryKey  *PrimaryKey    `json:"primary_key,omitempty"`
	Indexes     []Index        `json:"indexes,omitempty"`
	Relations   []Relation     `json:"relations,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
}

// Ref returns a ModelRef for this model.
func (m *Model) Ref() ModelRef {
	storage := m.StorageName
	if storage == "" {
		storage = m.Table
		if storage == "" {
			storage = m.Name
		}
	}
	pkName := ""
	if m.PrimaryKey != nil && len(m.PrimaryKey.Columns) > 0 {
		pkName = m.PrimaryKey.Columns[0]
	}
	if pkName == "" {
		for _, attr := range m.Attributes {
			if attr.IsPrimaryKey {
				pkName = attr.Name
				break
			}
		}
	}
	if pkName == "" {
		pkName = "id"
	}
	return ModelRef{
		ID:          m.ID,
		Name:        m.Name,
		StorageName: storage,
		Database:    m.Database,
		PrimaryKey:  pkName,
	}
}

// GetAttribute finds an attribute by name, ref_name, column_name, or json_field (case-insensitive search).
func (m *Model) GetAttribute(name string) *Attribute {
	for i := range m.Attributes {
		if strings.EqualFold(m.Attributes[i].Name, name) ||
			(m.Attributes[i].RefName != "" && strings.EqualFold(m.Attributes[i].RefName, name)) ||
			(m.Attributes[i].ColumnName != "" && strings.EqualFold(m.Attributes[i].ColumnName, name)) ||
			(m.Attributes[i].JSONField != "" && strings.EqualFold(m.Attributes[i].JSONField, name)) {
			return &m.Attributes[i]
		}
	}
	return nil
}

// IsPrimaryKey returns true if the attribute is part of the model's primary key.
func (m *Model) IsPrimaryKey(name string) bool {
	if m.PrimaryKey != nil {
		for _, col := range m.PrimaryKey.Columns {
			if strings.EqualFold(col, name) {
				return true
			}
		}
	}
	for _, attr := range m.Attributes {
		if strings.EqualFold(attr.Name, name) && attr.IsPrimaryKey {
			return true
		}
	}
	return false
}

// GetPrimaryKeyAttributes returns all attributes marked as primary key.
func (m *Model) GetPrimaryKeyAttributes() []Attribute {
	var pks []Attribute
	for _, attr := range m.Attributes {
		if m.IsPrimaryKey(attr.Name) {
			pks = append(pks, attr)
		}
	}
	return pks
}

// ModelConfig defines the configuration of a model entity.
//
// Example Definition:
//
//	cfg := &model.ModelConfig{
//	    ID:          "employee",
//	    Schema:      "public",
//	    Name:        "Employee",
//	    Table:       "employees",
//	    RefName:     "employees",
//	    Description: "Auto-imported enterprise employee entity",
//	    Status:      model.ModelConfigStatusActive,
//	    Version:     1,
//	}
type ModelConfig struct {
	ID                   string            `json:"id"`
	Schema               string            `json:"schema,omitempty"`       // Database schema (e.g. public, tenant_a, hr, sales)
	Name                 string            `json:"name"`                   // Name used by query engine
	Table                string            `json:"table,omitempty"`        // Underlying table name
	RefName              string            `json:"ref_name,omitempty"`      // Optional reference name
	IsTable              bool              `json:"is_table,omitempty"`     // true if mapped to table
	IsAttributeReference bool              `json:"is_attribute_reference"` // true means this model can be used as a model/attribute reference
	Description          string            `json:"description,omitempty"`   // Model description
	Status               ModelConfigStatus `json:"status"`                 // draft, active, inactive, archived
	Version              int               `json:"version"`                // Model configuration version
	IsSystem             bool              `json:"is_system"`              // System-defined model
	CreatedAt            time.Time         `json:"created_at"`             // Creation timestamp
	CreatedBy            string            `json:"created_by"`             // Created By
	UpdatedAt            time.Time         `json:"updated_at"`             // Updated timestamp
	UpdatedBy            string            `json:"updated_by"`             // Updated By
}

// DataModel defines a field/column definition for a dynamic model.
//
// Example Definition:
//
//	field := &model.DataModel{
//	    ID:           "employee_email",
//	    ModelID:      "employee",
//	    ColumnName:   "email",
//	    JSONField:    "email",
//	    DataType:     model.TypeString,
//	    IsNullable:   false,
//	    IsRequired:   true,
//	    IsUnique:     true,
//	    Status:       model.DataModelStatusActive,
//	}
type DataModel struct {
	ID                         string                `json:"id"`                                        // Field ID
	ModelID                    string                `json:"model_id"`                                  // model_config reference (FK)
	ColumnName                 string                `json:"column_name"`                               // Actual DB column/field
	JSONField                  string                `json:"json_field"`                                // API/JSON property
	RefName                    string                `json:"ref_name,omitempty"`                         // Reference display name
	Description                string                `json:"description,omitempty"`                     // Description
	DataType                   DataType              `json:"data_type"`                                 // Logical datatype
	CustomTypeID               *string               `json:"custom_type_id,omitempty"`                 // Reference to model_config (Address struct)
	CustomType                 string                `json:"custom_type,omitempty"`                    // Specific custom type (e.g. geo_point_radius)
	IsArray                    bool                  `json:"is_array"`                                  // Array field
	IsNullable                 bool                  `json:"is_nullable"`                               // NULL allowed
	IsRequired                 bool                  `json:"is_required"`                               // Application mandatory
	IsPrimaryKey               bool                  `json:"is_primary_key"`                            // Primary key
	IsUnique                   bool                  `json:"is_unique"`                                 // Unique
	IsImmutable                bool                  `json:"is_immutable"`                              // Cannot update
	IsGenerated                bool                  `json:"is_generated"`                              // System/DB generated
	DefaultValue               any                   `json:"default_value,omitempty"`                   // Default value
	Min                        *float64              `json:"min,omitempty"`
	Max                        *float64              `json:"max,omitempty"`
	MinLength                  *int                  `json:"min_length,omitempty"`
	MaxLength                  *int                  `json:"max_length,omitempty"`
	Pattern                    string                `json:"pattern,omitempty"`
	Enum                       []any                 `json:"enum,omitempty"`
	Precision                  *int                  `json:"precision,omitempty"`
	Scale                      *int                  `json:"scale,omitempty"`
	Items                      *ItemRule             `json:"items,omitempty"`
	IsOrbitalReference         bool                  `json:"is_orbital_reference"`                      // Field references another model/field
	OrbitalReferenceModelID    *string               `json:"orbital_reference_model_id,omitempty"`      // Referenced model
	OrbitalReferenceFieldID    *string               `json:"orbital_reference_field_id,omitempty"`      // Referenced field
	OrbitalReferenceValidation OrbitalValidationType `json:"orbital_reference_validation,omitempty"` // exists, exists_active, exists_in_scope, not_exists
	Reference                  *OrbitalRefSpec       `json:"reference,omitempty"`                       // Full reference specification
	Status                     DataModelStatus       `json:"status"`                                    // active / inactive
	CreatedAt                  time.Time             `json:"created_at"`                                // Creation
	CreatedBy                  string                `json:"created_by"`                                // Created By
	UpdatedAt                  time.Time             `json:"updated_at"`                                // Updated
	UpdatedBy                  string                `json:"updated_by"`                                // Updated By
}

// ToAttribute converts a DataModel field to an execution Attribute.
func (dm *DataModel) ToAttribute() Attribute {
	name := dm.ColumnName
	if name == "" {
		name = dm.JSONField
	}
	nullable := dm.IsNullable
	if !dm.IsRequired && !dm.IsPrimaryKey {
		nullable = true
	}

	normType := NormalizeDataType(string(dm.DataType))

	isRequired := dm.IsRequired && !dm.IsPrimaryKey
	var ruleSet *RuleSet
	if isRequired || dm.Min != nil || dm.Max != nil || dm.MinLength != nil || dm.MaxLength != nil || dm.Pattern != "" || len(dm.Enum) > 0 || dm.Precision != nil || dm.Scale != nil || dm.Items != nil {
		ruleSet = &RuleSet{
			Required:  isRequired,
			Min:       dm.Min,
			Max:       dm.Max,
			MinLength: dm.MinLength,
			MaxLength: dm.MaxLength,
			Pattern:   dm.Pattern,
			Enum:      dm.Enum,
			Precision: dm.Precision,
			Scale:     dm.Scale,
			Items:     dm.Items,
		}
	}

	ref := dm.Reference
	if ref == nil && dm.IsOrbitalReference && dm.OrbitalReferenceModelID != nil {
		attrName := "id"
		if dm.OrbitalReferenceFieldID != nil && *dm.OrbitalReferenceFieldID != "" {
			attrName = *dm.OrbitalReferenceFieldID
		}
		ref = &OrbitalRefSpec{
			Model:     *dm.OrbitalReferenceModelID,
			Attribute: attrName,
		}
	}

	prec := 0
	if dm.Precision != nil {
		prec = *dm.Precision
	}
	scale := 0
	if dm.Scale != nil {
		scale = *dm.Scale
	}

	return Attribute{
		Name:         name,
		RefName:      dm.RefName,
		ColumnName:   dm.ColumnName,
		JSONField:    dm.JSONField,
		Type:         normType,
		CustomType:   dm.CustomType,
		Precision:    prec,
		Scale:        scale,
		Nullable:     nullable,
		Default:      dm.DefaultValue,
		Unique:       dm.IsUnique,
		IsPrimaryKey: dm.IsPrimaryKey,
		Comment:      dm.Description,
		Validation:   ruleSet,
		Reference:    ref,
	}
}

// BuildModel converts a ModelConfig and a list of DataModel fields into an active execution Model.
func BuildModel(cfg *ModelConfig, fields []*DataModel, database string, storageType StorageType) *Model {
	if cfg == nil {
		return nil
	}
	storageName := cfg.Table
	if storageName == "" {
		storageName = cfg.RefName
		if storageName == "" {
			storageName = cfg.Name
		}
	}
	if cfg.Schema != "" && cfg.Schema != "public" && !strings.Contains(storageName, ".") {
		storageName = fmt.Sprintf("%s.%s", cfg.Schema, storageName)
	}
	if storageType == "" {
		storageType = StorageRelational
	}

	attrs := make([]Attribute, 0, len(fields))
	var pkCols []string
	var relations []Relation
	for _, f := range fields {
		if f != nil && (f.Status == "" || f.Status == DataModelStatusActive) {
			attrs = append(attrs, f.ToAttribute())
			if f.IsPrimaryKey {
				pkName := f.ColumnName
				if pkName == "" {
					pkName = f.JSONField
				}
				pkCols = append(pkCols, pkName)
			}

			if f.IsOrbitalReference && f.OrbitalReferenceModelID != nil && *f.OrbitalReferenceModelID != "" {
				col := f.ColumnName
				if col == "" {
					col = f.JSONField
				}
				targetTable := *f.OrbitalReferenceModelID
				targetCol := "id"
				if f.OrbitalReferenceFieldID != nil && *f.OrbitalReferenceFieldID != "" {
					targetCol = *f.OrbitalReferenceFieldID
				}
				relations = append(relations, Relation{
					Name:        fmt.Sprintf("fk_%s_%s", cfg.ID, col),
					Type:        RelManyToOne,
					TargetModel: targetTable,
					TargetKey:   targetCol,
					ForeignKey:  col,
					OnDelete:    "CASCADE",
					OnUpdate:    "CASCADE",
				})
			} else if f.Reference != nil && f.Reference.Model != "" {
				col := f.ColumnName
				if col == "" {
					col = f.JSONField
				}
				targetTable := f.Reference.Model
				targetCol := f.Reference.Attribute
				if targetCol == "" {
					targetCol = "id"
				}
				onDel := f.Reference.OnDelete
				if onDel == "" {
					onDel = "CASCADE"
				}
				onUpd := f.Reference.OnUpdate
				if onUpd == "" {
					onUpd = "CASCADE"
				}
				relations = append(relations, Relation{
					Name:        fmt.Sprintf("fk_%s_%s", cfg.ID, col),
					Type:        RelManyToOne,
					TargetModel: targetTable,
					TargetKey:   targetCol,
					ForeignKey:  col,
					OnDelete:    onDel,
					OnUpdate:    onUpd,
				})
			}
		}
	}

	modelStatus := StatusDraft
	switch cfg.Status {
	case ModelConfigStatusActive:
		modelStatus = StatusActive
	case ModelConfigStatusArchived:
		modelStatus = StatusDegraded
	default:
		modelStatus = StatusDraft
	}

	var pk *PrimaryKey
	if len(pkCols) > 0 {
		pk = &PrimaryKey{
			Columns: pkCols,
		}
	}

	return &Model{
		ID:          cfg.ID,
		Schema:      cfg.Schema,
		Name:        cfg.Name,
		Table:       cfg.Table,
		StorageName: storageName,
		Database:    database,
		StorageType: storageType,
		Version:     cfg.Version,
		Status:      modelStatus,
		Description: cfg.Description,
		Attributes:  attrs,
		PrimaryKey:  pk,
		Relations:   relations,
		CreatedAt:   cfg.CreatedAt,
		UpdatedAt:   cfg.UpdatedAt,
	}
}

