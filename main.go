package main

import (
	"bufio"
	"compress/bzip2"
	"encoding/xml"
	"flag"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/blevesearch/bleve"
	"github.com/d4l3k/wikigopher/wikitext"
	"github.com/microcosm-cc/bluemonday"
	"github.com/pkg/errors"
)

var (
	indexFile       = flag.String("index", "enwiki-latest-pages-articles-multistream-index.txt.bz2", "the index file to load")
	articlesFile    = flag.String("articles", "enwiki-latest-pages-articles-multistream.xml.bz2", "the article dump file to load")
	search          = flag.Bool("search", false, "whether or not to build a search index")
	searchIndexFile = flag.String("searchIndex", "index.bleve", "the search index file")
	httpAddr        = flag.String("http", ":8080", "the address to bind HTTP to")
)

var tmpls = map[string]*template.Template{}

func loadTemplates() error {
	files, err := filepath.Glob("templates/*")
	if err != nil {
		return err
	}
	for _, file := range files {
		name := filepath.Base(file)
		tmpls[name], err = template.ParseFiles("templates/base.html", file)
		if err != nil {
			return err
		}
	}
	return nil
}

func executeTemplate(wr io.Writer, name string, data interface{}) error {
	return tmpls[name].ExecuteTemplate(wr, "base", data)
}

type indexEntry struct {
	id, seek int
}

var mu = struct {
	sync.Mutex

	offsets map[string]indexEntry
}{
	offsets: map[string]indexEntry{},
}
var index bleve.Index

func loadIndex() error {
	mapping := bleve.NewIndexMapping()
	os.RemoveAll(*searchIndexFile)
	var err error
	index, err = bleve.New(*searchIndexFile, mapping)
	if err != nil {
		return err
	}
	f, err := os.Open(*indexFile)
	if err != nil {
		return err
	}
	defer f.Close()
	r := bzip2.NewReader(f)
	scanner := bufio.NewScanner(r)

	log.Printf("Reading index file...")
	i := 0
	for scanner.Scan() {
		parts := strings.Split(scanner.Text(), ":")
		if len(parts) < 3 {
			return errors.Errorf("expected at least 3 parts, got: %#v", parts)
		}
		seek, err := strconv.Atoi(parts[0])
		if err != nil {
			return err
		}
		id, err := strconv.Atoi(parts[1])
		if err != nil {
			return err
		}
		title := strings.Join(parts[2:], ":")
		entry := indexEntry{
			id:   id,
			seek: seek,
		}

		mu.Lock()
		mu.offsets[title] = entry
		mu.Unlock()

		i++
		if i%100000 == 0 {
			log.Printf("read %d entries", i)
		}
	}
	log.Printf("Done reading!")

	if !*search {
		return nil
	}
	log.Printf("Indexing titles...")
	i = 0
	batch := index.NewBatch()

	mu.Lock()
	defer mu.Unlock()

	for key, entry := range mu.offsets {
		mu.Unlock()

		if err := batch.Index(key, entry); err != nil {
			mu.Lock()
			return err
		}
		i++
		if i%100000 == 0 {
			if err := index.Batch(batch); err != nil {
				mu.Lock()
				return err
			}
			batch.Reset()
			log.Printf("indexed %d entries", i)
		}

		mu.Lock()
	}

	if err := scanner.Err(); err != nil {
		return err
	}
	log.Printf("Done indexing!")
	return nil
}

/*
Example:
  <page>
    <title>AccessibleComputing</title>
    <ns>0</ns>
    <id>10</id>
    <redirect title="Computer accessibility" />
    <revision>
      <id>834079434</id>
      <parentid>767284433</parentid>
      <timestamp>2018-04-03T20:38:02Z</timestamp>
      <contributor>
        <username>امیر اعوانی</username>
        <id>8214454</id>
      </contributor>
      <minor />
      <model>wikitext</model>
      <format>text/x-wiki</format>
      <text xml:space="preserve">#REDIRECT [[Computer accessibility]]

{{Redirect category shell}}
{{R from move}}
{{R from CamelCase}}
{{R unprintworthy}}</text>
      <sha1>qdiw0cwardl0qpkyeutu3pd77fwym8y</sha1>
    </revision>
  </page>
*/

type redirect struct {
	Title string `xml:"title,attr"`
}

type page struct {
	XMLName    xml.Name   `xml:"page"`
	Title      string     `xml:"title"`
	NS         int        `xml:"ns"`
	ID         int        `xml:"id"`
	Redirect   []redirect `xml:"redirect"`
	RevisionID string     `xml:"revision>id"`
	Timestamp  string     `xml:"revision>timestamp"`
	Username   string     `xml:"revision>contributor>username"`
	UserID     string     `xml:"revision>contributor>id"`
	Model      string     `xml:"revision>model"`
	Format     string     `xml:"revision>format"`
	Text       string     `xml:"revision>text"`
}

func readArticle(meta indexEntry) (page, error) {
	f, err := os.Open(*articlesFile)
	if err != nil {
		return page{}, err
	}
	defer f.Close()

	r := bzip2.NewReader(f)

	if _, err := f.Seek(int64(meta.seek), 0); err != nil {
		return page{}, err
	}

	d := xml.NewDecoder(r)

	var p page
	for {
		if err := d.Decode(&p); err != nil {
			return page{}, err
		}
		if p.ID == meta.id {
			break
		}
	}

	return p, nil
}

func fetchArticle(name string) (indexEntry, error) {
	articleMeta, ok := mu.offsets[name]
	if ok {
		return articleMeta, nil
	}
	articleMeta, ok = mu.offsets[strings.Title(strings.ToLower(name))]
	if ok {
		return articleMeta, nil
	}
	return indexEntry{}, statusErrorf(http.StatusNotFound, "article not found")
}

type statusError int

func (s statusError) Error() string {
	return fmt.Sprintf("%d - %s", int(s), http.StatusText(int(s)))
}

func statusErrorf(code int, str string, args ...interface{}) error {
	return errors.Wrapf(statusError(code), str, args...)
}

func errorHandler(f func(w http.ResponseWriter, r *http.Request) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := f(w, r); err != nil {
			cause := errors.Cause(err)
			status := http.StatusInternalServerError
			if cause, ok := cause.(statusError); ok {
				status = int(cause)
			}
			http.Error(w, err.Error(), status)
		}
	}

}

func handleArticle(w http.ResponseWriter, r *http.Request) error {
	articleName := path.Base(r.URL.Path)

	mu.Lock()
	defer mu.Unlock()

	if articleName == "Special:Random" {
		for name := range mu.offsets {
			http.Redirect(w, r, fmt.Sprintf("/wiki/%s", name), http.StatusTemporaryRedirect)
			return nil
		}
	}

	articleMeta, err := fetchArticle(articleName)
	if err != nil {
		return err
	}

	p, err := readArticle(articleMeta)
	if err != nil {
		return err
	}

	if p.Title != articleName {
		http.Redirect(w, r, fmt.Sprintf("/wiki/%s", p.Title), http.StatusTemporaryRedirect)
		return nil
	}

	body, err := wikitext.Convert([]byte(p.Text))
	if err != nil {
		return err
	}
	if err := executeTemplate(w, "article.html", struct {
		Title string
		Body  template.HTML
	}{
		Title: articleName,
		Body:  template.HTML(bluemonday.UGCPolicy().Sanitize(string(body))),
	}); err != nil {
		return err
	}
	return nil
}

func handleSource(w http.ResponseWriter, r *http.Request) error {
	articleName := path.Base(r.URL.Path)

	mu.Lock()
	defer mu.Unlock()

	if articleName == "Special:Random" {
		for name := range mu.offsets {
			http.Redirect(w, r, fmt.Sprintf("/wiki/%s", name), http.StatusTemporaryRedirect)
		}
	}

	articleMeta, err := fetchArticle(articleName)
	if err != nil {
		return err
	}
	p, err := readArticle(articleMeta)
	if err != nil {
		return err
	}
	return executeTemplate(w, "source.html", p)
}

func handleIndex(w http.ResponseWriter, r *http.Request) error {
	_, err := w.Write([]byte("todo"))
	return err
}

func main() {
	if err := run(); err != nil {
		log.Fatal("%+v", err)
	}
}

func run() error {
	flag.Parse()
	log.SetFlags(log.Flags() | log.Lshortfile)

	go func() {
		if err := loadIndex(); err != nil {
			log.Fatalf("%+v", err)
		}
	}()

	if err := loadTemplates(); err != nil {
		return err
	}

	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("./static"))))
	http.HandleFunc("/source/", errorHandler(handleSource))
	http.HandleFunc("/wiki/", errorHandler(handleArticle))
	http.HandleFunc("/", errorHandler(handleIndex))

	log.Printf("Listening on %s...", *httpAddr)
	return http.ListenAndServe(*httpAddr, nil)
}
