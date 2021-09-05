// Tiny to-do list web app

package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"net/http"
	"strconv"

	_ "modernc.org/sqlite"
)

func main() {
	dbPath := flag.String("db", "simplelists.sqlite", "`path` to SQLite 3 database")
	timezone := flag.String("timezone", "", "IANA timezone `name` (default local)")
	port := flag.Int("port", 8080, "HTTP `port` to listen on")
	showLists := flag.Bool("lists", false, "show lists on homepage")
	username := flag.String("username", "", "optional username to access site")
	passwordHash := flag.String("passhash", "", "hash of password to access site")
	genPass := flag.Bool("genpass", false, "interactively generate password hash (don't run server)")
	flag.Parse()

	if *genPass {
		var password string
		for len(password) < 6 {
			fmt.Printf("Enter password at least 6 characters long: ")
			fmt.Scanln(&password)
		}
		hash, err := generatePasswordHash(password)
		exitOnError(err)
		fmt.Println(hash)
		return
	}

	if *username != "" {
		if *passwordHash == "" {
			log.Fatal("passhash must be set if username is set")
		}
		err := checkPasswordHash(*passwordHash)
		exitOnError(err)
	}

	db, err := sql.Open("sqlite", *dbPath)
	exitOnError(err)
	model, err := newSQLModel(db)
	exitOnError(err)
	s, err := newServer(model, *timezone, *username, *passwordHash, *showLists)
	exitOnError(err)

	log.Printf("listening on port %d", *port)
	err = http.ListenAndServe(":"+strconv.Itoa(*port), s)
	exitOnError(err)
}

func exitOnError(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
