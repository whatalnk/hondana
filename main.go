package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io/ioutil"
	"os"
	"path/filepath"
	"sync"

	"log"
	"net/http"

	"os/user"

	"rsc.io/pdf"
)

type Config struct {
	path string
	Root string
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
		f, err := os.Create(configfile)
		if err != nil {
			log.Fatal(fmt.Sprintf("Cannot create %s\n", configfile), err)
		}
		data, _ := json.Marshal(config)
		if _, err := f.Write(data); err != nil {
			log.Fatal(fmt.Sprint("Cannot write settings"), err)
		}

		return config
	}
	f, _ := ioutil.ReadFile(configfile)
	json.Unmarshal(f, &config)
	config.path = configfile
	return config
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

func visit(fileList *[]Book) filepath.WalkFunc {
	return func(path string, info os.FileInfo, err error) error {
		if info.IsDir() || filepath.Ext(path) != ".pdf" {
			return nil
		}
		f, _ := pdf.Open(path)
		var title, author string
		var numPage int
		rel, _ := filepath.Rel(config.Root, path)
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
	data     Shelf
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

func (c *configHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == "POST" {
		r.ParseForm()
		v := r.Form["root"][0]
		c.config.Root = v
		data, _ := json.Marshal(c.config)
		f, err := os.Create(c.config.path)
		if err != nil {
			log.Fatal(fmt.Sprintf("Cannot open %s\n", c.config.path), err)
		}
		if _, err := f.Write(data); err != nil {
			log.Fatal(fmt.Sprint("Cannot write settings"), err)
		}
		defer f.Close()
		log.Printf("Update Root to: %s", v)
	}
	c.once.Do(func() {
		c.templ = template.Must(template.ParseFiles(filepath.Join("templates", c.filename)))
	})
	err := c.templ.Execute(w, c.config)
	if err != nil {
		log.Fatal("template.Execute: ", err)
	}

}

func main() {
	config = getConfig()
	fileList := []Book{}
	if config.Root != "" {
		err := filepath.Walk(config.Root, visit(&fileList))
		if err != nil {
			log.Fatal("filepath.Walk: ", err)
		}
	}
	fmt.Println("accepting connections at http://localhost:8080")
	http.Handle("/", &templateHandler{filename: "index.html", data: Shelf{config.Root, fileList}})
	http.Handle("/settings", &configHandler{filename: "settings.html", config: config})
	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}
