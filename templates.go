package main

var homeTmpl = `<!DOCTYPE html>
<html>
 <head>
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Simple Lists</title>
 </head>
 <body>
  <h1>Simple Lists</h1>
  <ul style="list-style-type: none; margin: 0; padding: 0;">
   {{ range .Lists }}
    <li style="margin: 0.7em 0">
     <a href="/{{ .ID }}">{{ .Name }}</a>
     <span style="color: gray; font-size: 75%; margin-left: 0.2em;" title="{{ .TimeCreated.Format "2006-01-02 15:04:05" }}">{{ .TimeCreated.Format "2 Jan" }}</span>
    </li>
   {{ end }}
   <li>
    <form style="margin: 1em 0" action="/create-list" method="POST" enctype="application/x-www-form-urlencoded">
     <input type="hidden" name="csrf-token" value="{{ $.Token }}">
     <input type="text" name="name" placeholder="list name" autofocus>
     <button>New List</button>
    </form>
   </li>
  </ul>
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
  <ul style="list-style-type: none; margin: 0; padding: 0;">
   {{ range .List.Items }}
    <li style="margin: 0.7em 0">
     <form style="display: inline;" action="/check-item" method="POST" enctype="application/x-www-form-urlencoded">
      <input type="hidden" name="csrf-token" value="{{ $.Token }}">
      <input type="hidden" name="list-id" value="{{ $.List.ID }}">
      <input type="hidden" name="item-id" value="{{ .ID }}">
      {{ if .Done }}
       <button id="check-{{ .ID }}" style="width: 1.7em">✓</button>
       <label for="check-{{ .ID }}"><del>{{ .Description }}</del></label>
      {{ else }}
       <input type="hidden" name="done" value="on">
       <button id="check-{{ .ID }}" style="width: 1.7em">&nbsp;</button>
       <label for="check-{{ .ID }}">{{ .Description }}</label>
      {{ end }}
     </form>
     <form style="display: inline;" action="/delete-item" method="POST" enctype="application/x-www-form-urlencoded">
      <input type="hidden" name="csrf-token" value="{{ $.Token }}">
      <input type="hidden" name="list-id" value="{{ $.List.ID }}">
      <input type="hidden" name="item-id" value="{{ .ID }}">
      <button style="padding: 0 0.5em; border: none; background: none; color: #ccc" title="Remove">✕</button>
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
