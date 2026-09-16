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

func isAggregateFunctionName(name string) bool {
	switch strings.ToUpper(strings.TrimSpace(name)) {
	case "SUM", "AVG", "COUNT", "COUNT_DISTINCT", "MIN", "MAX", "COUNT_ALL", "COUNT(*)", "COUNT_IF", "SUM_IF":
		return true
	default:
		return false
	}
}

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

func isNumericString(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	_, err := strconv.ParseFloat(s, 64)
	return err == nil
}
