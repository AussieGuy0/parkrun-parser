# Parkrun Parser

> [!WARNING]
> Only use this for personal use.  Please see https://www.parkrun.com.au/terms-conditions/ for more details regarding acceptable use of parkrun data. 

A Go program that parses parkrun results and generates interesting reports.

## Features

- Retrieves results from any Parkrun location 
- Stores data in a SQLite database
- Generates statistical reports 

## Prerequisites

- Go 1.21 or later
- SQLite3

## Installation

1. Clone the repository:

2. Install dependencies:
   ```bash
   go mod download
   ```

## Usage

Note: Can also use `go run .` to run the program.

### Parse Results
To fetch and store results for a parkrun location:
```bash
parkrun parse <location-slug>
```

### Generate Reports
To view statistics for a single parkrun location:
```bash
parkrun report <location-slug>
```

### Compare Locations
To compare statistics between two parkrun locations:
```bash
parkrun compare <location-slug1> <location-slug2>
```


## Database Schema

The database contains the following tables:
- `locations`: Stores parkrun location details
- `events`: Individual parkrun events
- `results`: Individual run results
