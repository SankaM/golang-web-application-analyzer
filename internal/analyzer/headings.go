package analyzer

import "golang.org/x/net/html"

var headingTags = map[string]bool{
	"h1": true, "h2": true, "h3": true,
	"h4": true, "h5": true, "h6": true,
}

// CountHeadings walks the parsed HTML tree and returns a map of heading tag
// names to their occurrence counts. Only tags with count > 0 are included.
func CountHeadings(doc *html.Node) map[string]int {
	counts := make(map[string]int)
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && headingTags[n.Data] {
			counts[n.Data]++
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return counts
}
