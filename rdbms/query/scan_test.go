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

type mockVehicle struct {
	ID             string `json:"id"`
	RegistrationNo string `json:"registration_no"`
}

type mockDriver struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type mockTrip struct {
	ID                 string       `json:"id"`
	Status             string       `json:"status"`
	AllocatedDriverID  string       `json:"allocated_driver_id"`
	AllocatedVehicleID string       `json:"allocated_vehicle_id"`
	AllocatedDriver    *mockDriver  `json:"allocated_driver"`
	AllocatedVehicle   *mockVehicle `json:"allocated_vehicle"`
}

func (mockTrip) TableName() string    { return "trip.regular_trips" }
func (mockDriver) TableName() string  { return "iam.employees" }
func (mockVehicle) TableName() string { return "fleet.vehicle" }

func TestSelectScanWithJoinedRelations(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	mock.ExpectQuery(`SELECT "rt"\.\*.*FROM "trip"\."regular_trips" AS "rt".*LEFT JOIN "iam"\."employees".*LEFT JOIN "fleet"\."vehicle"`).
		WillReturnRows(
			sqlmock.NewRows([]string{"id", "status", "allocated_driver_id", "allocated_vehicle_id", "AllocatedDriver__name", "AllocatedVehicle__registration_no"}).
				AddRow("trip-1", "completed", "driver-1", "veh-1", "Aadhavan", "TN 01 AB 1234"),
		)

	db := NewDB(database, dialect.NewPostgreSQL())
	var trips []mockTrip
	if err := db.NewSelect().
		Model(&trips).
		Relation("AllocatedDriver").
		Relation("AllocatedVehicle").
		Scan(context.Background()); err != nil {
		t.Fatal(err)
	}

	if len(trips) != 1 {
		t.Fatalf("expected 1 trip, got %d", len(trips))
	}
	if trips[0].ID != "trip-1" || trips[0].Status != "completed" {
		t.Fatalf("unexpected trip data: %#v", trips[0])
	}
	if trips[0].AllocatedDriver == nil || trips[0].AllocatedDriver.Name != "Aadhavan" {
		t.Fatalf("expected AllocatedDriver to be Aadhavan, got: %#v", trips[0].AllocatedDriver)
	}
	if trips[0].AllocatedVehicle == nil || trips[0].AllocatedVehicle.RegistrationNo != "TN 01 AB 1234" {
		t.Fatalf("expected AllocatedVehicle to be TN 01 AB 1234, got: %#v", trips[0].AllocatedVehicle)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

