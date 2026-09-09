package adapter

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func runIssueJSON(t *testing.T, args ...string) map[string]any {
	t.Helper()
	handled, stdout, stderr, code, err := (NativeSite{}).Run(context.Background(), append(args, "--json"), true)
	if err != nil || !handled || code != 0 {
		t.Fatalf("command failed: handled=%v code=%d err=%v stdout=%s stderr=%s", handled, code, err, stdout, stderr)
	}
	var result map[string]any
	if err := json.Unmarshal(stdout, &result); err != nil {
		t.Fatalf("invalid JSON output %q: %v", stdout, err)
	}
	return result
}

func TestSiteRedactionKeepsInternalPageActionsUsable(t *testing.T) {
	const rawToken = "raw-token-12345678901234567890"
	const password = "password-secret-12345678901234567890"
	const csrf = "csrf-secret-12345678901234567890"
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		switch request.URL.Path {
		case "/":
			_, _ = fmt.Fprintf(writer, `<html><head><meta name="csrf-token" content="%s"></head><body><div data-token="%s"></div><input type="password" name="password" value="%s"><input type="hidden" name="csrf" value="%s"><textarea name="token">%s</textarea><script>csrfToken = "%s"</script><a href="/next?token=%s">继续</a></body></html>`, csrf, csrf, password, csrf, csrf, csrf, rawToken)
		case "/next":
			if request.URL.Query().Get("token") != rawToken {
				writer.WriteHeader(http.StatusBadRequest)
				_, _ = writer.Write([]byte("token invalid"))
				return
			}
			writer.Header().Set("Content-Type", "text/plain; charset=utf-8")
			_, _ = writer.Write([]byte("操作成功"))
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	t.Setenv("CSUST_BASE_URL", server.URL)
	t.Setenv("CSUST_COOKIE_FILE", filepath.Join(t.TempDir(), "cookies.txt"))

	requestResult := runIssueJSON(t, "site", "request", "--service", "official", "--path", "/")
	response := requestResult["response"].(map[string]any)
	publicBody := response["body"].(string)
	for _, secret := range []string{rawToken, password, csrf} {
		if strings.Contains(string(mustMarshalIssue(requestResult)), secret) || strings.Contains(publicBody, secret) {
			t.Fatalf("secret leaked in public response: %s", secret)
		}
	}
	for _, key := range []string{"raw_url", "body_internal"} {
		if _, exists := response[key]; exists {
			t.Fatalf("internal response field leaked: %s", key)
		}
	}

	pageResult := runIssueJSON(t, "site", "get", "--service", "official", "--path", "/")
	page := pageResult["response"].(map[string]any)
	links := page["links"].([]any)
	if len(links) != 1 {
		t.Fatalf("unexpected page links: %#v", links)
	}
	link := links[0].(map[string]any)
	if strings.Contains(fmt.Sprint(link["href"]), rawToken) || strings.Contains(fmt.Sprint(link["path"]), rawToken) {
		t.Fatalf("page link was not redacted: %#v", link)
	}

	result := runIssueJSON(t, "site", "action", "--service", "official", "--index", "1", "--yes")
	if result["ok"] != true || result["confirmed"] != true {
		t.Fatalf("internal raw action was not executed and confirmed: %#v", result)
	}
	if text := redactSiteText("csrf=plain-secret token=plain-token", false); strings.Contains(text, "plain-secret") || strings.Contains(text, "plain-token") {
		t.Fatalf("plain-text secret was not redacted: %q", text)
	}
	errorOutput := string(errorJSON(&siteError{Code: "failed", Message: "password=error-secret", Details: map[string]any{
		"response": map[string]any{"body_internal": "csrf=error-secret", "password": "error-secret"},
	}}))
	if strings.Contains(errorOutput, "error-secret") || strings.Contains(errorOutput, "body_internal") {
		t.Fatalf("error output leaked internal response data: %s", errorOutput)
	}
}

func TestAdmissionNoticePasswordUsesPOSTBody(t *testing.T) {
	const password = "notice-password-secret"
	var method, rawQuery, body string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		method, rawQuery = request.Method, request.URL.RawQuery
		content, _ := io.ReadAll(request.Body)
		body = string(content)
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"code":200,"data":{"id":"notice-1"}}`))
	}))
	defer server.Close()
	t.Setenv("CSUST_BASE_URL", server.URL)
	t.Setenv("CSUST_COOKIE_FILE", filepath.Join(t.TempDir(), "cookies.txt"))

	result := runIssueJSON(t, "admission-notice", "query", "--id-card", "430000000000000000", "--password", password)
	if method != http.MethodPost || rawQuery != "" {
		t.Fatalf("notice query was not a body POST: method=%s query=%q", method, rawQuery)
	}
	if !strings.Contains(body, `"password":"`+password+`"`) {
		t.Fatalf("password missing from JSON body: %s", body)
	}
	if strings.Contains(string(mustMarshalIssue(result)), password) {
		t.Fatal("password leaked in command output")
	}
}

func TestAcademicGradesRefererUsesActualQueryPage(t *testing.T) {
	var mu sync.Mutex
	var referer string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/jsxsd/kscj/cjcx_query":
			_, _ = writer.Write([]byte(`<html><body>成绩查询</body></html>`))
		case request.Method == http.MethodPost && request.URL.Path == "/jsxsd/kscj/cjcx_list":
			mu.Lock()
			referer = request.Header.Get("Referer")
			mu.Unlock()
			_, _ = writer.Write([]byte(`<table id="dataList"><tr><th>序号</th><th>学期</th><th>课程</th><th>成绩</th><th>学分</th><th>备注</th></tr><tr><td>1</td><td>2026-1</td><td>数学</td><td>95</td><td>3</td><td></td></tr></table>`))
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	t.Setenv("CSUST_BASE_URL", server.URL)
	t.Setenv("CSUST_COOKIE_FILE", filepath.Join(t.TempDir(), "cookies.txt"))

	result := runIssueJSON(t, "grades", "--term", "2026-1")
	if items, ok := result["items"].([]any); !ok || len(items) != 1 {
		t.Fatalf("unexpected grades result: %#v", result)
	}
	mu.Lock()
	gotReferer := referer
	mu.Unlock()
	wantReferer := server.URL + "/jsxsd/kscj/cjcx_query"
	if gotReferer != wantReferer {
		t.Fatalf("unexpected grades Referer: got %q want %q", gotReferer, wantReferer)
	}
}

func TestGlobalSiteOverridesApplyToAllAdapters(t *testing.T) {
	base := "https://override.example.test"
	cookie := filepath.Join(t.TempDir(), "shared.cookies")
	t.Setenv("CSUST_BASE_URL", base)
	t.Setenv("CSUST_COOKIE_FILE", cookie)
	for _, service := range []string{"official", "onlinejudge", "graduate-admissions"} {
		target, gotCookie, err := resolveSite(siteRequest{Service: service, Path: "/probe"})
		if err != nil {
			t.Fatal(err)
		}
		if target.String() != base+"/probe" || gotCookie != cookie {
			t.Fatalf("global override missed for %s: target=%s cookie=%s", service, target, gotCookie)
		}
	}
	for _, invalid := range []string{"ftp://override.example.test", "http://user:password@override.example.test", "http://override.example.test/?token=secret"} {
		t.Setenv("CSUST_BASE_URL", invalid)
		if _, _, err := resolveSite(siteRequest{Service: "official", Path: "/probe"}); err == nil {
			t.Fatalf("unsafe base URL was accepted: %s", invalid)
		}
	}
}

func TestBusinessArgumentsRejectUnknownFlagsBeforeDispatch(t *testing.T) {
	for _, args := range [][]string{
		{"services", "--unknown"},
		{"onlinejudge", "problems", "--unknown"},
		{"archive", "status", "--system", "student", "--unknown"},
		{"student-record", "request", "--college", "c", "--major", "m", "--unknown"},
	} {
		if err := validateBusinessArgs(args); err == nil || err.Code != "invalid_argument" {
			t.Fatalf("unknown flag was accepted: %v", args)
		}
	}
	for _, args := range [][]string{
		{"onlinejudge", "tags", "--insecure"},
		{"archive", "status", "--system", "student", "--access-token", "token"},
		{"student-record", "request", "--college", "c", "--major", "m"},
	} {
		if err := validateBusinessArgs(args); err != nil {
			t.Fatalf("valid flag was rejected: %v: %v", args, err)
		}
	}
}

func TestBusinessLoginResponsesNeedExplicitEvidence(t *testing.T) {
	for _, body := range []string{"", `<html><body>temporary failure</body></html>`, `<html><body>error: upstream rejected</body></html>`} {
		if err := businessLoginFailure(body); err == nil {
			t.Fatalf("login response was treated as usable: %q", body)
		}
	}
	if err := businessLoginResponseFailure(map[string]any{"response": map[string]any{"json_internal": map[string]any{"code": float64(500), "message": "rejected"}}}); err == nil {
		t.Fatal("JSON login error was treated as success")
	}
	if err := businessLoginResponseFailure(map[string]any{"response": map[string]any{"body_internal": "<html><body>home</body></html>"}}); err != nil {
		t.Fatalf("ordinary login response should wait for the session probe: %v", err)
	}
}

func TestSessionWritesMergeConcurrentCallers(t *testing.T) {
	target, _ := url.Parse("http://session.example/")
	root := t.TempDir()
	cookiePath := filepath.Join(root, "cookies.txt")
	sessionPath := filepath.Join(root, "vpn.json")
	const count = 12
	var group sync.WaitGroup
	errors := make(chan error, count*2)
	for index := 0; index < count; index++ {
		index := index
		group.Add(1)
		go func() {
			defer group.Done()
			jar, err := cookiejar.New(nil)
			if err != nil {
				errors <- err
				return
			}
			jar.SetCookies(target, []*http.Cookie{{Name: fmt.Sprintf("cookie-%d", index), Value: "ok", Path: "/"}})
			if err := saveCookies(jar, cookiePath, target); err != nil {
				errors <- err
			}
			if err := saveVPNSession(sessionPath, map[string]any{fmt.Sprintf("session-%d", index): "ok"}); err != nil {
				errors <- err
			}
		}()
	}
	group.Wait()
	close(errors)
	for err := range errors {
		t.Fatal(err)
	}

	content, err := os.ReadFile(sessionPath)
	if err != nil {
		t.Fatal(err)
	}
	var session map[string]any
	if err := json.Unmarshal(content, &session); err != nil {
		t.Fatal(err)
	}
	for index := 0; index < count; index++ {
		if session[fmt.Sprintf("session-%d", index)] != "ok" {
			t.Fatalf("session update lost for %d: %#v", index, session)
		}
	}
	loaded, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := loadCookies(loaded, cookiePath, target); err != nil {
		t.Fatal(err)
	}
	seen := make(map[string]bool)
	for _, cookie := range loaded.Cookies(target) {
		seen[cookie.Name] = cookie.Value == "ok"
	}
	for index := 0; index < count; index++ {
		if !seen[fmt.Sprintf("cookie-%d", index)] {
			t.Fatalf("cookie update lost for %d: %#v", index, seen)
		}
	}
}

func TestCookiePersistenceKeepsPathVariantsAndDeletion(t *testing.T) {
	var seenRoot, seenChild string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/set":
			http.SetCookie(writer, &http.Cookie{Name: "SID", Value: "root", Path: "/"})
			http.SetCookie(writer, &http.Cookie{Name: "SID", Value: "child", Path: "/jsxsd"})
		case "/jsxsd/probe":
			seenChild = request.Header.Get("Cookie")
		case "/clear":
			http.SetCookie(writer, &http.Cookie{Name: "SID", MaxAge: -1, Path: "/"})
		case "/echo":
			seenRoot = request.Header.Get("Cookie")
		}
		_, _ = writer.Write([]byte("ok"))
	}))
	defer server.Close()
	cookieFile := filepath.Join(t.TempDir(), "cookies.txt")
	parseTarget := func(path string) *url.URL {
		target, err := url.Parse(server.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		return target
	}
	for _, path := range []string{"/set", "/jsxsd/probe", "/clear", "/echo"} {
		if _, err := (NativeSite{}).execute(context.Background(), siteRequest{Target: parseTarget(path), CookieFile: cookieFile, Method: "GET", ReadOnly: true, Yes: true}); err != nil {
			t.Fatal(err)
		}
	}
	if !strings.Contains(seenChild, "SID=child") || !strings.Contains(seenChild, "SID=root") {
		t.Fatalf("path-specific cookies were lost: %q", seenChild)
	}
	if strings.Contains(seenRoot, "SID=root") {
		t.Fatalf("deleted root cookie was resurrected: %q", seenRoot)
	}
}

func TestGradeHeadersKeepOriginalAndRetakeFields(t *testing.T) {
	document, err := parsePage(`<table id="dataList"><tr><th>学期</th><th>重修学期</th><th>成绩</th><th>原始成绩</th><th>课程</th><th>学分</th></tr><tr><td>T1</td><td>T2</td><td>90</td><td>60</td><td>课程</td><td>3</td></tr></table>`)
	if err != nil {
		t.Fatal(err)
	}
	rows, parseErr := parseGradesPage(document, "http://example.test/grades")
	if parseErr != nil || len(rows) != 1 {
		t.Fatalf("unexpected grades: %#v %v", rows, parseErr)
	}
	if rows[0]["semester"] != "T1" || rows[0]["retake_semester"] != "T2" || rows[0]["score"] != "90" || rows[0]["original_score"] != "60" {
		t.Fatalf("grade columns were overwritten: %#v", rows[0])
	}
}

func TestTeachingLookupUsesMethodForSameEndpoint(t *testing.T) {
	item, known, ambiguous := teachingLookup("course-search", "GET", false)
	if !known || ambiguous || item.method != "GET" || item.path != "/meol/course.do" {
		t.Fatalf("unexpected default teaching lookup: %#v known=%v ambiguous=%v", item, known, ambiguous)
	}
	item, known, ambiguous = teachingLookup("course-search", "POST", true)
	if !known || ambiguous || item.method != "POST" || item.path != "/meol/course.do" {
		t.Fatalf("unexpected POST teaching lookup: %#v known=%v ambiguous=%v", item, known, ambiguous)
	}
}

func TestVirtualLabLoginUsesRawBusinessState(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"state":"success","message":"登录成功"}`))
	}))
	defer server.Close()
	t.Setenv("CSUST_BASE_URL", server.URL)
	t.Setenv("CSUST_COOKIE_FILE", filepath.Join(t.TempDir(), "cookies.txt"))
	handled, stdout, _, code, err := (NativeSite{}).Run(context.Background(), []string{"virtual-lab", "login", "--username", "student", "--password", "password", "--captcha", "ok", "--json"}, true)
	if err != nil || !handled || code != 0 {
		t.Fatalf("virtual lab login: handled=%v code=%d err=%v output=%s", handled, code, err, stdout)
	}
	var result map[string]any
	if err := json.Unmarshal(stdout, &result); err != nil {
		t.Fatal(err)
	}
	if result["ok"] != true || result["operation"] != "login" || result["username"] != "student" {
		t.Fatalf("unexpected virtual lab login: %#v", result)
	}
}

func TestRequestBodyStreamsFilesAndRejectsOversize(t *testing.T) {
	root := t.TempDir()
	smallPath := filepath.Join(root, "small.txt")
	if err := os.WriteFile(smallPath, []byte("payload"), 0600); err != nil {
		t.Fatal(err)
	}
	part, err := siteFilePart("file", smallPath, "上传文件")
	if err != nil {
		t.Fatal(err)
	}
	body, contentType, length, cleanup, runErr := requestBody(siteRequest{Method: http.MethodPost, Files: []filePart{part}})
	if runErr != nil {
		t.Fatal(runErr)
	}
	content, readErr := io.ReadAll(body)
	cleanup()
	if readErr != nil || length != int64(len(content)) || !strings.HasPrefix(contentType, "multipart/form-data;") || !strings.Contains(string(content), "payload") {
		t.Fatalf("unexpected streamed multipart body: length=%d contentType=%q readErr=%v", length, contentType, readErr)
	}

	largePath := filepath.Join(root, "large.bin")
	file, createErr := os.Create(largePath)
	if createErr != nil {
		t.Fatal(createErr)
	}
	if err := file.Truncate(maxSiteRequestBody + 1); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	_ = file.Close()
	if _, err := siteFilePart("file", largePath, "上传文件"); err == nil || err.Code != "request_too_large" {
		t.Fatalf("oversize upload was accepted: %v", err)
	}
}

func TestArchiveCatalogOnlyAdvertisesImplementedCapabilities(t *testing.T) {
	result, err := (NativeSite{}).executeArchive(context.Background(), []string{"catalog"})
	if err != nil {
		t.Fatal(err)
	}
	systems := result["systems"].([]map[string]any)
	for _, system := range systems {
		capabilities := fmt.Sprint(system["capabilities"])
		for _, advertised := range []string{"record-query", "person-archive", "collection", "user-management", "import-export"} {
			if strings.Contains(capabilities, advertised) {
				t.Fatalf("unimplemented archive capability remains advertised: %#v", system)
			}
		}
	}
}

func mustMarshalIssue(value any) []byte {
	encoded, _ := json.Marshal(value)
	return encoded
}
