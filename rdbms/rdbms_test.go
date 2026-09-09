package rdbms_test

import (
	"strings"
	"testing"

	coreQuery "github.com/SanjayDrop5528/models-go-engine/query"
	"github.com/SanjayDrop5528/models-go-engine/rdbms"
	"github.com/SanjayDrop5528/models-go-engine/rdbms/dialect"
	"github.com/SanjayDrop5528/models-go-engine/rdbms/dialect/feature"
	"github.com/SanjayDrop5528/models-go-engine/rdbms/internal"
	"github.com/SanjayDrop5528/models-go-engine/rdbms/query"
	"github.com/SanjayDrop5528/models-go-engine/rdbms/schema"
)

// TestInternalStringHelpers tests string case conversions and ASCII functions.
func TestInternalStringHelpers(t *testing.T) {
	if !internal.IsUpper('A') || internal.IsUpper('a') {
		t.Fatalf("IsUpper failed")
	}
	if !internal.IsLower('z') || internal.IsLower('Z') {
		t.Fatalf("IsLower failed")
	}
	if internal.ToUpper('b') != 'B' || internal.ToUpper('B') != 'B' {
		t.Fatalf("ToUpper failed")
	}
	if internal.ToLower('B') != 'b' || internal.ToLower('b') != 'b' {
		t.Fatalf("ToLower failed")
	}

	if got := internal.Underscore("CamelCasedString"); got != "camel_cased_string" {
		t.Fatalf("Underscore expected 'camel_cased_string', got '%s'", got)
	}
	if got := internal.CamelCased("snake_cased_string"); got != "SnakeCasedString" {
		t.Fatalf("CamelCased expected 'SnakeCasedString', got '%s'", got)
	}
	if got := internal.ToExported("userEmail"); got != "UserEmail" {
		t.Fatalf("ToExported expected 'UserEmail', got '%s'", got)
	}

	b := []byte("hello")
	s := internal.String(b)
	if s != "hello" {
		t.Fatalf("internal.String failed")
	}
	if string(internal.Bytes(s)) != "hello" {
		t.Fatalf("internal.Bytes failed")
	}
}

// TestDialects tests dialect features and quoting.
func TestDialects(t *testing.T) {
	pg := dialect.NewPostgreSQL()
	if pg.Name() != dialect.PostgreSQL {
		t.Fatalf("expected pg, got %s", pg.Name())
	}
	if !pg.HasFeature(feature.SelectExists) || !pg.HasFeature(feature.DistinctOn) {
		t.Fatalf("pg missing features")
	}
	ident := string(pg.AppendIdent(nil, "public.users"))
	if ident != `"public"."users"` {
		t.Fatalf("unexpected pg ident quoting: %s", ident)
	}

	my := dialect.NewMySQL()
	if my.Name() != dialect.MySQL {
		t.Fatalf("expected mysql, got %s", my.Name())
	}
	if !my.HasFeature(feature.IndexHints) {
		t.Fatalf("mysql should have index hints")
	}
	myIdent := string(my.AppendIdent(nil, "db.orders"))
	if myIdent != "`db`.`orders`" {
		t.Fatalf("unexpected mysql ident quoting: %s", myIdent)
	}

	ms := dialect.NewMSSQL()
	msIdent := string(ms.AppendIdent(nil, "dbo.orders"))
	if msIdent != "[dbo].[orders]" {
		t.Fatalf("unexpected mssql ident quoting: %s", msIdent)
	}
}

// TestSelectQueryBasic tests SELECT, FROM, WHERE, ORDER BY, LIMIT, OFFSET.
func TestSelectQueryBasic(t *testing.T) {
	db := rdbms.NewDB(nil, dialect.NewPostgreSQL())
	q := db.NewSelect().
		Table("users").
		Column("id", "name", "email").
		Where("status = ?", "active").
		WhereOr("role = ?", "admin").
		Order("id ASC").
		Limit(10).
		Offset(20)

	sqlStr := q.String()

	if !strings.Contains(sqlStr, `SELECT "id", "name", "email" FROM "users"`) {
		t.Fatalf("unexpected select clause in: %s", sqlStr)
	}
	if !strings.Contains(sqlStr, `WHERE status = 'active' OR role = 'admin'`) {
		t.Fatalf("unexpected where clause in: %s", sqlStr)
	}
	if !strings.Contains(sqlStr, `ORDER BY id ASC LIMIT 10 OFFSET 20`) {
		t.Fatalf("unexpected order/limit/offset in: %s", sqlStr)
	}
}

// TestSelectQueryWhereGroups tests grouped WHERE conditions.
func TestSelectQueryWhereGroups(t *testing.T) {
	db := rdbms.NewDB(nil, dialect.NewPostgreSQL())
	q := db.NewSelect().
		Table("orders").
		Where("tenant_id = ?", 1).
		WhereGroup(" OR ", func(sq *rdbms.SelectQuery) *rdbms.SelectQuery {
			return sq.Where("status = ?", "pending").Where("total > ?", 100)
		})

	sqlStr := q.String()
	expected := `SELECT * FROM "orders" WHERE tenant_id = 1 AND (status = 'pending' OR total > 100)`
	if sqlStr != expected {
		t.Fatalf("expected:\n%s\ngot:\n%s", expected, sqlStr)
	}
}

// TestSelectQueryUnions tests UNION, INTERSECT, EXCEPT and set operations.
func TestSelectQueryUnions(t *testing.T) {
	db := rdbms.NewDB(nil, dialect.NewPostgreSQL())

	q1 := db.NewSelect().Table("archived_users").Column("id", "email")
	q2 := db.NewSelect().Table("deleted_users").Column("id", "email")

	mainQ := db.NewSelect().
		Table("active_users").
		Column("id", "email").
		Union(q1).
		UnionAll(q2)

	sqlStr := mainQ.String()

	if !strings.HasPrefix(sqlStr, "(SELECT") {
		t.Fatalf("expected union query to be wrapped in parentheses: %s", sqlStr)
	}
	if !strings.Contains(sqlStr, `UNION (SELECT "id", "email" FROM "archived_users")`) {
		t.Fatalf("missing UNION q1 in: %s", sqlStr)
	}
	if !strings.Contains(sqlStr, `UNION ALL (SELECT "id", "email" FROM "deleted_users")`) {
		t.Fatalf("missing UNION ALL q2 in: %s", sqlStr)
	}
}

// TestSelectQueryJoins tests explicit JOIN clauses with ON and ON OR.
func TestSelectQueryJoins(t *testing.T) {
	db := rdbms.NewDB(nil, dialect.NewPostgreSQL())
	q := db.NewSelect().
		Table("users").
		Column("users.id", "profiles.bio").
		Join("JOIN profiles").
		JoinOn("profiles.user_id = users.id").
		JoinOnOr("profiles.backup_user_id = users.id")

	sqlStr := q.String()
	if !strings.Contains(sqlStr, `JOIN profiles ON (profiles.user_id = users.id) OR (profiles.backup_user_id = users.id)`) {
		t.Fatalf("unexpected join query: %s", sqlStr)
	}
}

// TestSelectQueryMySQLIndexHints tests MySQL-specific index hint generation.
func TestSelectQueryMySQLIndexHints(t *testing.T) {
	db := rdbms.NewDB(nil, dialect.NewMySQL())
	q := db.NewSelect().
		Table("orders").
		UseIndex("idx_orders_tenant").
		UseIndexForJoin("idx_orders_customer").
		ForceIndexForOrderBy("idx_orders_created")

	sqlStr := q.String()
	if !strings.Contains(sqlStr, "USE INDEX (idx_orders_tenant)") {
		t.Fatalf("missing USE INDEX in: %s", sqlStr)
	}
	if !strings.Contains(sqlStr, "USE INDEX FOR JOIN (idx_orders_customer)") {
		t.Fatalf("missing USE INDEX FOR JOIN in: %s", sqlStr)
	}
	if !strings.Contains(sqlStr, "FORCE INDEX FOR ORDER BY (idx_orders_created)") {
		t.Fatalf("missing FORCE INDEX FOR ORDER BY in: %s", sqlStr)
	}
}

// Mock TableModel with nested child relations to test LoadWithChildren configuration.
type testAuthorModel struct {
	table *schema.Table
	joins []*schema.RelationJoin
}

func (m *testAuthorModel) Table() *schema.Table {
	return m.table
}

func (m *testAuthorModel) Clone() schema.TableModel {
	return m
}

func (m *testAuthorModel) GetJoins() []*schema.RelationJoin {
	return m.joins
}

func (m *testAuthorModel) Join(name string) *schema.RelationJoin {
	for _, j := range m.joins {
		if j.Relation != nil && j.Relation.Name == name {
			return j
		}
	}
	return nil
}

type testBookModel struct {
	table *schema.Table
	joins []*schema.RelationJoin
}

func (m *testBookModel) Table() *schema.Table {
	return m.table
}

func (m *testBookModel) Clone() schema.TableModel {
	return m
}

func (m *testBookModel) GetJoins() []*schema.RelationJoin {
	return m.joins
}

func (m *testBookModel) Join(name string) *schema.RelationJoin {
	for _, j := range m.joins {
		if j.Relation != nil && j.Relation.Name == name {
			return j
		}
	}
	return nil
}

// TestRelationLoadWithChildren verifies that relation joins honor the LoadWithChildren configuration.
func TestRelationLoadWithChildren(t *testing.T) {
	db := rdbms.NewDB(nil, dialect.NewPostgreSQL())

	// Profile table (child of Author)
	profileTable := schema.NewTable("profiles")
	profileTable.AddField(&schema.Field{Name: "bio", SQLName: "bio"})
	profileModel := &testAuthorModel{table: profileTable}

	// Author table (parent of Profile, child of Book)
	authorTable := schema.NewTable("authors")
	authorTable.AddField(&schema.Field{Name: "id", SQLName: "id", IsPK: true})
	authorTable.AddField(&schema.Field{Name: "name", SQLName: "name"})

	profileRel := &schema.Relation{
		Name:      "Profile",
		Type:      schema.HasOneRelation,
		JoinTable: profileTable,
	}
	authorModel := &testAuthorModel{
		table: authorTable,
		joins: []*schema.RelationJoin{
			schema.NewRelationJoin(profileRel, profileModel),
		},
	}

	// Book table (root)
	bookTable := schema.NewTable("books")
	bookTable.AddField(&schema.Field{Name: "id", SQLName: "id", IsPK: true})
	bookTable.AddField(&schema.Field{Name: "title", SQLName: "title"})

	authorRel := &schema.Relation{
		Name:      "Author",
		Type:      schema.BelongsToRelation,
		JoinTable: authorTable,
	}
	bookModel := &testBookModel{
		table: bookTable,
		joins: []*schema.RelationJoin{
			schema.NewRelationJoin(authorRel, authorModel),
		},
	}

	// Case 1: Query with LoadWithChildren = true (loads Book -> Author -> Profile)
	q1 := db.NewSelect().
		Model(bookModel).
		Relation("Author").
		LoadWithChildren(true)

	sqlWithChildren := q1.String()
	if !strings.Contains(sqlWithChildren, `"Author__name"`) {
		t.Fatalf("expected Author columns loaded: %s", sqlWithChildren)
	}
	if !strings.Contains(sqlWithChildren, `"Profile__bio"`) {
		t.Fatalf("expected nested child Profile columns loaded when LoadWithChildren=true: %s", sqlWithChildren)
	}

	// Case 2: Query with LoadWithChildren = false (loads Book -> Author only, omits child Profile)
	q2 := db.NewSelect().
		Model(bookModel).
		RelationWithOpts("Author", query.RelationOpts{
			LoadWithChildren: false,
		})

	sqlWithoutChildren := q2.String()
	if !strings.Contains(sqlWithoutChildren, `"Author__name"`) {
		t.Fatalf("expected Author columns loaded: %s", sqlWithoutChildren)
	}
	if strings.Contains(sqlWithoutChildren, `"Profile__bio"`) {
		t.Fatalf("expected nested child Profile columns to be omitted when LoadWithChildren=false: %s", sqlWithoutChildren)
	}
}

// TestSelectQueryCTEs tests WITH and WITH RECURSIVE generation.
func TestSelectQueryCTEs(t *testing.T) {
	db := rdbms.NewDB(nil, dialect.NewPostgreSQL())

	sub := db.NewSelect().Table("departments").Where("parent_id IS NULL")
	q := db.NewSelect().
		WithRecursive("dept_tree", sub).
		Table("dept_tree").
		Column("id", "name")

	sqlStr := q.String()
	if !strings.HasPrefix(sqlStr, `WITH RECURSIVE "dept_tree" AS (SELECT * FROM "departments" WHERE parent_id IS NULL)`) {
		t.Fatalf("unexpected CTE output: %s", sqlStr)
	}
}

// TestSelectQueryBuilderInterface verifies the fluent QueryBuilder wrapper.
func TestSelectQueryBuilderInterface(t *testing.T) {
	db := rdbms.NewDB(nil, dialect.NewPostgreSQL())

	applyFilter := func(qb query.QueryBuilder) query.QueryBuilder {
		return qb.Where("active = ?", true).WherePK("id")
	}

	table := schema.NewTable("accounts")
	table.AddField(&schema.Field{Name: "id", SQLName: "id", IsPK: true})
	tableModel := &testAuthorModel{table: table}
	q := db.NewSelect().
		Model(tableModel).
		ApplyQueryBuilder(applyFilter)

	sqlStr := q.String()
	if !strings.Contains(sqlStr, "active = TRUE") || !strings.Contains(sqlStr, "id = ?") {
		t.Fatalf("ApplyQueryBuilder failed: %s", sqlStr)
	}
}

// TestNewSelectFromQuery verifies that a unified query.Query converts to an RDBMS SelectQuery seamlessly.
func TestNewSelectFromQuery(t *testing.T) {
	db := rdbms.NewDB(nil, dialect.NewPostgreSQL())

	uq := coreQuery.New().
		Table("users").
		Column("id", "name", "email").
		Where("status = ?", "active").
		WhereOr("role = ?", "admin").
		WhereGroup(" AND ", func(sub coreQuery.Query) coreQuery.Query {
			return sub.Where("age >= ?", 18).Where("verified = ?", true)
		}).
		OrderBy("created_at", coreQuery.SortDesc).
		Limit(25).
		Offset(50)

	sq := rdbms.NewSelectFromQuery(db, uq)
	sqlStr := sq.String()

	if !strings.Contains(sqlStr, `SELECT "id", "name", "email" FROM "users"`) {
		t.Fatalf("unexpected select clause: %s", sqlStr)
	}
	if !strings.Contains(sqlStr, `WHERE status = 'active' OR role = 'admin' AND (age >= 18 AND verified = TRUE)`) {
		t.Fatalf("unexpected where clause: %s", sqlStr)
	}
	if !strings.Contains(sqlStr, `ORDER BY created_at DESC LIMIT 25 OFFSET 50`) {
		t.Fatalf("unexpected order/limit/offset: %s", sqlStr)
	}
}

