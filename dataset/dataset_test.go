package dataset_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/SanjayDrop5528/models-go-engine/adapter"
	"github.com/SanjayDrop5528/models-go-engine/dataset/compiler"
	"github.com/SanjayDrop5528/models-go-engine/dataset/domain"
	"github.com/SanjayDrop5528/models-go-engine/dataset/planner"
	"github.com/SanjayDrop5528/models-go-engine/dataset/repository"
	"github.com/SanjayDrop5528/models-go-engine/dataset/resolver"
	"github.com/SanjayDrop5528/models-go-engine/dataset/service"
	"github.com/SanjayDrop5528/models-go-engine/execution"
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

func TestCommon_CreateFilterParams(t *testing.T) {
	filterParams := []domain.FilterParam{
		{
			ParamName:     "dept_id",
			ParamDataType: "string",
			DefaultValue:  "dept_eng",
		},
		{
			ParamName:     "min_salary",
			ParamDataType: "int",
			DefaultValue:  50000,
		},
		{
			ParamName:     "is_active",
			ParamDataType: "bool",
			DefaultValue:  true,
		},
		{
			ParamName:     "user_org",
			ParamDataType: "string",
			DefaultValue:  "KTON|org_id",
		},
		{
			ParamName:     "start_date",
			ParamDataType: "time.Time",
			DefaultValue:  "CD|+0|ST",
		},
		{
			ParamName:     "end_date",
			ParamDataType: "date",
			DefaultValue:  "CD|+0|ED",
		},
		{
			ParamName:     "bonus_rate",
			ParamDataType: "decimal",
			DefaultValue:  0.15,
			Paramvalue:    0.20, // Runtime override
		},
	}

	userToken := map[string]any{
		"org_id":   "org_corp_999",
		"timezone": "America/New_York",
	}

	rawPipeline := `[
		{"$match": {"department": {"paramName":"dept_id","paramDataType":"string"}}},
		{"$match": {"salary": {"$gte": {"paramName":"min_salary","paramDataType":"int"}}}},
		{"$match": {"active": {"paramName":"is_active","paramDataType":"bool"}}},
		{"$match": {"organization": {"paramName":"user_org","paramDataType":"string"}}},
		{"$match": {"created_at": {"$gte": {"paramName":"start_date","paramDataType":"time.Time"}}}},
		{"$match": {"expired_at": {"$lte": {"paramName":"end_date","paramDataType":"date"}}}},
		{"$match": {"bonus": {"paramName":"bonus_rate","paramDataType":"decimal"}}}
	]`

	resolved := domain.CreateFilterParams(filterParams, rawPipeline, userToken)

	// Check string value is quoted
	if !strings.Contains(resolved, `"department": "dept_eng"`) {
		t.Errorf("expected department dept_eng, got:\n%s", resolved)
	}
	// Check int value is unquoted
	if !strings.Contains(resolved, `"salary": {"$gte": 50000}`) {
		t.Errorf("expected salary 50000, got:\n%s", resolved)
	}
	// Check bool value is unquoted
	if !strings.Contains(resolved, `"active": true`) {
		t.Errorf("expected active true, got:\n%s", resolved)
	}
	// Check KTON token resolution
	if !strings.Contains(resolved, `"organization": "org_corp_999"`) {
		t.Errorf("expected organization org_corp_999, got:\n%s", resolved)
	}
	// Check CD Start of Day resolution with timezone (00:00:00)
	if !strings.Contains(resolved, `T00:00:00`) {
		t.Errorf("expected start_date to have T00:00:00, got:\n%s", resolved)
	}
	// Check CD End of Day resolution with timezone (23:59:59)
	if !strings.Contains(resolved, `T23:59:59`) {
		t.Errorf("expected end_date to have T23:59:59, got:\n%s", resolved)
	}
	// Check runtime Paramvalue override (0.20 instead of 0.15)
	if !strings.Contains(resolved, `"bonus": 0.2`) {
		t.Errorf("expected bonus 0.2, got:\n%s", resolved)
	}
}

type mockExecutionAdapter struct {
	adapter.Adapter
	lastReq execution.ExecutionRequest
}

func (m *mockExecutionAdapter) Execute(ctx context.Context, req execution.ExecutionRequest) (*execution.ExecutionResult, error) {
	m.lastReq = req
	return &execution.ExecutionResult{
		Data:   []map[string]any{{"mock_id": 1}},
		Status: "SUCCESS",
	}, nil
}

func TestDataSetService_Execute_SaveModes(t *testing.T) {
	ctx := context.Background()
	repo := repository.NewDataSetRepository()
	fnRegistry := resolver.NewFunctionRegistry()
	modelResolver := resolver.NewModelResolver(nil)

	mockAdp := &mockExecutionAdapter{}
	svc := service.NewDataSetService(repo, modelResolver, modelResolver, fnRegistry, nil)
	svc.SetAdapter(mockAdp)

	// 1. Test Procedure SaveMode
	dsProc := &domain.DataSet{
		ID:            "ds_proc",
		Name:          "Procedure Dataset",
		ReferenceName: "proc_dataset",
		SaveMode:      domain.SaveModeProcedure,
		FilterParams: []domain.FilterParam{
			{ParamName: "dept", ParamDataType: "string", DefaultValue: "engineering"},
		},
	}
	if err := repo.Save(ctx, dsProc); err != nil {
		t.Fatalf("failed saving proc dataset: %v", err)
	}

	_, err := svc.Execute(ctx, "proc_dataset", map[string]any{"dept": "sales"})
	if err != nil {
		t.Fatalf("failed executing procedure dataset: %v", err)
	}
	if mockAdp.lastReq.Operation != "PROCEDURE" {
		t.Errorf("expected operation PROCEDURE, got: %s", mockAdp.lastReq.Operation)
	}
	if mockAdp.lastReq.Target != "sp_proc_dataset" {
		t.Errorf("expected target sp_proc_dataset, got: %s", mockAdp.lastReq.Target)
	}
	if mockAdp.lastReq.Arguments["dept"] != "sales" {
		t.Errorf("expected arg dept=sales, got: %v", mockAdp.lastReq.Arguments["dept"])
	}

	// 2. Test Function SaveMode
	dsFn := &domain.DataSet{
		ID:            "ds_fn",
		Name:          "Function Dataset",
		ReferenceName: "fn_dataset",
		SaveMode:      domain.SaveModeFunction,
		FilterParams: []domain.FilterParam{
			{ParamName: "limit", ParamDataType: "int", DefaultValue: 10},
		},
	}
	if err := repo.Save(ctx, dsFn); err != nil {
		t.Fatalf("failed saving fn dataset: %v", err)
	}

	_, err = svc.Execute(ctx, "fn_dataset", map[string]any{"limit": 25})
	if err != nil {
		t.Fatalf("failed executing function dataset: %v", err)
	}
	if mockAdp.lastReq.Operation != "FUNCTION" {
		t.Errorf("expected operation FUNCTION, got: %s", mockAdp.lastReq.Operation)
	}
	if mockAdp.lastReq.Target != "fn_fn_dataset" {
		t.Errorf("expected target fn_fn_dataset, got: %s", mockAdp.lastReq.Target)
	}

	// 3. Test Direct Query SaveMode with CreateFilterParams substitution
	dsQuery := &domain.DataSet{
		ID:                "ds_query",
		Name:              "Query Dataset",
		ReferenceName:     "query_dataset",
		SaveMode:          domain.SaveModeQuery,
		Pipeline:          `SELECT * FROM users WHERE status = 'active'`,
		ReferencePipeline: `SELECT * FROM users WHERE status = {"paramName":"status","paramDataType":"string"}`,
		FilterParams: []domain.FilterParam{
			{ParamName: "status", ParamDataType: "string", DefaultValue: "active"},
		},
	}
	if err := repo.Save(ctx, dsQuery); err != nil {
		t.Fatalf("failed saving query dataset: %v", err)
	}

	_, err = svc.Execute(ctx, "query_dataset", map[string]any{"status": "pending"})
	if err != nil {
		t.Fatalf("failed executing query dataset: %v", err)
	}
	if mockAdp.lastReq.Operation != "QUERY" {
		t.Errorf("expected operation QUERY, got: %s", mockAdp.lastReq.Operation)
	}
	if !strings.Contains(mockAdp.lastReq.Target, `"pending"`) {
		t.Errorf("expected target to have substituted pending, got: %s", mockAdp.lastReq.Target)
	}
}

func TestDataSet_PayloadVsDefaultValue_Precedence(t *testing.T) {
	ctx := context.Background()
	repo := repository.NewDataSetRepository()
	fnRegistry := resolver.NewFunctionRegistry()
	modelResolver := resolver.NewModelResolver(nil)

	mockAdp := &mockExecutionAdapter{}
	svc := service.NewDataSetService(repo, modelResolver, modelResolver, fnRegistry, nil)
	svc.SetAdapter(mockAdp)

	ds := &domain.DataSet{
		ID:                "ds_precedence",
		Name:              "Precedence Dataset",
		ReferenceName:     "precedence_ds",
		SaveMode:          domain.SaveModeQuery,
		Pipeline:          `SELECT * FROM items`,
		ReferencePipeline: `SELECT * FROM items WHERE col1 = {"paramName":"param1","paramDataType":"string"} AND col2 = {"paramName":"param2","paramDataType":"string"} AND col3 = {"paramName":"param3","paramDataType":"string"} AND col4 = {"paramName":"param4","paramDataType":"string"}`,
		FilterParams: []domain.FilterParam{
			{
				ParamName:     "param1",
				ParamDataType: "string",
				DefaultValue:  "default_val_1",
			},
			{
				ParamName:     "param2",
				ParamDataType: "string",
				DefaultValue:  "default_val_2",
			},
			{
				ParamName:     "param3",
				ParamDataType: "string",
				DefaultValue:  "default_val_3",
			},
			{
				ParamName:     "param4",
				ParamDataType: "string",
				DefaultValue:  "KTON|org_id",
			},
		},
	}
	if err := repo.Save(ctx, ds); err != nil {
		t.Fatalf("failed saving dataset: %v", err)
	}

	// Payload provides param1 (should use payload value)
	// Payload does not provide param2 (should use default value)
	// Payload provides empty string for param3 (should fallback to default value)
	// Payload does not provide param4 (should resolve KTON|org_id from userToken)
	payload := map[string]any{
		"param1": "payload_custom_1",
		"param3": "", // empty, should fallback
	}
	userToken := map[string]any{
		"org_id": "org_secret_777",
	}

	_, err := svc.ExecuteWithUserToken(ctx, "precedence_ds", payload, userToken)
	if err != nil {
		t.Fatalf("failed executing dataset: %v", err)
	}

	query := mockAdp.lastReq.Target

	// param1 must use payload value
	if !strings.Contains(query, `"payload_custom_1"`) {
		t.Errorf("expected param1 to use payload value 'payload_custom_1', got:\n%s", query)
	}
	// param2 must use default value
	if !strings.Contains(query, `"default_val_2"`) {
		t.Errorf("expected param2 to use default value 'default_val_2', got:\n%s", query)
	}
	// param3 must fallback to default value
	if !strings.Contains(query, `"default_val_3"`) {
		t.Errorf("expected param3 to fallback to default value 'default_val_3', got:\n%s", query)
	}
	// param4 must resolve from userToken
	if !strings.Contains(query, `"org_secret_777"`) {
		t.Errorf("expected param4 to resolve KTON token 'org_secret_777', got:\n%s", query)
	}

	// Now verify that if payload DOES provide param4, it overrides the KTON token default
	payloadWithParam4 := map[string]any{
		"param1": "custom_1",
		"param4": "override_org_888",
	}
	_, err = svc.ExecuteWithUserToken(ctx, "precedence_ds", payloadWithParam4, userToken)
	if err != nil {
		t.Fatalf("failed executing dataset with param4 override: %v", err)
	}
	query2 := mockAdp.lastReq.Target
	if !strings.Contains(query2, `"override_org_888"`) {
		t.Errorf("expected param4 to be overridden by payload 'override_org_888', got:\n%s", query2)
	}
}

func TestDataSet_ExecuteWithOptions_Payload(t *testing.T) {
	ctx := context.Background()
	repo := repository.NewDataSetRepository()
	fnRegistry := resolver.NewFunctionRegistry()
	modelResolver := resolver.NewModelResolver(nil)

	mockAdp := &mockExecutionAdapter{}
	svc := service.NewDataSetService(repo, modelResolver, modelResolver, fnRegistry, nil)
	svc.SetAdapter(mockAdp)

	ds := &domain.DataSet{
		ID:                "ds_exec_test",
		Name:              "Exec Test DS",
		ReferenceName:     "exec_test_ds",
		Driver:            "postgres",
		SaveMode:          domain.SaveModeQuery,
		Pipeline:          `SELECT "id", "name", "status" FROM "users"`,
		ReferencePipeline: `SELECT "id", "name", "status" FROM "users" WHERE "status" = {"paramName":"status","paramDataType":"string"}`,
		FilterParams: []domain.FilterParam{
			{ParamName: "status", ParamDataType: "string", DefaultValue: "active"},
			{ParamName: "dept", ParamDataType: "string", DefaultValue: "engineering"},
		},
	}

	if err := repo.Save(ctx, ds); err != nil {
		t.Fatalf("failed to save dataset: %v", err)
	}

	// 1. Execute with standardized payload including start, limit, filter, sort, filterParams
	req := &domain.ExecuteRequest{
		Start: 0,
		Limit: 2,
		Filter: map[string]any{
			"status": "active",
		},
		Sort: map[string]any{
			"id": "asc",
		},
		FilterParams: []any{
			map[string]any{
				"ParamName":     "status",
				"ParamDatatype": "string",
				"ParamValue":    "active",
			},
			map[string]any{
				"ParamName":     "dept",
				"ParamDatatype": "string",
				"ParamValue":    "sales",
			},
		},
	}

	rows, err := svc.ExecuteWithOptions(ctx, "exec_test_ds", req)
	if err != nil {
		t.Fatalf("ExecuteWithOptions failed: %v", err)
	}
	if len(rows) > 2 {
		t.Fatalf("expected at most 2 rows, got %d", len(rows))
	}

	targetSQL := mockAdp.lastReq.Target
	if !strings.Contains(targetSQL, "LIMIT 2") {
		t.Errorf("expected target SQL to contain 'LIMIT 2', got:\n%s", targetSQL)
	}
	if !strings.Contains(targetSQL, "\"_exec_sub\".\"status\" = 'active'") {
		t.Errorf("expected target SQL to contain status filter, got:\n%s", targetSQL)
	}
	if !strings.Contains(targetSQL, "\"_exec_sub\".\"id\" ASC") {
		t.Errorf("expected target SQL to contain sort clause, got:\n%s", targetSQL)
	}
}

func TestDataSet_ExecuteRequest_AppendFilter_JSON(t *testing.T) {
	// Case 1: explicit "first"
	jsonStr1 := `{"start":0,"limit":10,"filter":{"a":"1"},"appendfilter":"first"}`
	var req1 domain.ExecuteRequest
	if err := json.Unmarshal([]byte(jsonStr1), &req1); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if req1.GetAppendFilter() != "first" {
		t.Errorf("expected appendfilter 'first', got '%s'", req1.GetAppendFilter())
	}

	// Case 2: explicit "last"
	jsonStr2 := `{"start":0,"limit":10,"filter":{"a":"1"},"appendfilter":"last"}`
	var req2 domain.ExecuteRequest
	if err := json.Unmarshal([]byte(jsonStr2), &req2); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if req2.GetAppendFilter() != "last" {
		t.Errorf("expected appendfilter 'last', got '%s'", req2.GetAppendFilter())
	}

	// Case 3: omitted (default to "last")
	jsonStr3 := `{"start":0,"limit":10,"filter":{"a":"1"}}`
	var req3 domain.ExecuteRequest
	if err := json.Unmarshal([]byte(jsonStr3), &req3); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if req3.GetAppendFilter() != "last" {
		t.Errorf("expected default appendfilter 'last', got '%s'", req3.GetAppendFilter())
	}

	// Case 4: camelCase "appendFilter": "FIRST"
	jsonStr4 := `{"start":0,"limit":10,"filter":{"a":"1"},"appendFilter":"FIRST"}`
	var req4 domain.ExecuteRequest
	if err := json.Unmarshal([]byte(jsonStr4), &req4); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if req4.GetAppendFilter() != "first" {
		t.Errorf("expected case-insensitive 'first', got '%s'", req4.GetAppendFilter())
	}

	// Case 5: snake_case "append_filter": "first"
	jsonStr5 := `{"start":0,"limit":10,"filter":{"a":"1"},"append_filter":"first"}`
	var req5 domain.ExecuteRequest
	if err := json.Unmarshal([]byte(jsonStr5), &req5); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if req5.GetAppendFilter() != "first" {
		t.Errorf("expected snake_case 'first', got '%s'", req5.GetAppendFilter())
	}
}

func TestDataSet_ExecuteWithOptions_AppendFilter_FirstAndLast(t *testing.T) {
	ctx := context.Background()
	repo := repository.NewDataSetRepository()
	fnRegistry := resolver.NewFunctionRegistry()
	modelResolver := resolver.NewModelResolver(nil)

	mockAdp := &mockExecutionAdapter{}
	svc := service.NewDataSetService(repo, modelResolver, modelResolver, fnRegistry, nil)
	svc.SetAdapter(mockAdp)

	ds := &domain.DataSet{
		ID:            "ds_append_filter_test",
		Name:          "Append Filter Test DS",
		ReferenceName: "append_filter_test_ds",
		Driver:        "postgres",
		SaveMode:      domain.SaveModeQuery,
		Pipeline:      `SELECT "id", "name", "role", "tenant_id" FROM "accounts"`,
		Filter: map[string]any{
			"tenant_id": "100",
		},
	}

	if err := repo.Save(ctx, ds); err != nil {
		t.Fatalf("failed to save dataset: %v", err)
	}

	// 1. Execute with appendfilter = "first": dynamic "role" filter should be prepended before "tenant_id"
	reqFirst := &domain.ExecuteRequest{
		Filter: map[string]any{
			"role": "admin",
		},
		AppendFilter: "first",
	}
	_, err := svc.ExecuteWithOptions(ctx, "append_filter_test_ds", reqFirst)
	if err != nil {
		t.Fatalf("ExecuteWithOptions first failed: %v", err)
	}
	sqlFirst := mockAdp.lastReq.Target
	expectedFirst := "WHERE \"_exec_sub\".\"role\" = 'admin' AND \"_exec_sub\".\"tenant_id\" = '100'"
	if !strings.Contains(sqlFirst, expectedFirst) {
		t.Errorf("expected appendfilter 'first' SQL to contain:\n%s\ngot:\n%s", expectedFirst, sqlFirst)
	}

	// 2. Execute with appendfilter = "last": dynamic "role" filter should be appended after "tenant_id"
	reqLast := &domain.ExecuteRequest{
		Filter: map[string]any{
			"role": "admin",
		},
		AppendFilter: "last",
	}
	_, err = svc.ExecuteWithOptions(ctx, "append_filter_test_ds", reqLast)
	if err != nil {
		t.Fatalf("ExecuteWithOptions last failed: %v", err)
	}
	sqlLast := mockAdp.lastReq.Target
	expectedLast := "WHERE \"_exec_sub\".\"tenant_id\" = '100' AND \"_exec_sub\".\"role\" = 'admin'"
	if !strings.Contains(sqlLast, expectedLast) {
		t.Errorf("expected appendfilter 'last' SQL to contain:\n%s\ngot:\n%s", expectedLast, sqlLast)
	}
}

func TestDataSet_ApplyFilterToSQL(t *testing.T) {
	// 1. With existing WHERE: appendfilter = "first"
	baseSQL1 := "SELECT * FROM users WHERE status = 'active'"
	f1 := map[string]any{"dept": "engineering"}
	out1 := service.ApplyFilterToSQL(baseSQL1, f1, "first")
	expected1 := `WHERE ("dept" = 'engineering') AND (status = 'active')`
	if !strings.Contains(out1, expected1) {
		t.Errorf("expected '%s', got '%s'", expected1, out1)
	}

	// 2. With existing WHERE: appendfilter = "last"
	out2 := service.ApplyFilterToSQL(baseSQL1, f1, "last")
	expected2 := `WHERE (status = 'active') AND ("dept" = 'engineering')`
	if !strings.Contains(out2, expected2) {
		t.Errorf("expected '%s', got '%s'", expected2, out2)
	}

	// 3. Without existing WHERE, with ORDER BY
	baseSQL2 := "SELECT id, name FROM users ORDER BY id ASC"
	out3 := service.ApplyFilterToSQL(baseSQL2, f1, "last")
	if !strings.Contains(out3, `WHERE "dept" = 'engineering'`) || !strings.Contains(out3, `ORDER BY id ASC`) {
		t.Errorf("expected WHERE before ORDER BY, got '%s'", out3)
	}
}

func TestDataSet_ExecuteWithOptions_MongoDB_AppendFilter(t *testing.T) {
	ctx := context.Background()
	repo := repository.NewDataSetRepository()
	fnRegistry := resolver.NewFunctionRegistry()
	modelResolver := resolver.NewModelResolver(nil)

	mockAdp := &mockExecutionAdapter{}
	svc := service.NewDataSetService(repo, modelResolver, modelResolver, fnRegistry, nil)
	svc.SetAdapter(mockAdp)

	ds := &domain.DataSet{
		ID:            "ds_mongo_test",
		Name:          "Mongo Test DS",
		ReferenceName: "mongo_test_ds",
		Driver:        "mongodb",
		SaveMode:      domain.SaveModeQuery,
		Pipeline:      `[{"$project":{"id":1,"name":1}}]`,
	}

	if err := repo.Save(ctx, ds); err != nil {
		t.Fatalf("failed to save dataset: %v", err)
	}

	// 1. appendfilter = "first" -> $match is at index 0
	reqFirst := &domain.ExecuteRequest{
		Filter: map[string]any{
			"status": "active",
		},
		AppendFilter: "first",
	}
	_, err := svc.ExecuteWithOptions(ctx, "mongo_test_ds", reqFirst)
	if err != nil {
		t.Fatalf("ExecuteWithOptions failed: %v", err)
	}
	mongoFirst := mockAdp.lastReq.Target
	var stagesFirst []map[string]any
	if err := json.Unmarshal([]byte(mongoFirst), &stagesFirst); err != nil {
		t.Fatalf("failed to parse mongo pipeline: %v", err)
	}
	if len(stagesFirst) != 2 {
		t.Fatalf("expected 2 stages, got %d", len(stagesFirst))
	}
	if _, hasMatch := stagesFirst[0]["$match"]; !hasMatch {
		t.Errorf("expected $match at index 0 for appendfilter=first, got: %v", stagesFirst[0])
	}

	// 2. appendfilter = "last" -> $match is at the end
	reqLast := &domain.ExecuteRequest{
		Filter: map[string]any{
			"status": "active",
		},
		AppendFilter: "last",
	}
	_, err = svc.ExecuteWithOptions(ctx, "mongo_test_ds", reqLast)
	if err != nil {
		t.Fatalf("ExecuteWithOptions failed: %v", err)
	}
	mongoLast := mockAdp.lastReq.Target
	var stagesLast []map[string]any
	if err := json.Unmarshal([]byte(mongoLast), &stagesLast); err != nil {
		t.Fatalf("failed to parse mongo pipeline: %v", err)
	}
	if len(stagesLast) != 2 {
		t.Fatalf("expected 2 stages, got %d", len(stagesLast))
	}
	if _, hasMatch := stagesLast[1]["$match"]; !hasMatch {
		t.Errorf("expected $match at index 1 for appendfilter=last, got: %v", stagesLast[1])
	}
}



