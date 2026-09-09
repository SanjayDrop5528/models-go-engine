package validation_test

import (
	"context"
	"testing"

	"github.com/SanjayDrop5528/models-go-engine/model"
	"github.com/SanjayDrop5528/models-go-engine/query"
	"github.com/SanjayDrop5528/models-go-engine/validation"
)

func TestValidationEngine(t *testing.T) {
	ve := validation.NewValidationEngine()

	// Test ValidateModel
	validModel := &model.Model{
		Name:        "users",
		StorageName: "users",
		StorageType: model.StorageRelational,
		PrimaryKey:  &model.PrimaryKey{Columns: []string{"id"}},
		Attributes: []model.Attribute{
			{Name: "id", Type: model.TypeUUID, IsPrimaryKey: true},
			{Name: "email", Type: model.TypeEmail, Validation: &model.RuleSet{Required: true}},
		},
	}

	if err := ve.ValidateModel(validModel); err != nil {
		t.Fatalf("ValidateModel expected success, got: %v", err)
	}

	// Test ValidateData
	validData := map[string]any{
		"id":    "123e4567-e89b-12d3-a456-426614174000",
		"email": "user@example.com",
	}
	if err := ve.ValidateData(validModel, validData); err != nil {
		t.Fatalf("ValidateData expected success, got: %v", err)
	}

	invalidData := map[string]any{
		"id":    "123e4567-e89b-12d3-a456-426614174000",
		"email": "not-an-email",
	}
	if err := ve.ValidateData(validModel, invalidData); err == nil {
		t.Fatalf("ValidateData expected failure on invalid email, got nil")
	}

	// Test ValidateUpdateData
	updateData := map[string]any{
		"email": "updated@example.com",
	}
	if err := ve.ValidateUpdateData(validModel, updateData); err != nil {
		t.Fatalf("ValidateUpdateData expected success, got: %v", err)
	}

	// Test ValidateQuery
	validQ := query.NewQuery().
		Where("email", query.OpEq, "user@example.com").
		OrderBy("id", query.SortAsc)
	if err := ve.ValidateQuery(validModel, validQ); err != nil {
		t.Fatalf("ValidateQuery expected success, got: %v", err)
	}

	invalidQ := query.NewQuery().
		Where("non_existent_column", query.OpEq, 123)
	if err := ve.ValidateQuery(validModel, invalidQ); err == nil {
		t.Fatalf("ValidateQuery expected error on non-existent column, got nil")
	}

	// Test ValidateOrbitalReferences with no adapter (safe no-op)
	if err := ve.ValidateOrbitalReferences(context.Background(), validModel, validData); err != nil {
		t.Fatalf("ValidateOrbitalReferences expected nil, got: %v", err)
	}
}
