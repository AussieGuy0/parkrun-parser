package main

import (
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"net/http"

	"github.com/PuerkitoBio/goquery"
)

type Result struct {
	Position    int
	Name        string
	Time        string // Raw time string
	TimeSeconds int    // Parsed time in seconds
	AgeGrade    string
	AgeCategory string
	Note        string
	TotalRuns   int
	EventID     int64
}

type Event struct {
	EventNumber int
	LocationID  int
	Date        time.Time
	URL         string
}

type Location struct {
	ID   int
	Slug string
	Name string
	// ISO 3166-1 alpha-3 country code
	Country string
}

type HTTPError struct {
	StatusCode int
	Message    string
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("%s (HTTP %d)", e.Message, e.StatusCode)
}

func ParseResults(urlSlug string, eventNumber int) (Event, []Result, error) {
	baseURL := "https://www.parkrun.com.au/%s/results/%d/"
	url := fmt.Sprintf(baseURL, urlSlug, eventNumber)

	return scrapeEvent(url, eventNumber)
}

func scrapeEvent(url string, eventNumber int) (Event, []Result, error) {
	client := &http.Client{}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return Event{}, nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.5")
	req.Header.Set("Connection", "keep-alive")

	resp, err := client.Do(req)
	if err != nil {
		return Event{}, nil, fmt.Errorf("failed to make HTTP request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return Event{}, nil, &HTTPError{
			StatusCode: resp.StatusCode,
			Message:    "HTTP error",
		}
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return Event{}, nil, fmt.Errorf("failed to parse HTML: %w", err)
	}

	// Extract event date from the page
	dateText := doc.Find(".Results-header .format-date").Text()
	log.Printf("Found date text: %s", dateText)

	eventDate, err := parseEventDate(dateText)
	if err != nil {
		log.Printf("Warning: Could not parse date for event %d: %v", eventNumber, err)
	}

	event := Event{
		EventNumber: eventNumber,
		Date:        eventDate,
		URL:         url,
	}

	var results []Result
	processedRows := 0
	skippedRows := 0

	// Find all result rows using the correct class
	resultRows := doc.Find(".Results-table-row")

	resultRows.Each(func(i int, s *goquery.Selection) {
		// Get data attributes
		position, _ := strconv.Atoi(s.AttrOr("data-position", "0"))
		name := s.AttrOr("data-name", "")
		ageGroup := s.AttrOr("data-agegroup", "")

		// Find the time cell
		timeCell := s.Find(".Results-table-td--time .compact").Text()

		// Get total runs from the detailed div
		runsText := s.Find(".detailed").First().Text()
		totalRuns := 0
		if strings.Contains(runsText, "parkrun") {
			runsStr := strings.Split(runsText, " ")[0]
			totalRuns, _ = strconv.Atoi(runsStr)
		}

		// Get age grade and achievement
		ageGrade := s.AttrOr("data-agegrade", "")
		achievement := s.AttrOr("data-achievement", "")

		time := strings.TrimSpace(timeCell)
		timeSeconds := 0
		if name != "Unknown" {
			timeSeconds, err = timeToSeconds(time)
			if err != nil {
				log.Printf("Warning: Could not parse time for position %d: %v", position, err)
				skippedRows++
				return
			}
		}
		result := Result{
			Position:    position,
			Name:        name,
			Time:        time,
			TimeSeconds: timeSeconds,
			AgeGrade:    ageGrade,
			AgeCategory: ageGroup,
			Note:        achievement,
			TotalRuns:   totalRuns,
		}
		results = append(results, result)
		processedRows++

	})

	log.Printf("Processed %d rows, skipped %d invalid rows", processedRows, skippedRows)
	return event, results, nil
}

func parseEventDate(dateText string) (time.Time, error) {
	dateText = strings.TrimSpace(dateText)

	// Try different date formats
	formats := []string{
		"02/01/2006", // DD/MM/YYYY
		"2/1/06",     // D/M/YY
		"2/1/2006",   // D/M/YYYY
	}

	var lastErr error
	for _, format := range formats {
		date, err := time.Parse(format, dateText)
		if err == nil {
			return date, nil
		}
		lastErr = err
	}

	// If we get here, none of the formats worked
	log.Printf("Failed to parse date '%s' with any known format", dateText)
	return time.Time{}, lastErr
}

// timeToSeconds converts a time string (MM:SS or HH:MM:SS) to total seconds
func timeToSeconds(timeStr string) (int, error) {
	if timeStr == "" || timeStr == "Unknown" {
		return 0, nil;
	}

	parts := strings.Split(timeStr, ":")
	if len(parts) == 2 {
		// MM:SS format
		minutes, err := strconv.Atoi(parts[0])
		if err != nil {
			return 0, fmt.Errorf("invalid minutes: %v", err)
		}
		seconds, err := strconv.Atoi(parts[1])
		if err != nil {
			return 0, fmt.Errorf("invalid seconds: %v", err)
		}
		return minutes*60 + seconds, nil
	} else if len(parts) == 3 {
		// HH:MM:SS format
		hours, err := strconv.Atoi(parts[0])
		if err != nil {
			return 0, fmt.Errorf("invalid hours: %v", err)
		}
		minutes, err := strconv.Atoi(parts[1])
		if err != nil {
			return 0, fmt.Errorf("invalid minutes: %v", err)
		}
		seconds, err := strconv.Atoi(parts[2])
		if err != nil {
			return 0, fmt.Errorf("invalid seconds: %v", err)
		}
		return hours*3600 + minutes*60 + seconds, nil
	}
	return 0, fmt.Errorf("invalid time format")
}
