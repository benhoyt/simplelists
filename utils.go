package main

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
)

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
		token := r.FormValue("csrf-token")
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

func generateCSRFToken() string {
	b := make([]byte, 16)
	_, err := rand.Read(b)
	if err != nil { // should never fail
		panic(err)
	}
	return hex.EncodeToString(b)
}

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
