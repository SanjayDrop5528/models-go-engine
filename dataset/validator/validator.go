// Package validator provides deep structural, semantic, and security validation
// for Dataset Studio definitions prior to planning and code generation.
//
// File: validator.go
// Usage:
//
//	This file implements the comprehensive DataSetValidator. It validates that dataset metadata,
//	collection names, relational join paths, custom column formulas, operand counts, GROUP BY
//	field constraints, and filter parameter declarations are valid, secure, and compatible
//	with the target database dialect before any AST planning or SQL/MongoDB compilation occurs.
package validator

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/SanjayDrop5528/models-go-engine/dataset/domain"
	"github.com/SanjayDrop5528/models-go-engine/dataset/resolver"
)

var validIdentifierRe = regexp.MustCompile(`^[a-zA-Z0-9_\-][a-zA-Z0-9_\-\.]*$`)

// DataSetValidator performs comprehensive, security-hardened validation of dataset definitions.
type DataSetValidator struct {
	modelResolver    resolver.ModelResolver
	fieldResolver    resolver.FieldResolver
	functionResolver resolver.FunctionResolver
}

// NewDataSetValidator creates a new validator instance.
//
// Purpose:
//
//	Constructs a DataSetValidator wired to resolvers for models, fields, and functions.
//
// Where it is used:
//   - Instantiated in DataSetService.NewDataSetService.
//   - Used directly in validation unit tests.
//
// When can it be used:
//   - Can be used before any dataset is saved, previewed, or executed.
//
// Parameters:
//   - mr (resolver.ModelResolver): Input value used by the operation.
//   - fr (resolver.FieldResolver): Input value used by the operation.
//   - fnr (resolver.FunctionResolver): Input value used by the operation.
//
// Returns:
//   - result1 (*DataSetValidator): Returned value.
//
// Business Rules:
//   - Follows the invariants documented by NewDataSetValidator and its owning package.
//
// Side Effects:
//   - Does not intentionally mutate caller-owned input; configured dependencies may still be consulted.
//
// Errors:
//   - This function does not return an error.
//
// Note:
//   - Callers should preserve the documented ownership and lifecycle of supplied values.
func NewDataSetValidator(mr resolver.ModelResolver, fr resolver.FieldResolver, fnr resolver.FunctionResolver) *DataSetValidator {
	return &DataSetValidator{
		modelResolver:    mr,
		fieldResolver:    fr,
		functionResolver: fnr,
	}
}

// Validate executes all validation passes on the dataset.
//
// Purpose:
//
//	Executes all structural, semantic, relational, and aggregation integrity checks on a DataSet.
//	Ensures valid identifiers, resolvable tables, ordered joins, valid operand counts, GROUP BY consistency,
//	and unique non-empty parameter names.
//
// Where it is used:
//   - Invoked as the first step in DataSetService.Preview.
//   - Invoked as the first step in DataSetService.Save.
//   - Invoked as the first step in DataSetService.Execute.
//
// When can it be used:
//   - Whenever a raw DataSet JSON payload is received from the UI or API and needs verification.
//
// Parameters:
//   - ctx (context.Context): Input value used by the operation.
//   - ds (*domain.DataSet): Input value used by the operation.
//
// Returns:
//   - result1 (error): Returned value.
//
// Business Rules:
//   - Follows the invariants documented by Validate and its owning package.
//
// Side Effects:
//   - Does not intentionally mutate caller-owned input; configured dependencies may still be consulted.
//
// Errors:
//   - Returns validation, configuration, context, or underlying dependency errors.
//
// Note:
//   - The context controls cancellation and request lifetime.
func (v *DataSetValidator) Validate(ctx context.Context, ds *domain.DataSet) error {
	if ds == nil {
		return domain.NewError(domain.ErrDataSetNotFound, "dataset definition cannot be nil")
	}

	// 1. Dataset metadata
	if strings.TrimSpace(ds.Name) == "" {
		return domain.NewError(domain.ErrDataSetNotFound, "dataset 'name' is required")
	}
	if strings.TrimSpace(ds.ReferenceName) == "" {
		return domain.NewError(domain.ErrDataSetNotFound, "dataset 'reference_name' is required")
	}
	if !validIdentifierRe.MatchString(ds.ReferenceName) {
		return domain.NewErrorf(domain.ErrDataSetNotFound, "invalid reference_name '%s': must match [a-zA-Z0-9_-][a-zA-Z0-9_.-]*", ds.ReferenceName)
	}

	driver := strings.ToLower(strings.TrimSpace(ds.Driver))
	if driver != "postgres" && driver != "mysql" && driver != "mongodb" && driver != "memory" {
		return domain.NewErrorf(domain.ErrUnsupportedDriver, "unsupported driver '%s': must be postgres, mysql, mongodb, or memory", ds.Driver)
	}

	if ds.SaveMode == "" {
		ds.SaveMode = domain.SaveModeQuery
	}
	if ds.SaveMode != domain.SaveModeProcedure && ds.SaveMode != domain.SaveModeFunction && ds.SaveMode != domain.SaveModeQuery {
		return domain.NewErrorf(domain.ErrUnsupportedSaveMode, "unsupported save mode '%s': must be PROCEDURE, FUNCTION, or QUERY", ds.SaveMode)
	}

	// If this is a Raw Dataset (directly supplied SQL or native aggregation pipeline),
	// validate the raw pipeline/query and parameters without requiring visual builder steps.
	if ds.IsRawDataset() {
		if strings.TrimSpace(ds.Pipeline) == "" && strings.TrimSpace(ds.ReferencePipeline) == "" {
			if !strings.EqualFold(ds.Driver, "mongodb") {
				return domain.NewError(domain.ErrFieldNotFound, "raw dataset requires 'pipeline' or 'reference_pipeline'")
			}
		}
		if strings.TrimSpace(ds.BaseCollection.Collection) != "" {
			if !validIdentifierRe.MatchString(ds.BaseCollection.Collection) {
				return domain.NewErrorf(domain.ErrModelNotFound, "invalid base collection name '%s'", ds.BaseCollection.Collection)
			}
		}
		paramNames := make(map[string]bool)
		for _, p := range ds.FilterParams {
			pName := strings.TrimSpace(p.ParamName)
			if pName == "" {
				return domain.NewError(domain.ErrFieldNotFound, "filter_param 'paramName' is required")
			}
			if paramNames[strings.ToLower(pName)] {
				return domain.NewErrorf(domain.ErrDuplicateField, "duplicate filter_param '%s'", pName)
			}
			paramNames[strings.ToLower(pName)] = true
		}
		return nil
	}

	// 2. Base Collection
	if strings.TrimSpace(ds.BaseCollection.Collection) == "" {
		return domain.NewError(domain.ErrModelNotFound, "base_collection 'collection' name is required")
	}
	if !validIdentifierRe.MatchString(ds.BaseCollection.Collection) {
		return domain.NewErrorf(domain.ErrModelNotFound, "invalid base collection name '%s'", ds.BaseCollection.Collection)
	}

	baseDef, err := v.modelResolver.ResolveModel(ctx, ds.BaseCollection.Schema, ds.BaseCollection.Collection)
	if err != nil {
		return domain.WrapError(domain.ErrModelNotFound, fmt.Sprintf("base collection '%s' failed to resolve", ds.BaseCollection.Collection), err)
	}
	if baseDef != nil && baseDef.IsAttributeReference {
		return domain.NewErrorf(domain.ErrModelNotFound, "base collection '%s' cannot be an attribute reference model (is_attribute_reference=true)", ds.BaseCollection.Collection)
	}

	// 3. Joins
	knownAliases := map[string]bool{
		strings.ToLower(ds.BaseCollection.Collection): true,
	}
	if ds.BaseCollection.Schema != "" {
		knownAliases[strings.ToLower(fmt.Sprintf("%s.%s", ds.BaseCollection.Schema, ds.BaseCollection.Collection))] = true
	}

	for i, j := range ds.JoinCollections {
		if strings.TrimSpace(j.ToCollection) == "" {
			return domain.NewErrorf(domain.ErrInvalidJoin, "join[%d] 'toCollection' is required", i)
		}
		targetDef, err := v.modelResolver.ResolveModel(ctx, j.Schema, j.ToCollection)
		if err == nil && targetDef != nil && targetDef.IsAttributeReference {
			return domain.NewErrorf(domain.ErrInvalidJoin, "join[%d] target collection '%s' cannot be an attribute reference model (is_attribute_reference=true)", i, j.ToCollection)
		}
		if strings.TrimSpace(j.FromCollectionField) == "" {
			return domain.NewErrorf(domain.ErrInvalidJoin, "join[%d] 'fromCollectionField' is required", i)
		}
		if strings.TrimSpace(j.ToCollectionField) == "" {
			return domain.NewErrorf(domain.ErrInvalidJoin, "join[%d] 'toCollectionField' is required", i)
		}

		alias := strings.TrimSpace(j.NamedAs)
		if alias == "" {
			alias = j.ToCollection
		}
		aliasLower := strings.ToLower(alias)
		if knownAliases[aliasLower] {
			return domain.NewErrorf(domain.ErrInvalidJoin, "duplicate join alias '%s'", alias)
		}
		knownAliases[aliasLower] = true

		if j.FromCollection != "" && !knownAliases[strings.ToLower(j.FromCollection)] {
			return domain.NewErrorf(domain.ErrInvalidJoin, "join[%d] fromCollection '%s' has not been declared before this join in join ordering", i, j.FromCollection)
		}
	}

	// 4. Custom Columns & Functions
	for i, col := range ds.CustomColumns {
		if strings.TrimSpace(col.CustomColumnName) == "" {
			return domain.NewErrorf(domain.ErrFieldNotFound, "custom_columns[%d] 'customColumnName' is required", i)
		}

		if col.CustomAggregateFnName != "" {
			fn, err := v.functionResolver.ResolveFunction(ctx, col.CustomAggregateFnName)
			if err != nil {
				return err
			}
			operandCount := len(col.Fields)
			if operandCount == 0 && col.Expression != "" {
				operandCount = 1
			}
			if fn.MinOperands > 0 && operandCount < fn.MinOperands {
				return domain.NewErrorf(domain.ErrInvalidOperandCount, "function '%s' requires at least %d operands, got %d", fn.Name, fn.MinOperands, operandCount)
			}
			if fn.MaxOperands > 0 && operandCount > fn.MaxOperands {
				return domain.NewErrorf(domain.ErrInvalidOperandCount, "function '%s' accepts at most %d operands, got %d", fn.Name, fn.MaxOperands, operandCount)
			}
		}
	}

	// 5. Group By validation
	if len(ds.GroupByFields) > 0 {
		groupedFields := make(map[string]bool)
		validCount := 0
		for _, g := range ds.GroupByFields {
			colName := g.FieldName
			if colName == "" {
				colName = g.Name
			}
			if colName != "" {
				validCount++
				groupedFields[strings.ToLower(colName)] = true
				if g.TableName != "" {
					groupedFields[strings.ToLower(g.TableName+"."+colName)] = true
				}
			}
		}

		if validCount > 0 {
			// Check if any explicitly selected field (that is NOT auto-selected) violates grouping
			// Note: The planner prunes un-grouped flat projections automatically for SQL & Mongo.
			for _, sel := range ds.SelectedList {
				if sel.Field == "" {
					continue
				}
				isAgg := false
				for _, cc := range ds.CustomColumns {
					if strings.EqualFold(cc.CustomColumnName, sel.Field) || strings.EqualFold(cc.CustomLabelName, sel.Field) {
						if cc.CustomAggregateFnName != "" {
							if fn, err := v.functionResolver.ResolveFunction(ctx, cc.CustomAggregateFnName); err == nil && fn.IsAggregate {
								isAgg = true
							}
						}
						break
					}
				}
				if isAgg {
					continue
				}
				fName := sel.Field
				if idx := strings.LastIndex(fName, "."); idx >= 0 {
					fName = fName[idx+1:]
				}
				if !groupedFields[strings.ToLower(fName)] && !groupedFields[strings.ToLower(sel.Field)] {
					return domain.NewErrorf(domain.ErrInvalidGroupBy, "selected non-aggregated column '%s' must be present in GROUP BY", sel.Field)
				}
			}
		}
	}

	// 6. Filter Params validation
	paramMap := make(map[string]bool)
	for i, p := range ds.FilterParams {
		if strings.TrimSpace(p.ParamName) == "" {
			return domain.NewErrorf(domain.ErrInvalidFilterParameter, "filter_params[%d] 'paramName' is required", i)
		}
		pName := strings.ToLower(p.ParamName)
		if paramMap[pName] {
			return domain.NewErrorf(domain.ErrInvalidFilterParameter, "duplicate filter parameter '%s'", p.ParamName)
		}
		paramMap[pName] = true
	}

	return nil
}
