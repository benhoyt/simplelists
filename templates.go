package main

var homeTmpl = `<!DOCTYPE html>
<html>
 <head>
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Simple Lists</title>
 </head>
 <body>
  <h1>Simple Lists</h1>
{{ if .ShowSignOut }}
  <form style="margin: 1em 0" action="/sign-out" method="POST" enctype="application/x-www-form-urlencoded">
   <input type="hidden" name="csrf-token" value="{{ $.Token }}">
   <button>Sign Out</button>
  </form>
{{ end }}
{{ if .ShowSignIn }}
  <form style="margin: 1em 0" action="/sign-in" method="POST" enctype="application/x-www-form-urlencoded">
   <input type="hidden" name="csrf-token" value="{{ $.Token }}">
   <input type="hidden" name="return-url" value="{{ .ReturnURL }}">
   <input type="text" name="username" placeholder="username" autofocus>
   <input type="password" name="password" placeholder="password" autofocus>
   <button>Sign In</button>
   {{ if .SignInError }}
   <div style="color: red; margin: 0.5em 0;">incorrect username or password</div>
   {{ end }}
  </form>
{{ else }}
  <ul style="list-style-type: none; margin: 0; padding: 0;">
   <li style="margin: 1em 0">
    <form action="/create-list" method="POST" enctype="application/x-www-form-urlencoded">
     <input type="hidden" name="csrf-token" value="{{ $.Token }}">
     <input type="text" name="name" placeholder="list name" autofocus>
     <button>New List</button>
    </form>
   </li>
   {{ range .Lists }}
    <li style="margin: 0.7em 0">
     <a href="/lists/{{ .ID }}">{{ .Name }}</a>
     <span style="color: gray; font-size: 75%; margin-left: 0.2em;" title="{{ .TimeCreated.Format "2006-01-02 15:04:05" }}">{{ .TimeCreated.Format "2 Jan" }}</span>
     <a style="padding-left: 0.5em; color: #ccc; text-decoration: none;" href="/lists/{{ .ID }}?delete=1" title="Delete List">✕</a>
    </li>
   {{ end }}
  </ul>
{{ end }}
  <div style="margin: 5em 0; border-top: 1px solid #ccc; text-align: center;">
   <a style="color: gray; font-size: 75%" href="https://github.com/benhoyt/simplelists">About</a>
  </div>
 </body>
</html>
`

var listTmpl = `<!DOCTYPE html>
<html>
 <head>
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>{{ .List.Name }}</title>
 </head>
 <body>
  <h1>{{ .List.Name }}</h1>
{{ if .ShowDelete }}
 <form style="margin-bottom: 2em" action="/delete-list" method="POST" enctype="application/x-www-form-urlencoded">
  <input type="hidden" name="csrf-token" value="{{ $.Token }}">
  <input type="hidden" name="list-id" value="{{ .List.ID }}">
  <span style="color: red">Are you sure you want to delete this list?</span>
  <button>Yes, delete it!</button>
 </form>
{{ end }}
  <ul style="list-style-type: none; margin: 0; padding: 0;">
   {{ range .List.Items }}
    <li style="margin: 0.7em 0">
     <form style="display: inline;" action="/update-done" method="POST" enctype="application/x-www-form-urlencoded">
      <input type="hidden" name="csrf-token" value="{{ $.Token }}">
      <input type="hidden" name="list-id" value="{{ $.List.ID }}">
      <input type="hidden" name="item-id" value="{{ .ID }}">
      {{ if .Done }}
       <button id="done-{{ .ID }}" style="width: 1.7em">✓</button>
       <label for="done-{{ .ID }}"><del>{{ .Description }}</del></label>
      {{ else }}
       <input type="hidden" name="done" value="on">
       <button id="done-{{ .ID }}" style="width: 1.7em">&nbsp;</button>
       <label for="done-{{ .ID }}">{{ .Description }}</label>
      {{ end }}
     </form>
     <form style="display: inline;" action="/delete-item" method="POST" enctype="application/x-www-form-urlencoded">
      <input type="hidden" name="csrf-token" value="{{ $.Token }}">
      <input type="hidden" name="list-id" value="{{ $.List.ID }}">
      <input type="hidden" name="item-id" value="{{ .ID }}">
      <button style="padding: 0 0.5em; border: none; background: none; color: #ccc" title="Delete Item">✕</button>
     </form>
    </li>
   {{ end }}
   <li style="margin: 0.5em 0">
    <form action="/add-item" method="POST" enctype="application/x-www-form-urlencoded">
     <input type="hidden" name="csrf-token" value="{{ $.Token }}">
     <input type="hidden" name="list-id" value="{{ .List.ID }}">
     <input type="text" name="description" placeholder="item description" autofocus>
     <button style="margin-top: 1em" type="submit">Add</button>
    </form>
   </li>
  </ul>
  <div style="margin: 5em 0; border-top: 1px solid #ccc; text-align: center;">
   <a style="color: gray; font-size: 75%; margin-right: 1em;" href="/">Home</a>
   <a style="color: gray; font-size: 75%" href="https://github.com/benhoyt/simplelists">About</a>
  </div>
 </body>
</html>
`
