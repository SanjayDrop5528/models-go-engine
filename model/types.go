package model
 
import "strings"

// DataType represents standard database-independent data types.
type DataType string

const (
	TypeString    DataType = "STRING"
	TypeText      DataType = "TEXT"
	TypeInt       DataType = "INT"
	TypeLong      DataType = "LONG"
	TypeFloat     DataType = "FLOAT"
	TypeDecimal   DataType = "DECIMAL"
	TypeBoolean   DataType = "BOOLEAN"
	TypeDateTime  DataType = "DATETIME"
	TypeDate      DataType = "DATE"
	TypeTime      DataType = "TIME"
	TypeJSON      DataType = "JSON"
	TypeUUID      DataType = "UUID"
	TypeBinary    DataType = "BINARY"
	TypeArray     DataType = "ARRAY"
	TypeEnum      DataType = "ENUM"
	TypeEmail     DataType = "EMAIL"
	TypeCustom    DataType = "CUSTOM"
	TypeReference DataType = "REFERENCE"
)

// NormalizeDataType normalizes arbitrary casing and common aliases into canonical DataType.
func NormalizeDataType(t string) DataType {
	switch strings.ToUpper(strings.TrimSpace(t)) {
	case "STRING", "VARCHAR", "CHAR":
		return TypeString
	case "TEXT":
		return TypeText
	case "EMAIL":
		return TypeEmail
	case "INT", "INTEGER", "INT4", "SMALLINT":
		return TypeInt
	case "LONG", "BIGINT", "INT8":
		return TypeLong
	case "FLOAT", "REAL", "FLOAT4", "FLOAT8", "DOUBLE":
		return TypeFloat
	case "DECIMAL", "NUMERIC", "MONEY":
		return TypeDecimal
	case "BOOLEAN", "BOOL":
		return TypeBoolean
	case "DATETIME", "TIMESTAMP", "TIMESTAMPTZ":
		return TypeDateTime
	case "DATE":
		return TypeDate
	case "TIME":
		return TypeTime
	case "JSON", "JSONB", "OBJECT":
		return TypeJSON
	case "UUID":
		return TypeUUID
	case "BINARY", "BYTES", "BLOB":
		return TypeBinary
	case "ARRAY", "LIST":
		return TypeArray
	case "ENUM":
		return TypeEnum
	case "CUSTOM":
		return TypeCustom
	case "REFERENCE":
		return TypeReference
	default:
		return DataType(strings.ToUpper(strings.TrimSpace(t)))
	}
}

// StorageType specifies the underlying storage engine paradigm.
type StorageType string

const (
	StorageRelational StorageType = "RELATIONAL"
	StorageDocument   StorageType = "DOCUMENT"
	StorageKeyValue   StorageType = "KEY_VALUE"
)

// ModelStatus represents the lifecycle state of a dynamic model.
type ModelStatus string

const (
	StatusDraft      ModelStatus = "DRAFT"
	StatusValidating ModelStatus = "VALIDATING"
	StatusApplying   ModelStatus = "APPLYING"
	StatusActive     ModelStatus = "ACTIVE"
	StatusFailed     ModelStatus = "FAILED"
	StatusDegraded   ModelStatus = "DEGRADED"
)

// ModelConfigStatus represents the lifecycle state of a model_config entity.
type ModelConfigStatus string

const (
	ModelConfigStatusDraft    ModelConfigStatus = "draft"
	ModelConfigStatusActive   ModelConfigStatus = "active"
	ModelConfigStatusInactive ModelConfigStatus = "inactive"
	ModelConfigStatusArchived ModelConfigStatus = "archived"
)

// DataModelStatus represents the status of a data_model (field) definition.
type DataModelStatus string

const (
	DataModelStatusActive   DataModelStatus = "active"
	DataModelStatusInactive DataModelStatus = "inactive"
)

// OrbitalValidationType defines the validation strategy for an orbital reference field.
type OrbitalValidationType string

const (
	OrbitalValidationExists        OrbitalValidationType = "exists"
	OrbitalValidationExistsActive  OrbitalValidationType = "exists_active"
	OrbitalValidationExistsInScope OrbitalValidationType = "exists_in_scope"
	OrbitalValidationNotExists     OrbitalValidationType = "not_exists"
)

// IndexType represents supported index types.
type IndexType string

const (
	IndexBTree    IndexType = "BTREE"
	IndexHash     IndexType = "HASH"
	IndexUnique   IndexType = "UNIQUE"
	IndexFullText IndexType = "FULLTEXT"
)

