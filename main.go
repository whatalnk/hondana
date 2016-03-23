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

	"rsc.io/pdf"
)

type Config struct {
	Root string
}

var root string

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

func main() {
	f, _ := ioutil.ReadFile("config.json")
	var config Config
	json.Unmarshal(f, &config)
	root = config.Root

	fileList := []Book{}
	err := filepath.Walk(root, visit(&fileList))
	if err != nil {
		log.Fatal("filepath.Walk: ", err)
	}

	fmt.Println("accepting connections at http://localhost:8080")
	http.Handle("/", &templateHandler{filename: "index.html", data: Shelf{root, fileList}})
	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}
