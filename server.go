package main

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"html/template"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// Server is the HTTP server for the to-do list app.
type Server struct {
	model        Model
	logger       Logger
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

	CreateSignIn() (string, error)
	IsSignInValid(id string) (bool, error)
	DeleteSignIn(id string) error
}

// Logger is the logger interface used by the server.
type Logger interface {
	Printf(format string, v ...interface{})
}

// NewServer creates a new server with the specified dependencies.
func NewServer(
	model Model,
	logger Logger,
	timezone string,
	username string,
	passwordHash string,
	showLists bool,
) (*Server, error) {
	location := time.Local // use server's local time if timezone not specified
	if timezone != "" {
		var err error
		location, err = time.LoadLocation(timezone)
		if err != nil {
			return nil, err
		}
	}
	s := &Server{
		model:        model,
		logger:       logger,
		location:     location,
		username:     username,
		passwordHash: passwordHash,
		showLists:    showLists,
		mux:          http.NewServeMux(),
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
	s.mux.HandleFunc("/sign-in", csrf(s.signIn))
	s.mux.HandleFunc("/sign-out", s.signedIn(csrf(s.signOut)))
	s.mux.HandleFunc("/lists/", s.signedIn(s.showList))
	s.mux.HandleFunc("/create-list", s.signedIn(csrf(s.createList)))
	s.mux.HandleFunc("/delete-list", s.signedIn(csrf(s.deleteList)))
	s.mux.HandleFunc("/add-item", s.signedIn(csrf(s.addItem)))
	s.mux.HandleFunc("/update-done", s.signedIn(csrf(s.updateDone)))
	s.mux.HandleFunc("/delete-item", s.signedIn(csrf(s.deleteItem)))
}

func (s *Server) signedIn(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.isSignedIn(r) {
			location := "/?return-url=" + url.QueryEscape(r.URL.Path)
			http.Redirect(w, r, location, http.StatusFound)
			return
		}
		h(w, r)
	}
}

func (s *Server) isSignedIn(r *http.Request) bool {
	if s.username == "" {
		return true
	}
	valid, err := s.model.IsSignInValid(getSignInCookie(r))
	return err == nil && valid
}

func getSignInCookie(r *http.Request) string {
	cookie, err := r.Cookie("sign-in")
	if err != nil {
		return ""
	}
	return cookie.Value
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
	s.logger.Printf("%s %s %v", r.Method, r.URL.Path, time.Since(startTime))
}

func (s *Server) home(w http.ResponseWriter, r *http.Request) {
	var lists []*List
	if s.showLists {
		var err error
		lists, err = s.model.GetLists()
		if err != nil {
			s.internalError(w, "fetching lists", err)
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
		Token:       getCSRFToken(w, r),
		Lists:       lists,
		ShowSignIn:  !isSignedIn,
		ShowSignOut: s.username != "" && isSignedIn,
		ReturnURL:   r.URL.Query().Get("return-url"),
		SignInError: r.URL.Query().Get("error") == "sign-in",
	}
	err := s.homeTmpl.Execute(w, data)
	if err != nil {
		s.internalError(w, "rendering template", err)
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
		location := "/?error=sign-in&return-url=" + url.QueryEscape(returnURL)
		http.Redirect(w, r, location, http.StatusFound)
		return
	}
	id, err := s.model.CreateSignIn()
	if err != nil {
		s.internalError(w, "creating sign in", err)
		return
	}
	cookie := &http.Cookie{
		Name:     "sign-in",
		Value:    id,
		MaxAge:   90 * 24 * 60 * 60,
		Path:     "/",
		Secure:   r.URL.Scheme == "https",
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
		Secure:   r.URL.Scheme == "https",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	}
	http.SetCookie(w, cookie)

	err := s.model.DeleteSignIn(getSignInCookie(r))
	if err != nil {
		s.internalError(w, "deleting sign in", err)
		return
	}

	http.Redirect(w, r, "/", http.StatusFound)
}

func (s *Server) showList(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Path[len("/lists/"):]
	list, err := s.model.GetList(id)
	if err != nil {
		s.internalError(w, "fetching list", err)
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
		Token:      getCSRFToken(w, r),
		List:       list,
		ShowDelete: r.URL.Query().Get("delete") != "",
	}
	err = s.listTmpl.Execute(w, data)
	if err != nil {
		s.internalError(w, "rendering template", err)
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
		s.internalError(w, "creating list", err)
		return
	}
	http.Redirect(w, r, "/lists/"+listID, http.StatusFound)
}

func (s *Server) deleteList(w http.ResponseWriter, r *http.Request) {
	id := r.FormValue("list-id")
	err := s.model.DeleteList(id)
	if err != nil {
		s.internalError(w, "deleting list", err)
		return
	}
	http.Redirect(w, r, "/", http.StatusFound)
}

func (s *Server) addItem(w http.ResponseWriter, r *http.Request) {
	listID := r.FormValue("list-id")
	list, err := s.model.GetList(listID)
	if err != nil {
		s.internalError(w, "fetching list", err)
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
		s.internalError(w, "adding item", err)
		return
	}
	http.Redirect(w, r, "/lists/"+list.ID, http.StatusFound)
}

func (s *Server) updateDone(w http.ResponseWriter, r *http.Request) {
	listID := r.FormValue("list-id")
	itemID := r.FormValue("item-id")
	done := r.FormValue("done") == "on"
	err := s.model.UpdateDone(listID, itemID, done)
	if err != nil {
		s.internalError(w, "updating done flag", err)
		return
	}
	http.Redirect(w, r, "/lists/"+listID, http.StatusFound)
}

func (s *Server) deleteItem(w http.ResponseWriter, r *http.Request) {
	listID := r.FormValue("list-id")
	itemID := r.FormValue("item-id")
	err := s.model.DeleteItem(listID, itemID)
	if err != nil {
		s.internalError(w, "deleting item", err)
		return
	}
	http.Redirect(w, r, "/lists/"+listID, http.StatusFound)
}

func (s *Server) internalError(w http.ResponseWriter, msg string, err error) {
	s.logger.Printf("error %s: %v", msg, err)
	http.Error(w, "error "+msg, http.StatusInternalServerError)
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

// csrf wraps the given handler, ensuring that the HTTP method is POST and
// that the CSRF token in the "csrf-token" cookie matches the token in the
// "csrf-token" form field.
func csrf(h http.HandlerFunc) http.HandlerFunc {
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

// getCSRFToken returns the current session's CSRF token, generating a new one
// and settings the "csrf-token" cookie if not present.
func getCSRFToken(w http.ResponseWriter, r *http.Request) string {
	cookie, err := r.Cookie("csrf-token")
	if err == nil && cookie.Value != "" {
		return cookie.Value
	}
	token := generateCSRFToken()
	cookie = &http.Cookie{
		Name:     "csrf-token",
		Value:    token,
		Path:     "/",
		Secure:   r.URL.Scheme == "https",
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
		panic(err)
	}
	return hex.EncodeToString(b)
}
