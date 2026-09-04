package diff

import (
	"fmt"
	"github.com/SanjayDrop5528/models-go-engine/model"
	"github.com/SanjayDrop5528/models-go-engine/schema"
	"reflect"
	"strings"
)

// SchemaDiff represents the full calculated difference between current and desired schemas.
type SchemaDiff struct {
	CurrentSchema *schema.Schema    `json:"current_schema,omitempty"`
	DesiredSchema *schema.Schema    `json:"desired_schema,omitempty"`
	Operations    []SchemaOperation `json:"operations"`
	HasChanges    bool              `json:"has_changes"`
	HasDestructive bool             `json:"has_destructive"`
}

// DiffEngine compares schemas and generates minimal, safe migration operations.
type DiffEngine struct{}

// NewDiffEngine creates a new DiffEngine instance.
func NewDiffEngine() *DiffEngine {
	return &DiffEngine{}
}

// Compare computes the delta from current database schema to desired model schema.
func (e *DiffEngine) Compare(current *schema.Schema, desired *schema.Schema, hints DiffHints) (*SchemaDiff, error) {
	diffResult := &SchemaDiff{
		CurrentSchema: current,
		DesiredSchema: desired,
		Operations:    make([]SchemaOperation, 0),
	}

	// Case 1: Brand new model/table
	if current == nil || len(current.Attributes) == 0 {
		if desired != nil && len(desired.Attributes) > 0 {
			diffResult.Operations = append(diffResult.Operations, SchemaOperation{
				Type:        OpCreateTable,
				TargetTable: desired.Name,
				ObjectName:  desired.Name,
				After:       desired,
				Safety:      SafetySafe,
				Destructive: false,
				Description: fmt.Sprintf("Create table '%s' with %d columns", desired.Name, len(desired.Attributes)),
			})

			// Add any explicit indexes defined
			for _, idx := range desired.Indexes {
				diffResult.Operations = append(diffResult.Operations, SchemaOperation{
					Type:        OpAddIndex,
					TargetTable: desired.Name,
					ObjectName:  idx.Name,
					After:       idx,
					Safety:      SafetySafe,
					Destructive: false,
					Description: fmt.Sprintf("Create index '%s' on (%s)", idx.Name, strings.Join(idx.Columns, ", ")),
				})
			}

			diffResult.HasChanges = true
			return diffResult, nil
		}
		return diffResult, nil
	}

	// Case 2: Desired schema is empty / deleted
	if desired == nil || len(desired.Attributes) == 0 {
		diffResult.Operations = append(diffResult.Operations, SchemaOperation{
			Type:        OpDropTable,
			TargetTable: current.Name,
			ObjectName:  current.Name,
			Before:      current,
			Safety:      SafetyDestructive,
			Destructive: true,
			Description: fmt.Sprintf("Drop table '%s' permanently", current.Name),
		})
		diffResult.HasChanges = true
		diffResult.HasDestructive = true
		return diffResult, nil
	}

	targetTable := desired.Name

	// Map current attributes and desired attributes for quick lookup
	currentAttrs := make(map[string]schema.SchemaAttribute)
	for _, attr := range current.Attributes {
		currentAttrs[attr.Name] = attr
	}

	desiredAttrs := make(map[string]schema.SchemaAttribute)
	for _, attr := range desired.Attributes {
		desiredAttrs[attr.Name] = attr
	}

	// Track processed attributes
	processedCurrent := make(map[string]bool)
	processedDesired := make(map[string]bool)

	// Step 1: Process explicit column renames from hints
	if hints.RenamedColumns != nil {
		for oldName, newName := range hints.RenamedColumns {
			oldAttr, oldExists := currentAttrs[oldName]
			newAttr, newExists := desiredAttrs[newName]
			if oldExists && newExists {
				diffResult.Operations = append(diffResult.Operations, SchemaOperation{
					Type:        OpRenameColumn,
					TargetTable: targetTable,
					ObjectName:  newName,
					OldName:     oldName,
					Before:      oldAttr,
					After:       newAttr,
					Safety:      SafetyPotentiallyUnsafe,
					Destructive: false,
					Description: fmt.Sprintf("Rename column '%s' to '%s'", oldName, newName),
				})
				processedCurrent[oldName] = true
				processedDesired[newName] = true

				// Check if properties of renamed column also changed
				e.checkAttributeModifications(targetTable, oldAttr, newAttr, diffResult)
			}
		}
	}

	// Step 2: Identify Added & Modified columns
	for _, newAttr := range desired.Attributes {
		if processedDesired[newAttr.Name] {
			continue
		}

		oldAttr, exists := currentAttrs[newAttr.Name]
		if !exists {
			// Added Column
			safety := SafetySafe
			if !newAttr.Nullable && newAttr.Default == nil && !newAttr.AutoIncrement {
				safety = SafetyPotentiallyUnsafe
			}

			diffResult.Operations = append(diffResult.Operations, SchemaOperation{
				Type:        OpAddColumn,
				TargetTable: targetTable,
				ObjectName:  newAttr.Name,
				After:       newAttr,
				Safety:      safety,
				Destructive: false,
				Description: fmt.Sprintf("Add column '%s' (%s)", newAttr.Name, newAttr.Type),
			})
		} else {
			// Existing column - check for modifications
			e.checkAttributeModifications(targetTable, oldAttr, newAttr, diffResult)
			processedCurrent[oldAttr.Name] = true
		}
		processedDesired[newAttr.Name] = true
	}

	// Step 3: Identify Removed columns
	for _, oldAttr := range current.Attributes {
		if processedCurrent[oldAttr.Name] {
			continue
		}

		diffResult.Operations = append(diffResult.Operations, SchemaOperation{
			Type:        OpRemoveColumn,
			TargetTable: targetTable,
			ObjectName:  oldAttr.Name,
			Before:      oldAttr,
			Safety:      SafetyDestructive,
			Destructive: true,
			Description: fmt.Sprintf("Drop column '%s' (%s) - Destructive operation", oldAttr.Name, oldAttr.Type),
		})
		diffResult.HasDestructive = true
	}

	// Step 4: Primary Key comparison
	e.diffPrimaryKeys(targetTable, current.PrimaryKey, desired.PrimaryKey, diffResult)

	// Step 5: Index comparison
	e.diffIndexes(targetTable, current.Indexes, desired.Indexes, diffResult)

	// Step 6: Foreign Key / Relation comparison
	e.diffRelations(targetTable, current.Relations, desired.Relations, diffResult)

	diffResult.HasChanges = len(diffResult.Operations) > 0
	for _, op := range diffResult.Operations {
		if op.Destructive {
			diffResult.HasDestructive = true
			break
		}
	}

	return diffResult, nil
}

func (e *DiffEngine) checkAttributeModifications(targetTable string, oldAttr schema.SchemaAttribute, newAttr schema.SchemaAttribute, diff *SchemaDiff) {
	// 1. Data Type change check
	typeChanged := oldAttr.Type != newAttr.Type

	lengthChanged := false
	if newAttr.Type == model.TypeString || oldAttr.Type == model.TypeString {
		oldLen := oldAttr.Length
		if oldLen == 0 {
			oldLen = 255
		}
		newLen := newAttr.Length
		if newLen == 0 {
			newLen = 255
		}
		lengthChanged = oldLen != newLen
	} else if oldAttr.Length != newAttr.Length {
		lengthChanged = true
	}

	precScaleChanged := false
	if newAttr.Type == model.TypeDecimal || oldAttr.Type == model.TypeDecimal {
		if oldAttr.Precision != newAttr.Precision || oldAttr.Scale != newAttr.Scale {
			if newAttr.Precision > 0 || oldAttr.Precision > 0 {
				precScaleChanged = true
			}
		}
	}

	if typeChanged || lengthChanged || precScaleChanged {
		safety := SafetyPotentiallyUnsafe
		if isNarrowingConversion(oldAttr.Type, newAttr.Type) {
			safety = SafetyDestructive
			diff.HasDestructive = true
		}

		diff.Operations = append(diff.Operations, SchemaOperation{
			Type:        OpAlterColumnType,
			TargetTable: targetTable,
			ObjectName:  newAttr.Name,
			Before:      oldAttr,
			After:       newAttr,
			Safety:      safety,
			Destructive: safety == SafetyDestructive,
			Description: fmt.Sprintf("Change column '%s' type from %s to %s", newAttr.Name, oldAttr.Type, newAttr.Type),
		})
	}

	// 2. Nullability change
	if oldAttr.Nullable != newAttr.Nullable {
		safety := SafetySafe
		if !newAttr.Nullable {
			safety = SafetyPotentiallyUnsafe
		}

		diff.Operations = append(diff.Operations, SchemaOperation{
			Type:        OpAlterColumnNullable,
			TargetTable: targetTable,
			ObjectName:  newAttr.Name,
			Before:      oldAttr.Nullable,
			After:       newAttr.Nullable,
			Safety:      safety,
			Destructive: false,
			Description: fmt.Sprintf("Change column '%s' nullable to %v", newAttr.Name, newAttr.Nullable),
		})
	}

	// 3. Default value change
	if !reflect.DeepEqual(oldAttr.Default, newAttr.Default) {
		diff.Operations = append(diff.Operations, SchemaOperation{
			Type:        OpAlterColumnDefault,
			TargetTable: targetTable,
			ObjectName:  newAttr.Name,
			Before:      oldAttr.Default,
			After:       newAttr.Default,
			Safety:      SafetySafe,
			Destructive: false,
			Description: fmt.Sprintf("Change column '%s' default value to '%v'", newAttr.Name, newAttr.Default),
		})
	}
}

func (e *DiffEngine) diffPrimaryKeys(targetTable string, currentPK *schema.SchemaKey, desiredPK *schema.SchemaKey, diff *SchemaDiff) {
	currentCols := []string{}
	if currentPK != nil {
		currentCols = currentPK.Columns
	}
	desiredCols := []string{}
	if desiredPK != nil {
		desiredCols = desiredPK.Columns
	}

	if !reflect.DeepEqual(currentCols, desiredCols) {
		if len(currentCols) > 0 {
			diff.Operations = append(diff.Operations, SchemaOperation{
				Type:        OpDropPrimaryKey,
				TargetTable: targetTable,
				ObjectName:  "PRIMARY",
				Before:      currentPK,
				Safety:      SafetyPotentiallyUnsafe,
				Destructive: false,
				Description: fmt.Sprintf("Drop primary key on (%s)", strings.Join(currentCols, ", ")),
			})
		}
		if len(desiredCols) > 0 {
			diff.Operations = append(diff.Operations, SchemaOperation{
				Type:        OpAddPrimaryKey,
				TargetTable: targetTable,
				ObjectName:  "PRIMARY",
				After:       desiredPK,
				Safety:      SafetyPotentiallyUnsafe,
				Destructive: false,
				Description: fmt.Sprintf("Add primary key on (%s)", strings.Join(desiredCols, ", ")),
			})
		}
	}
}

func (e *DiffEngine) diffIndexes(targetTable string, currentIndexes []schema.SchemaIndex, desiredIndexes []schema.SchemaIndex, diff *SchemaDiff) {
	currMap := make(map[string]schema.SchemaIndex)
	for _, idx := range currentIndexes {
		currMap[idx.Name] = idx
	}

	desMap := make(map[string]schema.SchemaIndex)
	for _, idx := range desiredIndexes {
		desMap[idx.Name] = idx
	}

	// New or modified indexes
	for _, desIdx := range desiredIndexes {
		currIdx, exists := currMap[desIdx.Name]
		if !exists {
			diff.Operations = append(diff.Operations, SchemaOperation{
				Type:        OpAddIndex,
				TargetTable: targetTable,
				ObjectName:  desIdx.Name,
				After:       desIdx,
				Safety:      SafetySafe,
				Destructive: false,
				Description: fmt.Sprintf("Create index '%s' on (%s)", desIdx.Name, strings.Join(desIdx.Columns, ", ")),
			})
		} else if !reflect.DeepEqual(currIdx.Columns, desIdx.Columns) || currIdx.Unique != desIdx.Unique {
			// Modified index -> drop & recreate
			diff.Operations = append(diff.Operations, SchemaOperation{
				Type:        OpDropIndex,
				TargetTable: targetTable,
				ObjectName:  currIdx.Name,
				Before:      currIdx,
				Safety:      SafetySafe,
				Destructive: false,
				Description: fmt.Sprintf("Drop index '%s'", currIdx.Name),
			})
			diff.Operations = append(diff.Operations, SchemaOperation{
				Type:        OpAddIndex,
				TargetTable: targetTable,
				ObjectName:  desIdx.Name,
				After:       desIdx,
				Safety:      SafetySafe,
				Destructive: false,
				Description: fmt.Sprintf("Recreate index '%s' on (%s)", desIdx.Name, strings.Join(desIdx.Columns, ", ")),
			})
		}
	}

	// Dropped indexes
	for _, currIdx := range currentIndexes {
		if _, exists := desMap[currIdx.Name]; !exists {
			diff.Operations = append(diff.Operations, SchemaOperation{
				Type:        OpDropIndex,
				TargetTable: targetTable,
				ObjectName:  currIdx.Name,
				Before:      currIdx,
				Safety:      SafetySafe,
				Destructive: false,
				Description: fmt.Sprintf("Drop index '%s'", currIdx.Name),
			})
		}
	}
}

func (e *DiffEngine) diffRelations(targetTable string, currentRels []schema.SchemaRelation, desiredRels []schema.SchemaRelation, diff *SchemaDiff) {
	currMap := make(map[string]schema.SchemaRelation)
	for _, r := range currentRels {
		currMap[r.Name] = r
	}

	desMap := make(map[string]schema.SchemaRelation)
	for _, r := range desiredRels {
		desMap[r.Name] = r
	}

	// Added relations
	for _, desRel := range desiredRels {
		if _, exists := currMap[desRel.Name]; !exists {
			diff.Operations = append(diff.Operations, SchemaOperation{
				Type:        OpAddForeignKey,
				TargetTable: targetTable,
				ObjectName:  desRel.Name,
				After:       desRel,
				Safety:      SafetyPotentiallyUnsafe,
				Destructive: false,
				Description: fmt.Sprintf("Add foreign key '%s' (%s -> %s.%s)", desRel.Name, desRel.Column, desRel.ForeignTable, desRel.ForeignColumn),
			})
		}
	}

	// Dropped relations
	for _, currRel := range currentRels {
		if _, exists := desMap[currRel.Name]; !exists {
			diff.Operations = append(diff.Operations, SchemaOperation{
				Type:        OpDropForeignKey,
				TargetTable: targetTable,
				ObjectName:  currRel.Name,
				Before:      currRel,
				Safety:      SafetySafe,
				Destructive: false,
				Description: fmt.Sprintf("Drop foreign key '%s'", currRel.Name),
			})
		}
	}
}

func isNarrowingConversion(from, to model.DataType) bool {
	if (from == model.TypeLong || from == model.TypeDecimal || from == model.TypeFloat) && (to == model.TypeInt || to == model.TypeBoolean) {
		return true
	}
	if (from == model.TypeText || from == model.TypeJSON) && to == model.TypeString {
		return true
	}
	return false
}
