package adapter

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSiteRequestParsingAndResolution(t *testing.T) {
	request, parseErr := parseSiteRequest([]string{
		"--service=app",
		"--path=/api",
		"--method=post",
		`--data-json={"page":1}`,
		"--header=X-Test=value",
		"--yes",
	})
	if parseErr != nil {
		t.Fatal(parseErr)
	}
	if request.Method != "POST" || !request.HasJSON || request.Service != "app" {
		t.Fatalf("unexpected request: %#v", request)
	}
	target, _, resolveErr := resolveSite(request)
	if resolveErr != nil {
		t.Fatal(resolveErr)
	}
	if target.String() != "http://app.csust.edu.cn:8087/api" {
		t.Fatalf("unexpected target: %s", target)
	}

	get, parseErr := parseSiteRequest([]string{"--service", "official", "--path", "/logout"})
	if parseErr != nil {
		t.Fatal(parseErr)
	}
	logout, _, resolveErr := resolveSite(get)
	if resolveErr != nil {
		t.Fatal(resolveErr)
	}
	if !sideEffectSiteURL(logout) {
		t.Fatal("expected logout path to require mutation confirmation")
	}
}

func TestSiteRequestRejectsUnsafeTargets(t *testing.T) {
	for _, request := range []siteRequest{
		{Service: "example.com", Path: "/"},
		{Service: "official", Path: "/%2e%2e/private"},
		{Service: "official", Path: "https://example.com/"},
	} {
		if _, _, resolveErr := resolveSite(request); resolveErr == nil {
			t.Fatalf("expected target rejection: %#v", request)
		}
	}
	target, _ := url.Parse("https://official.csust.edu.cn/api?token=secret-value&name=public")
	redacted := safeSiteURL(target)
	if strings.Contains(redacted, "secret-value") || !strings.Contains(redacted, "redacted") {
		t.Fatalf("sensitive URL was not redacted: %s", redacted)
	}
}

func TestSiteRedirectPolicy(t *testing.T) {
	base, _ := url.Parse("https://ehall.csust.edu.cn/")
	service, _ := url.Parse("https://ehall.csust.edu.cn/login")
	auth, _ := url.Parse("https://authserver.csust.edu.cn/authserver/login")
	other, _ := url.Parse("https://evil.example/")
	if !safeSiteSSORedirect(base, service, auth) || safeSiteSSORedirect(base, service, other) {
		t.Fatal("unexpected service to SSO redirect policy")
	}
	if !safeSiteSSORedirect(base, auth, service) {
		t.Fatal("expected SSO return redirect")
	}
	downgrade, _ := url.Parse("http://ehall.csust.edu.cn/")
	if safeSiteSSORedirect(base, auth, downgrade) {
		t.Fatal("unexpected SSO HTTPS downgrade")
	}
	handoff, _ := url.Parse("http://authserver.csust.edu.cn/authserver/login")
	normalizeSSOHTTPRedirect(service, handoff, base)
	if handoff.Scheme != "https" {
		t.Fatalf("expected HTTPS SSO handoff, got %s", handoff)
	}
}

func TestEHallSSOUsesCurrentPortalCallback(t *testing.T) {
	target, _ := url.Parse("https://ehall.csust.edu.cn/")
	callback := ssoServiceTarget(target)
	want := "https://ehall.csust.edu.cn/login?portalService=https%3A%2F%2Fehall.csust.edu.cn%2Findex.html%23%2F"
	if callback.String() != want {
		t.Fatalf("unexpected eHall SSO callback: got %s want %s", callback, want)
	}
	other, _ := url.Parse("https://www.csust.edu.cn/")
	if ssoServiceTarget(other) != other {
		t.Fatal("ordinary SSO service target should not be rewritten")
	}
}

func TestSiteLoginParsesPasswordAndPasswordlessModes(t *testing.T) {
	password, err := parseSiteCommand([]string{"login", "--service", "ehall", "--auth", "sso", "--password-stdin"})
	if err != nil || !password.login.passwordStdin {
		t.Fatalf("site SSO password stdin was not parsed: %#v %v", password, err)
	}
	qr, err := parseSiteCommand([]string{"login", "--service", "ehall", "--auth", "qr", "--qr-image", "./login.png"})
	if err != nil || qr.login.auth != "qr" || qr.login.qrImage != "./login.png" {
		t.Fatalf("site QR login was not parsed: %#v %v", qr, err)
	}
	dynamic, err := parseSiteCommand([]string{"login", "--service", "ehall", "--auth", "dynamic", "--mobile", "13800138000", "--send-code", "--yes"})
	if err != nil || dynamic.login.auth != "dynamic" || dynamic.login.mobile != "13800138000" || !dynamic.login.sendCode || !dynamic.request.Yes {
		t.Fatalf("site dynamic login was not parsed: %#v %v", dynamic, err)
	}
}

func TestSiteBusinessStateAndBinaryResponse(t *testing.T) {
	if state, known, decoded := businessState([]byte(`{"success":true,"data":{"value":1}}`), "application/json"); !state || !known || decoded == nil {
		t.Fatalf("unexpected JSON success state: %v %v %#v", state, known, decoded)
	}
	if state, known, _ := businessState([]byte(`{"code":"500","message":"failed"}`), "application/json"); state || !known {
		t.Fatalf("unexpected JSON failure state: %v %v", state, known)
	}
	if state, known, _ := businessState([]byte("<html><script>var error = true</script></html>"), "text/html"); state || known {
		t.Fatalf("HTML shell should remain unverified: %v %v", state, known)
	}
	if state, known, _ := businessState([]byte("操作成功"), "text/plain"); !state || !known {
		t.Fatalf("plain success was not recognized: %v %v", state, known)
	}
	if state, known, _ := businessState([]byte("预约成功"), "text/plain"); !state || !known {
		t.Fatalf("appointment success was not recognized: %v %v", state, known)
	}
	if state, known, _ := businessState([]byte(`<script>alert('密码重置成功!')</script>`), "text/html"); !state || !known {
		t.Fatalf("password reset success was not recognized: %v %v", state, known)
	}
	if state, known, _ := businessState([]byte(`<script>alert('证件号码或考生编号不存在!')</script>`), "text/html"); state || !known {
		t.Fatalf("password reset rejection was not recognized: %v %v", state, known)
	}
	if !isBinarySiteResponse(&http.Response{Header: http.Header{"Content-Type": []string{"application/pdf"}}}) {
		t.Fatal("PDF should require --output")
	}
	if isBinarySiteResponse(&http.Response{Header: http.Header{"Content-Type": []string{"text/html; charset=utf-8"}}}) {
		t.Fatal("HTML should be printable")
	}
	payload := responsePayload(&http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"text/html"}}}, []byte("<html></html>"), nil)
	if payload["confidence"] != "low" || payload["confidence_evidence"] == nil {
		t.Fatalf("HTML response lacks confidence evidence: %#v", payload)
	}
}

func TestSiteExecuteHonorsExplicitMutationForGET(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, _ = writer.Write([]byte("result unknown"))
	}))
	defer server.Close()
	target, _ := url.Parse(server.URL + "/operation")
	_, err := (NativeSite{}).execute(context.Background(), siteRequest{
		Target: target, Method: http.MethodGet, CookieFile: filepath.Join(t.TempDir(), "cookies.txt"),
		ReadOnly: false, Yes: true, mutating: true,
	})
	if err == nil || err.Code != "mutation_unverified" {
		t.Fatalf("expected explicit GET mutation to require success evidence, got %#v", err)
	}
}

func TestNativeSiteRunReturnsContractReadyParseErrors(t *testing.T) {
	handled, stdout, stderr, code, err := (NativeSite{}).Run(context.Background(), []string{
		"site", "request", "--service", "official", "--method", "POST", "--data", "x=y", "--json",
	}, true)
	if err != nil || !handled || code != 2 || len(stderr) != 0 {
		t.Fatalf("unexpected run result: handled=%v code=%d err=%v stderr=%q", handled, code, err, stderr)
	}
	var result map[string]any
	if err := json.Unmarshal(stdout, &result); err != nil {
		t.Fatal(err)
	}
	if result["code"] != "confirmation_required" {
		t.Fatalf("unexpected error: %#v", result)
	}
	if handled, stdout, _, code, _ := (NativeSite{}).Run(context.Background(), []string{"site", "request", "--help"}, false); !handled || code != 0 || !strings.Contains(string(stdout), "Go 原生协议 CLI") {
		t.Fatalf("help should be handled by Go: handled=%v code=%d output=%q", handled, code, stdout)
	}
}

func TestAtomicSiteWrite(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "response.bin")
	if writeErr := atomicWrite(filename, []byte("ok")); writeErr != nil {
		t.Fatal(writeErr)
	}
	content, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(content)) != "ok" {
		t.Fatalf("unexpected output: %q", content)
	}
}
