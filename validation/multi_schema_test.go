package validation_test

import (
	"context"
	"github.com/SanjayDrop5528/models-go-engine/adapter"
	"github.com/SanjayDrop5528/models-go-engine/mapping"
	"github.com/SanjayDrop5528/models-go-engine/model"
	"github.com/SanjayDrop5528/models-go-engine/project"
	"github.com/SanjayDrop5528/models-go-engine/validation"
	"strings"
	"testing"
)

// Helper function to build float pointer
func fPtr(f float64) *float64 { return &f }

// Helper function to build int pointer
func iPtr(i int) *int { return &i }

func setupEnterpriseMultiSchemaEngine(t *testing.T) (*project.Engine, map[string]*model.Model) {
	mockAdapter := adapter.NewMockAdapter()

	proj, err := project.NewProject(project.ProjectConfig{
		ID:   "enterprise_multi_schema",
		Name: "Enterprise Multi-Schema System",
		AdapterConfig: project.AdapterConfig{
			AdapterType: "memory",
			Database:    "enterprise_db",
		},
	}, mockAdapter)
	if err != nil {
		t.Fatalf("failed to create project: %v", err)
	}

	engine := proj.Engine

	// 1. company.organization
	orgModel := &model.Model{
		ID:          "organization",
		Schema:      "company",
		Name:        "organization",
		Table:       "organizations",
		StorageName: "organizations",
		Status:      model.StatusActive,
		PrimaryKey:  &model.PrimaryKey{Columns: []string{"id"}},
		Attributes: []model.Attribute{
			{Name: "id", Type: model.TypeUUID, Nullable: false},
			{Name: "code", Type: model.TypeString, Nullable: false, Unique: true, Validation: &model.RuleSet{Required: true, MinLength: iPtr(3), MaxLength: iPtr(20), Pattern: `^[A-Z0-9_-]+$`}},
			{Name: "name", Type: model.TypeString, Nullable: false, Validation: &model.RuleSet{Required: true, MinLength: iPtr(3), MaxLength: iPtr(150)}},
			{Name: "status", Type: model.TypeEnum, Nullable: false, Validation: &model.RuleSet{Required: true, Enum: []any{"draft", "active", "inactive", "archived"}}},
			{Name: "employee_count", Type: model.TypeInt, Nullable: true, Validation: &model.RuleSet{Min: fPtr(0), Max: fPtr(1000000)}},
			{Name: "annual_budget", Type: model.TypeDecimal, Precision: 18, Scale: 2, Nullable: true, Validation: &model.RuleSet{Min: fPtr(0), Precision: iPtr(18), Scale: iPtr(2)}},
			{Name: "settings", Type: model.TypeJSON, Nullable: true},
			{Name: "tags", Type: model.TypeArray, Nullable: true, Validation: &model.RuleSet{Items: &model.ItemRule{Type: model.TypeString, MaxLength: iPtr(50)}}},
			{Name: "created_at", Type: model.TypeDateTime, Nullable: false, Validation: &model.RuleSet{Required: true}},
		},
	}

	// 2. company.department (with orbital reference to organization and self reference parent_department_id)
	deptModel := &model.Model{
		ID:          "department",
		Schema:      "company",
		Name:        "department",
		Table:       "departments",
		StorageName: "departments",
		Status:      model.StatusActive,
		PrimaryKey:  &model.PrimaryKey{Columns: []string{"id"}},
		Attributes: []model.Attribute{
			{Name: "id", Type: model.TypeUUID, Nullable: false},
			{Name: "organization_id", Type: model.TypeReference, Nullable: false, Reference: &model.OrbitalRefSpec{Schema: "company", Model: "organization", Attribute: "id"}},
			{Name: "code", Type: model.TypeString, Nullable: false, Unique: true, Validation: &model.RuleSet{Required: true, Pattern: `^[A-Z]{2,10}$`}},
			{Name: "name", Type: model.TypeString, Nullable: false, Validation: &model.RuleSet{Required: true, MinLength: iPtr(2), MaxLength: iPtr(100)}},
			{Name: "parent_department_id", Type: model.TypeReference, Nullable: true, Reference: &model.OrbitalRefSpec{Schema: "company", Model: "department", Attribute: "id"}},
			{Name: "department_type", Type: model.TypeEnum, Nullable: false, Validation: &model.RuleSet{Required: true, Enum: []any{"engineering", "finance", "hr", "operations", "sales", "management"}}},
		},
	}

	// 3. company.location
	locModel := &model.Model{
		ID:          "location",
		Schema:      "company",
		Name:        "location",
		Table:       "locations",
		StorageName: "locations",
		Status:      model.StatusActive,
		PrimaryKey:  &model.PrimaryKey{Columns: []string{"id"}},
		Attributes: []model.Attribute{
			{Name: "id", Type: model.TypeUUID, Nullable: false},
			{Name: "organization_id", Type: model.TypeReference, Nullable: false, Reference: &model.OrbitalRefSpec{Schema: "company", Model: "organization", Attribute: "id"}},
			{Name: "code", Type: model.TypeString, Nullable: false, Unique: true, Validation: &model.RuleSet{Required: true, Pattern: `^[A-Z]{3}-[0-9]{3}$`}},
			{Name: "name", Type: model.TypeString, Nullable: false, Validation: &model.RuleSet{Required: true, MaxLength: iPtr(100)}},
			{Name: "latitude", Type: model.TypeDecimal, Precision: 10, Scale: 7, Nullable: false, Validation: &model.RuleSet{Required: true, Min: fPtr(-90), Max: fPtr(90)}},
			{Name: "longitude", Type: model.TypeDecimal, Precision: 10, Scale: 7, Nullable: false, Validation: &model.RuleSet{Required: true, Min: fPtr(-180), Max: fPtr(180)}},
			{Name: "radius_meters", Type: model.TypeDecimal, Precision: 10, Scale: 2, Nullable: false, Validation: &model.RuleSet{Required: true, Min: fPtr(1), Max: fPtr(100000)}},
			{Name: "is_active", Type: model.TypeBoolean, Nullable: false, Validation: &model.RuleSet{Required: true}},
		},
	}

	// 4. hr.employee (cross-schema orbital references to company.organization and company.department, manager self-reference)
	empModel := &model.Model{
		ID:          "employee",
		Schema:      "hr",
		Name:        "employee",
		Table:       "employees",
		StorageName: "employees",
		Status:      model.StatusActive,
		PrimaryKey:  &model.PrimaryKey{Columns: []string{"id"}},
		Attributes: []model.Attribute{
			{Name: "id", Type: model.TypeUUID, Nullable: false},
			{Name: "employee_code", Type: model.TypeString, Nullable: false, Unique: true, Validation: &model.RuleSet{Required: true, MinLength: iPtr(6), MaxLength: iPtr(20), Pattern: `^EMP-[0-9]{6}$`}},
			{Name: "organization_id", Type: model.TypeReference, Nullable: false, Reference: &model.OrbitalRefSpec{Schema: "company", Model: "organization", Attribute: "id"}},
			{Name: "department_id", Type: model.TypeReference, Nullable: false, Reference: &model.OrbitalRefSpec{Schema: "company", Model: "department", Attribute: "id"}},
			{Name: "manager_id", Type: model.TypeReference, Nullable: true, Reference: &model.OrbitalRefSpec{Schema: "hr", Model: "employee", Attribute: "id"}},
			{Name: "first_name", Type: model.TypeString, Nullable: false, Validation: &model.RuleSet{Required: true, MinLength: iPtr(2), MaxLength: iPtr(50), Pattern: `^[A-Za-z .'-]+$`}},
			{Name: "last_name", Type: model.TypeString, Nullable: false, Validation: &model.RuleSet{Required: true, MinLength: iPtr(2), MaxLength: iPtr(50), Pattern: `^[A-Za-z .'-]+$`}},
			{Name: "email", Type: model.TypeEmail, Nullable: false, Unique: true, Validation: &model.RuleSet{Required: true, MaxLength: iPtr(150)}},
			{Name: "phone", Type: model.TypeString, Nullable: true, Validation: &model.RuleSet{Pattern: `^\+?[1-9][0-9]{7,14}$`}},
			{Name: "date_of_birth", Type: model.TypeDate, Nullable: true},
			{Name: "joining_date", Type: model.TypeDate, Nullable: false, Validation: &model.RuleSet{Required: true}},
			{Name: "employment_type", Type: model.TypeEnum, Nullable: false, Validation: &model.RuleSet{Required: true, Enum: []any{"full_time", "part_time", "contract", "intern"}}},
			{Name: "experience_years", Type: model.TypeDecimal, Precision: 5, Scale: 2, Nullable: false, Validation: &model.RuleSet{Required: true, Min: fPtr(0), Max: fPtr(60)}},
			{Name: "is_active", Type: model.TypeBoolean, Nullable: false, Validation: &model.RuleSet{Required: true}},
			{Name: "skills", Type: model.TypeArray, Nullable: true, Validation: &model.RuleSet{Items: &model.ItemRule{Type: model.TypeString, MinLength: iPtr(2), MaxLength: iPtr(50)}}},
			{Name: "metadata", Type: model.TypeJSON, Nullable: true},
			{Name: "created_at", Type: model.TypeDateTime, Nullable: false, Validation: &model.RuleSet{Required: true}},
		},
	}

	// 5. projects.project (cross-schema to company.organization and hr.employee)
	projModel := &model.Model{
		ID:          "project",
		Schema:      "projects",
		Name:        "project",
		Table:       "projects",
		StorageName: "projects",
		Status:      model.StatusActive,
		PrimaryKey:  &model.PrimaryKey{Columns: []string{"id"}},
		Attributes: []model.Attribute{
			{Name: "id", Type: model.TypeUUID, Nullable: false},
			{Name: "organization_id", Type: model.TypeReference, Nullable: false, Reference: &model.OrbitalRefSpec{Schema: "company", Model: "organization", Attribute: "id"}},
			{Name: "code", Type: model.TypeString, Nullable: false, Unique: true, Validation: &model.RuleSet{Required: true, Pattern: `^PRJ-[0-9]{4}$`}},
			{Name: "name", Type: model.TypeString, Nullable: false, Validation: &model.RuleSet{Required: true, MinLength: iPtr(3), MaxLength: iPtr(150)}},
			{Name: "project_type", Type: model.TypeEnum, Nullable: false, Validation: &model.RuleSet{Required: true, Enum: []any{"internal", "client", "research", "maintenance"}}},
			{Name: "budget", Type: model.TypeDecimal, Precision: 18, Scale: 2, Nullable: false, Validation: &model.RuleSet{Required: true, Min: fPtr(0)}},
			{Name: "start_date", Type: model.TypeDate, Nullable: false, Validation: &model.RuleSet{Required: true}},
			{Name: "project_manager_id", Type: model.TypeReference, Nullable: false, Reference: &model.OrbitalRefSpec{Schema: "hr", Model: "employee", Attribute: "id"}},
			{Name: "configuration", Type: model.TypeJSON, Nullable: true},
		},
	}

	// 6. operations.work_site (custom type geo_point_radius, references company.organization and company.location)
	workSiteModel := &model.Model{
		ID:          "work_site",
		Schema:      "operations",
		Name:        "work_site",
		Table:       "work_sites",
		StorageName: "work_sites",
		Status:      model.StatusActive,
		PrimaryKey:  &model.PrimaryKey{Columns: []string{"id"}},
		Attributes: []model.Attribute{
			{Name: "id", Type: model.TypeUUID, Nullable: false},
			{Name: "organization_id", Type: model.TypeReference, Nullable: false, Reference: &model.OrbitalRefSpec{Schema: "company", Model: "organization", Attribute: "id"}},
			{Name: "location_id", Type: model.TypeReference, Nullable: false, Reference: &model.OrbitalRefSpec{Schema: "company", Model: "location", Attribute: "id"}},
			{Name: "name", Type: model.TypeString, Nullable: false, Validation: &model.RuleSet{Required: true, MaxLength: iPtr(150)}},
			{Name: "geofence", Type: model.TypeCustom, CustomType: "geo_point_radius", Nullable: false, Validation: &model.RuleSet{Required: true}},
			{Name: "status", Type: model.TypeEnum, Nullable: false, Validation: &model.RuleSet{Required: true, Enum: []any{"draft", "active", "inactive"}}},
		},
	}

	// 7. operations.attendance (multi-hop reference to hr.employee, operations.work_site, projects.project, custom type geo_point)
	attModel := &model.Model{
		ID:          "attendance",
		Schema:      "operations",
		Name:        "attendance",
		Table:       "attendance",
		StorageName: "attendance",
		Status:      model.StatusActive,
		PrimaryKey:  &model.PrimaryKey{Columns: []string{"id"}},
		Attributes: []model.Attribute{
			{Name: "id", Type: model.TypeUUID, Nullable: false},
			{Name: "employee_id", Type: model.TypeReference, Nullable: false, Reference: &model.OrbitalRefSpec{Schema: "hr", Model: "employee", Attribute: "id"}},
			{Name: "work_site_id", Type: model.TypeReference, Nullable: false, Reference: &model.OrbitalRefSpec{Schema: "operations", Model: "work_site", Attribute: "id"}},
			{Name: "project_id", Type: model.TypeReference, Nullable: true, Reference: &model.OrbitalRefSpec{Schema: "projects", Model: "project", Attribute: "id"}},
			{Name: "attendance_date", Type: model.TypeDate, Nullable: false, Validation: &model.RuleSet{Required: true}},
			{Name: "check_in", Type: model.TypeDateTime, Nullable: false, Validation: &model.RuleSet{Required: true}},
			{Name: "status", Type: model.TypeEnum, Nullable: false, Validation: &model.RuleSet{Required: true, Enum: []any{"present", "absent", "half_day", "leave", "holiday"}}},
			{Name: "check_in_location", Type: model.TypeCustom, CustomType: "geo_point", Nullable: true},
			{Name: "metadata", Type: model.TypeJSON, Nullable: true},
		},
	}

	// Register all models in the project registry
	models := map[string]*model.Model{
		"organization": orgModel,
		"department":   deptModel,
		"location":     locModel,
		"employee":     empModel,
		"project":      projModel,
		"work_site":    workSiteModel,
		"attendance":   attModel,
	}

	for _, m := range models {
		_, err := engine.GetRegistry().SaveDraft(m)
		if err != nil {
			t.Fatalf("failed to save draft model '%s': %v", m.ID, err)
		}
	}

	return engine, models
}

func TestEnterpriseMultiSchemaDomain_ValidationMatrix(t *testing.T) {
	engine, models := setupEnterpriseMultiSchemaEngine(t)
	ctx := context.Background()
	orgModel := models["organization"]
	empModel := models["employee"]

	// 1. Valid organization creation
	t.Run("Valid organization", func(t *testing.T) {
		validOrg := map[string]any{
			"code":           "CYBERDYNE_01",
			"name":           "Cyberdyne Systems Corp",
			"status":         "active",
			"employee_count": 500,
			"annual_budget":  5000000.50,
			"settings":       map[string]any{"tier": "enterprise", "auto_scale": true},
			"tags":           []any{"tech", "ai", "robotics"},
			"created_at":     "2026-08-28T10:30:00Z",
		}
		created, err := engine.Create(ctx, "organization", validOrg)
		if err != nil {
			t.Fatalf("expected valid organization creation, got err: %v", err)
		}
		if created["id"] == nil || len(created["id"].(string)) != 32 {
			t.Fatalf("expected auto-generated UUID for organization, got: %v", created["id"])
		}
	})

	// 2. Missing required field (code)
	t.Run("Missing required field", func(t *testing.T) {
		missingCode := map[string]any{
			"name":       "Test Org",
			"status":     "active",
			"created_at": "2026-08-28T10:30:00Z",
		}
		_, err := engine.Create(ctx, "organization", missingCode)
		if err == nil || !strings.Contains(err.Error(), "code") {
			t.Fatalf("expected missing required field 'code' error, got: %v", err)
		}
	})

	// 3. String too short
	t.Run("String too short", func(t *testing.T) {
		shortName := map[string]any{
			"id":         mapping.GenerateUUID(),
			"code":       "VALID_CODE",
			"name":       "A", // Min is 3
			"status":     "active",
			"created_at": "2026-08-28T10:30:00Z",
		}
		err := validation.ValidateData(orgModel, shortName)
		if err == nil || !strings.Contains(err.Error(), "min") {
			t.Fatalf("expected min length violation on name, got: %v", err)
		}
	})

	// 4. String too long
	t.Run("String too long", func(t *testing.T) {
		longCode := map[string]any{
			"id":         mapping.GenerateUUID(),
			"code":       "VERY_LONG_CODE_THAT_EXCEEDS_TWENTY_CHARS",
			"name":       "Valid Org Name",
			"status":     "active",
			"created_at": "2026-08-28T10:30:00Z",
		}
		err := validation.ValidateData(orgModel, longCode)
		if err == nil || !strings.Contains(err.Error(), "max") {
			t.Fatalf("expected max length violation on code, got: %v", err)
		}
	})

	// 5. Regex failure
	t.Run("Regex failure", func(t *testing.T) {
		invalidCodeRegex := map[string]any{
			"id":         mapping.GenerateUUID(),
			"code":       "invalid_code",
			"name":       "Valid Org Name",
			"status":     "active",
			"created_at": "2026-08-28T10:30:00Z",
		}
		err := validation.ValidateData(orgModel, invalidCodeRegex)
		if err == nil || !strings.Contains(err.Error(), "does not match") {
			t.Fatalf("expected regex pattern mismatch error, got: %v", err)
		}
	})

	// 6. Integer min violation
	t.Run("Integer min violation", func(t *testing.T) {
		negCount := map[string]any{
			"id":             mapping.GenerateUUID(),
			"code":           "ORG_01",
			"name":           "Valid Org Name",
			"status":         "active",
			"employee_count": -5,
			"created_at":     "2026-08-28T10:30:00Z",
		}
		err := validation.ValidateData(orgModel, negCount)
		if err == nil || !strings.Contains(err.Error(), "must be >=") {
			t.Fatalf("expected min integer constraint error, got: %v", err)
		}
	})

	// 7. Decimal precision and scale violation
	t.Run("Decimal scale violation", func(t *testing.T) {
		badScale := map[string]any{
			"id":            mapping.GenerateUUID(),
			"code":          "ORG_01",
			"name":          "Valid Org Name",
			"status":        "active",
			"annual_budget": "50000.12345", // scale allowed is 2
			"created_at":    "2026-08-28T10:30:00Z",
		}
		err := validation.ValidateData(orgModel, badScale)
		if err == nil || !strings.Contains(err.Error(), "scale") {
			t.Fatalf("expected decimal scale error, got: %v", err)
		}
	})

	// 8. Enum invalid
	t.Run("Enum invalid", func(t *testing.T) {
		invalidEnum := map[string]any{
			"id":         mapping.GenerateUUID(),
			"code":       "ORG_01",
			"name":       "Valid Org Name",
			"status":     "permanent", // allowed: draft, active, inactive, archived
			"created_at": "2026-08-28T10:30:00Z",
		}
		err := validation.ValidateData(orgModel, invalidEnum)
		if err == nil || !strings.Contains(err.Error(), "must be one of") {
			t.Fatalf("expected enum error, got: %v", err)
		}
	})

	// 9. Boolean invalid
	t.Run("Boolean invalid", func(t *testing.T) {
		badBool := map[string]any{
			"id":              mapping.GenerateUUID(),
			"organization_id": "550e8400-e29b-41d4-a716-446655440000",
			"code":            "LOC-101",
			"name":            "Main Location",
			"latitude":        37.7749,
			"longitude":       -122.4194,
			"radius_meters":   100.0,
			"is_active":       "not_a_boolean",
		}
		locModel := models["location"]
		err := validation.ValidateData(locModel, badBool)
		if err == nil || !strings.Contains(err.Error(), "boolean") {
			t.Fatalf("expected boolean validation error, got: %v", err)
		}
	})

	// 10. UUID invalid (tested before orbital lookup)
	t.Run("UUID invalid format", func(t *testing.T) {
		invalidUUID := map[string]any{
			"id":              mapping.GenerateUUID(),
			"organization_id": "not-a-valid-uuid",
			"code":            "ENG",
			"name":            "Engineering",
			"department_type": "engineering",
		}
		deptModel := models["department"]
		err := validation.ValidateData(deptModel, invalidUUID)
		if err == nil || !strings.Contains(err.Error(), "UUID") {
			t.Fatalf("expected UUID format validation error before orbital lookup, got: %v", err)
		}
	})

	// 11. Date & Datetime invalid
	t.Run("Date format invalid", func(t *testing.T) {
		badDate := map[string]any{
			"employee_code":   "EMP-000001",
			"organization_id": "550e8400-e29b-41d4-a716-446655440000",
			"department_id":   "550e8400-e29b-41d4-a716-446655440001",
			"first_name":      "Sanjay",
			"last_name":       "Kumar",
			"email":           "sanjay@example.com",
			"joining_date":    "28/08/2026", // Expected YYYY-MM-DD
			"employment_type": "full_time",
			"experience_years": 5.0,
			"is_active":       true,
			"created_at":      "2026-08-28T10:30:00Z",
		}
		err := validation.ValidateData(empModel, badDate)
		if err == nil || !strings.Contains(err.Error(), "YYYY-MM-DD") {
			t.Fatalf("expected date format error, got: %v", err)
		}
	})

	// 12. Array item wrong length/type
	t.Run("Array item rule violation", func(t *testing.T) {
		badSkills := map[string]any{
			"employee_code":   "EMP-000001",
			"organization_id": "550e8400-e29b-41d4-a716-446655440000",
			"department_id":   "550e8400-e29b-41d4-a716-446655440001",
			"first_name":      "Sanjay",
			"last_name":       "Kumar",
			"email":           "sanjay@example.com",
			"joining_date":    "2026-08-01",
			"employment_type": "full_time",
			"experience_years": 5.0,
			"is_active":       true,
			"skills":          []any{"G", "ValidGoSkill"}, // "G" is less than min_length 2
			"created_at":      "2026-08-28T10:30:00Z",
		}
		err := validation.ValidateData(empModel, badSkills)
		if err == nil || !strings.Contains(err.Error(), "min") {
			t.Fatalf("expected array item min length violation, got: %v", err)
		}
	})

	// 13. Custom type validation: geo_point_radius & geo_point
	t.Run("Custom type geo_point_radius invalid", func(t *testing.T) {
		workSiteModel := models["work_site"]
		invalidGeofence := map[string]any{
			"organization_id": "550e8400-e29b-41d4-a716-446655440000",
			"location_id":     "550e8400-e29b-41d4-a716-446655440002",
			"name":            "Downtown Site",
			"status":          "active",
			"geofence": map[string]any{
				"latitude":  195.0, // Invalid: must be -90..90
				"longitude": -122.4,
				"radius":    100.0,
			},
		}
		err := validation.ValidateData(workSiteModel, invalidGeofence)
		if err == nil || !strings.Contains(err.Error(), "latitude") {
			t.Fatalf("expected custom type latitude validation error, got: %v", err)
		}
	})

	// 14. Type Coercion tests (string integer -> int, string bool -> bool, string decimal -> float)
	t.Run("Type coercion success", func(t *testing.T) {
		coercible := map[string]any{
			"code":           "COERCE_01",
			"name":           "Coercion Enterprise",
			"status":         "active",
			"employee_count": "500",    // string -> int
			"annual_budget":  "9500.5", // string -> decimal
			"created_at":     "2026-08-28T10:30:00Z",
		}
		sanitized, err := mapping.SanitizeInput(orgModel, coercible)
		if err != nil {
			t.Fatalf("expected successful coercion in SanitizeInput: %v", err)
		}
		if sanitized["employee_count"] != 500 {
			t.Fatalf("expected coerced employee_count 500, got: %v (%T)", sanitized["employee_count"], sanitized["employee_count"])
		}
		if sanitized["annual_budget"] != 9500.5 {
			t.Fatalf("expected coerced annual_budget 9500.5, got: %v", sanitized["annual_budget"])
		}
	})

	// 15. Orbital References: Exists & REFERENCE_NOT_FOUND error
	t.Run("Orbital reference not found", func(t *testing.T) {
		nonExistentDept := map[string]any{
			"organization_id": "550e8400-e29b-41d4-a716-446655999999", // Does not exist
			"code":            "SALES",
			"name":            "Enterprise Sales",
			"department_type": "sales",
		}
		_, err := engine.Create(ctx, "department", nonExistentDept)
		if err == nil {
			t.Fatalf("expected orbital reference failure for missing organization")
		}
		valErr, ok := err.(*validation.ValidationError)
		if !ok || valErr.Code != "REFERENCE_NOT_FOUND" {
			t.Fatalf("expected structured REFERENCE_NOT_FOUND error, got: %v", err)
		}
	})

	// 16. Multi-Hop Cross-Schema Orbital Reference Resolution:
	// company.organization -> company.department -> hr.employee -> projects.project -> operations.attendance
	t.Run("Multi-Hop Cross-Schema Pipeline", func(t *testing.T) {
		// Step 1: Create Organization in company schema
		orgID := "550e8400-e29b-41d4-a716-446655440000"
		orgData := map[string]any{
			"id":         orgID,
			"code":       "CYBER_ORG",
			"name":       "Cyberdyne Corp",
			"status":     "active",
			"created_at": "2026-08-28T10:30:00Z",
		}
		_, err := engine.Create(ctx, "organization", orgData)
		if err != nil {
			t.Fatalf("failed creating root organization: %v", err)
		}

		// Step 2: Create Department referencing company.organization
		deptID := "550e8400-e29b-41d4-a716-446655440001"
		deptData := map[string]any{
			"id":              deptID,
			"organization_id": orgID,
			"code":            "ENG",
			"name":            "Engineering",
			"department_type": "engineering",
		}
		_, err = engine.Create(ctx, "department", deptData)
		if err != nil {
			t.Fatalf("failed creating department: %v", err)
		}

		// Step 3: Create Self-referencing sub-department
		subDeptID := "550e8400-e29b-41d4-a716-446655440002"
		subDeptData := map[string]any{
			"id":                   subDeptID,
			"organization_id":      orgID,
			"code":                 "AILAB",
			"name":                 "AI Research Lab",
			"parent_department_id": deptID, // Self-reference to parent department
			"department_type":      "engineering",
		}
		_, err = engine.Create(ctx, "department", subDeptData)
		if err != nil {
			t.Fatalf("failed creating self-referencing sub-department: %v", err)
		}

		// Step 4: Create Location in company schema
		locID := "550e8400-e29b-41d4-a716-446655440003"
		locData := map[string]any{
			"id":              locID,
			"organization_id": orgID,
			"code":            "SFO-001",
			"name":            "San Francisco HQ",
			"latitude":        37.7749,
			"longitude":       -122.4194,
			"radius_meters":   500.0,
			"is_active":       true,
		}
		_, err = engine.Create(ctx, "location", locData)
		if err != nil {
			t.Fatalf("failed creating location: %v", err)
		}

		// Step 5: Create Employee in hr schema (Cross-Schema to company.organization and company.department)
		empID := "550e8400-e29b-41d4-a716-446655440004"
		empData := map[string]any{
			"id":               empID,
			"employee_code":    "EMP-100001",
			"organization_id":  orgID,
			"department_id":    deptID,
			"first_name":       "Sanjay",
			"last_name":        "Kumar",
			"email":            "sanjay.kumar@cyberdyne.com",
			"joining_date":     "2026-08-01",
			"employment_type":  "full_time",
			"experience_years": 8.5,
			"is_active":        true,
			"skills":           []any{"Go", "Distributed Systems", "MongoDB"},
			"created_at":       "2026-08-28T10:30:00Z",
		}
		_, err = engine.Create(ctx, "employee", empData)
		if err != nil {
			t.Fatalf("failed creating employee: %v", err)
		}

		// Step 6: Create Project in projects schema (Cross-Schema to company.organization and hr.employee)
		projID := "550e8400-e29b-41d4-a716-446655440005"
		projData := map[string]any{
			"id":                 projID,
			"organization_id":    orgID,
			"project_manager_id": empID,
			"code":               "PRJ-2026",
			"name":               "Skynet Core Engine",
			"project_type":       "research",
			"budget":             15000000.00,
			"start_date":         "2026-09-01",
		}
		_, err = engine.Create(ctx, "project", projData)
		if err != nil {
			t.Fatalf("failed creating project: %v", err)
		}

		// Step 7: Create WorkSite in operations schema (Cross-Schema to company.location)
		siteID := "550e8400-e29b-41d4-a716-446655440006"
		siteData := map[string]any{
			"id":              siteID,
			"organization_id": orgID,
			"location_id":     locID,
			"name":            "Quantum Computing Facility",
			"status":          "active",
			"geofence": map[string]any{
				"latitude":  37.7749,
				"longitude": -122.4194,
				"radius":    250.0,
			},
		}
		_, err = engine.Create(ctx, "work_site", siteData)
		if err != nil {
			t.Fatalf("failed creating work_site: %v", err)
		}

		// Step 8: Create Attendance in operations schema (Multi-hop orbital references: hr.employee, operations.work_site, projects.project)
		attData := map[string]any{
			"employee_id":     empID,
			"work_site_id":    siteID,
			"project_id":      projID,
			"attendance_date": "2026-08-28",
			"check_in":        "2026-08-28T09:00:00Z",
			"status":          "present",
			"check_in_location": map[string]any{
				"latitude":  37.7749,
				"longitude": -122.4194,
			},
		}
		createdAtt, err := engine.Create(ctx, "attendance", attData)
		if err != nil {
			t.Fatalf("failed multi-hop attendance creation: %v", err)
		}
		if createdAtt["id"] == nil {
			t.Fatalf("expected attendance ID created")
		}
	})
}
