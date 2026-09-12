package adapter

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func TestNativeQualityGraduationStatusAndLogout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
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

	handled, stdout, _, code, err := (NativeSite{}).Run(context.Background(), []string{"quality", "graduation-design", "--json"}, true)
	if err != nil || !handled || code != 0 {
		t.Fatalf("graduation: handled=%v code=%d err=%v output=%s", handled, code, err, stdout)
	}
	var payload map[string]any
	if err := json.Unmarshal(stdout, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["external"] != true || payload["url"] != "https://oauth.fanyu.com/sso/cas/10536/1004" {
		t.Fatalf("unexpected graduation: %#v", payload)
	}

	handled, stdout, _, code, err = (NativeSite{}).Run(context.Background(), []string{"quality", "status", "--json"}, true)
	if err != nil || !handled || code != 0 {
		t.Fatalf("status without session: handled=%v code=%d err=%v output=%s", handled, code, err, stdout)
	}
	payload = map[string]any{}
	if err := json.Unmarshal(stdout, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["logged_in"] != false {
		t.Fatalf("unexpected quality status: %#v", payload)
	}

	handled, stdout, _, code, err = (NativeSite{}).Run(context.Background(), []string{"quality", "logout", "--json"}, true)
	if err != nil || !handled || code != 0 {
		t.Fatalf("logout without session: handled=%v code=%d err=%v output=%s", handled, code, err, stdout)
	}
	payload = map[string]any{}
	if err := json.Unmarshal(stdout, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["logged_out"] != true || payload["confirmed"] != true {
		t.Fatalf("unexpected quality logout: %#v", payload)
	}
}
