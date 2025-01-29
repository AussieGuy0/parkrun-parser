package main

import (
	"database/sql"
	"fmt"
	"log"
)

// CreateTables creates the necessary database tables if they don't exist
func CreateTables(db *sql.DB) {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS locations (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			slug TEXT UNIQUE NOT NULL,
			name TEXT,
			country TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS events (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			event_number INTEGER NOT NULL,
			location_id INTEGER NOT NULL,
			date DATE NOT NULL,
			url TEXT NOT NULL,
			UNIQUE(event_number, location_id),
			FOREIGN KEY (location_id) REFERENCES locations(id)
		)`,
		`CREATE TABLE IF NOT EXISTS results (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			position INTEGER NOT NULL,
			name TEXT NOT NULL,
			time_seconds INTEGER,
			age_grade TEXT,
			age_category TEXT,
			note TEXT,
			total_runs INTEGER,
			event_id INTEGER,
			UNIQUE(position, event_id),
			FOREIGN KEY (event_id) REFERENCES events(id)
		)`,
	}

	for _, query := range queries {
		_, err := db.Exec(query)
		if err != nil {
			log.Fatal("Failed to create table:", err)
		}
	}
	log.Printf("Database tables ready")
}

// StoreEvent stores an event in the database and returns its ID
func StoreEvent(db *sql.DB, event Event) (int64, error) {
	query := `
	INSERT OR REPLACE INTO events (
		event_number, location_id, date, url
	) VALUES (?, ?, ?, ?)`

	result, err := db.Exec(query, event.EventNumber, event.LocationID, event.Date, event.URL)
	if err != nil {
		return 0, err
	}

	return result.LastInsertId()
}

// StoreResults stores multiple results in the database
func StoreResults(db *sql.DB, results []Result, eventID int64) {
	query := `
	INSERT OR REPLACE INTO results (
		position, name, time_seconds, age_grade, age_category, note, total_runs, event_id
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`

	successCount := 0
	errorCount := 0

	for _, result := range results {
		var timeSeconds *int
		if result.TimeSeconds > 0 {
			timeSeconds = &result.TimeSeconds
		}
		result.EventID = eventID
		_, err := db.Exec(query,
			result.Position,
			result.Name,
			timeSeconds,
			result.AgeGrade,
			result.AgeCategory,
			result.Note,
			result.TotalRuns,
			result.EventID,
		)
		if err != nil {
			log.Printf("Error storing result for position %d: %v", result.Position, err)
			errorCount++
			continue
		}
		successCount++
	}

	log.Printf("Database storage complete: %d successful, %d failed", successCount, errorCount)
}

// GetNextEventNumber returns the next event number for a location
func GetNextEventNumber(db *sql.DB, locationID int) int {
	var eventID int = 0
	err := db.QueryRow(`
		SELECT COALESCE(MAX(event_number), 0)
		FROM events 
		WHERE location_id = ?`, locationID).Scan(&eventID)
	if err != nil {
		log.Printf("Error getting last event number: %v, starting from 1", err)
		return 1
	}
	return eventID + 1
}

// ClearLocationData removes all data for a specific location
func ClearLocationData(db *sql.DB, urlSlug string) error {
	// First get the location ID
	var locationID int
	err := db.QueryRow(`SELECT id FROM locations WHERE slug = ?`, urlSlug).Scan(&locationID)
	if err == sql.ErrNoRows {
		// Location doesn't exist, nothing to clear
		return nil
	}
	if err != nil {
		return fmt.Errorf("error finding location: %v", err)
	}

	// Start a transaction to ensure all deletes succeed or none do
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("error starting transaction: %v", err)
	}

	// Delete results for all events at this location
	_, err = tx.Exec(`
		DELETE FROM results 
		WHERE event_id IN (
			SELECT id FROM events WHERE location_id = ?
		)`, locationID)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("error deleting results: %v", err)
	}

	// Delete events for this location
	_, err = tx.Exec(`DELETE FROM events WHERE location_id = ?`, locationID)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("error deleting events: %v", err)
	}

	// Delete the location itself
	_, err = tx.Exec(`DELETE FROM locations WHERE id = ?`, locationID)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("error deleting location: %v", err)
	}

	// Commit the transaction
	err = tx.Commit()
	if err != nil {
		return fmt.Errorf("error committing transaction: %v", err)
	}

	return nil
}
