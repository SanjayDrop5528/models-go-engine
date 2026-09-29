package query

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/SanjayDrop5528/models-go-engine/rdbms/dialect"
)

type scanFixture struct {
	ID   int    `model:"id"`
	Name string `model:"display_name"`
}

func TestSelectScanPopulatesStructSlice(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	mock.ExpectQuery(`SELECT \* FROM "fixtures"`).WillReturnRows(
		sqlmock.NewRows([]string{"id", "display_name"}).AddRow(1, "one").AddRow(2, "two"),
	)

	db := NewDB(database, dialect.NewPostgreSQL())
	var result []scanFixture
	if err := db.NewSelect().Table("fixtures").Scan(context.Background(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result) != 2 || result[0].ID != 1 || result[1].Name != "two" {
		t.Fatalf("unexpected scan result: %#v", result)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestSelectScanPopulatesScalar(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	mock.ExpectQuery(`SELECT current_database\(\)`).WillReturnRows(
		sqlmock.NewRows([]string{"current_database"}).AddRow("fleetone"),
	)

	db := NewDB(database, dialect.NewPostgreSQL())
	var name string
	if err := db.NewSelect().ColumnExpr("current_database()").Scan(context.Background(), &name); err != nil {
		t.Fatal(err)
	}
	if name != "fleetone" {
		t.Fatalf("unexpected scalar: %q", name)
	}
}
