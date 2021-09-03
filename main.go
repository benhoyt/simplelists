package main

import (
	"database/sql"
	"flag"
	"html/template"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// TODO: add XSRF protection?
// TODO: simplify? use sqlx?

func main() {
	dbPath := flag.String("db", "simplelists.sqlite", "`path` to SQLite 3 database")
	timezone := flag.String("timezone", "", "IANA timezone `name` (default local)")
	port := flag.Int("port", 8080, "HTTP `port` to listen on")
	showLists := flag.Bool("lists", false, "show lists on homepage")
	flag.Parse()

	db, err := sql.Open("sqlite", *dbPath)
	if err != nil {
		log.Fatal(err)
	}
	model, err := newSQLModel(db)
	if err != nil {
		log.Fatal(err)
	}

	s, err := newServer(model, *timezone, *showLists)
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("listening on port %d", *port)
	err = http.ListenAndServe(":"+strconv.Itoa(*port), s)
	log.Fatal(err)
}

// allow wraps the given handler in a handler that only responds if the
// request method is the given method, otherwise it responds with HTTP 405
// Method Not Allowed.
func allow(method string, h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if method != r.Method {
			w.Header().Set("Allow", method)
			http.Error(w, "405 method not allowed", http.StatusMethodNotAllowed)
			return
		}
		h(w, r)
	}
}

type server struct {
	model     *sqlModel
	location  *time.Location
	showLists bool

	mux      *http.ServeMux
	homeTmpl *template.Template
	listTmpl *template.Template
}

func newServer(model *sqlModel, timezone string, showLists bool) (*server, error) {
	location := time.Local
	if timezone != "" {
		var err error
		location, err = time.LoadLocation(timezone)
		if err != nil {
			return nil, err
		}
	}
	s := &server{
		mux:       http.NewServeMux(),
		location:  location,
		showLists: showLists,
		model:     model,
	}
	s.addRoutes()
	s.addTemplates()
	return s, nil
}

func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	startTime := time.Now()
	w.Header().Set("Cache-Control", "no-cache")
	s.mux.ServeHTTP(w, r)
	log.Printf("%s %s %v", r.Method, r.URL.Path, time.Since(startTime))
}

func (s *server) addRoutes() {
	s.mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			s.home(w, r)
		} else {
			// GET /{list-id}
			id := r.URL.Path[1:]
			s.showList(w, r, id)
		}
	})
	s.mux.HandleFunc("/create-list", allow("POST", s.createList))
	s.mux.HandleFunc("/add-item", allow("POST", s.addItem))
	s.mux.HandleFunc("/check-item", allow("POST", s.checkItem))
	s.mux.HandleFunc("/delete-item", allow("POST", s.deleteItem))
}

func (s *server) addTemplates() {
	s.homeTmpl = template.Must(template.New("home").Parse(homeTmpl))
	s.listTmpl = template.Must(template.New("list").Parse(listTmpl))
}

func (s *server) home(w http.ResponseWriter, r *http.Request) {
	var data struct {
		Lists []*List
	}
	if s.showLists {
		lists, err := s.model.GetLists()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		for _, list := range lists {
			list.TimeCreated = list.TimeCreated.In(s.location)
		}
		data.Lists = lists
	}
	err := s.homeTmpl.Execute(w, data)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func (s *server) showList(w http.ResponseWriter, r *http.Request, id string) {
	lst, err := s.model.GetList(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if lst == nil {
		http.NotFound(w, r)
		return
	}
	err = s.listTmpl.Execute(w, lst)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func (s *server) createList(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}
	lst, err := s.model.CreateList(name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/"+lst.ID, http.StatusFound)
}

func (s *server) addItem(w http.ResponseWriter, r *http.Request) {
	listID := r.FormValue("list-id")
	lst, err := s.model.GetList(listID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	description := strings.TrimSpace(r.FormValue("description"))
	if description == "" {
		http.Redirect(w, r, "/"+lst.ID, http.StatusFound)
		return
	}
	_, err = s.model.AddItem(lst.ID, description)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/"+lst.ID, http.StatusFound)
}

func (s *server) checkItem(w http.ResponseWriter, r *http.Request) {
	listID := r.FormValue("list-id")
	itemID := r.FormValue("item-id")
	done := r.FormValue("done") == "on"
	err := s.model.CheckItem(listID, itemID, done)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/"+listID, http.StatusFound)
}

func (s *server) deleteItem(w http.ResponseWriter, r *http.Request) {
	listID := r.FormValue("list-id")
	itemID := r.FormValue("item-id")
	err := s.model.DeleteItem(listID, itemID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/"+listID, http.StatusFound)
}
