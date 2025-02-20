package main

import (
	"database/sql"
	"fmt"
	"sort"
	"time"
)

// RunnerStat represents statistics about a runner
type RunnerStat struct {
	Name       string
	TotalRuns  int
	AgeGrade   float64
	BestTime   string
	FirstEvent time.Time
	LastEvent  time.Time
}

// TimeStats represents time statistics for a group
type TimeStats struct {
	Category string
	Median   string
	Count    int
}

// GetTopParticipants returns the runners with the most parkruns at a location
func GetTopParticipants(db *sql.DB, locationID int, limit int) ([]RunnerStat, error) {
	query := `
		SELECT 
			r.name,
			COUNT(*) as run_count
		FROM results r
		JOIN events e ON r.event_id = e.id
		WHERE e.location_id = ?
		AND r.name != 'Unknown'
		GROUP BY r.name
		ORDER BY run_count DESC
		LIMIT ?`

	rows, err := db.Query(query, locationID, limit)
	if err != nil {
		return nil, fmt.Errorf("query error: %v", err)
	}
	defer rows.Close()

	var stats []RunnerStat
	for rows.Next() {
		var stat RunnerStat
		err := rows.Scan(
			&stat.Name,
			&stat.TotalRuns,
		)
		if err != nil {
			return nil, fmt.Errorf("scan error: %v", err)
		}
		stats = append(stats, stat)
	}

	return stats, nil
}

// GetMedianTimesByAgeCategory calculates median finishing times by age category
func GetMedianTimesByAgeCategory(db *sql.DB, locationID int) ([]TimeStats, error) {
	query := `
		SELECT age_category, time_seconds
		FROM results r
		JOIN events e ON r.event_id = e.id
		WHERE e.location_id = ? 
		AND time_seconds > 0
		AND age_category != ''
		ORDER BY age_category`

	rows, err := db.Query(query, locationID)
	if err != nil {
		return nil, fmt.Errorf("query error: %v", err)
	}
	defer rows.Close()

	// Group times by category
	categoryTimes := make(map[string][]int)
	for rows.Next() {
		var category string
		var timeSeconds int
		if err := rows.Scan(&category, &timeSeconds); err != nil {
			return nil, fmt.Errorf("scan error: %v", err)
		}
		categoryTimes[category] = append(categoryTimes[category], timeSeconds)
	}

	// Calculate median for each category
	var stats []TimeStats
	for category, times := range categoryTimes {
		sort.Ints(times)
		var median int
		n := len(times)
		if n%2 == 0 && n > 0 {
			// For even number of samples, average the two middle values
			median = (times[n/2-1] + times[n/2]) / 2
		} else if n > 0 {
			median = times[n/2]
		}
		stats = append(stats, TimeStats{
			Category: category,
			Median:   secondsToTime(median),
			Count:    len(times),
		})
	}

	// Sort by category
	sort.Slice(stats, func(i, j int) bool {
		return stats[i].Category < stats[j].Category
	})

	return stats, nil
}

// GetLocationStats returns overall statistics for a location
func GetLocationStats(db *sql.DB, locationID int) (map[string]interface{}, error) {
	stats := make(map[string]interface{})

	// Get first and last event dates
	var firstEventStr, lastEventStr string
	err := db.QueryRow(`
		SELECT 
			MIN(date) as first_event,
			MAX(date) as last_event
		FROM events 
		WHERE location_id = ?`, locationID).Scan(&firstEventStr, &lastEventStr)
	if err != nil {
		return nil, fmt.Errorf("event dates error: %v", err)
	}

	// Parse the date strings
	firstEvent, err := parseDateTime(firstEventStr)
	if err != nil {
		return nil, err
	}
	stats["first_event"] = firstEvent

	lastEvent, err := parseDateTime(lastEventStr)
	if err != nil {
		return nil, err
	}
	stats["last_event"] = lastEvent

	// Get biggest and smallest events
	query := `
		SELECT 
			e.date,
			COUNT(*) as participant_count
		FROM events e
		JOIN results r ON e.id = r.event_id
		WHERE e.location_id = ?
		GROUP BY e.id
		ORDER BY participant_count DESC
		LIMIT 1`

	var biggestDate time.Time
	var biggestCount int
	err = db.QueryRow(query, locationID).Scan(&biggestDate, &biggestCount)
	if err != nil {
		return nil, fmt.Errorf("biggest event error: %v", err)
	}
	stats["biggest_event_date"] = biggestDate
	stats["biggest_event_count"] = biggestCount

	// Get smallest event
	query = `
		SELECT 
			e.date,
			COUNT(*) as participant_count
		FROM events e
		JOIN results r ON e.id = r.event_id
		WHERE e.location_id = ?
		GROUP BY e.id
		ORDER BY participant_count ASC
		LIMIT 1`

	var smallestDate time.Time
	var smallestCount int
	err = db.QueryRow(query, locationID).Scan(&smallestDate, &smallestCount)
	if err != nil {
		return nil, fmt.Errorf("smallest event error: %v", err)
	}
	stats["smallest_event_date"] = smallestDate
	stats["smallest_event_count"] = smallestCount

	// Total number of events
	var eventCount int
	err = db.QueryRow(`
		SELECT COUNT(*) 
		FROM events 
		WHERE location_id = ?`, locationID).Scan(&eventCount)
	if err != nil {
		return nil, fmt.Errorf("event count error: %v", err)
	}
	stats["total_events"] = eventCount

	// Total number of runners
	var runnerCount int
	err = db.QueryRow(`
		SELECT COUNT(DISTINCT name) 
		FROM results r
		JOIN events e ON r.event_id = e.id
		WHERE e.location_id = ?`, locationID).Scan(&runnerCount)
	if err != nil {
		return nil, fmt.Errorf("runner count error: %v", err)
	}
	stats["total_runners"] = runnerCount

	// Average participants per event
	var avgParticipants float64
	err = db.QueryRow(`
		SELECT AVG(participant_count)
		FROM (
			SELECT COUNT(*) as participant_count
			FROM results r
			JOIN events e ON r.event_id = e.id
			WHERE e.location_id = ?
			GROUP BY e.id
		) subquery`, locationID).Scan(&avgParticipants)
	if err != nil {
		return nil, fmt.Errorf("avg participants error: %v", err)
	}
	stats["avg_participants"] = avgParticipants

	return stats, nil
}

// calculateMedianTime calculates the median time from a slice of time strings
func calculateMedianTime(times []string) string {
	if len(times) == 0 {
		return "N/A"
	}
	sort.Strings(times)
	return times[len(times)/2]
}

// GetAvailableLocations returns a list of all locations in the database
func GetAvailableLocations(db *sql.DB) ([]string, error) {
	rows, err := db.Query(`
		SELECT slug 
		FROM locations 
		ORDER BY slug`)
	if err != nil {
		return nil, fmt.Errorf("error querying locations: %v", err)
	}
	defer rows.Close()

	var locations []string
	for rows.Next() {
		var slug string
		if err := rows.Scan(&slug); err != nil {
			return nil, fmt.Errorf("error scanning location: %v", err)
		}
		locations = append(locations, slug)
	}
	return locations, nil
}

// PrintReports prints various reports for a location
func PrintReports(db *sql.DB, locationSlug string) error {
	// Get location ID
	var locationID int
	err := db.QueryRow(`SELECT id FROM locations WHERE slug = ?`, locationSlug).Scan(&locationID)
	if err == sql.ErrNoRows {
		// Get available locations
		locations, err := GetAvailableLocations(db)
		if err != nil {
			return fmt.Errorf("location '%s' not found and error getting available locations: %v", locationSlug, err)
		}

		// Build error message
		msg := fmt.Sprintf("Location '%s' not found in database.\n\nAvailable locations:", locationSlug)
		if len(locations) == 0 {
			msg += "\n  No locations found. Try parsing some data first."
		} else {
			for _, loc := range locations {
				msg += fmt.Sprintf("\n  %s", loc)
			}
		}
		return fmt.Errorf(msg)
	}
	if err != nil {
		return fmt.Errorf("database error: %v", err)
	}

	// Print location stats
	stats, err := GetLocationStats(db, locationID)
	if err != nil {
		return err
	}
	fmt.Printf("\n=== Overall Statistics for %s ===\n", locationSlug)
	fmt.Printf("First Event: %s\n", stats["first_event"].(time.Time).Format("2 January 2006"))
	fmt.Printf("Last Event: %s\n", stats["last_event"].(time.Time).Format("2 January 2006"))
	fmt.Printf("Total Events: %d\n", stats["total_events"])
	fmt.Printf("Total Unique Runners: %d\n", stats["total_runners"])
	fmt.Printf("Average Participants per Event: %.1f\n", stats["avg_participants"])
	fmt.Printf("Biggest Event: %d runners (%s)\n",
		stats["biggest_event_count"],
		stats["biggest_event_date"].(time.Time).Format("2 January 2006"))
	fmt.Printf("Smallest Event: %d runners (%s)\n",
		stats["smallest_event_count"],
		stats["smallest_event_date"].(time.Time).Format("2 January 2006"))

	// Print top participants
	runners, err := GetTopParticipants(db, locationID, 10)
	if err != nil {
		return err
	}
	fmt.Printf("\n=== Top 10 Participants ===\n")
	for i, runner := range runners {
		fmt.Printf("%d. %s (%d runs)\n",
			i+1, runner.Name, runner.TotalRuns)
	}

	// Print median times by age category with grouping
	times, err := GetMedianTimesByAgeCategory(db, locationID)
	if err != nil {
		return err
	}

	// Group the times by category type
	groups := make(map[string][]TimeStats)
	groupTimes := make(map[string][]string) // Store all times for each group

	for _, stat := range times {
		prefix := stat.Category[:2]
		var groupName string
		switch prefix {
		case "JM", "JW":
			groupName = "Juniors"
		case "SM", "VM":
			groupName = "Men"
		case "SW", "VW":
			groupName = "Women"
		default:
			groupName = "Other"
		}

		groups[groupName] = append(groups[groupName], stat)
		// Add this category's times to the group's overall times
		for i := 0; i < stat.Count; i++ {
			groupTimes[groupName] = append(groupTimes[groupName], stat.Median)
		}
	}

	// Print with groupings
	fmt.Printf("\n=== Median Times by Age Category ===\n")

	// Define the order we want to print the groups
	groupOrder := []string{"Juniors", "Men", "Women", "Other"}

	for _, groupName := range groupOrder {
		if stats, ok := groups[groupName]; ok && len(stats) > 0 {
			overallMedian := calculateMedianTime(groupTimes[groupName])
			fmt.Printf("\n--- %s (Overall Median: %s) ---\n", groupName, overallMedian)
			for _, stat := range stats {
				fmt.Printf("%s: %s (from %d results)\n",
					stat.Category, stat.Median, stat.Count)
			}
		}
	}

	return nil
}

// secondsToTime converts seconds to a time string (MM:SS or HH:MM:SS)
func secondsToTime(seconds int) string {
	if seconds == 0 {
		return "Unknown"
	}

	hours := seconds / 3600
	minutes := (seconds % 3600) / 60
	secs := seconds % 60

	if hours > 0 {
		return fmt.Sprintf("%d:%02d:%02d", hours, minutes, secs)
	}
	return fmt.Sprintf("%d:%02d", minutes, secs)
}

// parseDateTime parses a date string that might be in different timezone formats
func parseDateTime(dateStr string) (time.Time, error) {
	// Try simple date format first
	t, err := time.Parse("2006-01-02", dateStr)
	if err == nil {
		return t, nil
	}

	// Try format with offset
	t, err = time.Parse("2006-01-02 15:04:05-07:00", dateStr)
	if err == nil {
		return t, nil
	}

	// Try format with UTC
	t, err = time.Parse("2006-01-02 15:04:05+00:00", dateStr)
	if err != nil {
		return time.Time{}, fmt.Errorf("error parsing date '%s': %v", dateStr, err)
	}
	return t, nil
}

// PrintComparisonReport prints a comparison between two parkrun locations
func PrintComparisonReport(db *sql.DB, location1, location2 string) error {
	// Get stats for both locations
	stats1, err := getLocationStats(db, location1)
	if err != nil {
		return err
	}
	stats2, err := getLocationStats(db, location2)
	if err != nil {
		return err
	}

	fmt.Printf("\n=== Comparison: %s | %s ===\n\n", location1, location2)

	// Compare basic stats in table format
	fmt.Printf("Total Events:       %6d | %6d\n",
		stats1["total_events"], stats2["total_events"])
	fmt.Printf("Total Runners:      %6d | %6d\n",
		stats1["total_runners"], stats2["total_runners"])
	fmt.Printf("Avg Participants:   %6.1f | %6.1f\n",
		stats1["avg_participants"], stats2["avg_participants"])
	fmt.Printf("Biggest Event:      %6d | %6d runners\n",
		stats1["biggest_event_count"], stats2["biggest_event_count"])

	// Compare median times
	times1, err := GetMedianTimesByAgeCategory(db, stats1["location_id"].(int))
	if err != nil {
		return err
	}
	times2, err := GetMedianTimesByAgeCategory(db, stats2["location_id"].(int))
	if err != nil {
		return err
	}

	// Create maps for easy lookup
	medians1 := make(map[string]TimeStats)
	for _, t := range times1 {
		medians1[t.Category] = t
	}
	medians2 := make(map[string]TimeStats)
	for _, t := range times2 {
		medians2[t.Category] = t
	}

	// Find all unique categories
	categories := make(map[string]bool)
	for _, t := range times1 {
		categories[t.Category] = true
	}
	for _, t := range times2 {
		categories[t.Category] = true
	}

	// Group categories
	juniors := make([]string, 0)
	males := make([]string, 0)
	females := make([]string, 0)
	others := make([]string, 0)

	for cat := range categories {
		prefix := cat[:2]
		switch {
		case prefix == "JM" || prefix == "JW":
			juniors = append(juniors, cat)
		case prefix == "SM" || prefix == "VM":
			males = append(males, cat)
		case prefix == "SW" || prefix == "VW":
			females = append(females, cat)
		default:
			others = append(others, cat)
		}
	}

	// Sort each group
	sort.Strings(juniors)
	sort.Strings(males)
	sort.Strings(females)
	sort.Strings(others)

	// Print median time comparisons
	fmt.Printf("\n=== Median Times by Category ===\n")

	if len(juniors) > 0 {
		fmt.Printf("\nJuniors:\n")
		printCategoryComparisons(juniors, medians1, medians2)
	}

	if len(males) > 0 {
		fmt.Printf("\nMales:\n")
		printCategoryComparisons(males, medians1, medians2)
	}

	if len(females) > 0 {
		fmt.Printf("\nFemales:\n")
		printCategoryComparisons(females, medians1, medians2)
	}

	if len(others) > 0 {
		fmt.Printf("\nOthers:\n")
		printCategoryComparisons(others, medians1, medians2)
	}

	return nil
}

func printCategoryComparisons(categories []string, medians1, medians2 map[string]TimeStats) {
	for _, cat := range categories {
		t1, ok1 := medians1[cat]
		t2, ok2 := medians2[cat]

		if ok1 && ok2 {
			fmt.Printf("%-8s:   %8s | %8s\n",
				cat, t1.Median, t2.Median)
		} else if ok1 {
			fmt.Printf("%-8s:   %8s | %8s\n",
				cat, t1.Median, "N/A")
		} else {
			fmt.Printf("%-8s:   %8s | %8s\n",
				cat, "N/A", t2.Median)
		}
	}
}

// Helper function to get location stats with ID included
func getLocationStats(db *sql.DB, locationSlug string) (map[string]interface{}, error) {
	// Get location ID
	var locationID int
	err := db.QueryRow(`SELECT id FROM locations WHERE slug = ?`, locationSlug).Scan(&locationID)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("location '%s' not found", locationSlug)
	}
	if err != nil {
		return nil, fmt.Errorf("database error: %v", err)
	}

	stats, err := GetLocationStats(db, locationID)
	if err != nil {
		return nil, err
	}

	// Add location ID to stats
	stats["location_id"] = locationID
	return stats, nil
}
