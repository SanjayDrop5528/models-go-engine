package plan

import (
	"fmt"
	"github.com/SanjayDrop5528/models-go-engine/diff"
	"sort"
	"time"
)

// SchemaPlan defines an ordered execution plan for applying schema mutations.
type SchemaPlan struct {
	ModelID        string                 `json:"model_id"`
	StorageName    string                 `json:"storage_name"`
	Database       string                 `json:"database"`
	Operations     []diff.SchemaOperation `json:"operations"`
	Destructive    bool                   `json:"destructive"`
	Warnings       []string               `json:"warnings,omitempty"`
	GeneratedAt    time.Time              `json:"generated_at"`
}

// NativeAction describes an adapter-specific command (e.g., SQL DDL, Mongo command).
type NativeAction struct {
	Type        string `json:"type"`        // SQL, BSON, COMMAND
	Description string `json:"description"`
	Statement   string `json:"statement"`
	Destructive bool   `json:"destructive"`
}

// SchemaPreview contains everything needed for the UI to display and confirm a migration.
type SchemaPreview struct {
	ModelID              string                 `json:"model_id"`
	StorageName          string                 `json:"storage_name"`
	Database             string                 `json:"database"`
	Changes              []diff.SchemaOperation `json:"changes"`
	NativeActions        []NativeAction         `json:"native_actions"`
	HasDestructive       bool                   `json:"has_destructive"`
	RequiresConfirmation bool                   `json:"requires_confirmation"`
	Warnings             []string               `json:"warnings,omitempty"`
	Status               string                 `json:"status"`
}

// BuildPlan takes a diff and orders operations for safe, dependency-aware execution.
func BuildPlan(modelID, storageName, database string, d *diff.SchemaDiff) *SchemaPlan {
	if d == nil || len(d.Operations) == 0 {
		return &SchemaPlan{
			ModelID:     modelID,
			StorageName: storageName,
			Database:    database,
			Operations:  nil,
			Destructive: false,
			GeneratedAt: time.Now().UTC(),
		}
	}

	ordered := make([]diff.SchemaOperation, len(d.Operations))
	copy(ordered, d.Operations)

	// Sort operations by safe dependency order:
	// 1. Table creation/renames
	// 2. Column additions & renames
	// 3. Column alterations
	// 4. PK & Foreign key changes
	// 5. Index additions
	// 6. Index drops
	// 7. Column removals
	// 8. Table drops
	sort.SliceStable(ordered, func(i, j int) bool {
		return getOperationPriority(ordered[i].Type) < getOperationPriority(ordered[j].Type)
	})

	var warnings []string
	hasDestructive := false

	for _, op := range ordered {
		if op.Destructive {
			hasDestructive = true
			warnings = append(warnings, fmt.Sprintf("Destructive operation '%s' on '%s': %s", op.Type, op.ObjectName, op.Description))
		} else if op.Safety == diff.SafetyPotentiallyUnsafe {
			warnings = append(warnings, fmt.Sprintf("Potentially unsafe operation '%s' on '%s': %s", op.Type, op.ObjectName, op.Description))
		}
	}

	return &SchemaPlan{
		ModelID:     modelID,
		StorageName: storageName,
		Database:    database,
		Operations:  ordered,
		Destructive: hasDestructive,
		Warnings:    warnings,
		GeneratedAt: time.Now().UTC(),
	}
}

func getOperationPriority(op diff.OperationType) int {
	switch op {
	case diff.OpCreateTable:
		return 10
	case diff.OpRenameTable:
		return 20
	case diff.OpRenameColumn:
		return 30
	case diff.OpAddColumn:
		return 40
	case diff.OpAlterColumnType, diff.OpAlterColumnNullable, diff.OpAlterColumnDefault:
		return 50
	case diff.OpDropPrimaryKey:
		return 60
	case diff.OpAddPrimaryKey:
		return 70
	case diff.OpDropIndex:
		return 80
	case diff.OpAddIndex:
		return 90
	case diff.OpDropForeignKey:
		return 100
	case diff.OpAddForeignKey:
		return 110
	case diff.OpRemoveColumn:
		return 120
	case diff.OpDropTable:
		return 130
	default:
		return 1000
	}
}
