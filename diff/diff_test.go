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

func TestDiff_UntouchedColumns_NoFalseAlterOperations(t *testing.T) {
	engine := diff.NewDiffEngine()

	// Live Postgres schema:
	// - id: BIGINT with sequence nextval default
	// - door_number: VARCHAR(50) (as introspected from DB)
	// - status: VARCHAR(20) with default 'active'::character varying
	current := &schema.Schema{
		Name: "fleet.vehicle",
		Attributes: []schema.SchemaAttribute{
			{Name: "id", Type: model.TypeLong, PrimaryKey: true, AutoIncrement: true, Default: "nextval('fleet.vehicle_id_seq'::regclass)"},
			{Name: "door_number", Type: model.TypeString, Length: 50, Nullable: true},
			{Name: "status", Type: model.TypeString, Length: 20, Nullable: false, Default: "'active'::character varying"},
		},
		PrimaryKey: &schema.SchemaKey{
			Columns: []string{"id"},
		},
	}

	// Desired model has untouched door_number, id, status, and 1 added column "mileage"
	desired := &schema.Schema{
		Name: "fleet.vehicle",
		Attributes: []schema.SchemaAttribute{
			{Name: "id", Type: model.TypeLong, PrimaryKey: true, AutoIncrement: true},
			{Name: "door_number", Type: model.TypeString, Length: 50, Nullable: true},
			{Name: "status", Type: model.TypeString, Length: 20, Nullable: false, Default: "active"},
			{Name: "mileage", Type: model.TypeInt, Nullable: true},
		},
		PrimaryKey: &schema.SchemaKey{
			Columns: []string{"id"},
		},
	}

	diffRes, err := engine.Compare(current, desired, diff.DiffHints{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Must ONLY produce 1 operation: ADD_COLUMN for mileage.
	// door_number, id, and status MUST NOT generate ALTER operations!
	if len(diffRes.Operations) != 1 {
		for i, op := range diffRes.Operations {
			t.Logf("Op[%d]: Type=%s Object=%s Desc=%s", i, op.Type, op.ObjectName, op.Description)
		}
		t.Fatalf("expected exactly 1 operation (ADD_COLUMN mileage), got %d", len(diffRes.Operations))
	}

	op := diffRes.Operations[0]
	if op.Type != diff.OpAddColumn || op.ObjectName != "mileage" {
		t.Fatalf("expected ADD_COLUMN mileage, got %s on %s", op.Type, op.ObjectName)
	}
}

func TestDiff_DeleteColumn_OnlyDropsSpecifiedColumn(t *testing.T) {
	engine := diff.NewDiffEngine()

	current := &schema.Schema{
		Name: "fleet.vehicle",
		Attributes: []schema.SchemaAttribute{
			{Name: "id", Type: model.TypeLong, PrimaryKey: true, AutoIncrement: true},
			{Name: "door_number", Type: model.TypeString, Length: 50, Nullable: true},
			{Name: "vin", Type: model.TypeString, Length: 50, Nullable: true},
		},
		PrimaryKey: &schema.SchemaKey{Columns: []string{"id"}},
	}

	// Desired model removed door_number
	desired := &schema.Schema{
		Name: "fleet.vehicle",
		Attributes: []schema.SchemaAttribute{
			{Name: "id", Type: model.TypeLong, PrimaryKey: true, AutoIncrement: true},
			{Name: "vin", Type: model.TypeString, Length: 50, Nullable: true},
		},
		PrimaryKey: &schema.SchemaKey{Columns: []string{"id"}},
	}

	diffRes, err := engine.Compare(current, desired, diff.DiffHints{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(diffRes.Operations) != 1 {
		t.Fatalf("expected exactly 1 operation (DROP_COLUMN door_number), got %d", len(diffRes.Operations))
	}
	op := diffRes.Operations[0]
	if op.Type != diff.OpRemoveColumn || op.ObjectName != "door_number" {
		t.Fatalf("expected DROP_COLUMN on door_number, got %s on %s", op.Type, op.ObjectName)
	}
}

func TestDiff_ModifySingleColumn_OnlyAltersSpecifiedColumn(t *testing.T) {
	engine := diff.NewDiffEngine()

	current := &schema.Schema{
		Name: "fleet.vehicle",
		Attributes: []schema.SchemaAttribute{
			{Name: "id", Type: model.TypeLong, PrimaryKey: true},
			{Name: "door_number", Type: model.TypeString, Length: 50, Nullable: true},
			{Name: "color", Type: model.TypeString, Length: 30, Nullable: true},
		},
		PrimaryKey: &schema.SchemaKey{Columns: []string{"id"}},
	}

	// Desired model modified color length from 30 to 100
	desired := &schema.Schema{
		Name: "fleet.vehicle",
		Attributes: []schema.SchemaAttribute{
			{Name: "id", Type: model.TypeLong, PrimaryKey: true},
			{Name: "door_number", Type: model.TypeString, Length: 50, Nullable: true},
			{Name: "color", Type: model.TypeString, Length: 100, Nullable: true},
		},
		PrimaryKey: &schema.SchemaKey{Columns: []string{"id"}},
	}

	diffRes, err := engine.Compare(current, desired, diff.DiffHints{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(diffRes.Operations) != 1 {
		for i, op := range diffRes.Operations {
			t.Logf("Op[%d]: Type=%s Object=%s Desc=%s", i, op.Type, op.ObjectName, op.Description)
		}
		t.Fatalf("expected exactly 1 operation (ALTER_COLUMN_TYPE color), got %d", len(diffRes.Operations))
	}
	op := diffRes.Operations[0]
	if op.Type != diff.OpAlterColumnType || op.ObjectName != "color" {
		t.Fatalf("expected ALTER_COLUMN_TYPE on color, got %s on %s", op.Type, op.ObjectName)
	}
}

