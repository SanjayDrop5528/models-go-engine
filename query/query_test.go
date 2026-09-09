package query_test

import (
	"testing"

	"github.com/SanjayDrop5528/models-go-engine/query"
)

func TestUnifiedQueryBuilder(t *testing.T) {
	q := query.New().
		Table("users").
		Column("id", "name", "email").
		ColumnExpr("COUNT(*) OVER()", "total_count").
		ExcludeColumn("password").
		Distinct().
		DistinctOn("email").
		Where("status = ?", "active").
		WhereFilter("age", query.OpGte, 18).
		WhereOr("role = ?", "admin").
		WhereGroup(" AND ", func(sub query.Query) query.Query {
			return sub.Where("verified = ?", true).Where("country = ?", "US")
		}).
		Group("department").
		Having("COUNT(*) > ?", 5).
		Order("name ASC").
		OrderBy("created_at", query.SortDesc).
		Limit(25).
		Offset(50).
		Join("profiles", "profiles.user_id = users.id").
		JoinOn("profiles.active = ?", true).
		Relation("Roles").
		RelationWithOpts("Manager", query.RelationOpts{
			LoadWithChildren: false,
		}).
		WithChildren(false).
		WithRelations("Avatar").
		UseIndex("idx_users_email").
		Comment("fetch active verified users")

	if len(q.Tables) != 1 || q.Tables[0] != "users" {
		t.Fatalf("unexpected tables: %v", q.Tables)
	}
	if len(q.Fields) != 3 || q.Fields[0] != "id" {
		t.Fatalf("unexpected fields: %v", q.Fields)
	}
	if len(q.ColumnExprs) != 1 {
		t.Fatalf("unexpected column exprs: %v", q.ColumnExprs)
	}
	if len(q.ExcludedColumns) != 1 || q.ExcludedColumns[0] != "password" {
		t.Fatalf("unexpected excluded columns: %v", q.ExcludedColumns)
	}
	if !q.IsDistinct {
		t.Fatalf("expected IsDistinct to be true")
	}
	if len(q.DistinctFields) != 1 || q.DistinctFields[0] != "email" {
		t.Fatalf("unexpected distinct fields: %v", q.DistinctFields)
	}
	if len(q.Filters) != 1 || q.Filters[0].Field != "age" {
		t.Fatalf("unexpected filters: %v", q.Filters)
	}
	if len(q.RawWheres) != 2 {
		t.Fatalf("unexpected raw wheres: %v", q.RawWheres)
	}
	if len(q.WhereGroups) != 1 {
		t.Fatalf("unexpected where groups: %v", q.WhereGroups)
	}
	if len(q.Groups) != 1 || q.Groups[0] != "department" {
		t.Fatalf("unexpected groups: %v", q.Groups)
	}
	if len(q.Havings) != 1 {
		t.Fatalf("unexpected havings: %v", q.Havings)
	}
	if len(q.Sorts) != 2 {
		t.Fatalf("unexpected sorts: %v", q.Sorts)
	}
	if q.Pagination.Limit != 25 || q.Pagination.Offset != 50 {
		t.Fatalf("unexpected pagination: %+v", q.Pagination)
	}
	if len(q.Joins) != 1 || q.Joins[0].Table != "profiles" {
		t.Fatalf("unexpected joins: %v", q.Joins)
	}
	if len(q.Relations) != 3 {
		t.Fatalf("unexpected relations count: %v", q.Relations)
	}
	if q.LoadWithChildren != false {
		t.Fatalf("expected LoadWithChildren false")
	}
	if len(q.IndexHints.Use) != 1 || q.IndexHints.Use[0] != "idx_users_email" {
		t.Fatalf("unexpected index hints: %+v", q.IndexHints)
	}
	if q.CommentText != "fetch active verified users" {
		t.Fatalf("unexpected comment: %s", q.CommentText)
	}
}
