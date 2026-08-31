package plan_test

import (
	"github.com/SanjayDrop5528/models-go-engine/diff"
	"github.com/SanjayDrop5528/models-go-engine/model"
	"github.com/SanjayDrop5528/models-go-engine/plan"
	"github.com/SanjayDrop5528/models-go-engine/schema"
	"testing"
)

func TestBuildPlan_OperationOrder(t *testing.T) {
	d := &diff.SchemaDiff{
		Operations: []diff.SchemaOperation{
			{Type: diff.OpRemoveColumn, ObjectName: "old_col", Destructive: true},
			{Type: diff.OpAddIndex, ObjectName: "idx_name"},
			{Type: diff.OpAddColumn, ObjectName: "new_col"},
			{Type: diff.OpCreateTable, ObjectName: "employees", After: &schema.Schema{Name: "employees", StorageType: model.StorageRelational}},
		},
		HasChanges:     true,
		HasDestructive: true,
	}

	p := plan.BuildPlan("emp", "employees", "postgres", d)

	if !p.Destructive {
		t.Fatal("expected plan to be destructive")
	}
	if len(p.Operations) != 4 {
		t.Fatalf("expected 4 operations, got %d", len(p.Operations))
	}

	// CREATE_TABLE must come first
	if p.Operations[0].Type != diff.OpCreateTable {
		t.Fatalf("expected first op to be CREATE_TABLE, got %s", p.Operations[0].Type)
	}
	// ADD_COLUMN before ADD_INDEX
	if p.Operations[1].Type != diff.OpAddColumn {
		t.Fatalf("expected second op to be ADD_COLUMN, got %s", p.Operations[1].Type)
	}
	// ADD_INDEX before REMOVE_COLUMN
	if p.Operations[2].Type != diff.OpAddIndex {
		t.Fatalf("expected third op to be ADD_INDEX, got %s", p.Operations[2].Type)
	}
	// REMOVE_COLUMN near end
	if p.Operations[3].Type != diff.OpRemoveColumn {
		t.Fatalf("expected fourth op to be REMOVE_COLUMN, got %s", p.Operations[3].Type)
	}
}
