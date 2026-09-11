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
	var encoded, account, password, captcha string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/jsxsd/":
			_, _ = writer.Write([]byte(`<form id="Form1" action="/jsxsd/xk/LoginToXk" method="post"><input name="userAccount"><input name="userPassword" type="password"><input name="RANDOMCODE"><input name="encoded"></form>`))
		case "/jsxsd/verifycode.servlet":
			writer.Header().Set("Content-Type", "image/png")
			_, _ = writer.Write([]byte("captcha"))
		case "/jsxsd/xk/LoginToXk":
			_ = request.ParseForm()
			account, password, captcha, encoded = request.Form.Get("userAccount"), request.Form.Get("userPassword"), request.Form.Get("RANDOMCODE"), request.Form.Get("encoded")
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
			http.NotFound(writer, request)
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
	if account != "student" || password != "password" || captcha != "good" || encoded != "c3R1ZGVudA==%%%cGFzc3dvcmQ=" {
		t.Fatalf("unexpected selection login payload: account=%q password=%q captcha=%q encoded=%q", account, password, captcha, encoded)
	}
	if _, statErr := os.Stat(cookieFile); statErr != nil {
		t.Fatal(statErr)
	}
	if mode := os.FileMode(mustFileMode(t, cookieFile)); mode.Perm() != 0o600 {
		t.Fatalf("cookie mode is %o", mode.Perm())
	}
}

func TestNativeLogoutVerifiesRemoteSessionAndClearsCookie(t *testing.T) {
	logoutCalled := false
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/jsxsd/xk/LoginToXk" && request.URL.Query().Get("method") == "exit" {
			logoutCalled = true
			_, _ = writer.Write([]byte("退出成功"))
			return
		}
		http.NotFound(writer, request)
	}))
	defer server.Close()
	cookieFile := filepath.Join(t.TempDir(), "cookies.txt")
	if err := os.WriteFile(cookieFile, []byte("# Netscape HTTP Cookie File\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CSUST_BASE_URL", server.URL)
	t.Setenv("CSUST_COOKIE_FILE", cookieFile)
	handled, stdout, _, code, err := (NativeSite{}).Run(context.Background(), []string{"logout", "--json"}, true)
	if err != nil || !handled || code != 0 {
		t.Fatalf("logout: handled=%v code=%d err=%v output=%s", handled, code, err, stdout)
	}
	if !logoutCalled || strings.Contains(string(stdout), `"confirmed":false`) {
		t.Fatalf("logout was not verified: called=%v output=%s", logoutCalled, stdout)
	}
	if _, err := os.Stat(cookieFile); !os.IsNotExist(err) {
		t.Fatalf("cookie file was not removed: %v", err)
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
