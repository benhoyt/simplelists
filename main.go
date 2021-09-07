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

	_ "modernc.org/sqlite"
)

func main() {
	dbPath := flag.String("db", "simplelists.sqlite", "`path` to SQLite 3 database")
	timezone := flag.String("timezone", "", "IANA timezone `name` (default local)")
	port := flag.Int("port", 8080, "HTTP `port` to listen on")
	showLists := flag.Bool("lists", false, "show lists on homepage")
	username := flag.String("username", "", "optional username to access site")
	genPass := flag.Bool("genpass", false, "interactively generate password hash (don't run server)")
	flag.Parse()

	if *genPass {
		var password string
		for len(password) < 6 {
			fmt.Printf("Enter password at least 6 characters long: ")
			fmt.Scanln(&password)
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
	s, err := NewServer(model, log.Default(), *timezone, *username, passwordHash, *showLists)
	exitOnError(err)

	log.Printf("listening on http://localhost:%d", *port)
	err = http.ListenAndServe(":"+strconv.Itoa(*port), s)
	exitOnError(err)
}

func exitOnError(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
