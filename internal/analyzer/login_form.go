package analyzer

import (
	"strings"

	"golang.org/x/net/html"
)

// loginIndicatorPhrases are keywords that suggest a login/auth page when found
// in title, meta, links, or URL.
var loginIndicatorPhrases = []string{
	"login", "log in", "sign in", "signin", "sign-in", "log-in",
	"authenticate", "forgot password", "reset password", "member",
}

// HasLoginForm reports whether the document contains a login form. It checks:
// 1. An actual <form> with password input + submit (definitive)
// 2. Login-related signals in static HTML (title, meta, links, URL) for JS-rendered pages
func HasLoginForm(doc *html.Node, targetURL string) bool {
	if hasActualLoginForm(doc) {
		return true
	}
	return hasLoginPageIndicators(doc, targetURL)
}

func hasActualLoginForm(doc *html.Node) bool {
	var check func(*html.Node) bool
	check = func(n *html.Node) bool {
		if n.Type == html.ElementNode && n.Data == "form" {
			if isLoginForm(n) {
				return true
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if check(c) {
				return true
			}
		}
		return false
	}
	return check(doc)
}

// hasLoginPageIndicators returns true if the static HTML suggests a login/auth
// page (e.g. SPA with form rendered by JS). Checks title, meta tags, links, and URL.
func hasLoginPageIndicators(doc *html.Node, targetURL string) bool {
	urlLower := strings.ToLower(targetURL)
	var score int

	// URL path or host contains login-related terms
	for _, p := range loginIndicatorPhrases {
		if strings.Contains(urlLower, p) {
			score += 2
			break
		}
	}

	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			switch n.Data {
			case "title":
				t := strings.ToLower(textContent(n))
				for _, p := range loginIndicatorPhrases {
					if strings.Contains(t, p) {
						score += 2
						return
					}
				}
			case "meta":
				content := attrVal(n, "content") + " " + attrVal(n, "property") + " " + attrVal(n, "name")
				contentLower := strings.ToLower(content)
				for _, p := range loginIndicatorPhrases {
					if strings.Contains(contentLower, p) {
						score += 1
						return
					}
				}
			case "a":
				href := strings.ToLower(attrVal(n, "href"))
				text := strings.ToLower(textContent(n))
				combined := href + " " + text
				for _, p := range loginIndicatorPhrases {
					if strings.Contains(combined, p) {
						score += 1
						return
					}
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)

	return score >= 2
}

func textContent(n *html.Node) string {
	var b strings.Builder
	var collect func(*html.Node)
	collect = func(node *html.Node) {
		if node.Type == html.TextNode {
			b.WriteString(node.Data)
		}
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			collect(c)
		}
	}
	collect(n)
	return b.String()
}

// isLoginForm inspects a single <form> node for the presence of both a
// password field and a submit control.
func isLoginForm(form *html.Node) bool {
	var hasPassword, hasSubmit bool
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			switch n.Data {
			case "input":
				t := attrVal(n, "type")
				if t == "password" {
					hasPassword = true
				}
				if t == "submit" {
					hasSubmit = true
				}
			case "button":
				t := attrVal(n, "type")
				// A <button> with no type or type="submit" acts as a submit button.
				if t == "" || t == "submit" {
					hasSubmit = true
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(form)
	return hasPassword && hasSubmit
}

// attrVal returns the value of the named attribute on a node, or "".
func attrVal(n *html.Node, name string) string {
	for _, a := range n.Attr {
		if a.Key == name {
			return a.Val
		}
	}
	return ""
}
