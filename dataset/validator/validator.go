package validator

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/SanjayDrop5528/models-go-engine/dataset/domain"
	"github.com/SanjayDrop5528/models-go-engine/dataset/resolver"
)

var validIdentifierRe = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_\.]*$`)

// DataSetValidator performs comprehensive, security-hardened validation of dataset definitions.
type DataSetValidator struct {
	modelResolver    resolver.ModelResolver
	fieldResolver    resolver.FieldResolver
	functionResolver resolver.FunctionResolver
}

// NewDataSetValidator creates a new validator instance.
func NewDataSetValidator(mr resolver.ModelResolver, fr resolver.FieldResolver, fnr resolver.FunctionResolver) *DataSetValidator {
	return &DataSetValidator{
		modelResolver:    mr,
		fieldResolver:    fr,
		functionResolver: fnr,
	}
}

// Validate executes all validation passes on the dataset.
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
		return domain.NewErrorf(domain.ErrDataSetNotFound, "invalid reference_name '%s': must match [a-zA-Z_][a-zA-Z0-9_]*", ds.ReferenceName)
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

	// 2. Base Collection
	if strings.TrimSpace(ds.BaseCollection.Collection) == "" {
		return domain.NewError(domain.ErrModelNotFound, "base_collection 'collection' name is required")
	}
	if !validIdentifierRe.MatchString(ds.BaseCollection.Collection) {
		return domain.NewErrorf(domain.ErrModelNotFound, "invalid base collection name '%s'", ds.BaseCollection.Collection)
	}

	if _, err := v.modelResolver.ResolveModel(ctx, ds.BaseCollection.Schema, ds.BaseCollection.Collection); err != nil {
		return domain.WrapError(domain.ErrModelNotFound, fmt.Sprintf("base collection '%s' failed to resolve", ds.BaseCollection.Collection), err)
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
		for _, g := range ds.GroupByFields {
			colName := g.FieldName
			if colName == "" {
				colName = g.Name
			}
			groupedFields[strings.ToLower(colName)] = true
		}

		// Ensure non-aggregated selected fields belong to GROUP BY
		for _, sel := range ds.SelectedList {
			// Check if this selected field is generated by an aggregate custom column
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
			if !isAgg {
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
