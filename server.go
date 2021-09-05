package main

import (
	"crypto/subtle"
	"errors"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

type server struct {
	model        *sqlModel
	location     *time.Location
	username     string
	passwordHash string
	showLists    bool

	mux      *http.ServeMux
	homeTmpl *template.Template
	listTmpl *template.Template
}

func newServer(model *sqlModel, timezone, username, passwordHash string, showLists bool) (*server, error) {
	location := time.Local
	if timezone != "" {
		var err error
		location, err = time.LoadLocation(timezone)
		if err != nil {
			return nil, err
		}
	}
	s := &server{
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

func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	startTime := time.Now()
	w.Header().Set("Cache-Control", "no-cache")
	s.mux.ServeHTTP(w, r)
	log.Printf("%s %s %v", r.Method, r.URL.Path, time.Since(startTime))
}

func generatePasswordHash(password string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(b), err
}

// checkPasswordHash returns a non-nil error if the given password hash is not
// a valid bcrypt hash.
func checkPasswordHash(passwordHash string) error {
	err := bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte("x"))
	if errors.Is(err, bcrypt.ErrMismatchedHashAndPassword) {
		return nil
	}
	return err
}

func (s *server) isSignedIn(r *http.Request) bool {
	if s.username == "" {
		return true
	}
	cookie, err := r.Cookie("sign-in")
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(s.username+":"+s.passwordHash)) == 1
}

func (s *server) ensureSignedIn(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.isSignedIn(r) {
			http.Redirect(w, r, "/?return-url="+url.QueryEscape(r.URL.Path), http.StatusFound)
			return
		}
		h(w, r)
	}
}

func (s *server) addRoutes() {
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
	s.mux.HandleFunc("/add-item", s.ensureSignedIn(csrfPost(s.addItem)))
	s.mux.HandleFunc("/check-item", s.ensureSignedIn(csrfPost(s.checkItem)))
	s.mux.HandleFunc("/delete-item", s.ensureSignedIn(csrfPost(s.deleteItem)))
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

func (s *server) signIn(w http.ResponseWriter, r *http.Request) {
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

func (s *server) signOut(w http.ResponseWriter, r *http.Request) {
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

func (s *server) showList(w http.ResponseWriter, r *http.Request) {
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
	http.Redirect(w, r, "/lists/"+listID, http.StatusFound)
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

func (s *server) checkItem(w http.ResponseWriter, r *http.Request) {
	listID := r.FormValue("list-id")
	itemID := r.FormValue("item-id")
	done := r.FormValue("done") == "on"
	err := s.model.CheckItem(listID, itemID, done)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/lists/"+listID, http.StatusFound)
}

func (s *server) deleteItem(w http.ResponseWriter, r *http.Request) {
	listID := r.FormValue("list-id")
	itemID := r.FormValue("item-id")
	err := s.model.DeleteItem(listID, itemID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/lists/"+listID, http.StatusFound)
}
