package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"text/template"

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

func main() {
	f, _ := ioutil.ReadFile("config.json")
	var config Config
	json.Unmarshal(f, &config)
	root = config.Root
	fileList := []Book{}
	err := filepath.Walk(root, visit(&fileList))
	if err != nil {
		fmt.Println(err.Error())
	}
	t := template.Must(template.ParseFiles("index.html.tpl"))
	err = t.Execute(os.Stdout, Shelf{root, fileList})
	if err != nil {
		fmt.Println(err.Error())
	}
}
