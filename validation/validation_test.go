package validation_test

import (
	"errors"
	"github.com/SanjayDrop5528/models-go-engine/model"
	"github.com/SanjayDrop5528/models-go-engine/plan"
	"github.com/SanjayDrop5528/models-go-engine/validation"
	"strings"
	"testing"
)

func TestValidateModel(t *testing.T) {
	tests := []struct {
		name        string
		m           *model.Model
		wantErr     bool
		errContains string
	}{
		{
			name: "valid relational model with primary key",
			m: &model.Model{
				Name:        "users",
				StorageType: model.StorageRelational,
				Attributes: []model.Attribute{
					{Name: "id", Type: model.TypeLong},
					{Name: "name", Type: model.TypeString},
				},
				PrimaryKey: &model.PrimaryKey{
					Columns: []string{"id"},
				},
			},
			wantErr: false,
		},
		{
			name: "valid document model without primary key",
			m: &model.Model{
				Name:        "logs",
				StorageType: model.StorageDocument,
				Attributes: []model.Attribute{
					{Name: "message", Type: model.TypeString},
				},
			},
			wantErr: false,
		},
		{
			name:        "nil model",
			m:           nil,
			wantErr:     true,
			errContains: "model cannot be nil",
		},
		{
			name: "empty model name",
			m: &model.Model{
				Name: "",
				Attributes: []model.Attribute{
					{Name: "id", Type: model.TypeLong},
				},
			},
			wantErr:     true,
			errContains: "model name cannot be empty",
		},
		{
			name: "invalid identifier name",
			m: &model.Model{
				Name: "invalid-name!",
				Attributes: []model.Attribute{
					{Name: "id", Type: model.TypeLong},
				},
			},
			wantErr:     true,
			errContains: "must be a valid identifier",
		},
		{
			name: "empty attributes list",
			m: &model.Model{
				Name:       "users",
				Attributes: []model.Attribute{},
			},
			wantErr:     true,
			errContains: "must contain at least one attribute",
		},
		{
			name: "empty attribute name",
			m: &model.Model{
				Name: "users",
				Attributes: []model.Attribute{
					{Name: "", Type: model.TypeString},
				},
			},
			wantErr:     true,
			errContains: "attribute name cannot be empty",
		},
		{
			name: "invalid attribute name",
			m: &model.Model{
				Name: "users",
				Attributes: []model.Attribute{
					{Name: "123_invalid", Type: model.TypeString},
				},
			},
			wantErr:     true,
			errContains: "must be a valid identifier",
		},
		{
			name: "duplicate attribute names (case-insensitive)",
			m: &model.Model{
				Name: "users",
				Attributes: []model.Attribute{
					{Name: "email", Type: model.TypeString},
					{Name: "EMAIL", Type: model.TypeString},
				},
				PrimaryKey: &model.PrimaryKey{Columns: []string{"email"}},
			},
			wantErr:     true,
			errContains: "duplicate attribute name",
		},
		{
			name: "unsupported attribute data type",
			m: &model.Model{
				Name: "users",
				Attributes: []model.Attribute{
					{Name: "data", Type: "UNSUPPORTED_DATA_TYPE"},
				},
				PrimaryKey: &model.PrimaryKey{Columns: []string{"data"}},
			},
			wantErr:     true,
			errContains: "unsupported data type",
		},
		{
			name: "relational model missing primary key",
			m: &model.Model{
				Name:        "users",
				StorageType: model.StorageRelational,
				Attributes: []model.Attribute{
					{Name: "name", Type: model.TypeString},
				},
			},
			wantErr:     true,
			errContains: "relational models must have at least one primary key attribute",
		},
		{
			name: "primary key column not found in attributes",
			m: &model.Model{
				Name:        "users",
				StorageType: model.StorageRelational,
				Attributes: []model.Attribute{
					{Name: "name", Type: model.TypeString},
				},
				PrimaryKey: &model.PrimaryKey{Columns: []string{"id"}},
			},
			wantErr:     true,
			errContains: "primary key column 'id' does not exist in model attributes",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validation.ValidateModel(tt.m)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateModel() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr && tt.errContains != "" && (err == nil || !strings.Contains(err.Error(), tt.errContains)) {
				t.Errorf("ValidateModel() error message = %v, want substring %q", err, tt.errContains)
			}
		})
	}
}

func TestValidateData(t *testing.T) {
	minLen := 3
	maxLen := 10
	minVal := 18.0
	maxVal := 65.0

	testModel := &model.Model{
		Name: "users",
		Attributes: []model.Attribute{
			{
				Name:     "username",
				Type:     model.TypeString,
				Nullable: false,
				Validation: &model.RuleSet{
					Required:  true,
					MinLength: &minLen,
					MaxLength: &maxLen,
					Pattern:   `^[a-zA-Z0-9]+$`,
				},
			},
			{
				Name:     "age",
				Type:     model.TypeInt,
				Nullable: true,
				Validation: &model.RuleSet{
					Min: &minVal,
					Max: &maxVal,
				},
			},
			{
				Name:     "is_active",
				Type:     model.TypeBoolean,
				Nullable: true,
			},
			{
				Name:     "role",
				Type:     model.TypeString,
				Nullable: false,
				Validation: &model.RuleSet{
					Enum: []any{"admin", "member", "guest"},
				},
			},
			{
				Name:     "bio",
				Type:     model.TypeText,
				Nullable: true,
				Validation: &model.RuleSet{
					MaxLength: &maxLen,
				},
			},
			{
				Name:     "score",
				Type:     model.TypeDecimal,
				Nullable: true,
			},
		},
		PrimaryKey: &model.PrimaryKey{Columns: []string{"username"}},
	}

	tests := []struct {
		name        string
		data        map[string]any
		wantErr     bool
		errContains string
	}{
		{
			name: "valid record data",
			data: map[string]any{
				"username":  "alice",
				"age":       25,
				"is_active": true,
				"role":      "admin",
				"bio":       "hello",
				"score":     98.5,
			},
			wantErr: false,
		},
		{
			name: "missing required field",
			data: map[string]any{
				"age":  25,
				"role": "admin",
			},
			wantErr:     true,
			errContains: "field 'username' is required",
		},
		{
			name: "nil required field",
			data: map[string]any{
				"username": nil,
				"role":     "admin",
			},
			wantErr:     true,
			errContains: "field 'username' is required",
		},
		{
			name: "non-nullable field is null",
			data: map[string]any{
				"username": "alice",
				"role":     nil,
			},
			wantErr:     true,
			errContains: "field 'role' cannot be null",
		},
		{
			name: "string type mismatch",
			data: map[string]any{
				"username": 12345,
				"role":     "admin",
			},
			wantErr:     true,
			errContains: "expects string value, got int",
		},
		{
			name: "numeric type mismatch",
			data: map[string]any{
				"username": "alice",
				"role":     "admin",
				"age":      "not-a-number",
			},
			wantErr:     true,
			errContains: "expects numeric value for type INT, got string",
		},
		{
			name: "boolean type mismatch",
			data: map[string]any{
				"username":  "alice",
				"role":      "admin",
				"is_active": "yes",
			},
			wantErr:     true,
			errContains: "expects boolean value, got string",
		},
		{
			name: "string length below min",
			data: map[string]any{
				"username": "ab",
				"role":     "admin",
			},
			wantErr:     true,
			errContains: "length 2 is less than min 3",
		},
		{
			name: "string_length_exceeds_max",
			data: map[string]any{
				"username": "alice_is_a_very_long_username",
				"role":     "admin",
			},
			wantErr:     true,
			errContains: "length 29 exceeds max 10",
		},
		{
			name: "pattern_mismatch",
			data: map[string]any{
				"username": "alice!",
				"role":     "admin",
			},
			wantErr:     true,
			errContains: "does not match pattern",
		},
		{
			name: "numeric value below min",
			data: map[string]any{
				"username": "alice",
				"role":     "admin",
				"age":      15,
			},
			wantErr:     true,
			errContains: "value 15 is less than min 18",
		},
		{
			name: "numeric value exceeds max",
			data: map[string]any{
				"username": "alice",
				"role":     "admin",
				"age":      70,
			},
			wantErr:     true,
			errContains: "value 70 exceeds max 65",
		},
		{
			name: "enum value invalid",
			data: map[string]any{
				"username": "alice",
				"role":     "superadmin",
			},
			wantErr:     true,
			errContains: "not in allowed enum values",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validation.ValidateData(testModel, tt.data)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateData() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr && tt.errContains != "" && (err == nil || !strings.Contains(err.Error(), tt.errContains)) {
				t.Errorf("ValidateData() error message = %v, want substring %q", err, tt.errContains)
			}
		})
	}
}

func TestValidatePlan(t *testing.T) {
	tests := []struct {
		name             string
		p                *plan.SchemaPlan
		allowDestructive bool
		wantErr          bool
		errContains      string
	}{
		{
			name:        "nil plan",
			p:           nil,
			wantErr:     true,
			errContains: "plan is nil",
		},
		{
			name: "non-destructive plan",
			p: &plan.SchemaPlan{
				Destructive: false,
			},
			allowDestructive: false,
			wantErr:          false,
		},
		{
			name: "destructive plan without allowDestructive",
			p: &plan.SchemaPlan{
				Destructive: true,
				Warnings:    []string{"Dropping column"},
			},
			allowDestructive: false,
			wantErr:          true,
			errContains:      "Explicit confirmation (allow_destructive=true) is required",
		},
		{
			name: "destructive plan with allowDestructive",
			p: &plan.SchemaPlan{
				Destructive: true,
				Warnings:    []string{"Dropping column"},
			},
			allowDestructive: true,
			wantErr:          false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validation.ValidatePlan(tt.p, tt.allowDestructive)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidatePlan() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr && tt.errContains != "" && (err == nil || !strings.Contains(err.Error(), tt.errContains)) {
				t.Errorf("ValidatePlan() error message = %v, want substring %q", err, tt.errContains)
			}
		})
	}
}

func TestValidateModelConfig(t *testing.T) {
	tests := []struct {
		name        string
		cfg         *model.ModelConfig
		wantErr     bool
		errContains string
	}{
		{
			name: "valid active model_config",
			cfg: &model.ModelConfig{
				Name:   "Account",
				Status: model.ModelConfigStatusActive,
			},
			wantErr: false,
		},
		{
			name: "valid draft model_config",
			cfg: &model.ModelConfig{
				Name:   "DraftModel",
				Status: model.ModelConfigStatusDraft,
			},
			wantErr: false,
		},
		{
			name: "valid inactive model_config",
			cfg: &model.ModelConfig{
				Name:   "InactiveModel",
				Status: model.ModelConfigStatusInactive,
			},
			wantErr: false,
		},
		{
			name: "valid archived model_config",
			cfg: &model.ModelConfig{
				Name:   "ArchivedModel",
				Status: model.ModelConfigStatusArchived,
			},
			wantErr: false,
		},
		{
			name: "invalid status",
			cfg: &model.ModelConfig{
				Name:   "Account",
				Status: "UNKNOWN_STATUS",
			},
			wantErr:     true,
			errContains: "invalid model_config status",
		},
		{
			name:        "nil config",
			cfg:         nil,
			wantErr:     true,
			errContains: "model_config cannot be nil",
		},
		{
			name: "empty name",
			cfg: &model.ModelConfig{
				Name: "",
			},
			wantErr:     true,
			errContains: "model_config name cannot be empty",
		},
		{
			name: "invalid identifier name",
			cfg: &model.ModelConfig{
				Name: "123-Invalid!",
			},
			wantErr:     true,
			errContains: "must be a valid identifier",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validation.ValidateModelConfig(tt.cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateModelConfig() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr && tt.errContains != "" && (err == nil || !strings.Contains(err.Error(), tt.errContains)) {
				t.Errorf("ValidateModelConfig() error message = %v, want substring %q", err, tt.errContains)
			}
		})
	}
}

func TestValidateDataModel(t *testing.T) {
	refModelID := "org_model"

	tests := []struct {
		name        string
		dm          *model.DataModel
		wantErr     bool
		errContains string
	}{
		{
			name: "valid data model field with column name",
			dm: &model.DataModel{
				ModelID:    "user",
				ColumnName: "username",
				DataType:   model.TypeString,
			},
			wantErr: false,
		},
		{
			name: "valid data model field with json_field fallback",
			dm: &model.DataModel{
				ModelID:   "user",
				JSONField: "profile_image",
				DataType:  model.TypeString,
			},
			wantErr: false,
		},
		{
			name: "valid orbital reference - exists",
			dm: &model.DataModel{
				ModelID:                    "user",
				ColumnName:                 "org_id",
				DataType:                   model.TypeString,
				IsOrbitalReference:         true,
				OrbitalReferenceModelID:    &refModelID,
				OrbitalReferenceValidation: model.OrbitalValidationExists,
			},
			wantErr: false,
		},
		{
			name: "valid orbital reference - exists_active",
			dm: &model.DataModel{
				ModelID:                    "user",
				ColumnName:                 "org_id",
				DataType:                   model.TypeString,
				IsOrbitalReference:         true,
				OrbitalReferenceModelID:    &refModelID,
				OrbitalReferenceValidation: model.OrbitalValidationExistsActive,
			},
			wantErr: false,
		},
		{
			name: "valid orbital reference - exists_in_scope",
			dm: &model.DataModel{
				ModelID:                    "user",
				ColumnName:                 "org_id",
				DataType:                   model.TypeString,
				IsOrbitalReference:         true,
				OrbitalReferenceModelID:    &refModelID,
				OrbitalReferenceValidation: model.OrbitalValidationExistsInScope,
			},
			wantErr: false,
		},
		{
			name: "valid orbital reference - not_exists",
			dm: &model.DataModel{
				ModelID:                    "user",
				ColumnName:                 "org_id",
				DataType:                   model.TypeString,
				IsOrbitalReference:         true,
				OrbitalReferenceModelID:    &refModelID,
				OrbitalReferenceValidation: model.OrbitalValidationNotExists,
			},
			wantErr: false,
		},
		{
			name: "orbital reference missing referenced model",
			dm: &model.DataModel{
				ModelID:            "user",
				ColumnName:         "org_id",
				DataType:           model.TypeString,
				IsOrbitalReference: true,
			},
			wantErr:     true,
			errContains: "requires orbital_reference_model_id",
		},
		{
			name: "orbital reference invalid validation mode",
			dm: &model.DataModel{
				ModelID:                    "user",
				ColumnName:                 "org_id",
				DataType:                   model.TypeString,
				IsOrbitalReference:         true,
				OrbitalReferenceModelID:    &refModelID,
				OrbitalReferenceValidation: "INVALID_ORBITAL_VALIDATION",
			},
			wantErr:     true,
			errContains: "invalid orbital_reference_validation",
		},
		{
			name:        "nil data model",
			dm:          nil,
			wantErr:     true,
			errContains: "data_model cannot be nil",
		},
		{
			name: "missing model_id",
			dm: &model.DataModel{
				ColumnName: "username",
				DataType:   model.TypeString,
			},
			wantErr:     true,
			errContains: "data_model model_id cannot be empty",
		},
		{
			name: "missing column name and json field",
			dm: &model.DataModel{
				ModelID:  "user",
				DataType: model.TypeString,
			},
			wantErr:     true,
			errContains: "must have column_name or json_field",
		},
		{
			name: "invalid identifier column name",
			dm: &model.DataModel{
				ModelID:    "user",
				ColumnName: "123_invalid!",
				DataType:   model.TypeString,
			},
			wantErr:     true,
			errContains: "must be a valid identifier",
		},
		{
			name: "invalid data type",
			dm: &model.DataModel{
				ModelID:    "user",
				ColumnName: "data",
				DataType:   "UNKNOWN_TYPE",
			},
			wantErr:     true,
			errContains: "has unsupported data type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validation.ValidateDataModel(tt.dm)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateDataModel() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr && tt.errContains != "" && (err == nil || !strings.Contains(err.Error(), tt.errContains)) {
				t.Errorf("ValidateDataModel() error message = %v, want substring %q", err, tt.errContains)
			}
		})
	}
}

func TestValidateCustomType(t *testing.T) {
	lookup := func(idOrName string) (*model.ModelConfig, error) {
		if idOrName == "address" {
			return &model.ModelConfig{
				Name:                 "Address",
				IsAttributeReference: true,
			}, nil
		}
		if idOrName == "employee" {
			return &model.ModelConfig{
				Name:                 "Employee",
				IsAttributeReference: false,
			}, nil
		}
		return nil, errors.New("not found")
	}

	addrID := "address"
	empID := "employee"
	unknownID := "unknown"

	// Nil / empty custom type ID passes
	if err := validation.ValidateCustomType(lookup, nil); err != nil {
		t.Errorf("expected nil data model to pass, got %v", err)
	}
	emptyDM := &model.DataModel{ColumnName: "col"}
	if err := validation.ValidateCustomType(lookup, emptyDM); err != nil {
		t.Errorf("expected empty custom type to pass, got %v", err)
	}

	// Valid custom type
	validDM := &model.DataModel{
		ColumnName:   "addr",
		CustomTypeID: &addrID,
	}
	if err := validation.ValidateCustomType(lookup, validDM); err != nil {
		t.Errorf("expected valid custom type to pass, got: %v", err)
	}

	// Invalid custom type (referenced model has is_attribute_reference=false)
	invalidDM := &model.DataModel{
		ColumnName:   "emp",
		CustomTypeID: &empID,
	}
	err := validation.ValidateCustomType(lookup, invalidDM)
	if err == nil {
		t.Error("expected error when referenced model is not attribute reference")
	} else if !strings.Contains(err.Error(), "not marked with is_attribute_reference = true") {
		t.Errorf("expected specific error message, got: %v", err)
	}

	// Not found model
	notFoundDM := &model.DataModel{
		ColumnName:   "unk",
		CustomTypeID: &unknownID,
	}
	err = validation.ValidateCustomType(lookup, notFoundDM)
	if err == nil {
		t.Error("expected error when referenced model is not found")
	} else if !strings.Contains(err.Error(), "referenced model 'unknown' not found") {
		t.Errorf("expected specific error message, got: %v", err)
	}
}

func TestValidatePartialData(t *testing.T) {
	minLen := 3
	maxLen := 10

	testModel := &model.Model{
		Name: "users",
		Attributes: []model.Attribute{
			{
				Name:     "username",
				Type:     model.TypeString,
				Nullable: false,
				Validation: &model.RuleSet{
					Required:  true,
					MinLength: &minLen,
					MaxLength: &maxLen,
				},
			},
			{
				Name:     "age",
				Type:     model.TypeInt,
				Nullable: true,
			},
		},
		PrimaryKey: &model.PrimaryKey{Columns: []string{"username"}},
	}

	// Valid partial update without required username field (allowed in patch)
	if err := validation.ValidatePartialData(testModel, map[string]any{"age": 30}); err != nil {
		t.Errorf("expected valid patch to pass, got: %v", err)
	}

	// Valid partial update of username
	if err := validation.ValidatePartialData(testModel, map[string]any{"username": "validname"}); err != nil {
		t.Errorf("expected valid patch to pass, got: %v", err)
	}

	// Invalid partial update: username length violation
	err := validation.ValidatePartialData(testModel, map[string]any{"username": "a"})
	if err == nil || !strings.Contains(err.Error(), "less than min 3") {
		t.Errorf("expected length error on patch, got: %v", err)
	}

	// Invalid partial update: null on non-nullable field
	err = validation.ValidatePartialData(testModel, map[string]any{"username": nil})
	if err == nil || !strings.Contains(err.Error(), "cannot be null") {
		t.Errorf("expected cannot be null error on patch, got: %v", err)
	}

	// Invalid partial update: type mismatch
	err = validation.ValidatePartialData(testModel, map[string]any{"age": "not-a-number"})
	if err == nil || !strings.Contains(err.Error(), "expects numeric value") {
		t.Errorf("expected numeric type error on patch, got: %v", err)
	}
}

func TestMultiValidationError(t *testing.T) {
	m := &model.Model{
		Name: "jobs",
		Attributes: []model.Attribute{
			{Name: "title", Type: model.TypeString, Validation: &model.RuleSet{Required: true}},
			{Name: "min_experience", Type: model.TypeInt, Validation: &model.RuleSet{Min: floatPtr(0)}},
			{Name: "max_experience", Type: model.TypeInt, Validation: &model.RuleSet{Min: floatPtr(0)}},
		},
	}

	invalidData := map[string]any{
		"min_experience": -5,
		"max_experience": -10,
	}

	err := validation.ValidateData(m, invalidData)
	if err == nil {
		t.Fatalf("expected validation error, got nil")
	}

	multiErr, ok := err.(*validation.MultiValidationError)
	if !ok {
		t.Fatalf("expected *validation.MultiValidationError, got %T: %v", err, err)
	}

	if len(multiErr.Errors) < 3 {
		t.Errorf("expected at least 3 validation errors, got %d", len(multiErr.Errors))
	}

	errStr := err.Error()
	if !strings.Contains(errStr, "title") || !strings.Contains(errStr, "min_experience") || !strings.Contains(errStr, "max_experience") {
		t.Errorf("expected error string to contain all 3 failing field names, got: %s", errStr)
	}
}

func floatPtr(v float64) *float64 {
	return &v
}

