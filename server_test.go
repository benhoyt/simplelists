package main

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/net/html"
	_ "modernc.org/sqlite"
)

func Test(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "db.sqlite")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("opening database: %v", err)
	}
	model, err := NewSQLModel(db)
	if err != nil {
		t.Fatalf("creating model: %v", err)
	}
	server, err := NewServer(model, nullLogger{}, "Pacific/Auckland", "", "", true)
	if err != nil {
		t.Fatalf("creating server: %v", err)
	}
	//jar, err := cookiejar.New(nil)
	//if err != nil {
	//	t.Fatalf("creating cookie jar: %v", err)
	//}

	// TODO: should this just be at the top level? subsequent tests probably shouldn't run if this fails
	t.Run("home", func(t *testing.T) {
		rec := httptest.NewRecorder()
		server.ServeHTTP(rec, newRequest(t, "GET", "/"))
		if rec.Code != 200 {
			t.Fatalf("got %d, want 200", rec.Code)
		}
		forms := parseForms(t, rec.Body.String())
		createForm, ok := forms["/create-list"]
		if !ok {
			t.Fatal("/create-list form not found")
		}
		_, ok = createForm.Inputs["csrf-token"]
		if !ok {
			t.Fatal("csrf-token input not found")
		}
		//jar.SetCookies(url, rec.Result().Cookies())
	})
}

func newRequest(t *testing.T, method, path string) *http.Request {
	t.Helper()
	req, err := http.NewRequest(method, path, nil)
	if err != nil {
		t.Fatalf("creating request: %v", err)
	}
	return req
}

type Form struct {
	// Inputs is a map of the form's inputs, keyed by name.
	Inputs map[string]string
}

// parseForms parses the forms in the HTML document and returns a map of the
// forms, keyed by action URL.
func parseForms(t *testing.T, htmlStr string) map[string]Form {
	t.Helper()
	doc, err := html.Parse(strings.NewReader(htmlStr))
	if err != nil {
		t.Fatalf("parsing HTML: %v", err)
	}
	forms := make(map[string]Form)

	var traverse func(*html.Node)
	traverse = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "form" {
			action := getAttr(n, "action")
			// TODO: are these tests useful?
			//method := getAttr(n, "method")
			//if method != "POST" {
			//	t.Fatalf("form %s method: got %s, want POST", action, method)
			//}
			//enctype := getAttr(n, "enctype")
			//if enctype != "application/x-www-form-urlencoded" {
			//	t.Fatalf("form %s enctype: got %s, want application/x-www-form-urlencoded", action, method)
			//}
			inputs := make(map[string]string)
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				if c.Type == html.ElementNode && c.Data == "input" {
					inputs[getAttr(c, "name")] = getAttr(c, "value")
				}
			}
			//fmt.Printf("TODO form: %s -> %v\n", action, inputs)
			forms[action] = Form{inputs}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			traverse(c)
		}
	}

	traverse(doc)
	return forms
}

func getAttr(n *html.Node, name string) string {
	for _, a := range n.Attr {
		if a.Key == name {
			return a.Val
		}
	}
	return "" // TODO: or Fatalf?
}

type nullLogger struct{}

func (nullLogger) Printf(format string, v ...interface{}) {}
