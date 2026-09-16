package adapter

// StorageCategory identifies the architectural classification of the database.
type StorageCategory string

const (
	StorageCategoryRelational StorageCategory = "RELATIONAL"
	StorageCategoryDocument   StorageCategory = "DOCUMENT"
	StorageCategoryMemory     StorageCategory = "MEMORY"
	StorageCategoryKeyValue   StorageCategory = "KEY_VALUE"
	StorageCategoryColumnar   StorageCategory = "COLUMNAR"
)

// Capabilities declares the feature matrix and execution capabilities supported by an adapter.
type Capabilities struct {
	// Storage engine category (e.g. RELATIONAL, DOCUMENT, MEMORY)
	Category StorageCategory `json:"category"`

	// Native transactional atomicity (Begin / Commit / Rollback)
	SupportsTransactions bool `json:"supports_transactions"`

	// Automated DDL migrations (CREATE TABLE, ALTER TABLE, ADD COLUMN)
	SupportsDDLMigration bool `json:"supports_ddl_migration"`

	// Relational Stored Procedures (CREATE PROCEDURE, CALL sp_...)
	SupportsProcedures bool `json:"supports_procedures"`

	// Stored Functions / UDFs (CREATE FUNCTION, SELECT fn_...)
	SupportsFunctions bool `json:"supports_functions"`

	// Native Aggregation Pipelines (e.g. MongoDB MQL $match, $lookup, $group)
	SupportsAggregationPipeline bool `json:"supports_aggregation_pipeline"`

	// Document or Schema JSON schema validations
	SupportsJSONValidation bool `json:"supports_json_validation"`

	// Custom index management (B-Tree, Hash, Compound, Geospatial)
	SupportsIndexes bool `json:"supports_indexes"`

	// Supported SaveModes in Dataset Studio ("PROCEDURE", "FUNCTION", "QUERY")
	SupportedSaveModes []string `json:"supported_save_modes"`
}

// CapableAdapter is an optional interface implemented by adapters that expose their capability matrix.
type CapableAdapter interface {
	Adapter
	Capabilities() Capabilities
}

// GetCapabilities returns the capabilities of the adapter, falling back to conservative defaults if not implemented.
func GetCapabilities(a Adapter) Capabilities {
	if ca, ok := a.(CapableAdapter); ok {
		return ca.Capabilities()
	}

	// Conservative defaults based on adapter name
	switch a.Name() {
	case "postgres":
		return Capabilities{
			Category:                    StorageCategoryRelational,
			SupportsTransactions:        true,
			SupportsDDLMigration:        true,
			SupportsProcedures:          true,
			SupportsFunctions:           true,
			SupportsAggregationPipeline: false,
			SupportsJSONValidation:      true,
			SupportsIndexes:             true,
			SupportedSaveModes:          []string{"PROCEDURE", "FUNCTION", "QUERY"},
		}
	case "mongodb":
		return Capabilities{
			Category:                    StorageCategoryDocument,
			SupportsTransactions:        true,
			SupportsDDLMigration:        false,
			SupportsProcedures:          false,
			SupportsFunctions:           false,
			SupportsAggregationPipeline: true,
			SupportsJSONValidation:      true,
			SupportsIndexes:             true,
			SupportedSaveModes:          []string{"QUERY"},
		}
	case "memory":
		return Capabilities{
			Category:                    StorageCategoryMemory,
			SupportsTransactions:        false,
			SupportsDDLMigration:        false,
			SupportsProcedures:          false,
			SupportsFunctions:           false,
			SupportsAggregationPipeline: false,
			SupportsJSONValidation:      false,
			SupportsIndexes:             false,
			SupportedSaveModes:          []string{"QUERY"},
		}
	default:
		return Capabilities{
			Category:           StorageCategoryRelational,
			SupportedSaveModes: []string{"QUERY"},
		}
	}
}
