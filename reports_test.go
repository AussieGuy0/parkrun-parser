package main

import (

	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
)



func TestGetTopParticipants(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()
	insertTestData(t, db)

	stats, err := GetTopParticipants(db, 1, 10)
	if err != nil {
		t.Fatalf("GetTopParticipants failed: %v", err)
	}

	if len(stats) != 3 {
		t.Errorf("Expected 3 participants, got %d", len(stats))
	}

	// Runner A should be first with 2 runs
	if stats[0].Name != "Runner A" || stats[0].TotalRuns != 2 {
		t.Errorf("Expected Runner A with 2 runs, got %s with %d runs",
			stats[0].Name, stats[0].TotalRuns)
	}
}

func TestGetMedianTimesByAgeCategory(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()
	insertTestData(t, db)

	stats, err := GetMedianTimesByAgeCategory(db, 1)
	if err != nil {
		t.Fatalf("GetMedianTimesByAgeCategory failed: %v", err)
	}

	if len(stats) != 2 {
		t.Errorf("Expected 2 age categories, got %d", len(stats))
	}

	// Check VM35-39 category
	found := false
	for _, stat := range stats {
		if stat.Category == "VM35-39" {
			found = true
			if stat.Count != 3 {
				t.Errorf("Expected 3 results for VM35-39, got %d", stat.Count)
			}
			if stat.Median != "19:50" { // 1190 seconds - middle value of (1200, 1190, 1180)
				t.Errorf("Expected median time 19:50 for VM35-39, got %s", stat.Median)
			}
		}
	}
	if !found {
		t.Error("VM35-39 category not found in results")
	}
}

func TestGetLocationStats(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()
	insertTestData(t, db)

	stats, err := GetLocationStats(db, 1)
	if err != nil {
		t.Fatalf("GetLocationStats failed: %v", err)
	}

	// Check total events
	if events, ok := stats["total_events"].(int); !ok || events != 2 {
		t.Errorf("Expected 2 total events, got %v", stats["total_events"])
	}

	// Check total runners
	if runners, ok := stats["total_runners"].(int); !ok || runners != 3 {
		t.Errorf("Expected 3 total runners, got %v", stats["total_runners"])
	}

	// Check first event date
	firstEvent, ok := stats["first_event"].(time.Time)
	if !ok {
		t.Error("First event date not found or wrong type")
	} else {
		expected := time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)
		if !firstEvent.Equal(expected) {
			t.Errorf("Expected first event date %v, got %v", expected, firstEvent)
		}
	}
}

func TestCalculateMedianTime(t *testing.T) {
	tests := []struct {
		name  string
		times []string
		want  string
	}{
		{
			name:  "Empty list",
			times: []string{},
			want:  "N/A",
		},
		{
			name:  "Single time",
			times: []string{"20:00"},
			want:  "20:00",
		},
		{
			name:  "Multiple times",
			times: []string{"22:00", "20:00", "21:00"},
			want:  "21:00",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := calculateMedianTime(tt.times)
			if got != tt.want {
				t.Errorf("calculateMedianTime() = %v, want %v", got, tt.want)
			}
		})
	}
}
