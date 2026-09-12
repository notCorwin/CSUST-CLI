package adapter

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestTransportMobileAchievementCreateUpdateAndStatus(t *testing.T) {
	const token = "achievement-token"
	var row map[string]any
	var createBody, updateBody, statusBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/api/user/getUserInfo":
			_, _ = fmt.Fprint(writer, `{"code":200,"success":true,"data":{"_id":"user-1"}}`)
		case request.Method == http.MethodPost && request.URL.Path == "/api/table/achievement":
			if err := json.NewDecoder(request.Body).Decode(&createBody); err != nil {
				t.Fatal(err)
			}
			row = cloneJSONMap(createBody)
			row["_id"], row["version"] = "achievement-1", "v1"
			_, _ = fmt.Fprint(writer, `{"code":200,"success":true,"data":{"_id":"achievement-1"}}`)
		case request.Method == http.MethodPut && request.URL.Path == "/api/table/achievement":
			_ = json.NewEncoder(writer).Encode(map[string]any{"code": 200, "success": true, "data": map[string]any{"total": 1, "records": []any{row}}})
		case request.Method == http.MethodPut && strings.HasPrefix(request.URL.Path, "/api/table/achievement/"):
			if err := json.NewDecoder(request.Body).Decode(&updateBody); err != nil {
				t.Fatal(err)
			}
			if updateBody["version"] != row["version"] {
				t.Errorf("achievement version was not sent: %#v", updateBody)
			}
			if status, ok := updateBody["status"].(string); ok {
				statusBody = updateBody
				row["status"] = status
				row["history"] = []any{updateBody["history"]}
			} else {
				for key, value := range updateBody {
					row[key] = value
				}
			}
			row["version"] = "v2"
			_, _ = fmt.Fprint(writer, `{"code":200,"success":true}`)
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	t.Setenv("CSUST_BASE_URL", server.URL)
	t.Setenv("CSUST_COOKIE_FILE", filepath.Join(t.TempDir(), "transport.cookies.txt"))

	created := runIssueJSON(t, "transport-mobile", "achievement-create", "--access-token", token, "--type", "paper", "--name", "论文题目", "--student-no", "S001", "--student-name", "学生一", "--journal", "期刊名称", "--index", "SCI", "--completed-at", "2026-09-12", "--file-id", "file-1", "--yes")
	if created["submitted"] != true || created["confirmed"] != true || created["id"] != "achievement-1" {
		t.Fatalf("achievement create was not confirmed: %#v", created)
	}
	if createBody["type"] != "论文" || createBody["name"] != "论文题目" || createBody["studentNo"] != "S001" || createBody["file"].([]any)[0] != "file-1" {
		t.Fatalf("achievement create entity was not mapped: %#v", createBody)
	}

	updated := runIssueJSON(t, "transport-mobile", "achievement-update", "--access-token", token, "--id", "achievement-1", "--remark", "修改后的备注", "--yes")
	if updated["submitted"] != true || updated["confirmed"] != true || updated["id"] != "achievement-1" {
		t.Fatalf("achievement update was not confirmed: %#v", updated)
	}
	if updateBody["remark"] != "修改后的备注" {
		t.Fatalf("achievement update entity was not mapped: %#v", updateBody)
	}

	status := runIssueJSON(t, "transport-mobile", "achievement-status", "--access-token", token, "--id", "achievement-1", "--action", "submit", "--yes")
	if status["submitted"] != true || status["confirmed"] != true || status["status"] != "已提交" || statusBody["status"] != "已提交" {
		t.Fatalf("achievement status was not confirmed: result=%#v body=%#v", status, statusBody)
	}

	detail := runIssueJSON(t, "transport-mobile", "achievement", "--access-token", token, "--id", "achievement-1")
	if detail["id"] != "achievement-1" || detail["achievement"].(map[string]any)["achievement_type"] != "论文" {
		t.Fatalf("achievement detail was not mapped: %#v", detail)
	}
}

func TestTransportMobileAchievementValidation(t *testing.T) {
	handled, _, _, code, err := (NativeSite{}).Run(t.Context(), []string{"transport-mobile", "achievement-create", "--type", "paper", "--name", "论文", "--completed-at", "2026-09-12", "--yes"}, false)
	if err != nil || !handled || code != 2 {
		t.Fatalf("achievement create validation was not rejected: handled=%v code=%d err=%v", handled, code, err)
	}

	handled, _, _, code, err = (NativeSite{}).Run(t.Context(), []string{"transport-mobile", "achievement-status", "--id", "achievement-1", "--action", "reject", "--yes"}, false)
	if err != nil || !handled || code != 2 {
		t.Fatalf("achievement status reason validation was not rejected: handled=%v code=%d err=%v", handled, code, err)
	}
}
