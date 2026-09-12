package adapter

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestLegacyHighwayJournalPages(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		switch {
		case request.URL.Path == "/journal/vol43/iss1/88":
			_, _ = fmt.Fprint(writer, `<html><body><h1>Paper</h1><a href="/cgi/viewcontent.cgi?article=1278&amp;context=journal">Download</a></body></html>`)
		case request.URL.Path == "/do/search/":
			if request.URL.Query().Get("q") != "bridge" {
				t.Fatalf("legacy journal search query was not mapped: %s", request.URL.RawQuery)
			}
			_, _ = fmt.Fprint(writer, `<html><body>search result</body></html>`)
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	t.Setenv("CSUST_BASE_URL", server.URL)
	t.Setenv("CSUST_COOKIE_FILE", filepath.Join(t.TempDir(), "cookies.txt"))

	article := runIssueJSON(t, "journal", "article", "--journal", "highway-legacy", "--volume", "43", "--issue", "1", "--article", "88")
	links, ok := article["links"].(map[string]any)
	if !ok || !strings.Contains(fmt.Sprint(links["download"]), "article=1278") || article["volume"] != float64(43) {
		t.Fatalf("legacy article fields or download link were not mapped: %#v", article)
	}
	search := runIssueJSON(t, "journal", "search", "--journal", "highway-legacy", "--query", "bridge")
	if search["confirmed"] != true || search["query"] != "bridge" {
		t.Fatalf("legacy journal search was not confirmed: %#v", search)
	}
}
