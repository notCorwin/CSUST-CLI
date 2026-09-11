package adapter

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func TestSunshinePublicQueriesAndCodeConfirmation(t *testing.T) {
	var issueQuery map[string]any
	var departmentQuery map[string]any
	var phone string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/issues":
			if request.Method != http.MethodPut || json.NewDecoder(request.Body).Decode(&issueQuery) != nil {
				t.Fatalf("unexpected issue query")
			}
			_, _ = writer.Write([]byte(`{"success":true,"data":{"rows":[{"no":7,"status":"受理中"}],"total":1}}`))
		case "/api/issues/issue-1":
			if request.Method != http.MethodPost {
				t.Fatalf("unexpected issue detail method: %s", request.Method)
			}
			_, _ = writer.Write([]byte(`{"success":true,"data":{"_id":"issue-1","status":"已处理"}}`))
		case "/api/departments":
			if request.Method != http.MethodPut || json.NewDecoder(request.Body).Decode(&departmentQuery) != nil {
				t.Fatalf("unexpected department query")
			}
			_, _ = writer.Write([]byte(`{"success":true,"data":[{"_id":"d1","name":"信息化处"}]}`))
		case "/api/issuestat":
			_, _ = writer.Write([]byte(`{"success":true,"data":{"numTotal":3353}}`))
		case "/api/systems":
			_, _ = writer.Write([]byte(`{"success":true,"data":{"issueAttachmentMaxNum":1}}`))
		case "/api/verifys":
			var body map[string]string
			if request.Method != http.MethodPost || json.NewDecoder(request.Body).Decode(&body) != nil {
				t.Fatalf("unexpected verification request")
			}
			phone = body["tel"]
			_, _ = writer.Write([]byte(`{"success":true,"data":[{"_id":"verify-1"}]}`))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	t.Setenv("CSUST_BASE_URL", server.URL)
	t.Setenv("CSUST_COOKIE_FILE", filepath.Join(t.TempDir(), "cookies.txt"))

	issues := runIssueJSON(t, "sunshine", "issues", "--status", "受理中")
	if issues["service"] != "sunshine" || issues["operation"] != "issues" || issues["data"] == nil {
		t.Fatalf("unexpected sunshine issues result: %#v", issues)
	}
	filter := issueQuery["filter"].(map[string]any)
	if filter["isPublic"] != true || filter["status"].(map[string]any)["$in"].([]any)[0] != "受理中" {
		t.Fatalf("public status filter was not mapped: %#v", issueQuery)
	}

	detail := runIssueJSON(t, "sunshine", "issue", "--id", "issue-1")
	if detail["data"].(map[string]any)["_id"] != "issue-1" {
		t.Fatalf("unexpected sunshine detail: %#v", detail)
	}
	departments := runIssueJSON(t, "sunshine", "departments")
	if departments["data"] == nil || departmentQuery["paginator"].(map[string]any)["needAll"] != true {
		t.Fatalf("unexpected sunshine departments: %#v %#v", departments, departmentQuery)
	}
	if stats := runIssueJSON(t, "sunshine", "stats"); stats["data"].(map[string]any)["numTotal"] != float64(3353) {
		t.Fatalf("unexpected sunshine stats: %#v", stats)
	}
	if config := runIssueJSON(t, "sunshine", "config"); config["data"].(map[string]any)["issueAttachmentMaxNum"] != float64(1) {
		t.Fatalf("unexpected sunshine config: %#v", config)
	}
	if code := runIssueJSON(t, "sunshine", "send-code", "--phone", "13800138000", "--yes"); code["verified_by"] != "response-success" || phone != "13800138000" {
		t.Fatalf("unexpected sunshine code result: %#v phone=%s", code, phone)
	}
}
