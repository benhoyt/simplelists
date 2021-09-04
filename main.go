// Tiny to-do list web app

package main

import (
	"database/sql"
	"flag"
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
	flag.Parse()

	db, err := sql.Open("sqlite", *dbPath)
	exitOnError(err)
	model, err := newSQLModel(db)
	exitOnError(err)
	s, err := newServer(model, *timezone, *showLists)
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
