package main

import (
	"testing"
	"time"	
	"database/sql"
	"os"
)

func TestCreateTables(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Try to insert test data to verify tables exist and have correct schema
	_, err := db.Exec(`
		INSERT INTO locations (slug, country) 
		VALUES ('test-location', 'AUS')`)
	if err != nil {
		t.Errorf("Failed to insert into locations: %v", err)
	}

	_, err = db.Exec(`
		INSERT INTO events (event_number, location_id, date, url) 
		VALUES (1, 1, '2023-01-01', 'http://example.com')`)
	if err != nil {
		t.Errorf("Failed to insert into events: %v", err)
	}

	_, err = db.Exec(`
		INSERT INTO results (position, name, time_seconds, age_grade, age_category, total_runs, event_id)
		VALUES (1, 'Test Runner', 1200, '65.5%', 'VM35-39', 1, 1)`)
	if err != nil {
		t.Errorf("Failed to insert into results: %v", err)
	}
}

func TestGetNextEventNumber(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Test with empty database
	eventNum := GetNextEventNumber(db, 1)
	if eventNum != 1 {
		t.Errorf("Expected first event number to be 1, got %d", eventNum)
	}

	// Insert some events
	_, err := db.Exec(`
		INSERT INTO locations (id, slug, country) 
		VALUES (1, 'test-location', 'AUS')`)
	if err != nil {
		t.Fatal(err)
	}

	_, err = db.Exec(`
		INSERT INTO events (event_number, location_id, date, url) VALUES 
		(1, 1, '2023-01-01', 'http://example.com/1'),
		(2, 1, '2023-01-08', 'http://example.com/2')`)
	if err != nil {
		t.Fatal(err)
	}

	// Test with existing events
	eventNum = GetNextEventNumber(db, 1)
	if eventNum != 3 {
		t.Errorf("Expected next event number to be 3, got %d", eventNum)
	}

	// Test with different location
	eventNum = GetNextEventNumber(db, 2)
	if eventNum != 1 {
		t.Errorf("Expected first event number for new location to be 1, got %d", eventNum)
	}
}

func TestClearLocationData(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Insert test data
	_, err := db.Exec(`
		INSERT INTO locations (id, slug, country) VALUES 
		(1, 'location-1', 'AUS'),
		(2, 'location-2', 'AUS')`)
	if err != nil {
		t.Fatal(err)
	}

	_, err = db.Exec(`
		INSERT INTO events (id, event_number, location_id, date, url) VALUES 
		(1, 1, 1, '2023-01-01', 'http://example.com/1'),
		(2, 1, 2, '2023-01-01', 'http://example.com/2')`)
	if err != nil {
		t.Fatal(err)
	}

	_, err = db.Exec(`
		INSERT INTO results (position, name, time_seconds, event_id) VALUES 
		(1, 'Runner A', 1200, 1),
		(1, 'Runner B', 1300, 2)`)
	if err != nil {
		t.Fatal(err)
	}

	// Clear location 1
	err = ClearLocationData(db, "location-1")
	if err != nil {
		t.Fatalf("Failed to clear location: %v", err)
	}

	// Verify location 1 data is gone
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM locations WHERE slug = 'location-1'").Scan(&count)
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Error("Location was not deleted")
	}

	err = db.QueryRow("SELECT COUNT(*) FROM events WHERE location_id = 1").Scan(&count)
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Error("Events were not deleted")
	}

	err = db.QueryRow("SELECT COUNT(*) FROM results WHERE event_id = 1").Scan(&count)
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Error("Results were not deleted")
	}

	// Verify location 2 data still exists
	err = db.QueryRow("SELECT COUNT(*) FROM locations WHERE slug = 'location-2'").Scan(&count)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Error("Other location was incorrectly deleted")
	}
}

func TestStoreEvent(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Insert test location
	_, err := db.Exec(`
		INSERT INTO locations (id, slug, country) 
		VALUES (1, 'test-location', 'AUS')`)
	if err != nil {
		t.Fatal(err)
	}

	// Test storing new event
	event := Event{
		EventNumber: 1,
		LocationID:  1,
		Date:        parseDate(t, "2023-01-01"),
		URL:         "http://example.com/1",
	}

	eventID, err := StoreEvent(db, event)
	if err != nil {
		t.Fatalf("Failed to store event: %v", err)
	}
	if eventID <= 0 {
		t.Error("Expected positive event ID")
	}

	// Verify stored event
	var storedEvent Event
	err = db.QueryRow(`
		SELECT event_number, location_id, date, url 
		FROM events WHERE id = ?`, eventID).Scan(
		&storedEvent.EventNumber,
		&storedEvent.LocationID,
		&storedEvent.Date,
		&storedEvent.URL)
	if err != nil {
		t.Fatal(err)
	}

	if storedEvent.EventNumber != event.EventNumber {
		t.Errorf("Event number mismatch: got %d, want %d", storedEvent.EventNumber, event.EventNumber)
	}
	if storedEvent.LocationID != event.LocationID {
		t.Errorf("Location ID mismatch: got %d, want %d", storedEvent.LocationID, event.LocationID)
	}
	if !storedEvent.Date.Equal(event.Date) {
		t.Errorf("Date mismatch: got %v, want %v", storedEvent.Date, event.Date)
	}
	if storedEvent.URL != event.URL {
		t.Errorf("URL mismatch: got %s, want %s", storedEvent.URL, event.URL)
	}
}

func TestStoreResults(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Insert test location and event
	_, err := db.Exec(`
		INSERT INTO locations (id, slug, country) 
		VALUES (1, 'test-location', 'AUS')`)
	if err != nil {
		t.Fatal(err)
	}

	_, err = db.Exec(`
		INSERT INTO events (id, event_number, location_id, date, url) 
		VALUES (1, 1, 1, '2023-01-01', 'http://example.com/1')`)
	if err != nil {
		t.Fatal(err)
	}

	// Test storing results
	results := []Result{
		{
			Position:    1,
			Name:        "Runner A",
			TimeSeconds: 1200,
			AgeGrade:    "65.5%",
			AgeCategory: "VM35-39",
			TotalRuns:   10,
			EventID:     1,
		},
		{
			Position:    2,
			Name:        "Runner B",
			TimeSeconds: 1300,
			AgeGrade:    "60.2%",
			AgeCategory: "VM40-44",
			TotalRuns:   5,
			EventID:     1,
		},
	}

	StoreResults(db, results, 1)

	// Verify stored results
	rows, err := db.Query(`
		SELECT position, name, time_seconds, age_grade, age_category, total_runs, event_id
		FROM results WHERE event_id = 1 ORDER BY position`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	var storedResults []Result
	for rows.Next() {
		var r Result
		err := rows.Scan(
			&r.Position,
			&r.Name,
			&r.TimeSeconds,
			&r.AgeGrade,
			&r.AgeCategory,
			&r.TotalRuns,
			&r.EventID,
		)
		if err != nil {
			t.Fatal(err)
		}
		storedResults = append(storedResults, r)
	}

	if len(storedResults) != len(results) {
		t.Errorf("Expected %d results, got %d", len(results), len(storedResults))
	}

	for i, want := range results {
		got := storedResults[i]
		if got.Position != want.Position {
			t.Errorf("Position mismatch at %d: got %d, want %d", i, got.Position, want.Position)
		}
		if got.Name != want.Name {
			t.Errorf("Name mismatch at %d: got %s, want %s", i, got.Name, want.Name)
		}
		if got.TimeSeconds != want.TimeSeconds {
			t.Errorf("Time mismatch at %d: got %d, want %d", i, got.TimeSeconds, want.TimeSeconds)
		}
		if got.AgeGrade != want.AgeGrade {
			t.Errorf("Age grade mismatch at %d: got %s, want %s", i, got.AgeGrade, want.AgeGrade)
		}
		if got.AgeCategory != want.AgeCategory {
			t.Errorf("Age category mismatch at %d: got %s, want %s", i, got.AgeCategory, want.AgeCategory)
		}
		if got.TotalRuns != want.TotalRuns {
			t.Errorf("Total runs mismatch at %d: got %d, want %d", i, got.TotalRuns, want.TotalRuns)
		}
		if got.EventID != want.EventID {
			t.Errorf("Event ID mismatch at %d: got %d, want %d", i, got.EventID, want.EventID)
		}
	}
}

// Test database setup
func setupTestDB(t *testing.T) (*sql.DB, func()) {
	// Create a temporary database file
	tmpfile, err := os.CreateTemp("", "parkrun_test_*.db")
	if err != nil {
		t.Fatalf("Could not create temp file: %v", err)
	}
	tmpfile.Close()

	// Open the database
	db, err := sql.Open("sqlite3", tmpfile.Name())
	if err != nil {
		os.Remove(tmpfile.Name())
		t.Fatalf("Could not open database: %v", err)
	}

	// Create tables
	CreateTables(db)

	// Return cleanup function
	cleanup := func() {
		db.Close()
		os.Remove(tmpfile.Name())
	}

	return db, cleanup
}

// Helper function to insert test data
func insertTestData(t *testing.T, db *sql.DB) {
	// Insert test locations
	_, err := db.Exec(`
		INSERT INTO locations (id, slug, country) VALUES 
		(1, 'test-park-1', 'AUS'),
		(2, 'test-park-2', 'AUS')`)
	if err != nil {
		t.Fatalf("Could not insert test locations: %v", err)
	}

	// Insert test events
	_, err = db.Exec(`
		INSERT INTO events (id, event_number, location_id, date, url) VALUES 
		(1, 1, 1, '2023-01-01', 'http://example.com/1'),
		(2, 2, 1, '2023-01-08', 'http://example.com/2'),
		(3, 1, 2, '2023-01-01', 'http://example.com/3')`)
	if err != nil {
		t.Fatalf("Could not insert test events: %v", err)
	}

	// Insert test results
	_, err = db.Exec(`
		INSERT INTO results (position, name, time_seconds, age_grade, age_category, total_runs, event_id) VALUES 
		(1, 'Runner A', 1200, '65.5%', 'VM35-39', 10, 1),
		(2, 'Runner B', 1500, '60.2%', 'VM40-44', 5, 1),
		(3, 'Runner A', 1180, '66.0%', 'VM35-39', 11, 2),
		(4, 'Runner D', 1190, '65.8%', 'VM35-39', 3, 2),
		(1, 'Runner C', 1300, '70.1%', 'VW35-39', 1, 3)`)
	if err != nil {
		t.Fatalf("Could not insert test results: %v", err)
	}
}

// Helper function to parse date strings in tests
func parseDate(t *testing.T, dateStr string) time.Time {
	date, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		t.Fatal(err)
	}
	return date
}
