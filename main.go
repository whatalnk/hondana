package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"sync"

	"database/sql"

	_ "github.com/mattn/go-sqlite3"
	"rsc.io/pdf"
)

type Config struct {
	path    string
	Roots   []string
	DataDir string
}

var config Config
var library Library

var db *sql.DB

func getConfig() Config {
	usr, _ := user.Current()
	home := usr.HomeDir
	configdir := filepath.Join(home, ".hondana")
	configfile := filepath.Join(configdir, "config.json")
	if _, err := os.Stat(configfile); err != nil {
		if _, err := os.Stat(configdir); err != nil {
			if err := os.Mkdir(configdir, 0777); err != nil {
				log.Fatal(fmt.Sprintf("Cannot create %s\n", configdir), err)
			}
		}
		config.path = configfile
		config.DataDir = filepath.ToSlash(configdir)
		config.updateConfig()
		return config
	}
	f, _ := ioutil.ReadFile(configfile)
	json.Unmarshal(f, &config)
	config.path = configfile
	return config
}

type Library struct {
	Shelves []Shelf
}

type Shelf struct {
	Root  string
	Books []Book
}

type Book struct {
	Title   string
	Author  string
	NumPage int
	File    string
}

func visit(fileList *[]Book, root string) filepath.WalkFunc {
	return func(path string, info os.FileInfo, err error) error {
		if info.IsDir() || filepath.Ext(path) != ".pdf" {
			return nil
		}
		f, _ := pdf.Open(path)
		var title, author string
		var numPage int
		rel, _ := filepath.Rel(root, path)
		defer func() {
			if r := recover(); r != nil {
				title = filepath.Base(path)
				*fileList = append(*fileList, Book{title, author, numPage, rel})
				return
			}
		}()
		numPage = f.NumPage()
		pdfInfo := f.Trailer().Key("Info")
		title = pdfInfo.Key("Title").Text()
		if title == "" {
			title = filepath.Base(path)
		}
		author = pdfInfo.Key("Author").Text()

		*fileList = append(*fileList, Book{title, author, numPage, rel})
		return nil
	}
}

type templateHandler struct {
	once     sync.Once
	filename string
	templ    *template.Template
}

func (t *templateHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		library = dbLoad()
	}
	t.once.Do(func() {
		t.templ = template.Must(template.ParseFiles(filepath.Join("templates", t.filename)))
	})
	err := t.templ.Execute(w, library)
	if err != nil {
		log.Fatal("template.Execute: ", err)
	}
}

type configHandler struct {
	once     sync.Once
	filename string
	templ    *template.Template
	config   *Config
}

func (c *Config) updateConfig() {
	data, _ := json.Marshal(c)
	f, err := os.Create(c.path)
	if err != nil {
		log.Fatal(fmt.Sprintf("Cannot open %s\n", c.path), err)
	}
	if _, err := f.Write(data); err != nil {
		log.Fatal(fmt.Sprint("Cannot write settings"), err)
	}
	defer f.Close()
}

func (c *configHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == "POST" {
		r.ParseForm()
		if v, ok := r.Form["root"]; ok && v[0] != "" {
			c.config.Roots = append(c.config.Roots, v[0])
			c.config.updateConfig()
			dbAdd(v[0])
			log.Printf("Add Root: %s", v[0])
		} else if v, ok := r.Form["_method"]; ok {
			if v[0] == "DELETE" {
				i, _ := strconv.Atoi(v[1])
				tmp := c.config.Roots[i]
				c.config.Roots = append(c.config.Roots[:i], c.config.Roots[i+1:]...)
				c.config.updateConfig()
				dbDelete(tmp)
				log.Printf("Delete: %s", tmp)
			}
		}
	}
	c.once.Do(func() {
		c.templ = template.Must(template.ParseFiles(filepath.Join("templates", c.filename)))
	})
	err := c.templ.Execute(w, c.config)
	if err != nil {
		log.Fatal("template.Execute: ", err)
	}

}

func dbAdd(root string) {
	shelf := createShelf(root)
	tx, err := db.Begin()
	if err != nil {
		log.Fatal(err)
	}
	stmt, err := tx.Prepare("insert into books(root, title, author, numPage, file) values(?, ?, ?, ?, ?)")
	if err != nil {
		log.Fatal(err)
	}
	defer stmt.Close()
	usr, _ := user.Current()
	home := usr.HomeDir
	rt, _ := filepath.Rel(home, shelf.Root)
	for _, bk := range shelf.Books {
		_, err = stmt.Exec(rt, bk.Title, bk.Author, bk.NumPage, bk.File)
		if err != nil {
			log.Fatal(err)
		}
	}
	tx.Commit()
	log.Printf("Add to DB: %s", root)
}

func dbDelete(root string) {
	tx, err := db.Begin()
	if err != nil {
		log.Fatal(err)
	}

	stmt, err := tx.Prepare("delete from books where root = ?")
	if err != nil {
		log.Fatal(err)
	}
	defer stmt.Close()

	usr, _ := user.Current()
	home := usr.HomeDir
	rt, _ := filepath.Rel(home, root)

	_, err = stmt.Exec(rt)
	if err != nil {
		log.Fatal(err)
	}
	tx.Commit()
	log.Printf("Delete from DB: %s", root)

}

func dbLoad() Library {
	tx, err := db.Begin()
	if err != nil {
		log.Fatal(err)
	}

	stmt, err := tx.Prepare("select title, author, numPage, file from books where root = ?")
	if err != nil {
		log.Fatal(err)
	}
	defer stmt.Close()

	var shelves []Shelf
	if len(config.Roots) != 0 {
		usr, _ := user.Current()
		home := usr.HomeDir
		for _, v := range config.Roots {
			rt, _ := filepath.Rel(home, v)
			rows, err := stmt.Query(rt)
			if err != nil {
				log.Fatal(err)
			}
			defer rows.Close()
			var books []Book
			for rows.Next() {
				var title, author, file string
				var numPage int
				rows.Scan(&title, &author, &numPage, &file)
				books = append(books, Book{title, author, numPage, file})
			}
			shelves = append(shelves, Shelf{rt, books})
		}
	}
	tx.Commit()
	return Library{shelves}
}

func dbUpdate() {
	sqlStmt := `
	create table if not exists temp (id integer not null primary key, root text, title text, author text, numPage integer, file text, thumbnail text);
  delete from temp;
	`
	_, err := db.Exec(sqlStmt)
	if err != nil {
		log.Printf("%q: %s\n", err, sqlStmt)
		return
	}

	tx, err := db.Begin()
	if err != nil {
		log.Fatal(err)
	}

	stmt, err := tx.Prepare("insert into temp(root, title, author, numPage, file) values(?, ?, ?, ?, ?)")
	if err != nil {
		log.Fatal(err)
	}
	defer stmt.Close()

	usr, _ := user.Current()
	home := usr.HomeDir

	if len(config.Roots) != 0 {
		for _, v := range config.Roots {
			shelf := createShelf(v)
			rt, _ := filepath.Rel(home, shelf.Root)
			for _, bk := range shelf.Books {
				_, err = stmt.Exec(rt, bk.Title, bk.Author, bk.NumPage, bk.File)
				if err != nil {
					log.Fatal(err)
				}
			}
		}
	}
	tx.Commit()

	// Delete books
	_, err = db.Exec("delete from books where not exists (select * from temp where 'books.file' = 'temp.file');")
	if err != nil {
		log.Printf("%q: %s\n", err, sqlStmt)
		return
	}
	// Add new books
	_, err = db.Exec("insert into books select * from temp where exists (select * from temp where 'books.file' <> 'temp.file');")
	if err != nil {
		log.Printf("%q: %s\n", err, sqlStmt)
		return
	}
}

func createShelf(root string) Shelf {
	filelist := []Book{}
	err := filepath.Walk(root, visit(&filelist, root))
	if err != nil {
		log.Fatal("filepath.Walk: ", err)
	}
	return Shelf{Root: root, Books: filelist}
}

func main() {
	config = getConfig()
	var err error
	db, err = sql.Open("sqlite3", filepath.Join(config.DataDir, "hondana.db"))
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	sqlStmt := `
	create table if not exists books (id integer not null primary key, root text, title text, author text, numPage integer, file text, thumbnail text);
	`
	_, err = db.Exec(sqlStmt)
	if err != nil {
		log.Printf("%q: %s\n", err, sqlStmt)
	}
	db.SetMaxIdleConns(100)

	dbUpdate()

	fmt.Println("accepting connections at http://localhost:8080")
	http.Handle("/", &templateHandler{filename: "index.html"})
	http.Handle("/settings", &configHandler{filename: "settings.html", config: &config})
	http.Handle("/data/", http.StripPrefix("/data/", http.FileServer(http.Dir(config.DataDir))))
	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}
