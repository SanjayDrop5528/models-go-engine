// Package planner converts validated domain datasets into query abstract syntax trees (QueryAST).
//
// File: planner.go
// Usage:
//   This file implements the core query planner (DataSetPlanner) for Dataset Studio.
//   It takes a validated DataSet domain model, resolves table aliases, organizes relational
//   joins, separates scalar projections from grouped/aggregate dimensions, builds custom
//   column calculation ASTs, parses WHERE filter conditions and runtime parameters, and emits
//   an optimized, dialect-independent QueryAST ready for adapter compilation.
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
//
// Purpose:
//   Initializes a DataSetPlanner with the provided FunctionResolver for function and aggregation introspection.
//
// Where it is used:
//   - Instantiated in DataSetService.NewDataSetService.
//   - Used in unit tests for planning and query AST generation.
//
// When can it be used:
//   - Can be used whenever dataset definitions need to be compiled into executable query plans.
func NewPlanner(fnr resolver.FunctionResolver) *DataSetPlanner {
	return &DataSetPlanner{
		functionResolver: fnr,
	}
}

// BuildAST creates the QueryAST from a dataset definition.
//
// Purpose:
//   Transforms an abstract DataSet domain struct into a fully-resolved, vendor-independent QueryAST.
//   Resolves aliases, organizes joins, normalizes GROUP BY fields, and binds parameter definitions.
//
// Where it is used:
//   - Called by DataSetService.Preview to prepare AST for live preview compilation.
//   - Called by DataSetService.Save to compile DDL routines or parameterized queries for storage.
//   - Called by DataSetService.Execute for direct query evaluation.
//
// When can it be used:
//   - Can be used after a DataSet definition has passed validator.Validate.
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

	tableAliases := map[string]string{
		strings.ToLower(ds.BaseCollection.Collection): ds.BaseCollection.Collection,
	}
	if ds.BaseCollection.Schema != "" {
		tableAliases[strings.ToLower(ds.BaseCollection.Schema+"."+ds.BaseCollection.Collection)] = ds.BaseCollection.Collection
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
		fromTbl = resolveTableAlias(fromTbl, tableAliases)

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
		tableAliases[strings.ToLower(j.ToCollection)] = alias
		if j.Schema != "" {
			tableAliases[strings.ToLower(j.Schema+"."+j.ToCollection)] = alias
		}
	}

	// 2. Map Group By first to establish grouping criteria
	for _, g := range ds.GroupByFields {
		tbl := g.TableName
		if tbl == "" {
			tbl = ds.BaseCollection.Collection
		}
		tbl = resolveTableAlias(tbl, tableAliases)
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

	// Check if any custom column contains aggregate functions (SUM, AVG, COUNT, MIN, MAX, COUNT_IF, SUM_IF, etc.)
	hasAggregate := false
	for _, cc := range ds.CustomColumns {
		if isAggregateFunctionName(cc.CustomAggregateFnName) {
			hasAggregate = true
			break
		}
		if p.functionResolver != nil {
			if fn, err := p.functionResolver.ResolveFunction(ctx, cc.CustomAggregateFnName); err == nil && fn.IsAggregate {
				hasAggregate = true
				break
			}
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
			tbl = resolveTableAlias(tbl, tableAliases)
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
			// Check if sel.Field matches a custom column; custom columns are handled separately in ast.CustomColumns
			isCustom := false
			for _, cc := range ds.CustomColumns {
				if strings.EqualFold(cc.CustomColumnName, sel.Field) || strings.EqualFold(cc.CustomLabelName, sel.Field) {
					isCustom = true
					break
				}
			}
			if isCustom {
				continue
			}

			tbl := ds.BaseCollection.Collection
			fld := sel.Field
			if idx := strings.Index(sel.Field, "."); idx >= 0 {
				tbl = sel.Field[:idx]
				fld = sel.Field[idx+1:]
			}
			tbl = resolveTableAlias(tbl, tableAliases)
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
			Alias:        cc.CustomColumnName,
			Label:        cc.CustomLabelName,
			FunctionName: cc.CustomAggregateFnName,
			Expression:   cc.Expression,
			DataType:     cc.Type,
		}

		if cc.CustomAggregateFnName != "" && p.functionResolver != nil {
			if fn, err := p.functionResolver.ResolveFunction(ctx, cc.CustomAggregateFnName); err == nil {
				astCol.Function = fn
				astCol.IsAggregate = fn.IsAggregate
			} else if isAggregateFunctionName(cc.CustomAggregateFnName) {
				astCol.IsAggregate = true
			}
		} else if isAggregateFunctionName(cc.CustomAggregateFnName) {
			astCol.IsAggregate = true
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
			if tbl != "" {
				tbl = resolveTableAlias(tbl, tableAliases)
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
		ast.WhereFilters = p.parseFilterMap(ds.Filter, ds.BaseCollection.Collection, tableAliases)
	}

	return ast, nil
}

// resolveTableAlias determines the effective alias or table name for a collection reference.
//
// Purpose:
//   Maps schema-qualified names or raw table names to their registered join aliases.
//
// Where it is used:
//   - Used throughout BuildAST when resolving join targets, projection sources, and custom column fields.
//
// When can it be used:
//   - Can be used whenever resolving an ambiguous column's source table or alias.
func resolveTableAlias(table string, aliases map[string]string) string {
	if table == "" {
		return table
	}
	if alias, ok := aliases[strings.ToLower(table)]; ok {
		return alias
	}
	if idx := strings.LastIndex(table, "."); idx >= 0 {
		if alias, ok := aliases[strings.ToLower(table[idx+1:])]; ok {
			return alias
		}
	}
	return table
}

// isAggregateFunctionName checks if a function name represents a standard SQL aggregate function.
//
// Purpose:
//   Quickly identifies aggregate functions (SUM, AVG, COUNT, MIN, MAX, etc.) without registry lookup.
//
// Where it is used:
//   - Used by BuildAST to determine if a query requires GROUP BY semantics and projection pruning.
//
// When can it be used:
//   - Can be used during AST building to detect aggregation vs scalar function calls.
func isAggregateFunctionName(name string) bool {
	switch strings.ToUpper(strings.TrimSpace(name)) {
	case "SUM", "AVG", "COUNT", "COUNT_DISTINCT", "MIN", "MAX", "COUNT_ALL", "COUNT(*)", "COUNT_IF", "SUM_IF":
		return true
	default:
		return false
	}
}

// parseFilterMap converts a raw map-based filter definition into a list of ASTCondition structs.
//
// Purpose:
//   Deconstructs key-value filters (e.g. "employees.department_id": {"paramName": "dept_id", ...})
//   into strongly-typed ASTCondition objects, detecting whether a value is a static literal or a parameter reference.
//
// Where it is used:
//   - Called by BuildAST to parse WHERE filter dictionaries from ds.Filter.
//
// When can it be used:
//   - When translating raw JSON filter maps into relational filter AST nodes.
func (p *DataSetPlanner) parseFilterMap(filter map[string]any, defaultTable string, tableAliases map[string]string) []ASTCondition {
	var conditions []ASTCondition
	for k, v := range filter {
		tbl := defaultTable
		col := k
		if idx := strings.Index(k, "."); idx >= 0 {
			tbl = k[:idx]
			col = k[idx+1:]
		}
		tbl = resolveTableAlias(tbl, tableAliases)

		cond := ASTCondition{
			Table:    tbl,
			Column:   col,
			Operator: "EQUALS",
			Value:    v,
		}

		// Check if value is a parameter reference
		if valMap, ok := v.(map[string]any); ok {
			pName := ""
			if pn, exists := valMap["paramName"].(string); exists {
				pName = pn
			} else if pn, exists := valMap["ParamsName"].(string); exists {
				pName = pn
			}
			if pName != "" {
				cond.IsParamRef = true
				cond.ParamName = pName
				if pType, typeExists := valMap["paramDataType"].(string); typeExists {
					cond.ParamDataType = pType
				} else if pType, typeExists := valMap["parmsDataType"].(string); typeExists {
					cond.ParamDataType = pType
				}
			}
		}

		conditions = append(conditions, cond)
	}
	return conditions
}

// isNumericString determines if a string literal contains a purely numeric value.
//
// Purpose:
//   Distinguishes numeric literals (e.g. "100", "0.05") from column identifiers in custom expressions.
//
// Where it is used:
//   - Used in BuildAST when analyzing custom column formula operands.
//
// When can it be used:
//   - When determining whether a custom formula token is a table column or a constant multiplier/threshold.
func isNumericString(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	_, err := strconv.ParseFloat(s, 64)
	return err == nil
}
