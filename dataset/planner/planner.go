// Package planner converts validated domain datasets into query abstract syntax trees (QueryAST).
//
// File: planner.go
// Usage:
//
//	This file implements the core query planner (DataSetPlanner) for Dataset Studio.
//	It takes a validated DataSet domain model, resolves table aliases, organizes relational
//	joins, separates scalar projections from grouped/aggregate dimensions, builds custom
//	column calculation ASTs, parses WHERE filter conditions and runtime parameters, and emits
//	an optimized, dialect-independent QueryAST ready for adapter compilation.
package planner

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/SanjayDrop5528/models-go-engine/dataset/domain"
	"github.com/SanjayDrop5528/models-go-engine/dataset/resolver"
)

// DataSetPlanner converts validated DataSet domain models into QueryAST execution plans.
type DataSetPlanner struct {
	functionResolver resolver.FunctionResolver
	modelResolver    resolver.ModelResolver
	fieldResolver    resolver.FieldResolver
}

// NewMetadataPlanner resolves JOIN fields from server metadata before compilation.
func NewMetadataPlanner(fnr resolver.FunctionResolver, mr resolver.ModelResolver, fr resolver.FieldResolver) *DataSetPlanner {
	return &DataSetPlanner{functionResolver: fnr, modelResolver: mr, fieldResolver: fr}
}

// NewPlanner creates a new DataSetPlanner.
//
// Purpose:
//
//	Initializes a DataSetPlanner with the provided FunctionResolver for function and aggregation introspection.
//
// Where it is used:
//   - Instantiated in DataSetService.NewDataSetService.
//   - Used in unit tests for planning and query AST generation.
//
// When can it be used:
//   - Can be used whenever dataset definitions need to be compiled into executable query plans.
//
// Parameters:
//   - fnr (resolver.FunctionResolver): Input value used by the operation.
//
// Returns:
//   - result1 (*DataSetPlanner): Returned value.
//
// Business Rules:
//   - Follows the invariants documented by NewPlanner and its owning package.
//
// Side Effects:
//   - Does not intentionally mutate caller-owned input; configured dependencies may still be consulted.
//
// Errors:
//   - This function does not return an error.
//
// Note:
//   - Callers should preserve the documented ownership and lifecycle of supplied values.
func NewPlanner(fnr resolver.FunctionResolver) *DataSetPlanner {
	return &DataSetPlanner{
		functionResolver: fnr,
	}
}

// BuildAST creates the QueryAST from a dataset definition.
//
// Purpose:
//
//	Transforms an abstract DataSet domain struct into a fully-resolved, vendor-independent QueryAST.
//	Resolves aliases, organizes joins, normalizes GROUP BY fields, and binds parameter definitions.
//
// Where it is used:
//   - Called by DataSetService.Preview to prepare AST for live preview compilation.
//   - Called by DataSetService.Save to compile DDL routines or parameterized queries for storage.
//   - Called by DataSetService.Execute for direct query evaluation.
//
// When can it be used:
//   - Can be used after a DataSet definition has passed validator.Validate.
//
// Parameters:
//   - ctx (context.Context): Input value used by the operation.
//   - ds (*domain.DataSet): Input value used by the operation.
//
// Returns:
//   - result1 (*QueryAST): Returned value.
//   - result2 (error): Returned value.
//
// Business Rules:
//   - Follows the invariants documented by BuildAST and its owning package.
//
// Side Effects:
//   - Does not intentionally mutate caller-owned input; configured dependencies may still be consulted.
//
// Errors:
//   - Returns validation, configuration, context, or underlying dependency errors.
//
// Note:
//   - The context controls cancellation and request lifetime.
func (p *DataSetPlanner) BuildAST(ctx context.Context, ds *domain.DataSet) (*QueryAST, error) {
	ast := &QueryAST{
		BaseTable: ASTBaseTable{
			Schema: ds.BaseCollection.Schema,
			Table:  ds.BaseCollection.Collection,
			Alias:  ds.BaseCollection.Collection,
			Filter: normalizeFilterToMap(ds.BaseCollection.Filter),
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

		fromDataType := j.FromFieldDataType
		toDataType := j.ToFieldDataType
		if fromDataType == "" || toDataType == "" {
			for _, sel := range ds.SelectedList {
				fldLower := strings.ToLower(sel.Field)
				fromFull := strings.ToLower(fromTbl + "." + j.FromCollectionField)
				fromColl := strings.ToLower(j.FromCollection + "." + j.FromCollectionField)
				if fromDataType == "" && (fldLower == fromFull || fldLower == fromColl || strings.EqualFold(sel.Field, j.FromCollectionField)) {
					fromDataType = sel.DataType
				}
				toFull := strings.ToLower(alias + "." + j.ToCollectionField)
				toColl := strings.ToLower(j.ToCollection + "." + j.ToCollectionField)
				if toDataType == "" && (fldLower == toFull || fldLower == toColl || strings.EqualFold(sel.Field, j.ToCollectionField)) {
					toDataType = sel.DataType
				}
			}
		}

		ast.Joins = append(ast.Joins, ASTJoin{
			Schema:            j.Schema,
			Type:              jType,
			FromTable:         fromTbl,
			FromField:         j.FromCollectionField,
			ToTable:           j.ToCollection,
			ToField:           j.ToCollectionField,
			Alias:             alias,
			JoinFilter:        normalizeFilterToMap(j.Filter),
			ConvertString:     j.ConvertToString,
			CastMode:          j.CastMode,
			FromFieldDataType: fromDataType,
			ToFieldDataType:   toDataType,
			RelationshipOp:    j.RelationshipOp,
			Cardinality:       j.Cardinality,
			OutputFormat:      j.OutputFormat,
			SourceJSONPath:    j.SourceJSONPath,
			TargetJSONPath:    j.TargetJSONPath,
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
			if fldName == "" && f.Value != nil {
				valStr := f.GetValueString()
				if valStr != "" {
					fldName = valStr
				}
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
			var litVal any = fldName
			if isLit && f.Value != nil {
				litVal = f.Value
			}
			astCol.Operands = append(astCol.Operands, ASTOperand{
				SourceTable: tbl,
				SourceField: fldName,
				IsLiteral:   isLit || isCustomRef,
				LiteralVal:  litVal,
			})
		}

		ast.CustomColumns = append(ast.CustomColumns, astCol)
	}

	// 5. Map Where Filters
	if len(ds.Filter) > 0 {
		ast.WhereFilters = p.parseFilterSlice(ds.Filter, ds.BaseCollection.Collection, tableAliases, ds.SelectedList)
	}

	if p.modelResolver != nil && p.fieldResolver != nil && strings.EqualFold(ds.Driver, "postgres") {
		if err := p.resolveJoinMetadata(ctx, ast); err != nil {
			return nil, err
		}
	}
	return ast, nil
}

func (p *DataSetPlanner) resolveJoinMetadata(ctx context.Context, ast *QueryAST) error {
	if len(ast.Joins) == 0 {
		return nil
	}
	base, err := p.modelResolver.ResolveModel(ctx, ast.BaseTable.Schema, ast.BaseTable.Table)
	if err != nil {
		return err
	}
	if base != nil {
		if base.Table != "" {
			ast.BaseTable.Table = base.Table
		}
		if base.Schema != "" {
			ast.BaseTable.Schema = base.Schema
		}
	}
	models := map[string]*resolver.ModelDefinition{strings.ToLower(ast.BaseTable.Alias): base}
	for i := range ast.Joins {
		j := &ast.Joins[i]
		source := models[strings.ToLower(j.FromTable)]
		if source == nil {
			return fmt.Errorf("join %q: source model %q is not resolved", j.Alias, j.FromTable)
		}
		target, err := p.modelResolver.ResolveModel(ctx, j.Schema, j.ToTable)
		if err != nil {
			return err
		}
		from, err := p.fieldResolver.ResolveField(ctx, source, j.FromField)
		if err != nil {
			return err
		}
		to, err := p.fieldResolver.ResolveField(ctx, target, j.ToField)
		if err != nil {
			return err
		}
		if from == nil || to == nil || from.DataType == "" || to.DataType == "" || strings.EqualFold(from.DataType, "ANY") || strings.EqualFold(to.DataType, "ANY") {
			return fmt.Errorf("join %q: source and target fields must exist in model metadata", j.Alias)
		}
		j.FromField, j.ToField = from.ColumnName, to.ColumnName
		if j.FromField == "" {
			j.FromField = from.Name
		}
		if j.ToField == "" {
			j.ToField = to.Name
		}
		if target.Table != "" {
			j.ToTable = target.Table
		}
		if target.Schema != "" {
			j.Schema = target.Schema
		}
		j.FromFieldDataType, j.ToFieldDataType = from.DataType, to.DataType
		models[strings.ToLower(j.Alias)] = target
	}
	return nil
}

// normalizeFilterToMap converts any filter representation (map[string]any, []any, etc.) into a flat map[string]any.
func normalizeFilterToMap(f any) map[string]any {
	if f == nil {
		return nil
	}
	if m, ok := f.(map[string]any); ok {
		return m
	}
	if m, ok := f.(map[string]interface{}); ok {
		res := make(map[string]any, len(m))
		for k, v := range m {
			res[k] = v
		}
		return res
	}
	if slice, ok := f.([]any); ok {
		res := make(map[string]any)
		for _, item := range slice {
			if m, ok := item.(map[string]any); ok {
				if col, ok := m["column"].(string); ok {
					res[col] = m["value"]
				} else {
					for k, v := range m {
						if k != "clause" && k != "conditions" {
							res[k] = v
						}
					}
				}
			}
		}
		return res
	}
	return nil
}

// resolveTableAlias determines the effective alias or table name for a collection reference.
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
func isAggregateFunctionName(name string) bool {
	switch strings.ToUpper(strings.TrimSpace(name)) {
	case "SUM", "AVG", "COUNT", "COUNT_DISTINCT", "MIN", "MAX", "COUNT_ALL", "COUNT(*)", "COUNT_IF", "SUM_IF":
		return true
	default:
		return false
	}
}

// parseFilterSlice converts a slice-based filter definition ([]any) into a list of ASTCondition structs.
// It supports AG-Grid condition groups, explicit condition maps, and flat key-value dictionaries.
func (p *DataSetPlanner) parseFilterSlice(filter []any, defaultTable string, tableAliases map[string]string, selectedList []domain.SelectedField) []ASTCondition {
	var conditions []ASTCondition
	aliasMap := make(map[string]domain.SelectedField)
	for _, s := range selectedList {
		if s.HeaderName != "" {
			aliasMap[strings.ToLower(s.HeaderName)] = s
		}
		if s.Field != "" {
			aliasMap[strings.ToLower(s.Field)] = s
		}
	}

	for _, item := range filter {
		if item == nil {
			continue
		}
		if cond, ok := item.(ASTCondition); ok {
			conditions = append(conditions, cond)
			continue
		}
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}

		// Case A: Group containing nested conditions
		if rawConds, hasConds := m["conditions"]; hasConds {
			if condSlice, isSlice := rawConds.([]any); isSlice {
				subConditions := p.parseFilterSlice(condSlice, defaultTable, tableAliases, selectedList)
				conditions = append(conditions, subConditions...)
				continue
			}
		}

		// Case B: Explicit condition object: {"column": "status", "operator": "EQUALS", "value": "active"}
		colRaw, hasCol := m["column"]
		if !hasCol {
			colRaw, hasCol = m["field"]
		}
		if hasCol {
			colStr := fmt.Sprintf("%v", colRaw)
			opStr := "EQUALS"
			if op, hasOp := m["operator"].(string); hasOp && op != "" {
				opStr = strings.ToUpper(strings.TrimSpace(op))
			}
			val := m["value"]

			tbl := defaultTable
			col := colStr
			if sel, exists := aliasMap[strings.ToLower(colStr)]; exists && sel.Field != "" {
				if idx := strings.Index(sel.Field, "."); idx >= 0 {
					tbl = sel.Field[:idx]
					col = sel.Field[idx+1:]
				} else {
					col = sel.Field
				}
			} else if idx := strings.Index(colStr, "."); idx >= 0 {
				tbl = colStr[:idx]
				col = colStr[idx+1:]
			}
			tbl = resolveTableAlias(tbl, tableAliases)

			cond := ASTCondition{
				Table:    tbl,
				Column:   col,
				Operator: opStr,
				Value:    val,
			}
			if valMap, isMap := val.(map[string]any); isMap {
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
			continue
		}

		// Case C: Standard key-value dictionary: {"status": "active", ...}
		subConds := p.parseFilterMap(m, defaultTable, tableAliases, selectedList)
		conditions = append(conditions, subConds...)
	}

	return conditions
}

// parseFilterMap converts a raw map-based filter definition into a list of ASTCondition structs.
//
// Purpose:
//
//	Deconstructs key-value filters (e.g. "employees.department_id": {"paramName": "dept_id", ...})
//	into strongly-typed ASTCondition objects, detecting whether a value is a static literal or a parameter reference.
//
// Where it is used:
//   - Called by BuildAST to parse WHERE filter dictionaries from ds.Filter.
//
// When can it be used:
//   - When translating raw JSON filter maps into relational filter AST nodes.
func (p *DataSetPlanner) parseFilterMap(filter map[string]any, defaultTable string, tableAliases map[string]string, selectedList []domain.SelectedField) []ASTCondition {
	var conditions []ASTCondition
	aliasMap := make(map[string]domain.SelectedField)
	for _, s := range selectedList {
		if s.HeaderName != "" {
			aliasMap[strings.ToLower(s.HeaderName)] = s
		}
		if s.Field != "" {
			aliasMap[strings.ToLower(s.Field)] = s
		}
	}

	for k, v := range filter {
		tbl := defaultTable
		col := k

		if sel, ok := aliasMap[strings.ToLower(k)]; ok && sel.Field != "" {
			if idx := strings.Index(sel.Field, "."); idx >= 0 {
				tbl = sel.Field[:idx]
				col = sel.Field[idx+1:]
			} else {
				col = sel.Field
			}
		} else if idx := strings.Index(k, "."); idx >= 0 {
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
//
//	Distinguishes numeric literals (e.g. "100", "0.05") from column identifiers in custom expressions.
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
