package diff_test

import (
	"github.com/SanjayDrop5528/models-go-engine/diff"
	"github.com/SanjayDrop5528/models-go-engine/model"
	"github.com/SanjayDrop5528/models-go-engine/schema"
	"testing"
)

func TestDiff_CreateTable(t *testing.T) {
	engine := diff.NewDiffEngine()

	desired := &schema.Schema{
		Name:        "employees",
		StorageType: model.StorageRelational,
		Attributes: []schema.SchemaAttribute{
			{Name: "id", Type: model.TypeLong, PrimaryKey: true},
			{Name: "name", Type: model.TypeString, Nullable: false},
		},
	}

	diffRes, err := engine.Compare(nil, desired, diff.DiffHints{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !diffRes.HasChanges {
		t.Fatal("expected has_changes to be true")
	}
	if len(diffRes.Operations) != 1 {
		t.Fatalf("expected 1 operation, got %d", len(diffRes.Operations))
	}
	if diffRes.Operations[0].Type != diff.OpCreateTable {
		t.Fatalf("expected OpCreateTable, got %s", diffRes.Operations[0].Type)
	}
}

func TestDiff_AddColumn_OnlyMinimalChange(t *testing.T) {
	engine := diff.NewDiffEngine()

	// Current DB: id, name, email, age
	current := &schema.Schema{
		Name:        "employees",
		StorageType: model.StorageRelational,
		Attributes: []schema.SchemaAttribute{
			{Name: "id", Type: model.TypeLong, PrimaryKey: true},
			{Name: "name", Type: model.TypeString, Nullable: false},
			{Name: "email", Type: model.TypeString, Nullable: false},
			{Name: "age", Type: model.TypeInt, Nullable: true},
		},
	}

	// Desired Model: id, name, email, age, salary
	desired := &schema.Schema{
		Name:        "employees",
		StorageType: model.StorageRelational,
		Attributes: []schema.SchemaAttribute{
			{Name: "id", Type: model.TypeLong, PrimaryKey: true},
			{Name: "name", Type: model.TypeString, Nullable: false},
			{Name: "email", Type: model.TypeString, Nullable: false},
			{Name: "age", Type: model.TypeInt, Nullable: true},
			{Name: "salary", Type: model.TypeDecimal, Precision: 10, Scale: 2, Nullable: true},
		},
	}

	diffRes, err := engine.Compare(current, desired, diff.DiffHints{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !diffRes.HasChanges {
		t.Fatal("expected changes")
	}
	if diffRes.HasDestructive {
		t.Fatal("adding a nullable column should not be destructive")
	}
	if len(diffRes.Operations) != 1 {
		t.Fatalf("expected exactly 1 operation, got %d", len(diffRes.Operations))
	}

	op := diffRes.Operations[0]
	if op.Type != diff.OpAddColumn {
		t.Fatalf("expected ADD_COLUMN, got %s", op.Type)
	}
	if op.ObjectName != "salary" {
		t.Fatalf("expected column name 'salary', got '%s'", op.ObjectName)
	}
}

func TestDiff_RenameColumn_ExplicitHint(t *testing.T) {
	engine := diff.NewDiffEngine()

	current := &schema.Schema{
		Name: "employees",
		Attributes: []schema.SchemaAttribute{
			{Name: "id", Type: model.TypeLong, PrimaryKey: true},
			{Name: "employee_name", Type: model.TypeString, Nullable: false},
		},
	}

	desired := &schema.Schema{
		Name: "employees",
		Attributes: []schema.SchemaAttribute{
			{Name: "id", Type: model.TypeLong, PrimaryKey: true},
			{Name: "name", Type: model.TypeString, Nullable: false},
		},
	}

	hints := diff.DiffHints{
		RenamedColumns: map[string]string{
			"employee_name": "name",
		},
	}

	diffRes, err := engine.Compare(current, desired, hints)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(diffRes.Operations) != 1 {
		t.Fatalf("expected 1 operation (RENAME_COLUMN), got %d", len(diffRes.Operations))
	}

	op := diffRes.Operations[0]
	if op.Type != diff.OpRenameColumn {
		t.Fatalf("expected RENAME_COLUMN, got %s", op.Type)
	}
	if op.OldName != "employee_name" || op.ObjectName != "name" {
		t.Fatalf("expected rename from employee_name to name, got %s -> %s", op.OldName, op.ObjectName)
	}
}

func TestDiff_RemoveColumn_IsDestructive(t *testing.T) {
	engine := diff.NewDiffEngine()

	current := &schema.Schema{
		Name: "employees",
		Attributes: []schema.SchemaAttribute{
			{Name: "id", Type: model.TypeLong, PrimaryKey: true},
			{Name: "name", Type: model.TypeString},
			{Name: "salary", Type: model.TypeDecimal},
		},
	}

	desired := &schema.Schema{
		Name: "employees",
		Attributes: []schema.SchemaAttribute{
			{Name: "id", Type: model.TypeLong, PrimaryKey: true},
			{Name: "name", Type: model.TypeString},
		},
	}

	diffRes, err := engine.Compare(current, desired, diff.DiffHints{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !diffRes.HasDestructive {
		t.Fatal("expected remove column to be flagged as destructive")
	}

	if len(diffRes.Operations) != 1 {
		t.Fatalf("expected 1 operation, got %d", len(diffRes.Operations))
	}

	op := diffRes.Operations[0]
	if op.Type != diff.OpRemoveColumn || op.ObjectName != "salary" {
		t.Fatalf("expected REMOVE_COLUMN on salary, got %s on %s", op.Type, op.ObjectName)
	}
	if !op.Destructive || op.Safety != diff.SafetyDestructive {
		t.Fatal("operation must have SafetyDestructive")
	}
}

func TestDiff_AlterColumn_TypeAndNullability(t *testing.T) {
	engine := diff.NewDiffEngine()

	current := &schema.Schema{
		Name: "employees",
		Attributes: []schema.SchemaAttribute{
			{Name: "id", Type: model.TypeLong, PrimaryKey: true},
			{Name: "age", Type: model.TypeInt, Nullable: true},
		},
	}

	desired := &schema.Schema{
		Name: "employees",
		Attributes: []schema.SchemaAttribute{
			{Name: "id", Type: model.TypeLong, PrimaryKey: true},
			{Name: "age", Type: model.TypeLong, Nullable: false},
		},
	}

	diffRes, err := engine.Compare(current, desired, diff.DiffHints{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have ALTER_COLUMN_TYPE and ALTER_COLUMN_NULLABLE
	if len(diffRes.Operations) != 2 {
		t.Fatalf("expected 2 operations, got %d", len(diffRes.Operations))
	}
}
