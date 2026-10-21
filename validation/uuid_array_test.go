package validation_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/SanjayDrop5528/models-go-engine/model"
	"github.com/SanjayDrop5528/models-go-engine/validation"
)

func TestDataModelUUIDArraysValidateCreateAndUpdate(t *testing.T) {
	var fields []*model.DataModel
	for _, name := range []string{"primary_location", "primary_plant_mine", "secondary_location", "secondary_plant_mine"} {
		fields = append(fields, &model.DataModel{ID: name, ModelID: "employees", ColumnName: name, DataType: "uuid", IsArray: true, IsNullable: true, Status: model.DataModelStatusActive})
	}
	m := model.BuildModel(&model.ModelConfig{ID: "employees", Name: "employees", Table: "employees", Status: model.ModelConfigStatusActive}, fields, "", model.StorageRelational)
	for _, attr := range m.Attributes {
		if attr.Type != model.TypeArray || attr.Validation == nil || attr.Validation.Items == nil || attr.Validation.Items.Type != model.TypeUUID {
			t.Fatalf("array metadata lost: %#v", attr)
		}
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(`{"primary_location":[],"primary_plant_mine":["4478c3cb-d928-4862-b468-316198ef0d1d"],"secondary_location":null,"secondary_plant_mine":["4478c3cb-d928-4862-b468-316198ef0d1d","f07d272b-4d45-47fd-ad82-74727a406f78"]}`), &payload); err != nil {
		t.Fatal(err)
	}
	for _, validate := range []func(*model.Model, map[string]any) error{validation.ValidateData, validation.ValidatePartialData} {
		if err := validate(m, payload); err != nil {
			t.Fatal(err)
		}
		for _, bad := range []any{"4478c3cb-d928-4862-b468-316198ef0d1d", []any{"bad-uuid"}, []any{42}, []any{nil}, []any{map[string]any{"id": "value"}}} {
			if err := validate(m, map[string]any{"primary_location": bad}); err == nil || !strings.Contains(err.Error(), "primary_location") {
				t.Fatalf("expected field error for %#v: %v", bad, err)
			}
		}
	}
	for _, f := range fields {
		if f.Items != nil || f.DataType != "uuid" {
			t.Fatal("conversion mutated caller metadata")
		}
	}
}
