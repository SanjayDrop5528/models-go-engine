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

func TestRawQueryScanClosesRowsInTx(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	mock.ExpectBegin()
	mock.ExpectQuery(`INSERT INTO fixtures \(id, name\) VALUES \(\$1, \$2\) RETURNING id, name`).
		WithArgs(10, "created").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name"}).AddRow(10, "created"))
	mock.ExpectExec(`INSERT INTO events \(name\) VALUES \(\$1\)`).
		WithArgs("created").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	db := NewDB(database, dialect.NewPostgreSQL())
	ctx := context.Background()
	err = db.RunInTx(ctx, nil, func(ctx context.Context, tx *Tx) error {
		var f mutationFixture
		if err := tx.NewRaw(`INSERT INTO fixtures (id, name) VALUES (?, ?) RETURNING id, name`, 10, "created").Scan(ctx, &f); err != nil {
			return err
		}
		if f.ID != 10 || f.Name != "created" {
			t.Fatalf("unexpected scanned fixture: %+v", f)
		}
		if _, err := tx.NewRaw(`INSERT INTO events (name) VALUES (?)`, "created").Exec(ctx); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

