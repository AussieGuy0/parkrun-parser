package main

import (
	"testing"
	"time"
)

func TestTimeToSeconds(t *testing.T) {
	tests := []struct {
		name    string
		timeStr string
		want    int
		wantErr bool
	}{
		{
			name:    "Minutes and seconds",
			timeStr: "23:45",
			want:    1425,
			wantErr: false,
		},
		{
			name:    "Hours, minutes and seconds",
			timeStr: "1:23:45",
			want:    5025,
			wantErr: false,
		},
		{
			name:    "Empty string",
			timeStr: "",
			want:    0,
			wantErr: false,
		},
		{
			name:    "Unknown time",
			timeStr: "Unknown",
			want:    0,
			wantErr: false,
		},
		{
			name:    "Invalid format",
			timeStr: "23:45:67:89",
			want:    0,
			wantErr: true,
		},
		{
			name:    "Invalid numbers",
			timeStr: "ab:cd",
			want:    0,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := timeToSeconds(tt.timeStr)
			if (err != nil) != tt.wantErr {
				t.Errorf("timeToSeconds() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("timeToSeconds() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseEventDate(t *testing.T) {
	// Helper function to create time.Time values for comparison
	date := func(year, month, day int) time.Time {
		return time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
	}

	tests := []struct {
		name     string
		dateText string
		want     time.Time
		wantErr  bool
	}{
		{
			name:     "DD/MM/YYYY format",
			dateText: "25/12/2023",
			want:     date(2023, 12, 25),
			wantErr:  false,
		},
		{
			name:     "D/M/YY format",
			dateText: "5/6/23",
			want:     date(2023, 6, 5),
			wantErr:  false,
		},
		{
			name:     "D/M/YYYY format",
			dateText: "5/6/2023",
			want:     date(2023, 6, 5),
			wantErr:  false,
		},
		{
			name:     "Invalid format",
			dateText: "2023-12-25",
			want:     time.Time{},
			wantErr:  true,
		},
		{
			name:     "Empty string",
			dateText: "",
			want:     time.Time{},
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseEventDate(tt.dateText)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseEventDate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && !got.Equal(tt.want) {
				t.Errorf("parseEventDate() = %v, want %v", got, tt.want)
			}
		})
	}
}
