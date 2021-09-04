package main

import (
	"html/template"
	"log"
	"net/http"
	"strings"
	"time"
)

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
			s.showList(w, r, r.URL.Path[1:])
		}
	})
	s.mux.HandleFunc("/create-list", csrfPost(s.createList))
	s.mux.HandleFunc("/add-item", csrfPost(s.addItem))
	s.mux.HandleFunc("/check-item", csrfPost(s.checkItem))
	s.mux.HandleFunc("/delete-item", csrfPost(s.deleteItem))
}

func (s *server) addTemplates() {
	s.homeTmpl = template.Must(template.New("home").Parse(homeTmpl))
	s.listTmpl = template.Must(template.New("list").Parse(listTmpl))
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
			// Change UTC timezone to display timezone
			list.TimeCreated = list.TimeCreated.In(s.location)
		}
	}

	var data = struct {
		Token string
		Lists []*List
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
		Token string
		List  *List
	}{
		Token: setCSRFCookie(w),
		List:  list,
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
		// Empty list name, just reload home page
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}
	listID, err := s.model.CreateList(name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/"+listID, http.StatusFound)
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
		// Empty item description, just reload list
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
