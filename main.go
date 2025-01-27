package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

func main() {
	flag.Parse()
	urlSlug := flag.Arg(0)
	if urlSlug == "" {
		fmt.Println("Usage: go run . <parkrun-slug>")
		fmt.Println("Example: go run . oaklandsestatereserve")
		os.Exit(1)
	}

	log.Printf("Starting parkrun scraper for %s...", urlSlug)
	parseAndStoreResults(urlSlug)
}

func parseAndStoreResults(urlSlug string) {
	db, err := sql.Open("sqlite3", "./parkrun.db")
	if err != nil {
		log.Fatal("Failed to connect to database:", err)
	}
	defer db.Close()
	log.Printf("Successfully connected to database")

	createTables(db)

	// Insert or get location
	var locationID int
	err = db.QueryRow(`
		INSERT OR IGNORE INTO locations (slug, country) 
		VALUES (?, ?) 
		RETURNING id`, urlSlug, "AUS").Scan(&locationID)

	if err != nil {
		// If insert didn't return id, get the existing one
		err = db.QueryRow(`
			SELECT id FROM locations 
			WHERE slug = ?`, urlSlug).Scan(&locationID)
		if err != nil {
			log.Fatal("Failed to get location ID:", err)
		}
	}
	log.Printf("Using location ID: %d", locationID)

	//  Database might be non-empty, so start from the next event number.
	eventID := getNextEventNumber(db, locationID)
	log.Printf("Starting from event number: %d", eventID)

	waitBetweenRequests := 5 * time.Second
	rateLimitBackoff := 180 * time.Second
	consecutiveErrors := 0
	maxConsecutiveErrors := 3 // Stop after 3 consecutive errors

	for {
		event, results, err := ParseResults(urlSlug, eventID)
		if err != nil {
			log.Printf("Error processing event %d: %v", eventID, err)

			if httpErr, ok := err.(*HTTPError); ok {
				switch httpErr.StatusCode {
				case 405:
					log.Printf("Rate limited, waiting %d seconds before retry...", rateLimitBackoff/time.Second)
					time.Sleep(rateLimitBackoff)
					continue
				case 425:
					log.Printf("Reached end of events (425 error). Scraping complete.")
					return
				}
			}

			consecutiveErrors++
			if consecutiveErrors >= maxConsecutiveErrors {
				log.Printf("Reached %d consecutive errors. Stopping.", maxConsecutiveErrors)
				break
			}
			time.Sleep(waitBetweenRequests)
			continue
		}

		// Set the location ID for the event
		event.LocationID = locationID

		// Reset error counter on success
		consecutiveErrors = 0

		// Store event data
		err = storeEvent(db, event)
		if err != nil {
			log.Printf("Error storing event %d: %v", eventID, err)
		}

		// Store results
		if len(results) > 0 {
			storeResults(db, results)
		}

		eventID++
		time.Sleep(waitBetweenRequests)
	}

	log.Printf("Scraping complete. Processed up to event %d", eventID-1)
}

func createTables(db *sql.DB) {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS locations (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			slug TEXT UNIQUE,
			name TEXT,
			country TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS events (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			event_number INTEGER,
			location_id INTEGER,
			date DATE,
			url TEXT,
			UNIQUE(event_number, location_id),
			FOREIGN KEY (location_id) REFERENCES locations(id)
		)`,
		`CREATE TABLE IF NOT EXISTS results (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			position INTEGER,
			name TEXT,
			time TEXT,
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

func storeEvent(db *sql.DB, event Event) error {
	query := `
	INSERT OR REPLACE INTO events (
		event_number, location_id, date, url
	) VALUES (?, ?, ?, ?)`

	_, err := db.Exec(query, event.EventNumber, event.LocationID, event.Date, event.URL)
	return err
}

func storeResults(db *sql.DB, results []Result) {
	query := `
	INSERT OR REPLACE INTO results (
		position, name, time, age_grade, age_category, note, total_runs, event_id
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`

	successCount := 0
	errorCount := 0

	for _, result := range results {
		_, err := db.Exec(query,
			result.Position,
			result.Name,
			result.Time,
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

func getNextEventNumber(db *sql.DB, locationID int) int {
	var eventID int = 1
	err := db.QueryRow(`
		SELECT COALESCE(MAX(event_number), 1)
		FROM events 
		WHERE location_id = ?`, locationID).Scan(&eventID)
	if err != nil {
		log.Printf("Error getting last event number: %v, starting from 1", err)
		return 1
	}
	return eventID + 1
}
