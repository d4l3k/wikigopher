# wikigopher

A fully self contained server that can read Wikipedia database dumps and display
them. It also contains a wikitext -> html converter.

## Install

```
$ go get -u github.com/d4l3k/wikigopher
```

## Download Wikipedia Database Dumps

You need to download the multistream article dumps

* enwiki-latest-pages-articles-multistream-index.txt.bz2
* enwiki-latest-pages-articles-multistream.xml.bz2

from https://dumps.wikimedia.org/enwiki/latest/

You'll need to place these in the wikigopher directory or specify their location
with `-index=....txt.bz2 -articles=....xml.bz2`.

The multistream varients are required. The index file is a mapping between
article titles and their locations in the multistream xml file.

More information can be found at https://en.wikipedia.org/wiki/Wikipedia:Database_download#Where_do_I_get_it?

## License

wikigopher is licensed under the MIT license.

## Attributions

The gopher image used was created by Takuya Ueda (https://twitter.com/tenntenn). Licensed under the Creative Commons 3.0 Attributions license.

Some CSS styles have been borrowed from MediaWiki.
