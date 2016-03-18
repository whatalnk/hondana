package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"text/template"
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
	Path string
}

func visit(fileList *[]Book) filepath.WalkFunc {
	return func(path string, info os.FileInfo, err error) error {
		if info.IsDir() || filepath.Ext(path) != ".pdf" {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		*fileList = append(*fileList, Book{rel})
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
