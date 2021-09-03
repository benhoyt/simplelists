package main

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"flag"
	"html/template"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

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

func checkCSRF(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		isValid := false
		token := r.FormValue("token")
		cookie, err := r.Cookie("csrf-token")
		if err == nil {
			isValid = token == cookie.Value
		}
		if !isValid {
			http.Error(w, "invalid CSRF token or cookie", http.StatusBadRequest)
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
	s.mux.HandleFunc("/create-list", allow("POST", checkCSRF(s.createList)))
	s.mux.HandleFunc("/add-item", allow("POST", checkCSRF(s.addItem)))
	s.mux.HandleFunc("/check-item", allow("POST", checkCSRF(s.checkItem)))
	s.mux.HandleFunc("/delete-item", allow("POST", checkCSRF(s.deleteItem)))
}

func (s *server) addTemplates() {
	s.homeTmpl = template.Must(template.New("home").Parse(homeTmpl))
	s.listTmpl = template.Must(template.New("list").Parse(listTmpl))
}

func generateToken() string {
	b := make([]byte, 16)
	_, err := rand.Read(b)
	if err != nil { // should never fail
		panic(err)
	}
	return hex.EncodeToString(b)
}

func setCSRFCookie(w http.ResponseWriter) string {
	token := generateToken()
	cookie := &http.Cookie{
		Name:     "csrf-token",
		Value:    token,
		Path:     "/",
		Secure:   true,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	}
	http.SetCookie(w, cookie)
	return token
}

func (s *server) home(w http.ResponseWriter, r *http.Request) {
	var lists []*List
	if s.showLists {
		var err error
		lists, err = s.model.GetLists()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		for _, list := range lists {
			list.TimeCreated = list.TimeCreated.In(s.location)
		}
	}

	var data = struct {
		Lists []*List
		Token string
	}{
		Token: setCSRFCookie(w),
		Lists: lists,
	}
	err := s.homeTmpl.Execute(w, data)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func (s *server) showList(w http.ResponseWriter, r *http.Request, id string) {
	list, err := s.model.GetList(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if list == nil {
		http.NotFound(w, r)
		return
	}

	var data = struct {
		List  *List
		Token string
	}{
		List:  list,
		Token: setCSRFCookie(w),
	}
	err = s.listTmpl.Execute(w, data)
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
	list, err := s.model.CreateList(name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/"+list.ID, http.StatusFound)
}

func (s *server) addItem(w http.ResponseWriter, r *http.Request) {
	listID := r.FormValue("list-id")
	list, err := s.model.GetList(listID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if list == nil {
		http.NotFound(w, r)
		return
	}
	description := strings.TrimSpace(r.FormValue("description"))
	if description == "" {
		http.Redirect(w, r, "/"+list.ID, http.StatusFound)
		return
	}
	_, err = s.model.AddItem(list.ID, description)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/"+list.ID, http.StatusFound)
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
