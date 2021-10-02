package main

import (
	"database/sql"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/html"
	_ "modernc.org/sqlite"
)

func TestServer(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("opening database: %v", err)
	}
	defer db.Close()
	model, err := NewSQLModel(db)
	if err != nil {
		t.Fatalf("creating model: %v", err)
	}
	server, err := NewServer(model, nullLogger{}, "Pacific/Auckland", "", "", true)
	if err != nil {
		t.Fatalf("creating server: %v", err)
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("creating cookie jar: %v", err)
	}

	// Fetch homepage
	var csrfToken string // CSRF token stays same for entire session
	{
		recorder := serve(t, server, jar, "GET", "/", nil)

		ensureCode(t, recorder, http.StatusOK)
		forms := parseForms(t, recorder.Body.String())
		ensureInt(t, len(forms), 1)
		ensureString(t, forms[0].Action, "/create-list")
		csrfToken = forms[0].Inputs["csrf-token"]
		if csrfToken == "" {
			t.Fatal("csrf-token input not found")
		}
	}

	// Create list
	var listID string
	var listIDs []string
	{
		form := url.Values{}
		form.Set("csrf-token", csrfToken)
		form.Set("name", "Shopping List")
		recorder := serve(t, server, jar, "POST", "/create-list", form)

		ensureCode(t, recorder, http.StatusFound)
		location := recorder.Result().Header.Get("Location")
		ensureRegex(t, location, "/lists/[a-z]{10}")
		listID = location[7:]
		listIDs = append(listIDs, listID)
	}

	// Create another list
	{
		time.Sleep(time.Millisecond) // wait at least 1ms to ensure time_created is newer
		form := url.Values{}
		form.Set("csrf-token", csrfToken)
		form.Set("name", "Another List")
		recorder := serve(t, server, jar, "POST", "/create-list", form)

		ensureCode(t, recorder, http.StatusFound)
		location := recorder.Result().Header.Get("Location")
		ensureRegex(t, location, "/lists/[a-z]{10}")
		listIDs = append(listIDs, location[7:])
	}

	// Fetch homepage again (should link to lists)
	{
		recorder := serve(t, server, jar, "GET", "/", nil)

		links := parseLinks(t, recorder.Body.String())
		ensureInt(t, len(links), 5) // 2 links per list (view + delete), 1 link for "About"
		ensureString(t, links[0].Href, "/lists/"+listIDs[1])
		ensureString(t, links[0].Text, "Another List")
		ensureString(t, links[1].Href, "/lists/"+listIDs[1]+"?delete=1")
		ensureString(t, links[1].Text, "✕")
		ensureString(t, links[2].Href, "/lists/"+listIDs[0])
		ensureString(t, links[2].Text, "Shopping List")
		ensureString(t, links[3].Href, "/lists/"+listIDs[0]+"?delete=1")
		ensureString(t, links[3].Text, "✕")
		ensureString(t, links[4].Text, "About")
	}

	// Fetch list page in "delete" mode
	{
		recorder := serve(t, server, jar, "GET", "/lists/"+listIDs[1]+"?delete=1", nil)

		forms := parseForms(t, recorder.Body.String())
		ensureInt(t, len(forms), 2)
		ensureString(t, forms[0].Action, "/delete-list")
		ensureString(t, forms[0].Inputs["csrf-token"], csrfToken)
		ensureString(t, forms[0].Inputs["list-id"], listIDs[1])
		ensureString(t, forms[1].Action, "/add-item")
		ensureString(t, forms[1].Inputs["csrf-token"], csrfToken)
		ensureString(t, forms[1].Inputs["list-id"], listIDs[1])
	}

	// Delete list
	{
		form := url.Values{}
		form.Set("csrf-token", csrfToken)
		form.Set("list-id", listIDs[1])
		recorder := serve(t, server, jar, "POST", "/delete-list", form)

		ensureRedirect(t, recorder, http.StatusFound, "/")
	}

	// Ensure list was deleted
	{
		recorder := serve(t, server, jar, "GET", "/", nil)

		links := parseLinks(t, recorder.Body.String())
		ensureInt(t, len(links), 3) // 2 links per list (view + delete), 1 link for "About"
		ensureString(t, links[0].Href, "/lists/"+listIDs[0])
		ensureString(t, links[0].Text, "Shopping List")
		ensureString(t, links[1].Href, "/lists/"+listIDs[0]+"?delete=1")
		ensureString(t, links[1].Text, "✕")
		ensureString(t, links[2].Text, "About")
	}

	// Fetch empty list
	{
		recorder := serve(t, server, jar, "GET", "/lists/"+listID, nil)

		forms := parseForms(t, recorder.Body.String())
		ensureInt(t, len(forms), 1)
		ensureString(t, forms[0].Action, "/add-item")
		ensureString(t, forms[0].Inputs["csrf-token"], csrfToken)
		ensureString(t, forms[0].Inputs["list-id"], listID)
	}

	// Add item
	{
		form := url.Values{}
		form.Set("csrf-token", csrfToken)
		form.Set("list-id", listID)
		form.Set("description", "Milk (2L)")
		recorder := serve(t, server, jar, "POST", "/add-item", form)

		ensureRedirect(t, recorder, http.StatusFound, "/lists/"+listID)
	}

	// Add another item
	{
		form := url.Values{}
		form.Set("csrf-token", csrfToken)
		form.Set("list-id", listID)
		form.Set("description", "A dozen eggs")
		recorder := serve(t, server, jar, "POST", "/add-item", form)

		ensureRedirect(t, recorder, http.StatusFound, "/lists/"+listID)
	}

	// Fetch populated list
	var itemIDs []string
	{
		recorder := serve(t, server, jar, "GET", "/lists/"+listID, nil)

		forms := parseForms(t, recorder.Body.String())
		ensureInt(t, len(forms), 5) // 2 forms for each list item, 1 for /add-item

		var labels []string
		for i := 0; i < 2; i++ {
			ensureString(t, forms[i*2].Action, "/update-done")
			ensureString(t, forms[i*2].Inputs["csrf-token"], csrfToken)
			ensureString(t, forms[i*2].Inputs["list-id"], listID)
			ensureString(t, forms[i*2].Inputs["done"], "on")
			itemIDs = append(itemIDs, forms[i*2].Inputs["item-id"])
			labels = append(labels, forms[i*2].Label)
			ensureString(t, forms[i*2+1].Action, "/delete-item")
			ensureString(t, forms[i*2+1].Inputs["csrf-token"], csrfToken)
			ensureString(t, forms[i*2+1].Inputs["list-id"], listID)
			ensureString(t, forms[i*2+1].Inputs["item-id"], forms[i*2].Inputs["item-id"])
		}
		ensureInt(t, len(labels), 2)
		ensureString(t, labels[0], "Milk (2L)")
		ensureString(t, labels[1], "A dozen eggs")

		ensureString(t, forms[len(forms)-1].Action, "/add-item")
		ensureString(t, forms[len(forms)-1].Inputs["csrf-token"], csrfToken)
		ensureString(t, forms[len(forms)-1].Inputs["list-id"], listID)
	}

	// Mark item done
	{
		form := url.Values{}
		form.Set("csrf-token", csrfToken)
		form.Set("list-id", listID)
		form.Set("item-id", itemIDs[1])
		form.Set("done", "on")
		recorder := serve(t, server, jar, "POST", "/update-done", form)

		ensureRedirect(t, recorder, http.StatusFound, "/lists/"+listID)
	}

	// Ensure item was marked done
	{
		recorder := serve(t, server, jar, "GET", "/lists/"+listID, nil)

		forms := parseForms(t, recorder.Body.String())
		ensureInt(t, len(forms), 5)
		ensureString(t, forms[0].Inputs["item-id"], itemIDs[0])
		ensureString(t, forms[0].Inputs["done"], "on")
		ensureString(t, forms[2].Inputs["item-id"], itemIDs[1])
		ensureString(t, forms[2].Inputs["done"], "")
	}

	// Delete item
	{
		form := url.Values{}
		form.Set("csrf-token", csrfToken)
		form.Set("list-id", listID)
		form.Set("item-id", itemIDs[0])
		recorder := serve(t, server, jar, "POST", "/delete-item", form)

		ensureRedirect(t, recorder, http.StatusFound, "/lists/"+listID)
	}

	// Ensure item was deleted
	{
		recorder := serve(t, server, jar, "GET", "/lists/"+listID, nil)
		forms := parseForms(t, recorder.Body.String())
		ensureInt(t, len(forms), 3)
		ensureString(t, forms[0].Inputs["item-id"], itemIDs[1])
		ensureString(t, forms[0].Label, "A dozen eggs")
	}
}

// ensureCode asserts that the HTTP status code is correct.
func ensureCode(t *testing.T, recorder *httptest.ResponseRecorder, expected int) {
	t.Helper()
	if recorder.Code != expected {
		t.Fatalf("got code %d, want %d, response body:\n%s",
			recorder.Code, expected, recorder.Body.String())
	}
}

// ensureRedirect asserts that the HTTP status code and the Location header are correct.
func ensureRedirect(t *testing.T, recorder *httptest.ResponseRecorder, code int, location string) {
	t.Helper()
	ensureCode(t, recorder, code)
	locationHeader := recorder.Result().Header.Get("Location")
	ensureString(t, locationHeader, location)
}

// ensureString asserts that got==want for strings.
func ensureString(t *testing.T, got, want string) {
	t.Helper()
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

// ensureString asserts that got==want for ints.
func ensureInt(t *testing.T, got, want int) {
	t.Helper()
	if got != want {
		t.Fatalf("got %d, want %d", got, want)
	}
}

// ensureRegex asserts that got (in its entirety) matches the given regex pattern.
func ensureRegex(t *testing.T, got, pattern string) {
	t.Helper()
	re := regexp.MustCompile("^" + pattern + "$")
	if !re.MatchString(got) {
		t.Fatalf("got %q, expected match to %q", got, pattern)
	}
}

// serve records a single HTTP request and returns the response recorder.
func serve(t *testing.T, server *Server, jar http.CookieJar, method, path string, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	r, err := http.NewRequest(method, "http://localhost"+path, body)
	if err != nil {
		t.Fatalf("creating request: %v", err)
	}
	if form != nil {
		r.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	}
	for _, c := range jar.Cookies(r.URL) {
		r.Header.Add("Cookie", c.Name+"="+c.Value)
	}
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, r)
	jar.SetCookies(r.URL, recorder.Result().Cookies())
	return recorder
}

type Form struct {
	Action string
	Inputs map[string]string
	Label  string
}

// parseForms parses the forms in an HTML document and returns the list of forms.
func parseForms(t *testing.T, htmlStr string) []Form {
	t.Helper()
	doc, err := html.Parse(strings.NewReader(htmlStr))
	if err != nil {
		t.Fatalf("parsing HTML: %v", err)
	}

	var forms []Form
	var traverse func(*html.Node)

	traverse = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "form" {
			action := getAttr(n, "action")
			method := getAttr(n, "method")
			if method != "POST" {
				t.Fatalf("form %s method: got %s, want POST", action, method)
			}
			enctype := getAttr(n, "enctype")
			if enctype != "application/x-www-form-urlencoded" {
				t.Fatalf("form %s enctype: got %s, want application/x-www-form-urlencoded",
					action, method)
			}

			inputs := make(map[string]string)
			var label string
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				if c.Type == html.ElementNode && c.Data == "input" {
					inputs[getAttr(c, "name")] = getAttr(c, "value")
				}
				if c.Type == html.ElementNode && c.Data == "label" {
					label = getText(c.FirstChild)
				}
			}

			forms = append(forms, Form{
				Action: action,
				Inputs: inputs,
				Label:  label,
			})
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			traverse(c)
		}
	}

	traverse(doc)
	return forms
}

type Link struct {
	Href string
	Text string
}

// parseLinks parses the links in an HTML document and returns the list of links.
func parseLinks(t *testing.T, htmlStr string) []Link {
	t.Helper()
	doc, err := html.Parse(strings.NewReader(htmlStr))
	if err != nil {
		t.Fatalf("parsing HTML: %v", err)
	}

	var links []Link
	var traverse func(*html.Node)

	traverse = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "a" {
			links = append(links, Link{
				Href: getAttr(n, "href"),
				Text: getText(n),
			})
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			traverse(c)
		}
	}

	traverse(doc)
	return links
}

// getAttr returns the value of the named attribute, or "" if not found.
func getAttr(n *html.Node, name string) string {
	for _, a := range n.Attr {
		if a.Key == name {
			return a.Val
		}
	}
	return ""
}

// getText recursively assembles the text nodes of n into a string.
func getText(n *html.Node) string {
	if n == nil {
		return ""
	}
	if n.Type == html.TextNode {
		return n.Data
	}
	text := ""
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		text += getText(c)
	}
	return text
}

type nullLogger struct{}

func (nullLogger) Printf(format string, v ...interface{}) {}
