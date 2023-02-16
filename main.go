// Tiny to-do list web app

package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"

	"golang.org/x/term"
	_ "modernc.org/sqlite"
)

func main() {
	// Config defaults
	port := 8080
	dbPath := "simplelists.sqlite"
	showLists := false
	timezone := ""
	username := ""

	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), `Usage: simplelists [options]

Options:
  -genpass              create password hash (instead of running server)

Environment variables:
  PORT                  HTTP port to listen on (default %d)
  SIMPLELISTS_DB        path to SQLite 3 database (default %q)
  SIMPLELISTS_LISTS     show lists on homepage (if set to 1 or "true")
  SIMPLELISTS_PASSHASH  password hash (required if username is set)
  SIMPLELISTS_TIMEZONE  IANA timezone name (defaults to local timezone)
  SIMPLELISTS_USERNAME  optional username to access site
`, port, dbPath)
	}
	genPass := flag.Bool("genpass", false, "-")
	flag.Parse()

	if *genPass {
		var password string
		for len(password) < 6 {
			fmt.Printf("Enter password (at least 6 chars): ")
			b, err := term.ReadPassword(int(os.Stdin.Fd()))
			fmt.Println()
			exitOnError(err)
			password = string(b)
		}
		hash, err := GeneratePasswordHash(password)
		exitOnError(err)
		fmt.Println(hash)
		return
	}

	// Parse config from environment variables
	var err error
	if portEnv, ok := os.LookupEnv("PORT"); ok {
		port, err = strconv.Atoi(portEnv)
		if err != nil {
			exitOnError(err)
		}
	}
	if dbEnv, ok := os.LookupEnv("SIMPLELISTS_DB"); ok {
		dbPath = dbEnv
	}
	if listsEnv, ok := os.LookupEnv("SIMPLELISTS_LISTS"); ok {
		showLists = listsEnv == "1" || listsEnv == "true"
	}
	if timezoneEnv, ok := os.LookupEnv("SIMPLELISTS_TIMEZONE"); ok {
		timezone = timezoneEnv
	}
	if usernameEnv, ok := os.LookupEnv("SIMPLELISTS_USERNAME"); ok {
		username = usernameEnv
	}

	var passwordHash string
	if username != "" {
		passwordHash = os.Getenv("SIMPLELISTS_PASSHASH")
		if passwordHash == "" {
			log.Fatal("SIMPLELISTS_PASSHASH must be set if username is set")
		}
		err := CheckPasswordHash(passwordHash)
		exitOnError(err)
	}

	db, err := sql.Open("sqlite", dbPath)
	exitOnError(err)
	model, err := NewSQLModel(db)
	exitOnError(err)
	server, err := NewServer(model, log.Default(), timezone, username, passwordHash, showLists)
	exitOnError(err)

	log.Printf("config: port=%d db=%q lists=%v timezone=%q username=%q",
		port, dbPath, showLists, timezone, username)
	log.Printf("listening on http://localhost:%d", port)
	err = http.ListenAndServe(":"+strconv.Itoa(port), server)
	exitOnError(err)
}

func exitOnError(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
