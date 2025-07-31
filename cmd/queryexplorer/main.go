package main

import (
	"database/sql"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	_ "github.com/lib/pq"
)

type QueryFile struct {
	Name     string
	Number   int
	FilePath string
}

func main() {
	if len(os.Args) < 2 {
		showUsage()
		return
	}

	// Database connection parameters - you can modify these or use environment variables
	dbHost := getEnv("DB_HOST", "localhost")
	dbPort := getEnv("DB_PORT", "15432")
	dbUser := getEnv("DB_USER", "postgres")
	dbPassword := getEnv("DB_PASSWORD", "postgres")
	dbName := getEnv("DB_NAME", "postgres")

	// Connect to database
	connStr := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		dbHost, dbPort, dbUser, dbPassword, dbName)

	db, err := sql.Open("postgres", connStr)
	if err != nil {
		log.Fatal("Failed to connect to database:", err)
	}
	defer db.Close()

	// Test connection
	if err := db.Ping(); err != nil {
		log.Fatal("Failed to ping database:", err)
	}

	// Get available query files
	queryFiles, err := getQueryFiles()
	if err != nil {
		log.Fatal("Failed to read query files:", err)
	}

	if len(queryFiles) == 0 {
		fmt.Println("No query files found in query/ directory")
		return
	}

	command := os.Args[1]

	switch command {
	case "list":
		listQueries(queryFiles)
	case "run":
		if len(os.Args) < 3 {
			fmt.Println("Usage: go run main.go run <query_number>")
			listQueries(queryFiles)
			return
		}
		queryNum, err := strconv.Atoi(os.Args[2])
		if err != nil {
			fmt.Printf("Invalid query number: %s\n", os.Args[2])
			return
		}
		runSingleQuery(db, queryFiles, queryNum)
	case "all":
		runAllQueries(db, queryFiles)
	default:
		showUsage()
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getQueryFiles() ([]QueryFile, error) {
	var queryFiles []QueryFile

	err := filepath.WalkDir("query", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			return nil
		}

		filename := d.Name()
		if strings.HasPrefix(filename, "query") && strings.HasSuffix(filename, ".sql") {
			// Extract number from filename (e.g., query0.sql, query10.sql, etc.)
			// Remove "query" prefix and ".sql" suffix
			nameWithoutExt := strings.TrimSuffix(filename, ".sql")
			numStr := strings.TrimPrefix(nameWithoutExt, "query")

			// Handle numbered queries (query0.sql, query1.sql, etc.)
			if numStr != "" {
				if num, err := strconv.Atoi(numStr); err == nil {
					queryFiles = append(queryFiles, QueryFile{
						Name:     filename,
						Number:   num,
						FilePath: path,
					})
				}
			} else if filename == "query.sql" {
				// Handle the generic query.sql file as a special case
				queryFiles = append(queryFiles, QueryFile{
					Name:     filename,
					Number:   -1, // Special number for query.sql
					FilePath: path,
				})
			}
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	// Sort by query number
	sort.Slice(queryFiles, func(i, j int) bool {
		return queryFiles[i].Number < queryFiles[j].Number
	})

	return queryFiles, nil
}

func listQueries(queryFiles []QueryFile) {
	fmt.Println("Available queries:")
	fmt.Println("==================")
	for _, qf := range queryFiles {
		if qf.Number == -1 {
			fmt.Printf("Query (generic): %s\n", qf.Name)
		} else {
			fmt.Printf("Query %d: %s\n", qf.Number, qf.Name)
		}
	}
	fmt.Println("\nUsage:")
	fmt.Println("  go run main.go list                 - Show this list")
	fmt.Println("  go run main.go run <query_number>   - Run specific query")
	fmt.Println("  go run main.go all                  - Run all queries")
}

func runSingleQuery(db *sql.DB, queryFiles []QueryFile, queryNum int) {
	var targetFile *QueryFile
	for _, qf := range queryFiles {
		if qf.Number == queryNum {
			targetFile = &qf
			break
		}
	}

	if targetFile == nil {
		fmt.Printf("Query %d not found\n", queryNum)
		listQueries(queryFiles)
		return
	}

	if targetFile.Number == -1 {
		fmt.Printf("Running Query (generic) (%s):\n", targetFile.Name)
		fmt.Println("=" + strings.Repeat("=", len(fmt.Sprintf("Running Query (generic) (%s):", targetFile.Name))))
	} else {
		fmt.Printf("Running Query %d (%s):\n", targetFile.Number, targetFile.Name)
		fmt.Println("=" + strings.Repeat("=", len(fmt.Sprintf("Running Query %d (%s):", targetFile.Number, targetFile.Name))))
	}

	executeQueryFile(db, *targetFile)
}

func runAllQueries(db *sql.DB, queryFiles []QueryFile) {
	fmt.Println("Running all queries:")
	fmt.Println("===================")

	for i, qf := range queryFiles {
		if i > 0 {
			fmt.Println("\n" + strings.Repeat("-", 50))
		}
		if qf.Number == -1 {
			fmt.Printf("\nQuery (generic) (%s):\n", qf.Name)
		} else {
			fmt.Printf("\nQuery %d (%s):\n", qf.Number, qf.Name)
		}
		executeQueryFile(db, qf)
	}
}

func executeQueryFile(db *sql.DB, queryFile QueryFile) {
	// Read query from file
	queryBytes, err := os.ReadFile(queryFile.FilePath)
	if err != nil {
		fmt.Printf("Error reading file %s: %v\n", queryFile.FilePath, err)
		return
	}

	content := string(queryBytes)
	lines := strings.Split(content, "\n")

	// Extract leading comments
	commentLines := []string{}
	queryLines := []string{}
	inCommentBlock := true

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if inCommentBlock && strings.HasPrefix(trimmed, "--") {
			commentLines = append(commentLines, strings.TrimPrefix(trimmed, "--"))
		} else {
			inCommentBlock = false
			queryLines = append(queryLines, line)
		}
	}

	// Print comments
	if len(commentLines) > 0 {
		fmt.Println("## Query Description:")
		for _, comment := range commentLines {
			fmt.Println(strings.TrimSpace(comment))
		}
		fmt.Println()
	}

	// Prepare the SQL query
	query := strings.TrimSpace(strings.Join(queryLines, "\n"))
	if query == "" {
		fmt.Println("Query file is empty")
		return
	}

	// Execute query
	rows, err := db.Query(query)
	if err != nil {
		fmt.Printf("Error executing query: %v\n", err)
		return
	}
	defer rows.Close()

	// Get column names
	columns, err := rows.Columns()
	if err != nil {
		fmt.Printf("Error getting columns: %v\n", err)
		return
	}

	// To store all rows data in [][]string (including header)
	data := [][]string{columns}

	// Scan all rows into data slice
	for rows.Next() {
		values := make([]interface{}, len(columns))
		scanArgs := make([]interface{}, len(columns))
		for i := range values {
			scanArgs[i] = &values[i]
		}

		if err := rows.Scan(scanArgs...); err != nil {
			fmt.Printf("Error scanning row: %v\n", err)
			return
		}

		strValues := make([]string, len(values))
		for i, val := range values {
			if val == nil {
				strValues[i] = "NULL"
			} else {
				strValues[i] = fmt.Sprintf("%v", val)
			}
		}
		data = append(data, strValues)
	}

	if err := rows.Err(); err != nil {
		fmt.Printf("Error iterating rows: %v\n", err)
		return
	}

	// Calculate max width for each column
	colWidths := make([]int, len(columns))
	for _, row := range data {
		for i, col := range row {
			if len(col) > colWidths[i] {
				colWidths[i] = len(col)
			}
		}
	}

	// Helper to print a row
	printRow := func(row []string) {
		for i, col := range row {
			// Left-align text, pad with spaces
			fmt.Printf("| %-*s ", colWidths[i], col)
		}
		fmt.Println("|")
	}

	// Print separator line
	printSeparator := func() {
		for _, w := range colWidths {
			fmt.Print("+")
			fmt.Print(strings.Repeat("-", w+2))
		}
		fmt.Println("+")
	}

	// Print table
	printSeparator()
	printRow(data[0]) // header
	printSeparator()
	for _, row := range data[1:] {
		printRow(row)
	}
	printSeparator()

	// Print row count
	fmt.Fprintf(os.Stderr, "\nRows returned: %d\n", len(data)-1)
}


func showUsage() {
	fmt.Println("PostgreSQL Query Runner")
	fmt.Println("======================")
	fmt.Println("Usage:")
	fmt.Println("  go run main.go list                 - List all available queries")
	fmt.Println("  go run main.go run <query_number>   - Run a specific query")
	fmt.Println("  go run main.go all                  - Run all queries")
	fmt.Println("\nEnvironment Variables (optional):")
	fmt.Println("  DB_HOST     - Database host (default: localhost)")
	fmt.Println("  DB_PORT     - Database port (default: 15432)")
	fmt.Println("  DB_USER     - Database user (default: postgres)")
	fmt.Println("  DB_PASSWORD - Database password (default: postgres)")
	fmt.Println("  DB_NAME     - Database name (default: postgres)")
	fmt.Println("\nExamples:")
	fmt.Println("  go run main.go list")
	fmt.Println("  go run main.go run 5")
	fmt.Println("  go run main.go all")
}
