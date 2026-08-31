package mapping_test

import (
	"github.com/SanjayDrop5528/models-go-engine/mapping"
	"github.com/SanjayDrop5528/models-go-engine/model"
	"strings"
	"testing"
)

func TestGenerateUUID(t *testing.T) {
	uuid1 := mapping.GenerateUUID()
	uuid2 := mapping.GenerateUUID()

	if len(uuid1) != 32 {
		t.Fatalf("expected UUID length 32, got %d (%s)", len(uuid1), uuid1)
	}
	if uuid1 == uuid2 {
		t.Fatalf("expected unique UUIDs, got duplicates: %s", uuid1)
	}
	if strings.Count(uuid1, "-") != 0 {
		t.Fatalf("expected 0 hyphens in UUID, got %s", uuid1)
	}
}

func TestSanitizeInput_AutoGenerateUUIDWhenIDMissing(t *testing.T) {
	m := &model.Model{
		ID:          "employee",
		Name:        "Employee",
		StorageName: "employees",
		StorageType: model.StorageDocument,
		Attributes: []model.Attribute{
			{Name: "id", Type: model.TypeString, Nullable: false},
			{Name: "first_name", Type: model.TypeString, Nullable: false},
			{Name: "salary", Type: model.TypeDecimal, Nullable: true},
		},
		PrimaryKey: &model.PrimaryKey{Columns: []string{"id"}},
	}

	// Case 1: Record without id
	input := map[string]any{
		"first_name": "Alice",
		"salary":     95000.0,
	}

	sanitized, err := mapping.SanitizeInput(m, input)
	if err != nil {
		t.Fatalf("SanitizeInput failed: %v", err)
	}

	idVal, exists := sanitized["id"]
	if !exists {
		t.Fatalf("expected 'id' to be auto-generated in sanitized map")
	}

	idStr, ok := idVal.(string)
	if !ok || len(idStr) != 32 {
		t.Fatalf("expected 'id' to be valid UUID string of length 32, got %v", idVal)
	}

	// Case 2: Record with existing id (should not overwrite)
	inputWithID := map[string]any{
		"id":         "custom_emp_007",
		"first_name": "James",
	}
	sanitized2, err := mapping.SanitizeInput(m, inputWithID)
	if err != nil {
		t.Fatalf("SanitizeInput failed: %v", err)
	}
	if sanitized2["id"] != "custom_emp_007" {
		t.Fatalf("expected custom id preserved, got %v", sanitized2["id"])
	}

	// Case 3: Record with empty string id (should auto-generate UUID)
	inputEmptyID := map[string]any{
		"id":         "",
		"first_name": "Bob",
	}
	sanitized3, err := mapping.SanitizeInput(m, inputEmptyID)
	if err != nil {
		t.Fatalf("SanitizeInput failed: %v", err)
	}
	if sanitized3["id"] == "" || len(sanitized3["id"].(string)) != 32 {
		t.Fatalf("expected empty id to be replaced with UUID, got %v", sanitized3["id"])
	}
}
