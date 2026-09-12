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

func TestInternalSiteRequestResolution(t *testing.T) {
	request := siteRequest{Service: "app", Path: "/api", Method: "POST", JSON: map[string]any{"page": 1}, HasJSON: true}
	target, _, resolveErr := resolveSite(request)
	if resolveErr != nil {
		t.Fatal(resolveErr)
	}
	if target.String() != "http://app.csust.edu.cn:8087/api" {
		t.Fatalf("unexpected target: %s", target)
	}

	logout, _, resolveErr := resolveSite(siteRequest{Service: "official", Path: "/logout"})
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
	mservice, _ := url.Parse("https://mservice.csust.edu.cn/transfer")
	casTransfer, _ := url.Parse("https://castransfer.chaoxing.com/transfer")
	if !safeSiteSSORedirect(mservice, mservice, casTransfer) || !safeSiteSSORedirect(mservice, casTransfer, auth) {
		t.Fatal("expected the fixed mservice CAS transfer bridge")
	}
	otherTransfer, _ := url.Parse("https://evil.example/transfer")
	if safeSiteSSORedirect(mservice, mservice, otherTransfer) || safeSiteSSORedirect(mservice, otherTransfer, auth) {
		t.Fatal("unexpected arbitrary transfer bridge")
	}
	httpTransfer, _ := url.Parse("http://castransfer.chaoxing.com/transfer")
	if safeSiteSSORedirect(mservice, mservice, httpTransfer) {
		t.Fatal("unexpected HTTPS downgrade through transfer bridge")
	}
	mooc, _ := url.Parse("http://mooc.csust.edu.cn/")
	moocEntry, _ := url.Parse("http://mooc.csust.edu.cn/fyportal/tomoocportal?courseid=1&ckenc=redacted")
	moocCourse, _ := url.Parse("http://mooc1.chaoxing.com/mooc-ans/course/portal/opaque")
	if !safeSiteKnownRedirect(mooc, moocEntry, moocCourse) {
		t.Fatal("expected the fixed MOOC Chaoxing course redirect")
	}
	otherCourse, _ := url.Parse("http://evil.example/mooc-ans/course/portal/opaque")
	if safeSiteKnownRedirect(mooc, moocEntry, otherCourse) {
		t.Fatal("unexpected arbitrary MOOC redirect accepted")
	}
	v1, _ := url.Parse("https://v1.chaoxing.com/appInter/openPcApp?mappId=19933721")
	chaoxingAuth, _ := url.Parse("https://auth.chaoxing.com/connect/oauth2/authorize?code=redacted")
	office, _ := url.Parse("https://office.csust.edu.cn/front/web/approve/apps/forms/fore/apply?id=266714")
	if !safeSiteSSORedirect(mservice, v1, chaoxingAuth) || !safeSiteSSORedirect(mservice, chaoxingAuth, office) {
		t.Fatal("service-hall form handoff redirect was rejected")
	}
	if safeSiteSSORedirect(mservice, v1, office) || safeSiteSSORedirect(mservice, chaoxingAuth, otherTransfer) {
		t.Fatal("unexpected service-hall form handoff redirect accepted")
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

func TestGenericCommandSurfaceRemoved(t *testing.T) {
	commands := [][]string{
		{"site", "discover"},
		{"site", "scripts"},
		{"site", "get"},
		{"site", "request"},
		{"web", "routes"},
		{"web", "get"},
		{"teaching", "get"},
		{"teaching", "service"},
		{"teaching", "courses", "--raw"},
		{"quality", "public"},
		{"quality", "evaluation", "form", "--path", "/jsxsd/xspj/xspj_course.do"},
		{"vpn", "api"},
		{"vpn", "page"},
		{"vpn", "catalog"},
	}
	for _, command := range commands {
		handled, stdout, stderr, code, err := (NativeSite{}).Run(context.Background(), command, true)
		if err != nil || !handled || code != 2 || len(stderr) != 0 {
			t.Fatalf("removed command %v: handled=%v code=%d err=%v stderr=%q stdout=%q", command, handled, code, err, stderr, stdout)
		}
		var payload map[string]any
		if err := json.Unmarshal(stdout, &payload); err != nil {
			t.Fatalf("removed command %v returned invalid JSON: %v", command, err)
		}
		if payload["code"] == nil || payload["error"] == nil {
			t.Fatalf("removed command %v did not return an error envelope: %#v", command, payload)
		}
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
	if state, known, decoded := businessStateForRequest([]byte(`{"state":"error","data":"1004"}`), "text/html; charset=utf-8", false); state || !known || decoded == nil {
		t.Fatalf("JSON body mislabeled as HTML was not decoded: %v %v %#v", state, known, decoded)
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

func TestNativeSiteRunReturnsContractReadyErrors(t *testing.T) {
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
	if result["code"] != "unknown_command" {
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
