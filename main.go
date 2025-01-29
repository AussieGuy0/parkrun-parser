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
	// Define commands
	parseCmd := flag.NewFlagSet("parse", flag.ExitOnError)
	clearData := parseCmd.Bool("clear", false, "Clear existing location data before parsing")

	// Check if we have enough arguments
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	command := os.Args[1]

	switch command {
	case "parse":
		// Parse flags for the parse command
		err := parseCmd.Parse(os.Args[2:])
		if err != nil {
			log.Fatal(err)
		}

		// Check if we have a location argument
		if parseCmd.NArg() < 1 {
			printUsage()
			os.Exit(1)
		}

		urlSlug := parseCmd.Arg(0)
		log.Printf("Starting parkrun scraper for %s...", urlSlug)
		parseAndStoreResults(urlSlug, *clearData)

	case "report":
		if len(os.Args) < 3 {
			printUsage()
			os.Exit(1)
		}

		urlSlug := os.Args[2]
		db, err := sql.Open("sqlite3", "./parkrun.db")
		if err != nil {
			log.Fatal("Failed to connect to database:", err)
		}
		defer db.Close()
		log.Printf("Successfully connected to database")

		log.Printf("Generating report for %s...", urlSlug)
		err = PrintReports(db, urlSlug)
		if err != nil {
			log.Fatal(err)
		}

	case "compare":
		if len(os.Args) != 4 {
			printUsage()
			os.Exit(1)
		}

		location1 := os.Args[2]
		location2 := os.Args[3]

		db, err := sql.Open("sqlite3", "./parkrun.db")
		if err != nil {
			log.Fatal("Failed to connect to database:", err)
		}
		defer db.Close()

		log.Printf("Generating comparison report for %s and %s...", location1, location2)
		err = PrintComparisonReport(db, location1, location2)
		if err != nil {
			log.Fatal(err)
		}

	default:
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("Usage:")
	fmt.Println("  Parse:    go run . parse [--clear] <parkrun-slug>")
	fmt.Println("  Report:   go run . report <parkrun-slug>")
	fmt.Println("  Compare:  go run . compare <parkrun-slug1> <parkrun-slug2>")
	fmt.Println("\nFlags for parse command:")
	fmt.Println("  --clear    Clear existing location data before parsing")
	fmt.Println("\nExamples:")
	fmt.Println("  go run . parse oaklandsestatereserve")
	fmt.Println("  go run . report oaklandsestatereserve")
	fmt.Println("  go run . compare bushy westerfolds")
}

func parseAndStoreResults(urlSlug string, clearData bool) {
	db, err := sql.Open("sqlite3", "./parkrun.db")
	if err != nil {
		log.Fatal("Failed to connect to database:", err)
	}
	defer db.Close()
	log.Printf("Successfully connected to database")

	createTables(db)

	// Clear existing data if requested
	if clearData {
		err := clearLocationData(db, urlSlug)
		if err != nil {
			log.Fatal("Failed to clear existing data:", err)
		}
		log.Printf("Cleared existing data for %s", urlSlug)
	}

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

		event.LocationID = locationID

		// Reset error counter on success
		consecutiveErrors = 0

		// Store event data and get the event ID
		dbEventID, err := storeEvent(db, event)
		if err != nil {
			log.Printf("Error storing event %d: %v", eventID, err)
			continue
		}

		// Store results with the correct event ID
		if len(results) > 0 {
			storeResults(db, results, dbEventID)
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

func storeEvent(db *sql.DB, event Event) (int64, error) {
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

func storeResults(db *sql.DB, results []Result, eventID int64) {
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

func getNextEventNumber(db *sql.DB, locationID int) int {
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

func clearLocationData(db *sql.DB, urlSlug string) error {
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
