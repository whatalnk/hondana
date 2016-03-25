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
	data     Library
}

func (t *templateHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	t.once.Do(func() {
		t.templ = template.Must(template.ParseFiles(filepath.Join("templates", t.filename)))
	})
	err := t.templ.Execute(w, t.data)
	if err != nil {
		log.Fatal("template.Execute: ", err)
	}
}

type configHandler struct {
	once     sync.Once
	filename string
	templ    *template.Template
	config   Config
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
		if v, ok := r.Form["root"]; ok {
			c.config.Roots = append(c.config.Roots, v[0])
			c.config.updateConfig()
			log.Printf("Add Root: %s", v)
		} else if v, ok := r.Form["_method"]; ok {
			if v[0] == "DELETE" {
				i, _ := strconv.Atoi(v[1])
				tmp := c.config.Roots[i]
				c.config.Roots = append(c.config.Roots[:i], c.config.Roots[i+1:]...)
				c.config.updateConfig()
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

func dbUpdate(lb Library) {
	db, err := sql.Open("sqlite3", filepath.Join(config.DataDir, "hondana.db"))
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	sqlStmt := `
	create table books (id integer not null primary key, root text, title text, author text, numPage integer, file text, thumbnail text);
	`
	_, err = db.Exec(sqlStmt)
	if err != nil {
		log.Printf("%q: %s\n", err, sqlStmt)
		return
	}
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
	for _, sh := range lb.Shelves {
		rt, _ := filepath.Rel(home, sh.Root)
		for _, bk := range sh.Books {
			_, err = stmt.Exec(rt, bk.Title, bk.Author, bk.NumPage, bk.File)
			if err != nil {
				log.Fatal(err)
			}
		}
	}
	tx.Commit()
}

func main() {
	config = getConfig()
	var shelves []Shelf
	var fileList []Book
	if len(config.Roots) != 0 {
		for _, v := range config.Roots {
			err := filepath.Walk(v, visit(&fileList, v))
			if err != nil {
				log.Fatal("filepath.Walk: ", err)
			}
			shelves = append(shelves, Shelf{Root: v, Books: fileList})
			fileList = fileList[:0]
		}
	}
	lb := Library{shelves}
	dbUpdate(lb)
	fmt.Println("accepting connections at http://localhost:8080")
	http.Handle("/", &templateHandler{filename: "index.html", data: lb})
	http.Handle("/settings", &configHandler{filename: "settings.html", config: config})
	http.Handle("/data/", http.StripPrefix("/data/", http.FileServer(http.Dir(config.DataDir))))
	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}
