package adapter

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func TestContinuingPlatformLoginProtocol(t *testing.T) {
	loggedIn := false
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/":
			writer.Header().Set("Content-Type", "text/html; charset=utf-8")
			if loggedIn {
				_, _ = fmt.Fprint(writer, `<html><body>继续教育首页</body></html>`)
			} else {
				_, _ = fmt.Fprint(writer, `<form id="formLoginMaster"><input type="hidden" name="__VIEWSTATE" value="view"><input type="hidden" name="__EVENTVALIDATION" value="event"><input id="textBoxAuthorityLoginName" name="ctl00$contentPlaceHolderLogin$textBoxAuthorityLoginName"><input id="textBoxAuthorityPassword" name="ctl00$contentPlaceHolderLogin$textBoxAuthorityPassword"><input id="buttonAuthorityLogin" name="ctl00$contentPlaceHolderLogin$buttonAuthorityLogin"></form>`)
			}
		case request.Method == http.MethodPost && request.URL.Path == "/Login/Login.aspx":
			if err := request.ParseForm(); err != nil || request.Form.Get("ctl00$contentPlaceHolderLogin$textBoxAuthorityLoginName") != "account" || request.Form.Get("ctl00$contentPlaceHolderLogin$textBoxAuthorityPassword") != "secret" || request.Form.Get("ctl00$contentPlaceHolderLogin$buttonAuthorityLogin") == "" || request.Form.Get("__VIEWSTATE") != "view" {
				t.Fatalf("continuing-platform login fields were not mapped: %v", request.Form)
			}
			loggedIn = true
			_, _ = fmt.Fprint(writer, `<html><body>继续教育首页</body></html>`)
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	t.Setenv("CSUST_BASE_URL", server.URL)
	t.Setenv("CSUST_COOKIE_FILE", filepath.Join(t.TempDir(), "cookies.txt"))

	login := runIssueJSON(t, "continuing-platform", "login", "--role", "authority", "--username", "account", "--password", "secret")
	if login["confirmed"] != true || login["role"] != "authority" || login["username"] != "account" {
		t.Fatalf("continuing-platform login was not confirmed: %#v", login)
	}
	logout := runIssueJSON(t, "continuing-platform", "logout", "--yes")
	if logout["confirmed"] != true || logout["logged_out"] != true {
		t.Fatalf("continuing-platform logout was not confirmed: %#v", logout)
	}
}
