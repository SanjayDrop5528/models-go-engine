package model_test

import (
	"testing"

	"github.com/SanjayDrop5528/models-go-engine/model"
)

func TestBuildModelGeneratesRelationOnlyForOrbitalReference(t *testing.T) {
	targetModel := "organisation"
	targetField := "id"
	cfg := &model.ModelConfig{
		ID:      "employee",
		Name:    "Employee",
		Table:   "employees",
		Status:  model.ModelConfigStatusActive,
		Version: 1,
	}

	m := model.BuildModel(cfg, []*model.DataModel{
		{
			ID:                         "employee_organisation_id",
			ModelID:                    "employee",
			ColumnName:                 "organisation_id",
			JSONField:                  "organisation_id",
			DataType:                   model.TypeUUID,
			IsOrbitalReference:         true,
			OrbitalReferenceModelID:    &targetModel,
			OrbitalReferenceFieldID:    &targetField,
			OrbitalReferenceValidation: model.OrbitalValidationExists,
			Status:                     model.DataModelStatusActive,
		},
		{
			ID:         "employee_manager_id",
			ModelID:    "employee",
			ColumnName: "manager_id",
			JSONField:  "manager_id",
			DataType:   model.TypeUUID,
			Reference: &model.OrbitalRefSpec{
				Model:     "manager",
				Attribute: "id",
			},
			Status: model.DataModelStatusActive,
		},
	}, "testdb", model.StorageRelational)

	if m == nil {
		t.Fatal("expected model")
	}
	if len(m.Relations) != 1 {
		t.Fatalf("expected only orbital reference to generate relation, got %+v", m.Relations)
	}
	rel := m.Relations[0]
	if rel.Name != "Organisation" {
		t.Fatalf("expected relation name Organisation, got %s", rel.Name)
	}
	if rel.ForeignKey != "organisation_id" || rel.TargetModel != "organisation" || rel.TargetKey != "id" {
		t.Fatalf("unexpected relation metadata: %+v", rel)
	}
}

func TestGenerateRelationsFromOrbitalReferences_Scenarios(t *testing.T) {
	orgModel := "organisation"
	deptModel := "company.department"
	emptyModel := ""

	fields := []*model.DataModel{
		// 1. Valid orbital reference
		{
			ID:                      "f1",
			ColumnName:              "organisation_id",
			IsOrbitalReference:      true,
			OrbitalReferenceModelID: &orgModel,
			Status:                  model.DataModelStatusActive,
		},
		// 2. Duplicate target model -> should get stable disambiguated name
		{
			ID:                      "f2",
			ColumnName:              "secondary_org_id",
			IsOrbitalReference:      true,
			OrbitalReferenceModelID: &orgModel,
			Status:                  model.DataModelStatusActive,
		},
		// 3. Schema-qualified target model (company.department -> Department)
		{
			ID:                      "f3",
			ColumnName:              "department_id",
			IsOrbitalReference:      true,
			OrbitalReferenceModelID: &deptModel,
			Status:                  model.DataModelStatusActive,
		},
		// 4. Custom relation alias in Reference
		{
			ID:                 "f4",
			ColumnName:         "team_lead_id",
			IsOrbitalReference: true,
			Reference: &model.OrbitalRefSpec{
				Model:        "employees",
				Attribute:    "emp_id",
				RelationName: "TeamLead",
			},
			Status: model.DataModelStatusActive,
		},
		// 5. Non-orbital field -> should NOT generate relation
		{
			ID:                 "f5",
			ColumnName:         "regular_code",
			IsOrbitalReference: false,
			Status:             model.DataModelStatusActive,
		},
		// 6. Missing target model -> should NOT generate relation
		{
			ID:                      "f6",
			ColumnName:              "ghost_id",
			IsOrbitalReference:      true,
			OrbitalReferenceModelID: &emptyModel,
			Status:                  model.DataModelStatusActive,
		},
	}

	rels := model.GenerateRelationsFromOrbitalReferences(fields, func(target string) string {
		if target == "organisation" {
			return "org_pk"
		}
		return "id"
	})

	if len(rels) != 4 {
		t.Fatalf("expected 4 generated relations, got %d", len(rels))
	}

	// Verify f1 (Organisation) with PK resolved via resolveTargetPK
	if rels[0].Name != "Organisation" || rels[0].ForeignKey != "organisation_id" || rels[0].TargetKey != "org_pk" {
		t.Fatalf("unexpected rel[0]: %+v", rels[0])
	}

	// Verify f2 (Organisation2) duplicate disambiguation
	if rels[1].Name != "Organisation2" || rels[1].ForeignKey != "secondary_org_id" || rels[1].TargetKey != "org_pk" {
		t.Fatalf("unexpected rel[1]: %+v", rels[1])
	}

	// Verify f3 (Department) schema qualification handling
	if rels[2].Name != "Department" || rels[2].TargetModel != "company.department" || rels[2].TargetKey != "id" {
		t.Fatalf("unexpected rel[2]: %+v", rels[2])
	}

	// Verify f4 (TeamLead) custom relation alias
	if rels[3].Name != "TeamLead" || rels[3].TargetModel != "employees" || rels[3].TargetKey != "emp_id" {
		t.Fatalf("unexpected rel[3]: %+v", rels[3])
	}
}
