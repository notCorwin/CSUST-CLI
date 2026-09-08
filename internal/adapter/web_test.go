package adapter

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func TestParseWebRunAlias(t *testing.T) {
	parsed, err := parseWebCommand([]string{"web", "run", "--path", "/jsxsd/example"})
	if err != nil || parsed.kind != "action" {
		t.Fatalf("unexpected web run alias: %#v, %v", parsed, err)
	}
	parsed, err = parseWebCommand([]string{"routes", "--path", "/jsxsd/menu"})
	if err != nil || parsed.kind != "routes" || parsed.path != "/jsxsd/menu" {
		t.Fatalf("unexpected routes alias: %#v, %v", parsed, err)
	}
}

func TestNativeWebRoutesNameAndGraduation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		switch request.URL.Path {
		case "/jsxsd/framework/xsMain.jsp":
			_, _ = writer.Write([]byte(`<html><title>首页</title><body><a href="/jsxsd/custom.do">特殊入口</a><a onclick="towptjbs('https://oauth.fanyu.com/sso/cas/10536/1004')">毕业设计</a></body></html>`))
		case "/jsxsd/custom.do":
			_, _ = writer.Write([]byte(`<html><title>特殊页面</title><body>ok</body></html>`))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	t.Setenv("CSUST_BASE_URL", server.URL)
	t.Setenv("CSUST_COOKIE_FILE", filepath.Join(t.TempDir(), "cookies.txt"))

	handled, stdout, _, code, err := (NativeSite{}).Run(context.Background(), []string{"web", "routes", "--json"}, true)
	if err != nil || !handled || code != 0 {
		t.Fatalf("routes: handled=%v code=%d err=%v output=%s", handled, code, err, stdout)
	}
	var payload map[string]any
	if err := json.Unmarshal(stdout, &payload); err != nil {
		t.Fatal(err)
	}
	discovered := payload["discovered"].([]any)
	if payload["url"] == nil || len(discovered) != 1 {
		t.Fatalf("unexpected routes: %#v", payload)
	}

	handled, stdout, _, code, err = (NativeSite{}).Run(context.Background(), []string{"web", "get", "--name", "特殊入口", "--json"}, true)
	if err != nil || !handled || code != 0 {
		t.Fatalf("dynamic name: handled=%v code=%d err=%v output=%s", handled, code, err, stdout)
	}
	payload = map[string]any{}
	if err := json.Unmarshal(stdout, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["response"].(map[string]any)["title"] != "特殊页面" {
		t.Fatalf("unexpected named page: %#v", payload)
	}

	handled, stdout, _, code, err = (NativeSite{}).Run(context.Background(), []string{"web", "graduation-design", "--json"}, true)
	if err != nil || !handled || code != 0 {
		t.Fatalf("graduation: handled=%v code=%d err=%v output=%s", handled, code, err, stdout)
	}
	payload = map[string]any{}
	if err := json.Unmarshal(stdout, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["external"] != true || payload["url"] != "https://oauth.fanyu.com/sso/cas/10536/1004" {
		t.Fatalf("unexpected graduation: %#v", payload)
	}
}
