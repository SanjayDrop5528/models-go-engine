package diff

// OperationType specifies the granular atomic schema mutation.
type OperationType string

const (
	OpCreateTable           OperationType = "CREATE_TABLE"
	OpDropTable             OperationType = "DROP_TABLE"
	OpRenameTable           OperationType = "RENAME_TABLE"
	OpAddColumn             OperationType = "ADD_COLUMN"
	OpRemoveColumn          OperationType = "REMOVE_COLUMN"
	OpRenameColumn          OperationType = "RENAME_COLUMN"
	OpAlterColumnType       OperationType = "ALTER_COLUMN_TYPE"
	OpAlterColumnNullable   OperationType = "ALTER_COLUMN_NULLABLE"
	OpAlterColumnDefault    OperationType = "ALTER_COLUMN_DEFAULT"
	OpAddPrimaryKey         OperationType = "ADD_PRIMARY_KEY"
	OpDropPrimaryKey        OperationType = "DROP_PRIMARY_KEY"
	OpAddIndex              OperationType = "ADD_INDEX"
	OpDropIndex             OperationType = "DROP_INDEX"
	OpAddForeignKey         OperationType = "ADD_FOREIGN_KEY"
	OpDropForeignKey        OperationType = "DROP_FOREIGN_KEY"
)

// SafetyLevel classifies the potential risk of a schema modification.
type SafetyLevel string

const (
	SafetySafe              SafetyLevel = "SAFE"
	SafetyPotentiallyUnsafe SafetyLevel = "POTENTIALLY_UNSAFE"
	SafetyDestructive       SafetyLevel = "DESTRUCTIVE"
)

// SchemaOperation describes an individual schema alteration.
type SchemaOperation struct {
	Type        OperationType `json:"type"`
	TargetTable string        `json:"target_table"`
	ObjectName  string        `json:"object_name"`
	OldName     string        `json:"old_name,omitempty"`
	Before      any           `json:"before,omitempty"`
	After       any           `json:"after,omitempty"`
	Safety      SafetyLevel   `json:"safety"`
	Destructive bool          `json:"destructive"`
	Description string        `json:"description"`
}

// DiffHints provides explicit guidance from user/UI for ambiguous diffs (e.g., renames).
type DiffHints struct {
	RenamedColumns map[string]string `json:"renamed_columns,omitempty"` // OldName -> NewName
	RenamedTables  map[string]string `json:"renamed_tables,omitempty"`  // OldName -> NewName
}
