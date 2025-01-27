# Parkrun Parser

> [!WARNING]
> Only use this for personal use.  Please see https://www.parkrun.com.au/terms-conditions/ for more details regarding acceptable use of parkrun data. 

A Go program that parses parkrun results and stores it into a SQLite database..

## Features

- Scrapes results from any Parkrun location using its URL slug
- Stores data in a SQLite database

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

Run the parser with a parkrun location URL slug:
   ```bash
   go run . <location-slug>
   ```
   Example:
   ```bash
   go run . bushy
   ```

The program will:
- Create a SQLite database named `parkrun.db` if it doesn't exist
- Fetch results starting from the earliest available event
- Store event information, and results in the database

## Database Schema

The database contains the following tables:
- `locations`: Stores parkrun location details
- `events`: Individual parkrun events
- `results`: Individual run results
