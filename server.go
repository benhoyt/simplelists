package main

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// Server is the HTTP server for the to-do list app.
type Server struct {
	model        Model
	location     *time.Location
	username     string
	passwordHash string
	showLists    bool

	mux      *http.ServeMux
	homeTmpl *template.Template
	listTmpl *template.Template
}

// Model is the database model interface used by the server.
type Model interface {
	GetLists() ([]*List, error)
	CreateList(name string) (string, error)
	DeleteList(id string) error
	GetList(id string) (*List, error)
	AddItem(listID, description string) (string, error)
	UpdateDone(listID, itemID string, done bool) error
	DeleteItem(listID, itemID string) error
}

// NewServer creates a new server with the specified dependencies.
func NewServer(model *SQLModel, timezone, username, passwordHash string, showLists bool) (*Server, error) {
	location := time.Local // use server's local time if timezone not specified
	if timezone != "" {
		var err error
		location, err = time.LoadLocation(timezone)
		if err != nil {
			return nil, err
		}
	}
	s := &Server{
		mux:          http.NewServeMux(),
		location:     location,
		username:     username,
		passwordHash: passwordHash,
		showLists:    showLists,
		model:        model,
	}
	s.addRoutes()
	s.addTemplates()
	return s, nil
}

func (s *Server) addRoutes() {
	s.mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" { // because "/" pattern matches /*
			s.home(w, r)
		} else {
			http.NotFound(w, r)
		}
	})
	s.mux.HandleFunc("/sign-in", csrfPost(s.signIn))
	s.mux.HandleFunc("/sign-out", s.ensureSignedIn(csrfPost(s.signOut)))
	s.mux.HandleFunc("/lists/", s.ensureSignedIn(s.showList))
	s.mux.HandleFunc("/create-list", s.ensureSignedIn(csrfPost(s.createList)))
	s.mux.HandleFunc("/delete-list", s.ensureSignedIn(csrfPost(s.deleteList)))
	s.mux.HandleFunc("/add-item", s.ensureSignedIn(csrfPost(s.addItem)))
	s.mux.HandleFunc("/check-item", s.ensureSignedIn(csrfPost(s.checkItem)))
	s.mux.HandleFunc("/delete-item", s.ensureSignedIn(csrfPost(s.deleteItem)))
}

func (s *Server) ensureSignedIn(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.isSignedIn(r) {
			http.Redirect(w, r, "/?return-url="+url.QueryEscape(r.URL.Path), http.StatusFound)
			return
		}
		h(w, r)
	}
}

func (s *Server) isSignedIn(r *http.Request) bool {
	if s.username == "" {
		return true
	}
	cookie, err := r.Cookie("sign-in")
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(s.username+":"+s.passwordHash)) == 1
}

func (s *Server) addTemplates() {
	s.homeTmpl = template.Must(template.New("home").Parse(homeTmpl))
	s.listTmpl = template.Must(template.New("list").Parse(listTmpl))
}

// ServeHTTP implements the http.Handler interface.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	startTime := time.Now()
	w.Header().Set("Cache-Control", "no-cache")
	s.mux.ServeHTTP(w, r)
	log.Printf("%s %s %v", r.Method, r.URL.Path, time.Since(startTime))
}

func (s *Server) home(w http.ResponseWriter, r *http.Request) {
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

	isSignedIn := s.isSignedIn(r)
	var data = struct {
		Token       string
		Lists       []*List
		ShowSignIn  bool
		ShowSignOut bool
		ReturnURL   string
		SignInError bool
	}{
		Token:       setCSRFCookie(w),
		Lists:       lists,
		ShowSignIn:  !isSignedIn,
		ShowSignOut: s.username != "" && isSignedIn,
		ReturnURL:   r.URL.Query().Get("return-url"),
		SignInError: r.URL.Query().Get("error") == "sign-in",
	}
	err := s.homeTmpl.Execute(w, data)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func (s *Server) signIn(w http.ResponseWriter, r *http.Request) {
	username := strings.TrimSpace(r.FormValue("username"))
	password := r.FormValue("password")
	returnURL := r.FormValue("return-url")
	if returnURL == "" {
		returnURL = "/"
	}
	if username != s.username || bcrypt.CompareHashAndPassword([]byte(s.passwordHash), []byte(password)) != nil {
		http.Redirect(w, r, "/?error=sign-in&return-url="+url.QueryEscape(returnURL), http.StatusFound)
		return
	}
	cookie := &http.Cookie{
		Name:     "sign-in",
		Value:    s.username + ":" + s.passwordHash,
		MaxAge:   365 * 24 * 60 * 60,
		Path:     "/",
		Secure:   true,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	}
	http.SetCookie(w, cookie)
	http.Redirect(w, r, returnURL, http.StatusFound)
}

func (s *Server) signOut(w http.ResponseWriter, r *http.Request) {
	cookie := &http.Cookie{
		Name:     "sign-in",
		MaxAge:   -1,
		Path:     "/",
		Secure:   true,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	}
	http.SetCookie(w, cookie)
	http.Redirect(w, r, "/", http.StatusFound)
}

func (s *Server) showList(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Path[len("/lists/"):]
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
		Token      string
		List       *List
		ShowDelete bool
	}{
		Token:      setCSRFCookie(w),
		List:       list,
		ShowDelete: r.URL.Query().Get("delete") != "",
	}
	err = s.listTmpl.Execute(w, data)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func (s *Server) createList(w http.ResponseWriter, r *http.Request) {
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
	http.Redirect(w, r, "/lists/"+listID, http.StatusFound)
}

func (s *Server) deleteList(w http.ResponseWriter, r *http.Request) {
	id := r.FormValue("id")
	err := s.model.DeleteList(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/", http.StatusFound)
}

func (s *Server) addItem(w http.ResponseWriter, r *http.Request) {
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
		http.Redirect(w, r, "/lists/"+list.ID, http.StatusFound)
		return
	}
	_, err = s.model.AddItem(list.ID, description)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/lists/"+list.ID, http.StatusFound)
}

func (s *Server) checkItem(w http.ResponseWriter, r *http.Request) {
	listID := r.FormValue("list-id")
	itemID := r.FormValue("item-id")
	done := r.FormValue("done") == "on"
	err := s.model.UpdateDone(listID, itemID, done)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/lists/"+listID, http.StatusFound)
}

func (s *Server) deleteItem(w http.ResponseWriter, r *http.Request) {
	listID := r.FormValue("list-id")
	itemID := r.FormValue("item-id")
	err := s.model.DeleteItem(listID, itemID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/lists/"+listID, http.StatusFound)
}

// GeneratePasswordHash generates a bcrypt hash from the given password.
func GeneratePasswordHash(password string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(b), err
}

// CheckPasswordHash returns a non-nil error if the given password hash is not
// a valid bcrypt hash.
func CheckPasswordHash(passwordHash string) error {
	err := bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte("x"))
	if errors.Is(err, bcrypt.ErrMismatchedHashAndPassword) {
		return nil
	}
	return err
}

// csrfPost wraps the given handler, ensuring that the HTTP method is POST and
// that the CSRF token in the "csrf-token" cookie matches the token in the
// "csrf-token" form field.
func csrfPost(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			w.Header().Set("Allow", "POST")
			http.Error(w, "405 method not allowed", http.StatusMethodNotAllowed)
			return
		}
		token := r.FormValue("csrf-token")
		cookie, err := r.Cookie("csrf-token")
		if err != nil || token != cookie.Value {
			http.Error(w, "invalid CSRF token or cookie", http.StatusBadRequest)
			return
		}
		h(w, r)
	}
}

// setCSRFCookie generates a new CSRF token and sets the "csrf-token" cookie,
// returning the new token.
func setCSRFCookie(w http.ResponseWriter) string {
	token := generateCSRFToken()
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

func generateCSRFToken() string {
	b := make([]byte, 16)
	_, err := rand.Read(b)
	if err != nil { // should never fail
		return ""
	}
	return hex.EncodeToString(b)
}
