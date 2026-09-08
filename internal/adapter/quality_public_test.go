package adapter

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func TestNativeQualityPublicAndGraduationCommands(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/http/quality.example/findmm.jsp":
			writer.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = writer.Write([]byte(`<html><title>找回密码</title><body>公开页面</body></html>`))
		case "/http/quality.example/Logon.do":
			writer.Header().Set("Content-Type", "text/plain")
			_, _ = writer.Write([]byte("操作成功"))
		case "/http/quality.example/jsxsd/framework/xsMain.jsp":
			writer.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = writer.Write([]byte(`<a onclick="towptjbs('https://oauth.fanyu.com/sso/cas/10536/1004')">毕业设计</a>`))
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	root := t.TempDir()
	t.Setenv("CSUST_VPN_BASE_URL", server.URL)
	t.Setenv("CSUST_QUALITY_PREFIX", "/http/quality.example")
	t.Setenv("CSUST_VPN_COOKIE_FILE", filepath.Join(root, "vpn.cookies"))
	t.Setenv("CSUST_VPN_SESSION_FILE", filepath.Join(root, "vpn.json"))

	handled, stdout, _, code, err := (NativeSite{}).Run(context.Background(), []string{"quality", "public", "get", "--path", "/findmm.jsp", "--json"}, true)
	if err != nil || !handled || code != 0 {
		t.Fatalf("public get: handled=%v code=%d err=%v output=%s", handled, code, err, stdout)
	}
	var payload map[string]any
	if err := json.Unmarshal(stdout, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["response"].(map[string]any)["title"] != "找回密码" {
		t.Fatalf("unexpected public page: %#v", payload)
	}

	handled, stdout, _, code, err = (NativeSite{}).Run(context.Background(), []string{"quality", "public", "post", "--path", "/Logon.do", "--yes", "--json"}, true)
	if err != nil || !handled || code != 0 {
		t.Fatalf("public post: handled=%v code=%d err=%v output=%s", handled, code, err, stdout)
	}
	payload = map[string]any{}
	if err := json.Unmarshal(stdout, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["confirmed"] != true || payload["submitted"] != true {
		t.Fatalf("unexpected public post: %#v", payload)
	}

	handled, stdout, _, code, err = (NativeSite{}).Run(context.Background(), []string{"quality", "graduation-design", "--json"}, true)
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
