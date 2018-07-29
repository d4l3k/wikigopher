package main

import (
	"bufio"
	"compress/bzip2"
	"encoding/xml"
	"flag"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"
	"sync"

	"github.com/blevesearch/bleve"
	"github.com/d4l3k/wikigopher/wikitext"
	"github.com/davecgh/go-spew/spew"
	"github.com/microcosm-cc/bluemonday"
	"github.com/pkg/errors"
)

var (
	indexFile       = flag.String("index", "enwiki-latest-pages-articles-multistream-index.txt.bz2", "the index file to load")
	articlesFile    = flag.String("articles", "enwiki-latest-pages-articles-multistream.xml.bz2", "the article dump file to load")
	searchIndexFile = flag.String("searchIndex", "index.bleve", "the search index file")
	httpAddr        = flag.String("http", ":8080", "the address to bind HTTP to")
)

var tmpls = template.Must(template.ParseGlob("templates/*"))

type indexEntry struct {
	id, seek int
	Title    string
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
		entry := indexEntry{
			id:    id,
			seek:  seek,
			Title: strings.Join(parts[2:], ":"),
		}

		mu.Lock()
		mu.offsets[entry.Title] = entry
		mu.Unlock()

		i++
		if i%100000 == 0 {
			log.Printf("read %d entries", i)
		}
	}
	log.Printf("Done reading!")

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

func main() {
	flag.Parse()
	log.SetFlags(log.Flags() | log.Lshortfile)

	go func() {
		if err := loadIndex(); err != nil {
			log.Fatalf("%+v", err)
		}
	}()

	http.HandleFunc("/wiki/", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s", r.Method, r.URL.Path)
		articleName := path.Base(r.URL.Path)

		mu.Lock()
		defer mu.Unlock()

		if articleName == "Special:Random" {
			for name := range mu.offsets {
				http.Redirect(w, r, fmt.Sprintf("/wiki/%s", name), http.StatusTemporaryRedirect)
				return
			}
		}

		articleMeta, ok := mu.offsets[articleName]
		if !ok {
			http.Error(w, "article not found", http.StatusNotFound)
			return
		}
		log.Printf("index %#v", articleMeta)

		p, err := readArticle(articleMeta)
		if err != nil {
			http.Error(w, fmt.Sprintf("%+v", err), 500)
			return
		}
		spew.Dump(p)
		body, err := wikitext.Convert([]byte(p.Text))
		if err != nil {
			http.Error(w, fmt.Sprintf("%+v", err), 500)
			return
		}
		if err := tmpls.ExecuteTemplate(w, "article.html", struct {
			Title string
			Body  template.HTML
		}{
			Title: articleName,
			Body:  template.HTML(bluemonday.UGCPolicy().Sanitize(string(body))),
		}); err != nil {
			http.Error(w, fmt.Sprintf("%+v", err), 500)
			return
		}
	})
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("todo"))
	})

	log.Printf("Listening on %s...", *httpAddr)
	if err := http.ListenAndServe(*httpAddr, nil); err != nil {
		log.Fatalf("%+v", err)
	}
}
