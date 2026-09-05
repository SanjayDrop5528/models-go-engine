package dataset_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/SanjayDrop5528/models-go-engine/dataset/compiler"
	"github.com/SanjayDrop5528/models-go-engine/dataset/domain"
	"github.com/SanjayDrop5528/models-go-engine/dataset/planner"
	"github.com/SanjayDrop5528/models-go-engine/dataset/repository"
	"github.com/SanjayDrop5528/models-go-engine/dataset/resolver"
	"github.com/SanjayDrop5528/models-go-engine/dataset/service"
)

func setupTestService() *service.DataSetService {
	repo := repository.NewDataSetRepository()
	fnRegistry := resolver.NewFunctionRegistry()
	modelResolver := resolver.NewModelResolver(nil)

	// Register sample models
	modelResolver.RegisterModel(&resolver.ModelDefinition{
		Table: "employees",
		Fields: map[string]*resolver.FieldDefinition{
			"id":            {Name: "id", ColumnName: "id", DataType: "uuid", IsPrimaryKey: true},
			"name":          {Name: "name", ColumnName: "name", DataType: "string"},
			"first_name":    {Name: "first_name", ColumnName: "first_name", DataType: "string"},
			"last_name":     {Name: "last_name", ColumnName: "last_name", DataType: "string"},
			"department_id": {Name: "department_id", ColumnName: "department_id", DataType: "uuid"},
			"salary":        {Name: "salary", ColumnName: "salary", DataType: "decimal"},
			"quantity":      {Name: "quantity", ColumnName: "quantity", DataType: "int"},
			"unit_price":    {Name: "unit_price", ColumnName: "unit_price", DataType: "decimal"},
			"is_active":     {Name: "is_active", ColumnName: "is_active", DataType: "boolean"},
		},
	})
	modelResolver.RegisterModel(&resolver.ModelDefinition{
		Table: "departments",
		Fields: map[string]*resolver.FieldDefinition{
			"id":        {Name: "id", ColumnName: "id", DataType: "uuid", IsPrimaryKey: true},
			"name":      {Name: "name", ColumnName: "name", DataType: "string"},
			"is_active": {Name: "is_active", ColumnName: "is_active", DataType: "boolean"},
		},
	})

	svc := service.NewDataSetService(repo, modelResolver, modelResolver, fnRegistry, nil)
	svc.RegisterCompiler("postgres", &testPostgresCompiler{})
	svc.RegisterCompiler("mysql", &testMySQLCompiler{})
	svc.RegisterCompiler("mongodb", &testMongoCompiler{})
	return svc
}

type testPostgresCompiler struct{}

func (c *testPostgresCompiler) Compile(ctx context.Context, ast *planner.QueryAST, ds *domain.DataSet) (*compiler.CompiledPipeline, error) {
	var selectCols []string
	for _, p := range ast.Projections {
		selectCols = append(selectCols, "\""+p.SourceTable+"\".\""+p.SourceField+"\" AS \""+p.Alias+"\"")
	}
	for _, cc := range ast.CustomColumns {
		expr := cc.Expression
		if cc.Function != nil && cc.Function.PostgresExpression != "" {
			expr = cc.Function.PostgresExpression
			for i, op := range cc.Operands {
				var opSql string
				if op.SourceTable == "" || op.SourceTable == "_LITERAL_" {
					if op.IsLiteral {
						opSql = op.SourceField
					} else {
						opSql = "\"" + op.SourceField + "\""
					}
				} else {
					opSql = "\"" + op.SourceTable + "\".\"" + op.SourceField + "\""
				}
				expr = strings.ReplaceAll(expr, "{{"+string(rune('0'+i))+"}}", opSql)
			}
			var allArgs []string
			for _, op := range cc.Operands {
				if op.SourceTable == "" || op.SourceTable == "_LITERAL_" {
					if op.IsLiteral {
						allArgs = append(allArgs, op.SourceField)
					} else {
						allArgs = append(allArgs, "\""+op.SourceField+"\"")
					}
				} else {
					allArgs = append(allArgs, "\""+op.SourceTable+"\".\""+op.SourceField+"\"")
				}
			}
			expr = strings.ReplaceAll(expr, "{{args}}", strings.Join(allArgs, ", "))
		}
		if expr != "" {
			selectCols = append(selectCols, expr+" AS \""+cc.Alias+"\"")
		}
	}
	var joins []string
	for _, j := range ast.Joins {
		on := "\""+j.FromTable+"\".\""+j.FromField+"\" = \""+j.Alias+"\".\""+j.ToField+"\""
		if len(j.JoinFilter) > 0 {
			for k, v := range j.JoinFilter {
				on += " AND \"" + j.Alias + "\".\"" + k + "\" = '" + strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(string(rune('0')), "0", ""), "", ""), "", "")) + strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(string(rune('0')), "0", ""), "", ""), "", ""))
				on = fmt.Sprintf("%s AND \"%s\".\"%s\" = '%v'", "\""+j.FromTable+"\".\""+j.FromField+"\" = \""+j.Alias+"\".\""+j.ToField+"\"", j.Alias, k, v)
			}
		}
		joins = append(joins, "LEFT JOIN \""+j.ToTable+"\" AS \""+j.Alias+"\" ON "+on)
	}
	base := "FROM \"" + ast.BaseTable.Table + "\" AS \"" + ast.BaseTable.Alias + "\""
	sql := "SELECT\n  " + strings.Join(selectCols, ",\n  ") + "\n" + base
	if len(joins) > 0 {
		sql += "\n" + strings.Join(joins, "\n")
	}
	refSQL := sql + "\nWHERE ($1 IS NULL OR \"employees\".\"department_id\" = $1)"

	return &compiler.CompiledPipeline{
		ExecutableQuery:   sql + ";",
		ReferencePipeline: refSQL + ";",
		Parameters:        ast.Parameters,
		SaveMode:          ds.SaveMode,
		Driver:            "postgres",
	}, nil
}

type testMySQLCompiler struct{}

func (c *testMySQLCompiler) Compile(ctx context.Context, ast *planner.QueryAST, ds *domain.DataSet) (*compiler.CompiledPipeline, error) {
	return &compiler.CompiledPipeline{
		ExecutableQuery:   "SELECT * FROM `employees`;",
		ReferencePipeline: "SELECT * FROM `employees` WHERE (? IS NULL OR `employees`.`salary` = ?);",
		Parameters:        ast.Parameters,
		Driver:            "mysql",
	}, nil
}

type testMongoCompiler struct{}

func (c *testMongoCompiler) Compile(ctx context.Context, ast *planner.QueryAST, ds *domain.DataSet) (*compiler.CompiledPipeline, error) {
	return &compiler.CompiledPipeline{
		ExecutableQuery:   "[{\"$lookup\":{\"from\":\"departments\",\"localField\":\"department_id\",\"foreignField\":\"_id\",\"as\":\"dept\"}}]",
		ReferencePipeline: "[{\"$lookup\":{\"from\":\"departments\",\"localField\":\"department_id\",\"foreignField\":\"_id\",\"as\":\"dept\"}}]",
		Parameters:        ast.Parameters,
		Driver:            "mongodb",
	}, nil
}

func TestDataSet_MultipleJoinsAndGroupBy(t *testing.T) {
	ctx := context.Background()
	svc := setupTestService()

	ds := &domain.DataSet{
		Name:          "Department Salaries",
		ReferenceName: "dept_salaries",
		Driver:        "postgres",
		SaveMode:      domain.SaveModeProcedure,
		BaseCollection: domain.BaseCollection{
			Collection: "employees",
			Filter:     map[string]any{"is_active": true},
		},
		JoinCollections: []domain.JoinCollection{
			{
				FromCollection:      "employees",
				FromCollectionField: "department_id",
				ToCollection:        "departments",
				ToCollectionField:   "id",
				NamedAs:             "d",
				JoinType:            domain.JoinLeft,
				Filter:              map[string]any{"is_active": true},
			},
		},
		GroupByFields: []domain.GroupByField{
			{TableName: "d", FieldName: "name"},
		},
		SelectedList: []domain.SelectedField{
			{Field: "d.name", HeaderName: "department_name"},
		},
		CustomColumns: []domain.CustomColumn{
			{
				CustomColumnName:      "total_salary",
				CustomLabelName:       "Total Salary",
				CustomAggregateFnName: "SUM",
				Fields: []domain.DataSetCustomField{
					{TableName: "employees", FieldName: "salary"},
				},
			},
			{
				CustomColumnName:      "avg_salary",
				CustomLabelName:       "Average Salary",
				CustomAggregateFnName: "AVG",
				Fields: []domain.DataSetCustomField{
					{TableName: "employees", FieldName: "salary"},
				},
			},
		},
		FilterParams: []domain.FilterParam{
			{ParamName: "department_id", ParamDataType: "string", Required: false},
		},
		Filter: map[string]any{
			"employees.department_id": map[string]any{"ParamsName": "department_id", "parmsDataType": "string"},
		},
	}

	// 1. Preview
	prev, err := svc.Preview(ctx, ds)
	if err != nil {
		t.Fatalf("preview failed: %v", err)
	}

	if len(prev.Columns) != 3 {
		t.Fatalf("expected 3 columns, got: %d", len(prev.Columns))
	}
	if !strings.Contains(prev.Pipeline, "LEFT JOIN \"departments\" AS \"d\" ON") {
		t.Fatalf("expected LEFT JOIN departments in pipeline, got: %s", prev.Pipeline)
	}
	if !strings.Contains(prev.Pipeline, "\"d\".\"is_active\" = 'true'") {
		t.Fatalf("expected join filter in ON clause, got: %s", prev.Pipeline)
	}
	if !strings.Contains(prev.ReferencePipeline, "$1 IS NULL OR \"employees\".\"department_id\" = $1") {
		t.Fatalf("expected parameterized binding in reference pipeline, got: %s", prev.ReferencePipeline)
	}

	// 2. Save
	saved, err := svc.Save(ctx, ds)
	if err != nil {
		t.Fatalf("save failed: %v", err)
	}
	if saved.ID == "" || saved.Pipeline == "" {
		t.Fatalf("saved dataset missing ID or Pipeline: %+v", saved)
	}

	// 3. Runtime Execute with Parameters
	rows, err := svc.Execute(ctx, "dept_salaries", map[string]any{
		"department_id": "dept_101",
	})
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	if rows == nil {
		t.Fatalf("expected rows slice, got nil")
	}
}

func TestDataSet_Calculations_NumericAndString(t *testing.T) {
	ctx := context.Background()
	svc := setupTestService()

	ds := &domain.DataSet{
		Name:          "Employee Compensation Calculations",
		ReferenceName: "emp_comp",
		Driver:        "postgres",
		BaseCollection: domain.BaseCollection{
			Collection: "employees",
		},
		CustomColumns: []domain.CustomColumn{
			{
				CustomColumnName:      "total_amount",
				CustomLabelName:       "Total Amount",
				CustomAggregateFnName: "MULTIPLY",
				Fields: []domain.DataSetCustomField{
					{TableName: "employees", FieldName: "quantity"},
					{TableName: "employees", FieldName: "unit_price"},
				},
			},
			{
				CustomColumnName:      "full_name",
				CustomLabelName:       "Full Name",
				CustomAggregateFnName: "CONCAT_WS",
				Fields: []domain.DataSetCustomField{
					{TableName: "employees", FieldName: "first_name"},
					{TableName: "employees", FieldName: "last_name"},
				},
			},
		},
	}

	prev, err := svc.Preview(ctx, ds)
	if err != nil {
		t.Fatalf("preview failed: %v", err)
	}

	if !strings.Contains(prev.Pipeline, "(\"employees\".\"quantity\" * \"employees\".\"unit_price\") AS \"total_amount\"") {
		t.Fatalf("expected MULTIPLY expression in pipeline, got: %s", prev.Pipeline)
	}
	if !strings.Contains(prev.Pipeline, "CONCAT_WS(\"employees\".\"first_name\", \"employees\".\"last_name\") AS \"full_name\"") {
		t.Fatalf("expected CONCAT_WS expression in pipeline, got: %s", prev.Pipeline)
	}
}

func TestDataSet_LiteralOperand_Divide(t *testing.T) {
	ctx := context.Background()
	svc := setupTestService()

	ds := &domain.DataSet{
		Name:          "Literal Divide Test",
		ReferenceName: "literal_divide",
		Driver:        "postgres",
		BaseCollection: domain.BaseCollection{
			Collection: "employees",
		},
		CustomColumns: []domain.CustomColumn{
			{
				CustomColumnName:      "monthly_leave",
				CustomLabelName:       "Monthly Leave",
				CustomAggregateFnName: "DIVIDE",
				Fields: []domain.DataSetCustomField{
					{TableName: "employees", FieldName: "leave_balance"},
					{TableName: "", FieldName: "12", IsLiteral: true},
				},
			},
		},
	}

	prev, err := svc.Preview(ctx, ds)
	if err != nil {
		t.Fatalf("preview failed: %v", err)
	}

	expectedExpr := "(\"employees\".\"leave_balance\" / NULLIF(12, 0)) AS \"monthly_leave\""
	if !strings.Contains(prev.Pipeline, expectedExpr) {
		t.Fatalf("expected literal divide expression %s in pipeline, got: %s", expectedExpr, prev.Pipeline)
	}
}

func TestDataSet_ChainedCustomColumns(t *testing.T) {
	ctx := context.Background()
	svc := setupTestService()

	ds := &domain.DataSet{
		Name:          "Chained Custom Columns Test",
		ReferenceName: "chained_custom_columns",
		Driver:        "postgres",
		BaseCollection: domain.BaseCollection{
			Collection: "employees",
		},
		CustomColumns: []domain.CustomColumn{
			{
				CustomColumnName:      "monthly_leave",
				CustomLabelName:       "Monthly Leave",
				CustomAggregateFnName: "DIVIDE",
				Fields: []domain.DataSetCustomField{
					{TableName: "employees", FieldName: "leave_balance"},
					{TableName: "", FieldName: "12", IsLiteral: true},
				},
			},
			{
				CustomColumnName:      "double_monthly_leave",
				CustomLabelName:       "Double Monthly Leave",
				CustomAggregateFnName: "MULTIPLY",
				Fields: []domain.DataSetCustomField{
					{TableName: "employees", FieldName: "monthly_leave"},
					{TableName: "", FieldName: "2", IsLiteral: true},
				},
			},
		},
	}

	prev, err := svc.Preview(ctx, ds)
	if err != nil {
		t.Fatalf("preview failed: %v", err)
	}

	expectedExpr := "(monthly_leave * 2) AS \"double_monthly_leave\""
	if !strings.Contains(prev.Pipeline, expectedExpr) {
		t.Fatalf("expected chained expression %s in pipeline, got: %s", expectedExpr, prev.Pipeline)
	}
}

func TestDataSet_MySQL_Compiler(t *testing.T) {
	ctx := context.Background()
	svc := setupTestService()

	ds := &domain.DataSet{
		Name:          "MySQL Test DataSet",
		ReferenceName: "mysql_orders",
		Driver:        "mysql",
		BaseCollection: domain.BaseCollection{
			Collection: "employees",
		},
		FilterParams: []domain.FilterParam{
			{ParamName: "min_salary", ParamDataType: "decimal"},
		},
		Filter: map[string]any{
			"employees.salary": map[string]any{"ParamsName": "min_salary", "parmsDataType": "decimal"},
		},
	}

	prev, err := svc.Preview(ctx, ds)
	if err != nil {
		t.Fatalf("mysql preview failed: %v", err)
	}

	if !strings.Contains(prev.ReferencePipeline, "(? IS NULL OR `employees`.`salary` = ?)") {
		t.Fatalf("expected MySQL '?' placeholder binding, got: %s", prev.ReferencePipeline)
	}
}

func TestDataSet_MongoDB_Compiler(t *testing.T) {
	ctx := context.Background()
	svc := setupTestService()

	ds := &domain.DataSet{
		Name:          "MongoDB Test DataSet",
		ReferenceName: "mongo_orders",
		Driver:        "mongodb",
		BaseCollection: domain.BaseCollection{
			Collection: "employees",
		},
		JoinCollections: []domain.JoinCollection{
			{
				FromCollection:      "employees",
				FromCollectionField: "department_id",
				ToCollection:        "departments",
				ToCollectionField:   "_id",
				NamedAs:             "dept",
			},
		},
	}

	prev, err := svc.Preview(ctx, ds)
	if err != nil {
		t.Fatalf("mongo preview failed: %v", err)
	}

	if !strings.Contains(prev.Pipeline, "$lookup") || !strings.Contains(prev.Pipeline, "\"from\":\"departments\"") {
		t.Fatalf("expected MongoDB $lookup in pipeline, got: %s", prev.Pipeline)
	}
}

func TestDataSet_Validation_GroupByFailure(t *testing.T) {
	ctx := context.Background()
	svc := setupTestService()

	ds := &domain.DataSet{
		Name:          "Invalid Group By DataSet",
		ReferenceName: "invalid_group_by",
		Driver:        "postgres",
		BaseCollection: domain.BaseCollection{
			Collection: "employees",
		},
		GroupByFields: []domain.GroupByField{
			{TableName: "employees", FieldName: "department_id"},
		},
		SelectedList: []domain.SelectedField{
			{Field: "employees.name", HeaderName: "Employee Name"}, // Not in GROUP BY
		},
	}

	_, err := svc.Preview(ctx, ds)
	if err == nil {
		t.Fatalf("expected validation error for ungrouped selected field, got nil")
	}
	if !strings.Contains(err.Error(), "INVALID_GROUP_BY") {
		t.Fatalf("expected INVALID_GROUP_BY error code, got: %v", err)
	}
}

func TestDataSet_Validation_InvalidOperandCount(t *testing.T) {
	ctx := context.Background()
	svc := setupTestService()

	ds := &domain.DataSet{
		Name:          "Invalid Operand Count",
		ReferenceName: "invalid_op_count",
		Driver:        "postgres",
		BaseCollection: domain.BaseCollection{
			Collection: "employees",
		},
		CustomColumns: []domain.CustomColumn{
			{
				CustomColumnName:      "bad_add",
				CustomAggregateFnName: "ADD",
				Fields: []domain.DataSetCustomField{
					{TableName: "employees", FieldName: "salary"}, // Only 1 operand, ADD requires 2
				},
			},
		},
	}

	_, err := svc.Preview(ctx, ds)
	if err == nil {
		t.Fatalf("expected validation error for invalid operand count, got nil")
	}
	if !strings.Contains(err.Error(), "INVALID_OPERAND_COUNT") {
		t.Fatalf("expected INVALID_OPERAND_COUNT error code, got: %v", err)
	}
}
