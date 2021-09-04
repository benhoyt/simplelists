package main

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
)

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
