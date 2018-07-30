package wikitext

import "strings"

func URLToTitle(u string) string {
	return strings.Replace(u, "_", " ", -1)
}

func TitleToURL(u string) string {
	return "./" + strings.Replace(u, " ", "_", -1)
}
