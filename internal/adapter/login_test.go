package adapter

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNativeLocalLoginPersistsOnlyConfirmedSession(t *testing.T) {
	temp := t.TempDir()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/verifycode.servlet":
			writer.Header().Set("Content-Type", "image/png")
			_, _ = writer.Write([]byte("captcha"))
		case "/Logon.do":
			if request.URL.Query().Get("flag") == "sess" {
				_, _ = writer.Write([]byte(strings.Repeat("a", 20) + "#" + strings.Repeat("0", 20)))
				return
			}
			http.SetCookie(writer, &http.Cookie{Name: "AUTH", Value: "1", Path: "/"})
			_, _ = writer.Write([]byte("登录成功"))
		case academicProbePath:
			if _, err := request.Cookie("AUTH"); err != nil {
				http.Error(writer, "unauthorized", http.StatusUnauthorized)
				return
			}
			writer.Header().Set("Content-Type", "text/html")
			_, _ = writer.Write([]byte(`<table id="kbtable"><tr><th>星期一</th></tr></table>`))
		default:
			_, _ = writer.Write([]byte(`<form id="loginForm"><input name="userAccount"><input name="userPassword" type="password"></form>`))
		}
	}))
	defer server.Close()
	cookieFile := filepath.Join(temp, "cookies.txt")
	t.Setenv("CSUST_BASE_URL", server.URL)
	t.Setenv("CSUST_COOKIE_FILE", cookieFile)
	t.Setenv("CSUST_USERNAME", "student")
	t.Setenv("CSUST_PASSWORD", "password")
	result, err := (NativeSite{}).loginAcademic(context.Background(), loginOptions{auth: "local", captcha: "good"})
	if err != nil {
		t.Fatal(err)
	}
	if result["auth"] != "local" || result["confirmed"] != true {
		t.Fatalf("unexpected login result: %#v", result)
	}
	if _, statErr := os.Stat(cookieFile); statErr != nil {
		t.Fatal(statErr)
	}
	if mode := os.FileMode(mustFileMode(t, cookieFile)); mode.Perm() != 0o600 {
		t.Fatalf("cookie mode is %o", mode.Perm())
	}
}

func mustFileMode(t *testing.T, path string) os.FileMode {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info.Mode()
}
