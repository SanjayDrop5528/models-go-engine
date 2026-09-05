package planner

import (
	"context"
	"strconv"
	"strings"

	"github.com/SanjayDrop5528/models-go-engine/dataset/domain"
	"github.com/SanjayDrop5528/models-go-engine/dataset/resolver"
)

// DataSetPlanner converts validated DataSet domain models into QueryAST execution plans.
type DataSetPlanner struct {
	functionResolver resolver.FunctionResolver
}

// NewPlanner creates a new DataSetPlanner.
func NewPlanner(fnr resolver.FunctionResolver) *DataSetPlanner {
	return &DataSetPlanner{
		functionResolver: fnr,
	}
}

// BuildAST creates the QueryAST from a dataset definition.
func (p *DataSetPlanner) BuildAST(ctx context.Context, ds *domain.DataSet) (*QueryAST, error) {
	ast := &QueryAST{
		BaseTable: ASTBaseTable{
			Schema: ds.BaseCollection.Schema,
			Table:  ds.BaseCollection.Collection,
			Alias:  ds.BaseCollection.Collection,
			Filter: ds.BaseCollection.Filter,
		},
		Parameters: ds.FilterParams,
	}

	// 1. Map Joins
	for _, j := range ds.JoinCollections {
		jType := j.JoinType
		if jType == "" {
			jType = domain.JoinLeft
		}
		alias := j.NamedAs
		if alias == "" {
			alias = j.ToCollection
		}
		fromTbl := j.FromCollection
		if fromTbl == "" {
			fromTbl = ds.BaseCollection.Collection
		}

		ast.Joins = append(ast.Joins, ASTJoin{
			Schema:        j.Schema,
			Type:          jType,
			FromTable:     fromTbl,
			FromField:     j.FromCollectionField,
			ToTable:       j.ToCollection,
			ToField:       j.ToCollectionField,
			Alias:         alias,
			JoinFilter:    j.Filter,
			ConvertString: j.ConvertToString,
			CastMode:      j.CastMode,
		})
	}

	// 2. Map Group By first to establish grouping criteria
	for _, g := range ds.GroupByFields {
		tbl := g.TableName
		if tbl == "" {
			tbl = ds.BaseCollection.Collection
		}
		fld := g.FieldName
		if fld == "" {
			fld = g.Name
		}
		if fld != "" {
			ast.GroupBy = append(ast.GroupBy, ASTGroupBy{
				Table: tbl,
				Field: fld,
			})
		}
	}

	// Check if any custom column contains aggregate functions (SUM, AVG, COUNT, MIN, MAX, etc.)
	hasAggregate := false
	for _, cc := range ds.CustomColumns {
		fnName := strings.ToUpper(cc.CustomAggregateFnName)
		if fnName == "SUM" || fnName == "AVG" || fnName == "COUNT" || fnName == "COUNT_DISTINCT" || fnName == "MIN" || fnName == "MAX" || fnName == "COUNT_ALL" {
			hasAggregate = true
			break
		}
		if fn, err := p.functionResolver.ResolveFunction(ctx, cc.CustomAggregateFnName); err == nil && fn.IsAggregate {
			hasAggregate = true
			break
		}
	}

	// 3. Map Projections (SelectedList)
	if len(ast.GroupBy) > 0 {
		groupedMap := make(map[string]bool)
		for _, g := range ast.GroupBy {
			groupedMap[strings.ToLower(g.Field)] = true
			groupedMap[strings.ToLower(g.Table+"."+g.Field)] = true
		}

		for _, sel := range ds.SelectedList {
			tbl := ds.BaseCollection.Collection
			fld := sel.Field
			if idx := strings.Index(sel.Field, "."); idx >= 0 {
				tbl = sel.Field[:idx]
				fld = sel.Field[idx+1:]
			}
			fullKey := strings.ToLower(tbl + "." + fld)
			shortKey := strings.ToLower(fld)
			if groupedMap[fullKey] || groupedMap[shortKey] {
				ast.Projections = append(ast.Projections, ASTProjection{
					SourceTable: tbl,
					SourceField: fld,
					Alias:       sel.HeaderName,
					DataType:    sel.DataType,
				})
			}
		}

		// Ensure all GroupBy dimensions are present in Projections if none were matched
		for _, g := range ast.GroupBy {
			alreadyMapped := false
			for _, p := range ast.Projections {
				if strings.EqualFold(p.SourceTable, g.Table) && strings.EqualFold(p.SourceField, g.Field) {
					alreadyMapped = true
					break
				}
			}
			if !alreadyMapped {
				ast.Projections = append(ast.Projections, ASTProjection{
					SourceTable: g.Table,
					SourceField: g.Field,
					Alias:       g.Field,
				})
			}
		}
	} else if !hasAggregate {
		for _, sel := range ds.SelectedList {
			tbl := ds.BaseCollection.Collection
			fld := sel.Field
			if idx := strings.Index(sel.Field, "."); idx >= 0 {
				tbl = sel.Field[:idx]
				fld = sel.Field[idx+1:]
			}
			ast.Projections = append(ast.Projections, ASTProjection{
				SourceTable: tbl,
				SourceField: fld,
				Alias:       sel.HeaderName,
				DataType:    sel.DataType,
			})
		}
	}

	// 4. Map Custom Columns & Functions
	for _, cc := range ds.CustomColumns {
		astCol := ASTCustomColumn{
			Alias:      cc.CustomColumnName,
			Label:      cc.CustomLabelName,
			Expression: cc.Expression,
			DataType:   cc.Type,
		}

		if cc.CustomAggregateFnName != "" {
			if fn, err := p.functionResolver.ResolveFunction(ctx, cc.CustomAggregateFnName); err == nil {
				astCol.Function = fn
				astCol.IsAggregate = fn.IsAggregate
			}
		}

		for _, f := range cc.Fields {
			tbl := f.TableName
			fldName := f.FieldName
			if fldName == "" {
				fldName = f.Name
			}
			if fldName == "" && f.Value != "" {
				fldName = f.Value
			}
			isLit := f.IsLiteral || tbl == "" || tbl == "_LITERAL_"
			isCustomRef := false
			if !isLit {
				// Check if fldName references another custom column in ds.CustomColumns
				for _, prevCC := range ds.CustomColumns {
					if prevCC.CustomColumnName != "" && strings.EqualFold(prevCC.CustomColumnName, fldName) {
						isCustomRef = true
						break
					}
				}
				if isCustomRef {
					tbl = ""
				} else {
					// Check if fldName is a plain numeric constant (e.g. 5, 2, 12, 0.05)
					trimmed := strings.TrimSpace(fldName)
					if isNumericString(trimmed) {
						isLit = true
						tbl = ""
					}
				}
			} else {
				tbl = ""
			}
			if tbl == "" && !isLit && !isCustomRef {
				tbl = ds.BaseCollection.Collection
			}
			astCol.Operands = append(astCol.Operands, ASTOperand{
				SourceTable: tbl,
				SourceField: fldName,
				IsLiteral:   isLit || isCustomRef,
				LiteralVal:  fldName,
			})
		}

		ast.CustomColumns = append(ast.CustomColumns, astCol)
	}

	// 5. Map Where Filters
	if len(ds.Filter) > 0 {
		ast.WhereFilters = p.parseFilterMap(ds.Filter, ds.BaseCollection.Collection)
	}

	return ast, nil
}

func (p *DataSetPlanner) parseFilterMap(filter map[string]any, defaultTable string) []ASTCondition {
	var conditions []ASTCondition
	for k, v := range filter {
		tbl := defaultTable
		col := k
		if idx := strings.Index(k, "."); idx >= 0 {
			tbl = k[:idx]
			col = k[idx+1:]
		}

		cond := ASTCondition{
			Table:    tbl,
			Column:   col,
			Operator: "EQUALS",
			Value:    v,
		}

		// Check if value is a parameter reference
		if valMap, ok := v.(map[string]any); ok {
			if pName, exists := valMap["ParamsName"].(string); exists {
				cond.IsParamRef = true
				cond.ParamName = pName
				if pType, typeExists := valMap["parmsDataType"].(string); typeExists {
					cond.ParamDataType = pType
				}
			}
		}

		conditions = append(conditions, cond)
	}
	return conditions
}

func isNumericString(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	_, err := strconv.ParseFloat(s, 64)
	return err == nil
}
