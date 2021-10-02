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
	defaultPort := 8080
	dbPath := flag.String("db", "simplelists.sqlite", "`path` to SQLite 3 database")
	timezone := flag.String("timezone", "", "IANA timezone `name` (default local)")
	port := flag.Int("port", 0, fmt.Sprintf("HTTP `port` to listen on (default $PORT or %d)", defaultPort))
	showLists := flag.Bool("lists", false, "show lists on homepage")
	username := flag.String("username", "", "optional username to access site")
	genPass := flag.Bool("genpass", false, "interactively generate password hash (don't run server)")
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

	var passwordHash string
	if *username != "" {
		passwordHash = os.Getenv("SIMPLELISTS_PASSHASH")
		if passwordHash == "" {
			log.Fatal("SIMPLELISTS_PASSHASH must be set if username is set")
		}
		err := CheckPasswordHash(passwordHash)
		exitOnError(err)
	}

	db, err := sql.Open("sqlite", *dbPath)
	exitOnError(err)
	model, err := NewSQLModel(db)
	exitOnError(err)
	server, err := NewServer(model, log.Default(), *timezone, *username, passwordHash, *showLists)
	exitOnError(err)

	if *port == 0 {
		port = &defaultPort
		envPort, err := strconv.Atoi(os.Getenv("PORT"))
		if err == nil {
			port = &envPort
		}
	}

	log.Printf("listening on http://localhost:%d", *port)
	err = http.ListenAndServe(":"+strconv.Itoa(*port), server)
	exitOnError(err)
}

func exitOnError(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
