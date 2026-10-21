package service

import (
	"reflect"
	"testing"

	"github.com/SanjayDrop5528/models-go-engine/dataset/domain"
)

func TestProjectDatasetRowsPreservesAliasedRelationshipFields(t *testing.T) {
	ds := &domain.DataSet{
		BaseCollection: domain.BaseCollection{Collection: "employees"},
		SelectedList: []domain.SelectedField{
			{Field: "employees.id", HeaderName: "id"},
			{Field: "primary_plant_mine_alias.name", HeaderName: "secondary_plant_name"},
			{Field: "primary_workplaces_alias.name", HeaderName: "primary_workplace_name"},
			{Field: "designations_details.name", HeaderName: "designations_name"},
			{Field: "secondary_plant_mine_alias.name", HeaderName: "primary_plant_name"},
		},
	}
	rows := []map[string]any{{
		"id":                       "employee-1",
		"primary_plant_mine_alias": []any{map[string]any{"secondary_plant_name": "Plant A", "unselected": "remove"}},
		"primary_workplaces_alias": []map[string]any{{"primary_workplace_name": "Workplace A", "unselected": "remove"}},
		"designations_details":     map[string]any{"designations_name": "Driver", "unselected": "remove"},
		"primary_plant_name":       "Plant B",
		"unselected":               "remove",
	}}
	want := []map[string]any{{
		"id":                       "employee-1",
		"primary_plant_mine_alias": []any{map[string]any{"secondary_plant_name": "Plant A"}},
		"primary_workplaces_alias": []map[string]any{{"primary_workplace_name": "Workplace A"}},
		"designations_details":     map[string]any{"designations_name": "Driver"},
		"primary_plant_name":       "Plant B",
	}}
	if got := projectDatasetRows(rows, ds); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v; want %#v", got, want)
	}
	if rows[0]["unselected"] != "remove" {
		t.Fatal("projection mutated the original rows")
	}
}

func TestProjectDatasetRowsOriginalAndEmptyRelationships(t *testing.T) {
	ds := &domain.DataSet{SelectedList: []domain.SelectedField{{Field: "plants.name", HeaderName: "plant.name"}}}
	for _, tc := range []struct{ input, want any }{
		{nil, nil},
		{[]any{}, []any{}},
		{[]any{map[string]any{"name": "Plant A", "other": true}}, []any{map[string]any{"name": "Plant A"}}},
		{[]any{map[string]any{"plant.name": "Plant A", "other": true}}, []any{map[string]any{"plant.name": "Plant A"}}},
	} {
		got := projectDatasetRows([]map[string]any{{"plants": tc.input}}, ds)
		if !reflect.DeepEqual(got[0]["plants"], tc.want) {
			t.Fatalf("got %#v; want %#v", got, tc.want)
		}
	}
}

func TestProjectDatasetRowsBaseNestedAlias(t *testing.T) {
	ds := &domain.DataSet{BaseCollection: domain.BaseCollection{Collection: "employees"}, SelectedList: []domain.SelectedField{{Field: "employees.address.city", HeaderName: "city_name"}}}
	rows := []map[string]any{{"address": map[string]any{"city_name": "Chennai", "street": "remove"}}}
	want := []map[string]any{{"address": map[string]any{"city_name": "Chennai"}}}
	if got := projectDatasetRows(rows, ds); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v; want %#v", got, want)
	}
}
