package izmac

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

/*
Every link of the documentation leads somewhere: a file of the repository
named by a Markdown link or an image, and the heading of a page named after a
#. Links to the web are not followed. What is in code, fenced or inline, is
not a link.
*/
func TestTheLinksOfTheDocumentationLeadSomewhere(t *testing.T) {
	pages := map[string]string{}
	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && path != "." && strings.HasPrefix(d.Name(), ".") {
			return filepath.SkipDir
		}
		if !d.IsDir() && strings.HasSuffix(path, ".md") {
			text, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			pages[path] = string(text)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	code := regexp.MustCompile("(?s)```.*?```|`[^`\n]*`")
	links := regexp.MustCompile(`\]\(([^)\s]+)\)|(?:src|href)="([^"]+)"`)
	for page, text := range pages {
		for _, m := range links.FindAllStringSubmatch(code.ReplaceAllString(text, ""), -1) {
			link := m[1] + m[2]
			if strings.Contains(link, "://") || strings.HasPrefix(link, "mailto:") {
				continue
			}
			target, anchor, _ := strings.Cut(link, "#")
			path := page
			if target != "" {
				path = filepath.Join(filepath.Dir(page), target)
			}
			if _, err := os.Stat(path); err != nil {
				t.Errorf("%v links to %v, which is not there", page, link)
				continue
			}
			if anchor != "" && !hasHeading(pages[path], anchor) {
				t.Errorf("%v links to %v, which has no heading %v", page, link, anchor)
			}
		}
	}
}

// hasHeading tells whether a page has a heading a link names with an anchor,
// as GitHub makes them: in lower case, with spaces as dashes and the
// punctuation left out
func hasHeading(page string, anchor string) bool {
	punctuation := regexp.MustCompile(`[^\p{L}\p{N}\- _]`)
	for _, line := range strings.Split(page, "\n") {
		heading := strings.TrimLeft(line, "#")
		if heading == line || !strings.HasPrefix(heading, " ") {
			continue
		}
		slug := strings.ToLower(strings.TrimSpace(heading))
		slug = strings.ReplaceAll(punctuation.ReplaceAllString(slug, ""), " ", "-")
		if slug == anchor {
			return true
		}
	}
	return false
}
