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

func TestVirtualLabMessageCreateMapsPublicEndpoint(t *testing.T) {
	var fields url.Values
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.Path != "/Home/LyglData" {
			t.Fatalf("unexpected virtual lab message request: %s %s", request.Method, request.URL.Path)
		}
		_ = request.ParseForm()
		fields = request.Form
		writer.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = fmt.Fprint(writer, "1")
	}))
	defer server.Close()
	t.Setenv("CSUST_BASE_URL", server.URL)
	t.Setenv("CSUST_COOKIE_FILE", filepath.Join(t.TempDir(), "cookies.txt"))
	result := runIssueJSON(t, "virtual-lab", "message-create", "--anonymous", "--content", "请补充实验说明", "--captcha", "1234", "--yes")
	if fields.Get("PostUser") != "" || fields.Get("Phone") != "" || fields.Get("Contents") != "请补充实验说明" || fields.Get("Code") != "1234" {
		t.Fatalf("virtual lab message fields were not mapped: %#v", fields)
	}
	if result["confirmed"] != true || result["anonymous"] != true || result["evidence"] != "LyglData-response-1" {
		t.Fatalf("unexpected virtual lab message result: %#v", result)
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
		for _, advertised := range []string{"record-query", "collection", "user-management", "import-export"} {
			if strings.Contains(capabilities, advertised) {
				t.Fatalf("unimplemented archive capability remains advertised: %#v", system)
			}
		}
	}
}

func TestArchiveUsesJeecgBootPersonAndDownloadAPIs(t *testing.T) {
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		paths = append(paths, request.URL.Path)
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/jeecg-boot/das.archive/dasInfoVolumes/personArchiveCode":
			_, _ = writer.Write([]byte(`{"success":true,"code":0,"result":"BD"}`))
		case "/jeecg-boot/das.archive/dasInfoVolumes/getTreeList":
			_, _ = writer.Write([]byte(`{"success":true,"code":0,"result":[]}`))
		case "/jeecg-boot/das.archive/dasInfoVolumes/personArchive":
			_, _ = writer.Write([]byte(`{"success":true,"code":0,"result":[{"id":"volume-1"}]}`))
		case "/jeecg-boot/das.archive/dasInfoVolumes/listDasInfoAttachmentByVolumeId":
			_, _ = writer.Write([]byte(`{"success":true,"code":0,"result":[{"id":"file-1","format":"pdf"}]}`))
		case "/jeecg-boot/das.archive/dasInfoVolumes/getDateTimeToString":
			_, _ = writer.Write([]byte(`{"success":true,"code":0,"result":"123"}`))
		case "/jeecg-boot/sys/randomImage/check-key":
			_, _ = writer.Write([]byte(`{"success":true,"code":0,"result":"data:image/png;base64,UE5H"}`))
		case "/jeecg-boot/das.archive/dasInfoVolumes/download/file-1":
			writer.Header().Set("Content-Type", "application/pdf")
			_, _ = writer.Write([]byte("%PDF-1.4 test"))
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	t.Setenv("CSUST_BASE_URL", server.URL)
	t.Setenv("CSUST_COOKIE_FILE", filepath.Join(t.TempDir(), "cookies.txt"))

	_ = runIssueJSON(t, "archive", "person-archive", "--system", "student", "--person-id", "person-1", "--category-code", "BD", "--access-token", "token")
	_ = runIssueJSON(t, "archive", "attachments", "--system", "student", "--volume-id", "volume-1", "--format", "pdf", "--access-token", "token")
	output := filepath.Join(t.TempDir(), "archive.pdf")
	_ = runIssueJSON(t, "archive", "download", "--system", "student", "--file-id", "file-1", "--output", output, "--access-token", "token")
	content, err := os.ReadFile(output)
	if err != nil || !strings.HasPrefix(string(content), "%PDF-") {
		t.Fatalf("archive download was not saved as PDF: err=%v content=%q", err, content)
	}
	captchaPath := filepath.Join(t.TempDir(), "captcha.png")
	handled, stdout, _, code, runErr := (NativeSite{}).Run(context.Background(), []string{"archive", "login", "--system", "student", "--username", "u", "--password", "p", "--check-key", "check-key", "--captcha-image", captchaPath, "--json"}, true)
	if runErr != nil || !handled || code != 2 {
		t.Fatalf("captcha challenge did not stop for input: handled=%v code=%d err=%v stdout=%s", handled, code, runErr, stdout)
	}
	captcha, err := os.ReadFile(captchaPath)
	if err != nil || string(captcha) != "PNG" {
		t.Fatalf("captcha data URL was not decoded: err=%v content=%q", err, captcha)
	}
	for _, path := range paths {
		if !strings.HasPrefix(path, "/jeecg-boot/") {
			t.Fatalf("archive API used wrong base path: %q", path)
		}
	}
}

func TestStudentRecordTraceRequiresStudentIdentity(t *testing.T) {
	_, err := (NativeSite{}).executeStudentRecord(context.Background(), []string{"trace", "--name", "测试"})
	if err == nil || err.Code != "invalid_argument" {
		t.Fatalf("trace accepted a name without student identity: %v", err)
	}
}

func mustMarshalIssue(value any) []byte {
	encoded, _ := json.Marshal(value)
	return encoded
}
