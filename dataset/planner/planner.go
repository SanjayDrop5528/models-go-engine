package planner

import (
	"context"
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
		})
	}

	// 2. Map Projections (SelectedList)
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

	// 3. Map Custom Columns & Functions
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
			if tbl == "" {
				tbl = ds.BaseCollection.Collection
			}
			fldName := f.FieldName
			if fldName == "" {
				fldName = f.Name
			}
			astCol.Operands = append(astCol.Operands, ASTOperand{
				SourceTable: tbl,
				SourceField: fldName,
			})
		}

		ast.CustomColumns = append(ast.CustomColumns, astCol)
	}

	// 4. Map Group By
	for _, g := range ds.GroupByFields {
		tbl := g.TableName
		if tbl == "" {
			tbl = ds.BaseCollection.Collection
		}
		fld := g.FieldName
		if fld == "" {
			fld = g.Name
		}
		ast.GroupBy = append(ast.GroupBy, ASTGroupBy{
			Table: tbl,
			Field: fld,
		})
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
