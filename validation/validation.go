package validation

import (
	"errors"
	"fmt"
	"github.com/SanjayDrop5528/models-go-engine/model"
	"github.com/SanjayDrop5528/models-go-engine/plan"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

var (
	validIdentifier = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)
	uuidRegex       = regexp.MustCompile(`^[0-9a-fA-F]{8}-?[0-9a-fA-F]{4}-?[0-9a-fA-F]{4}-?[0-9a-fA-F]{4}-?[0-9a-fA-F]{12}$`)
	emailRegex      = regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)
)

// ValidationError represents a structured validation failure.
type ValidationError struct {
	Code           string `json:"code"`
	Field          string `json:"field,omitempty"`
	Message        string `json:"message"`
	ReferenceModel string `json:"reference_model,omitempty"`
	ReferenceValue any    `json:"reference_value,omitempty"`
}

func (e *ValidationError) Error() string {
	return e.Message
}

// MultiValidationError aggregates multiple validation errors into a single error.
type MultiValidationError struct {
	Code   string             `json:"code"`
	Errors []*ValidationError `json:"errors"`
}

func (m *MultiValidationError) Error() string {
	if len(m.Errors) == 0 {
		return "validation failed"
	}
	if len(m.Errors) == 1 {
		return m.Errors[0].Message
	}
	msgs := make([]string, len(m.Errors))
	for i, err := range m.Errors {
		msgs[i] = err.Message
	}
	return strings.Join(msgs, "; ")
}

// NewValidationError creates a standard validation error.
func NewValidationError(field, message string) *ValidationError {
	return &ValidationError{
		Code:    "VALIDATION_ERROR",
		Field:   field,
		Message: message,
	}
}

// NewReferenceNotFoundError creates a structured error when an orbital reference lookup fails.
func NewReferenceNotFoundError(field, refModel string, refVal any) *ValidationError {
	return &ValidationError{
		Code:           "REFERENCE_NOT_FOUND",
		Field:          field,
		ReferenceModel: refModel,
		ReferenceValue: refVal,
		Message:        fmt.Sprintf("orbital reference validation failed on field '%s': referenced value '%v' does not exist in model '%s'", field, refVal, refModel),
	}
}

// NewMultiValidationError constructs a MultiValidationError if multiple errors exist, or a single error if 1 exists.
func NewMultiValidationError(errs []*ValidationError) error {
	if len(errs) == 0 {
		return nil
	}
	if len(errs) == 1 {
		return errs[0]
	}
	return &MultiValidationError{
		Code:   "MULTI_VALIDATION_ERROR",
		Errors: errs,
	}
}

// ValidateModel checks model metadata integrity before storage or schema generation.
func ValidateModel(m *model.Model) error {
	if m == nil {
		return errors.New("model cannot be nil")
	}
	if strings.TrimSpace(m.Name) == "" {
		return errors.New("model name cannot be empty")
	}
	if !validIdentifier.MatchString(m.Name) {
		return fmt.Errorf("model name '%s' must be a valid identifier (alphanumeric and underscores)", m.Name)
	}

	if len(m.Attributes) == 0 {
		return errors.New("model must contain at least one attribute")
	}

	attrNames := make(map[string]bool)
	for _, attr := range m.Attributes {
		name := strings.TrimSpace(attr.Name)
		if name == "" {
			return errors.New("attribute name cannot be empty")
		}
		if !validIdentifier.MatchString(name) {
			return fmt.Errorf("attribute name '%s' must be a valid identifier", name)
		}
		if attrNames[strings.ToLower(name)] {
			return fmt.Errorf("duplicate attribute name '%s'", name)
		}
		attrNames[strings.ToLower(name)] = true

		if !isValidDataType(attr.Type) {
			return fmt.Errorf("attribute '%s' has unsupported data type '%s'", name, attr.Type)
		}
	}

	pkCols := make(map[string]bool)
	if m.PrimaryKey != nil {
		for _, col := range m.PrimaryKey.Columns {
			pkCols[strings.ToLower(col)] = true
		}
	}
	for _, attr := range m.Attributes {
		if attr.IsPrimaryKey {
			pkCols[strings.ToLower(attr.Name)] = true
		}
	}

	if len(pkCols) == 0 && m.StorageType != model.StorageDocument {
		return errors.New("relational models must have at least one primary key attribute")
	}

	if len(pkCols) > 0 {
		for pkCol := range pkCols {
			if !attrNames[pkCol] {
				return fmt.Errorf("primary key column '%s' does not exist in model attributes", pkCol)
			}
		}
	}

	return nil
}

func isSequenceOrAutoAttr(attr *model.Attribute) bool {
	if attr == nil {
		return false
	}
	if attr.AutoIncrement {
		return true
	}
	name := strings.ToLower(attr.Name)
	if attr.Default != nil {
		defStr := fmt.Sprintf("%v", attr.Default)
		if strings.Contains(strings.ToLower(defStr), "nextval") || strings.Contains(strings.ToLower(defStr), "uuid") {
			return true
		}
	}
	return name == "id" && (attr.Type == model.TypeInt || attr.Type == model.TypeLong || attr.Type == model.TypeUUID)
}

// ValidateData validates a record data map against model constraints, data types, and validation rulesets.
func ValidateData(m *model.Model, data map[string]any) error {
	if m == nil {
		return errors.New("model cannot be nil")
	}
	if data == nil {
		data = make(map[string]any)
	}

	var errs []*ValidationError

	for _, attr := range m.Attributes {
		val, exists := data[attr.Name]

		// If val is empty string "" or "auto" for auto-increment / sequence / default fields, treat as omitted
		if exists && val != nil {
			if strVal, ok := val.(string); ok && (strVal == "" || strings.EqualFold(strVal, "auto")) {
				if attr.AutoIncrement || attr.Default != nil || isSequenceOrAutoAttr(&attr) {
					exists = false
					val = nil
				}
			}
		}

		if !exists || val == nil {
			if attr.Validation != nil && attr.Validation.Required {
				errs = append(errs, NewValidationError(attr.Name, fmt.Sprintf("field '%s' is required", attr.Name)))
			} else if !attr.Nullable && !attr.AutoIncrement && !isSequenceOrAutoAttr(&attr) && !m.IsPrimaryKey(attr.Name) && attr.Default == nil {
				errs = append(errs, NewValidationError(attr.Name, fmt.Sprintf("field '%s' cannot be null", attr.Name)))
			}
			continue
		}

		if err := validateAttributeValue(&attr, val); err != nil {
			if ve, ok := err.(*ValidationError); ok {
				errs = append(errs, ve)
			} else if me, ok := err.(*MultiValidationError); ok {
				errs = append(errs, me.Errors...)
			} else {
				errs = append(errs, NewValidationError(attr.Name, err.Error()))
			}
		}
	}
	return NewMultiValidationError(errs)
}

// ValidatePartialData validates a partial record data map (e.g. for PATCH) against model constraints.
func ValidatePartialData(m *model.Model, data map[string]any) error {
	if m == nil {
		return errors.New("model cannot be nil")
	}
	var errs []*ValidationError
	for k, val := range data {
		attr := m.GetAttribute(k)
		if attr == nil {
			continue
		}
		if val == nil {
			if !attr.Nullable {
				errs = append(errs, NewValidationError(attr.Name, fmt.Sprintf("field '%s' cannot be null", attr.Name)))
			}
			continue
		}

		if err := validateAttributeValue(attr, val); err != nil {
			if ve, ok := err.(*ValidationError); ok {
				errs = append(errs, ve)
			} else if me, ok := err.(*MultiValidationError); ok {
				errs = append(errs, me.Errors...)
			} else {
				errs = append(errs, NewValidationError(attr.Name, err.Error()))
			}
		}
	}
	return NewMultiValidationError(errs)
}

func validateAttributeValue(attr *model.Attribute, val any) error {
	if attr == nil {
		return nil
	}

	normType := model.NormalizeDataType(string(attr.Type))

	// 1. Input Type Validations & Specific Data Types
	switch normType {
	case model.TypeString, model.TypeText:
		strVal, ok := val.(string)
		if !ok {
			return NewValidationError(attr.Name, fmt.Sprintf("field '%s' expects string value, got %T", attr.Name, val))
		}
		if err := validateStringConstraints(attr.Name, strVal, attr.Validation); err != nil {
			return err
		}

	case model.TypeEmail:
		strVal, ok := val.(string)
		if !ok {
			return NewValidationError(attr.Name, fmt.Sprintf("field '%s' expects string email, got %T", attr.Name, val))
		}
		if !emailRegex.MatchString(strVal) {
			return NewValidationError(attr.Name, fmt.Sprintf("field '%s' value '%s' is not a valid email address", attr.Name, strVal))
		}
		if err := validateStringConstraints(attr.Name, strVal, attr.Validation); err != nil {
			return err
		}

	case model.TypeUUID:
		strVal, ok := val.(string)
		if !ok {
			return NewValidationError(attr.Name, fmt.Sprintf("field '%s' expects UUID string, got %T", attr.Name, val))
		}
		if !uuidRegex.MatchString(strVal) {
			return NewValidationError(attr.Name, fmt.Sprintf("field '%s' value '%s' is not a valid UUID", attr.Name, strVal))
		}

	case model.TypeInt, model.TypeLong:
		numVal, ok := toFloat64(val)
		if !ok {
			return NewValidationError(attr.Name, fmt.Sprintf("field '%s' expects numeric value for type %s, got %T", attr.Name, attr.Type, val))
		}
		if err := validateNumericConstraints(attr.Name, numVal, attr.Validation); err != nil {
			return err
		}

	case model.TypeFloat, model.TypeDecimal:
		numVal, ok := toFloat64(val)
		if !ok {
			return NewValidationError(attr.Name, fmt.Sprintf("field '%s' expects numeric value for type %s, got %T", attr.Name, attr.Type, val))
		}
		if err := validateNumericConstraints(attr.Name, numVal, attr.Validation); err != nil {
			return err
		}
		if attr.Precision > 0 || (attr.Validation != nil && attr.Validation.Precision != nil) {
			prec := attr.Precision
			if attr.Validation != nil && attr.Validation.Precision != nil {
				prec = *attr.Validation.Precision
			}
			scale := attr.Scale
			if attr.Validation != nil && attr.Validation.Scale != nil {
				scale = *attr.Validation.Scale
			}
			if err := validateDecimalPrecisionScale(attr.Name, val, prec, scale); err != nil {
				return err
			}
		}

	case model.TypeBoolean:
		if _, ok := val.(bool); !ok {
			return NewValidationError(attr.Name, fmt.Sprintf("field '%s' expects boolean value, got %T", attr.Name, val))
		}

	case model.TypeDate:
		switch v := val.(type) {
		case time.Time:
			// valid
		case string:
			if _, err := time.Parse("2006-01-02", v); err != nil {
				return NewValidationError(attr.Name, fmt.Sprintf("field '%s' date value '%s' must match format YYYY-MM-DD", attr.Name, v))
			}
		default:
			return NewValidationError(attr.Name, fmt.Sprintf("field '%s' expects date value (YYYY-MM-DD), got %T", attr.Name, val))
		}

	case model.TypeDateTime:
		switch v := val.(type) {
		case time.Time:
			// valid
		case string:
			formats := []string{
				time.RFC3339Nano,
				time.RFC3339,
				"2006-01-02T15:04:05Z07:00",
				"2006-01-02T15:04:05",
				"2006-01-02 15:04:05",
			}
			parsed := false
			for _, f := range formats {
				if _, err := time.Parse(f, v); err == nil {
					parsed = true
					break
				}
			}
			if !parsed {
				return NewValidationError(attr.Name, fmt.Sprintf("field '%s' datetime value '%s' must match ISO 8601 / RFC 3339", attr.Name, v))
			}
		default:
			return NewValidationError(attr.Name, fmt.Sprintf("field '%s' expects datetime value, got %T", attr.Name, val))
		}

	case model.TypeArray:
		items, ok := val.([]any)
		if !ok {
			// Try typed slice like []string
			if strSlice, ok := val.([]string); ok {
				items = make([]any, len(strSlice))
				for i, s := range strSlice {
					items[i] = s
				}
			} else {
				return NewValidationError(attr.Name, fmt.Sprintf("field '%s' expects array/list, got %T", attr.Name, val))
			}
		}
		if attr.Validation != nil && attr.Validation.Items != nil {
			itemRule := attr.Validation.Items
			for i, itm := range items {
				if err := validateArrayItem(attr.Name, i, itm, itemRule); err != nil {
					return err
				}
			}
		}

	case model.TypeJSON:
		switch val.(type) {
		case map[string]any, []any, string:
			// valid JSON object or raw JSON string
		default:
			return NewValidationError(attr.Name, fmt.Sprintf("field '%s' expects JSON object or array, got %T", attr.Name, val))
		}

	case model.TypeEnum:
		// Will be handled in Enum check below

	case model.TypeCustom:
		if attr.CustomType != "" {
			if err := validateCustomTypeConstraint(attr.Name, attr.CustomType, val); err != nil {
				return err
			}
		}

	case model.TypeReference:
		// Handled via UUID / Type check and orbital resolver
		strVal, ok := val.(string)
		if !ok {
			return NewValidationError(attr.Name, fmt.Sprintf("field '%s' expects reference string value, got %T", attr.Name, val))
		}
		if !uuidRegex.MatchString(strVal) {
			return NewValidationError(attr.Name, fmt.Sprintf("field '%s' reference value '%s' is not a valid UUID", attr.Name, strVal))
		}
	}

	// 2. Enum Validation
	if attr.Validation != nil && len(attr.Validation.Enum) > 0 {
		found := false
		for _, enumVal := range attr.Validation.Enum {
			if fmt.Sprintf("%v", enumVal) == fmt.Sprintf("%v", val) {
				found = true
				break
			}
		}
		if !found {
			allowed := make([]string, len(attr.Validation.Enum))
			for i, ev := range attr.Validation.Enum {
				allowed[i] = fmt.Sprintf("%v", ev)
			}
			return NewValidationError(attr.Name, fmt.Sprintf("field '%s' value '%v' is not in allowed enum values (must be one of: %s)", attr.Name, val, strings.Join(allowed, ", ")))
		}
	}

	return nil
}

func validateStringConstraints(fieldName, strVal string, v *model.RuleSet) error {
	if v == nil {
		return nil
	}
	length := utf8.RuneCountInString(strVal)
	if v.MinLength != nil && length < *v.MinLength {
		return NewValidationError(fieldName, fmt.Sprintf("field '%s' length %d is less than min %d", fieldName, length, *v.MinLength))
	}
	if v.MaxLength != nil && length > *v.MaxLength {
		return NewValidationError(fieldName, fmt.Sprintf("field '%s' length %d exceeds max %d", fieldName, length, *v.MaxLength))
	}
	if v.Pattern != "" {
		re, err := regexp.Compile(v.Pattern)
		if err != nil || !re.MatchString(strVal) {
			return NewValidationError(fieldName, fmt.Sprintf("%s does not match pattern '%s'", fieldName, v.Pattern))
		}
	}
	return nil
}

func validateNumericConstraints(fieldName string, numVal float64, v *model.RuleSet) error {
	if v == nil {
		return nil
	}
	if v.Min != nil && numVal < *v.Min {
		return NewValidationError(fieldName, fmt.Sprintf("field '%s' value %v is less than min %v (%s must be >= %v)", fieldName, numVal, *v.Min, fieldName, *v.Min))
	}
	if v.Max != nil && numVal > *v.Max {
		return NewValidationError(fieldName, fmt.Sprintf("field '%s' value %v exceeds max %v (%s must be <= %v)", fieldName, numVal, *v.Max, fieldName, *v.Max))
	}
	return nil
}

func validateDecimalPrecisionScale(fieldName string, val any, precision, scale int) error {
	var str string
	if num, ok := toFloat64(val); ok {
		str = strconv.FormatFloat(num, 'f', -1, 64)
	} else {
		str = fmt.Sprintf("%v", val)
	}
	parts := strings.Split(str, ".")
	intPart := strings.TrimPrefix(parts[0], "-")
	fracPart := ""
	if len(parts) > 1 {
		fracPart = parts[1]
	}

	if scale > 0 && len(fracPart) > scale {
		return NewValidationError(fieldName, fmt.Sprintf("field '%s' decimal scale %d exceeds allowed scale %d", fieldName, len(fracPart), scale))
	}
	totalDigits := len(intPart) + len(fracPart)
	if precision > 0 && totalDigits > precision {
		return NewValidationError(fieldName, fmt.Sprintf("field '%s' decimal total precision %d exceeds allowed precision %d", fieldName, totalDigits, precision))
	}
	return nil
}

func validateArrayItem(fieldName string, idx int, itm any, rule *model.ItemRule) error {
	if rule == nil {
		return nil
	}
	normType := model.NormalizeDataType(string(rule.Type))
	if normType == model.TypeString || normType == model.TypeText {
		strVal, ok := itm.(string)
		if !ok {
			return NewValidationError(fieldName, fmt.Sprintf("field '%s' array item at index %d expects string, got %T", fieldName, idx, itm))
		}
		length := utf8.RuneCountInString(strVal)
		if rule.MinLength != nil && length < *rule.MinLength {
			return NewValidationError(fieldName, fmt.Sprintf("field '%s' array item '%s' length %d is less than min %d", fieldName, strVal, length, *rule.MinLength))
		}
		if rule.MaxLength != nil && length > *rule.MaxLength {
			return NewValidationError(fieldName, fmt.Sprintf("field '%s' array item '%s' length %d exceeds max %d", fieldName, strVal, length, *rule.MaxLength))
		}
		if rule.Pattern != "" {
			if matched, _ := regexp.MatchString(rule.Pattern, strVal); !matched {
				return NewValidationError(fieldName, fmt.Sprintf("field '%s' array item '%s' does not match pattern '%s'", fieldName, strVal, rule.Pattern))
			}
		}
	}
	return nil
}

func validateCustomTypeConstraint(fieldName, customType string, val any) error {
	obj, ok := val.(map[string]any)
	if !ok {
		return NewValidationError(fieldName, fmt.Sprintf("field '%s' expects custom object for type '%s', got %T", fieldName, customType, val))
	}

	switch customType {
	case "geo_point_radius":
		lat, okLat := toFloat64(obj["latitude"])
		lon, okLon := toFloat64(obj["longitude"])
		rad, okRad := toFloat64(obj["radius"])
		if !okLat || !okLon || !okRad {
			return NewValidationError(fieldName, fmt.Sprintf("custom type 'geo_point_radius' requires numeric latitude, longitude, and radius"))
		}
		if lat < -90 || lat > 90 {
			return NewValidationError(fieldName, fmt.Sprintf("latitude must be between -90 and 90, got %v", lat))
		}
		if lon < -180 || lon > 180 {
			return NewValidationError(fieldName, fmt.Sprintf("longitude must be between -180 and 180, got %v", lon))
		}
		if rad <= 0 {
			return NewValidationError(fieldName, fmt.Sprintf("radius must be > 0, got %v", rad))
		}

	case "geo_point":
		lat, okLat := toFloat64(obj["latitude"])
		lon, okLon := toFloat64(obj["longitude"])
		if !okLat || !okLon {
			return NewValidationError(fieldName, fmt.Sprintf("custom type 'geo_point' requires numeric latitude and longitude"))
		}
		if lat < -90 || lat > 90 {
			return NewValidationError(fieldName, fmt.Sprintf("latitude must be between -90 and 90, got %v", lat))
		}
		if lon < -180 || lon > 180 {
			return NewValidationError(fieldName, fmt.Sprintf("longitude must be between -180 and 180, got %v", lon))
		}
	}
	return nil
}

func toFloat64(val any) (float64, bool) {
	switch n := val.(type) {
	case int:
		return float64(n), true
	case int8:
		return float64(n), true
	case int16:
		return float64(n), true
	case int32:
		return float64(n), true
	case int64:
		return float64(n), true
	case uint:
		return float64(n), true
	case uint8:
		return float64(n), true
	case uint16:
		return float64(n), true
	case uint32:
		return float64(n), true
	case uint64:
		return float64(n), true
	case float32:
		return float64(n), true
	case float64:
		return n, true
	case string:
		if f, err := strconv.ParseFloat(n, 64); err == nil {
			return f, true
		}
		return 0, false
	default:
		return 0, false
	}
}

// ValidatePlan checks safety restrictions before executing a schema plan.
func ValidatePlan(p *plan.SchemaPlan, allowDestructive bool) error {
	if p == nil {
		return errors.New("plan is nil")
	}

	if p.Destructive && !allowDestructive {
		return fmt.Errorf("schema plan contains destructive operations (%d warning(s)). Explicit confirmation (allow_destructive=true) is required", len(p.Warnings))
	}

	return nil
}

// ValidateModelConfig validates a model_config metadata entity.
func ValidateModelConfig(cfg *model.ModelConfig) error {
	if cfg == nil {
		return errors.New("model_config cannot be nil")
	}
	if strings.TrimSpace(cfg.Name) == "" {
		return errors.New("model_config name cannot be empty")
	}
	if !validIdentifier.MatchString(cfg.Name) {
		return fmt.Errorf("model_config name '%s' must be a valid identifier", cfg.Name)
	}
	switch cfg.Status {
	case "", model.ModelConfigStatusDraft, model.ModelConfigStatusActive, model.ModelConfigStatusInactive, model.ModelConfigStatusArchived:
		// valid
	default:
		return fmt.Errorf("invalid model_config status '%s'", cfg.Status)
	}
	return nil
}

// ValidateDataModel validates a data_model field definition.
func ValidateDataModel(dm *model.DataModel) error {
	if dm == nil {
		return errors.New("data_model cannot be nil")
	}
	if strings.TrimSpace(dm.ModelID) == "" {
		return errors.New("data_model model_id cannot be empty")
	}
	fieldName := dm.ColumnName
	if fieldName == "" {
		fieldName = dm.JSONField
	}
	if strings.TrimSpace(fieldName) == "" {
		return errors.New("data_model must have column_name or json_field")
	}
	if !validIdentifier.MatchString(fieldName) {
		return fmt.Errorf("data_model field name '%s' must be a valid identifier", fieldName)
	}
	if !isValidDataType(dm.DataType) {
		return fmt.Errorf("data_model '%s' has unsupported data type '%s'", fieldName, dm.DataType)
	}
	if dm.IsOrbitalReference {
		if dm.OrbitalReferenceModelID == nil && dm.Reference == nil {
			return fmt.Errorf("orbital reference field '%s' requires orbital_reference_model_id or reference specification", fieldName)
		}
		if dm.OrbitalReferenceValidation != "" {
			switch dm.OrbitalReferenceValidation {
			case model.OrbitalValidationExists, model.OrbitalValidationExistsActive, model.OrbitalValidationExistsInScope, model.OrbitalValidationNotExists:
				// valid
			default:
				return fmt.Errorf("invalid orbital_reference_validation '%s' for field '%s'", dm.OrbitalReferenceValidation, fieldName)
			}
		}
	}
	return nil
}

// ValidateCustomType ensures that custom_type_id references a valid model marked with is_attribute_reference = true.
func ValidateCustomType(getRefModel func(idOrName string) (*model.ModelConfig, error), dm *model.DataModel) error {
	if dm == nil || dm.CustomTypeID == nil || *dm.CustomTypeID == "" {
		return nil
	}
	targetID := *dm.CustomTypeID
	targetConfig, err := getRefModel(targetID)
	if err != nil {
		return fmt.Errorf("custom_type_id reference error for field '%s': referenced model '%s' not found: %w", dm.ColumnName, targetID, err)
	}
	if !targetConfig.IsAttributeReference {
		return fmt.Errorf("custom_type_id '%s' references model '%s' which is not marked with is_attribute_reference = true", targetID, targetConfig.Name)
	}
	return nil
}

func isValidDataType(t model.DataType) bool {
	norm := model.NormalizeDataType(string(t))
	switch norm {
	case model.TypeString, model.TypeText, model.TypeInt, model.TypeLong,
		model.TypeFloat, model.TypeDecimal, model.TypeBoolean, model.TypeDateTime,
		model.TypeDate, model.TypeTime, model.TypeJSON, model.TypeUUID,
		model.TypeBinary, model.TypeArray, model.TypeEnum, model.TypeEmail,
		model.TypeCustom, model.TypeReference:
		return true
	default:
		return false
	}
}
