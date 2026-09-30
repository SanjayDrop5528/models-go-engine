package query

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/SanjayDrop5528/models-go-engine/rdbms/dialect"
)

type mutationFixture struct {
	ID   int    `model:"id"`
	Name string `model:"name"`
}

func (mutationFixture) TableName() string { return "fixtures" }

func TestInsertAndUpdateQueries(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	db := NewDB(database, dialect.NewPostgreSQL())
	fixture := mutationFixture{ID: 7, Name: "engine"}
	mock.ExpectExec(`INSERT INTO fixtures \(id, name\) VALUES \(\$1, \$2\)`).
		WithArgs(7, "engine").
		WillReturnResult(sqlmock.NewResult(0, 1))
	if _, err := db.NewInsert().Model(&fixture).Exec(context.Background()); err != nil {
		t.Fatal(err)
	}

	mock.ExpectExec(`UPDATE fixtures SET name = \$1 WHERE id = \$2`).
		WithArgs("engine", 7).
		WillReturnResult(sqlmock.NewResult(0, 1))
	if _, err := db.NewUpdate().Model(&fixture).Column("name").WherePK().Exec(context.Background()); err != nil {
		t.Fatal(err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestRawQueryExpandsInValues(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	mock.ExpectQuery(`SELECT name FROM fixtures WHERE id IN \(\$1, \$2\)`).
		WithArgs(3, 4).
		WillReturnRows(sqlmock.NewRows([]string{"name"}).AddRow("adapter"))

	db := NewDB(database, dialect.NewPostgreSQL())
	var name string
	if err := db.NewRaw("SELECT name FROM fixtures WHERE id IN (?)", In([]int{3, 4})).Scan(context.Background(), &name); err != nil {
		t.Fatal(err)
	}
	if name != "adapter" {
		t.Fatalf("unexpected name: %q", name)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
