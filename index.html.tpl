<!DOCTYPE html>
<html lang="ja">

<head>
  <meta charset="UTF-8"></meta>
  <title>List of PDF Books</title>
</head>

<body>
  <h1>List of PDF Books in {{.Root}}</h1>
  <ul>
    {{range .Books}}
    <li>{{.Path}}</li>
    {{end}}
  </ul>
</body>

</html>