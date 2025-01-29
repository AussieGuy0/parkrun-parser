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
	fmt.Println("Commands:")
	fmt.Println("  Parse:    parkrun parse [--clear] <parkrun-slug>")
	fmt.Println("  Report:   parkrun report <parkrun-slug>")
	fmt.Println("  Compare:  parkrun compare <parkrun-slug1> <parkrun-slug2>")
	fmt.Println("\nFlags for parse command:")
	fmt.Println("  --clear    Clear existing location data before parsing")
	fmt.Println("\nExamples:")
	fmt.Println("  parkrun parse oaklandsestatereserve")
	fmt.Println("  parkrun report oaklandsestatereserve")
	fmt.Println("  parkrun compare bushy westerfolds")
}

func parseAndStoreResults(urlSlug string, clearData bool) {
	db, err := sql.Open("sqlite3", "./parkrun.db")
	if err != nil {
		log.Fatal("Failed to connect to database:", err)
	}
	defer db.Close()
	log.Printf("Successfully connected to database")

	CreateTables(db)

	// Clear existing data if requested
	if clearData {
		err := ClearLocationData(db, urlSlug)
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
	eventID := GetNextEventNumber(db, locationID)
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
		dbEventID, err := StoreEvent(db, event)
		if err != nil {
			log.Printf("Error storing event %d: %v", eventID, err)
			continue
		}

		// Store results with the correct event ID
		if len(results) > 0 {
			StoreResults(db, results, dbEventID)
		}

		eventID++
		time.Sleep(waitBetweenRequests)
	}

	log.Printf("Scraping complete. Processed up to event %d", eventID-1)
}
