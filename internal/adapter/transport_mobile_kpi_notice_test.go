package adapter

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func TestTransportMobileKPICreateAndUpdate(t *testing.T) {
	const token = "kpi-token"
	var row map[string]any
	var createBody, updateBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/api/user/getUserInfo":
			_, _ = fmt.Fprint(writer, `{"code":200,"success":true,"data":{"_id":"user-1"}}`)
		case request.Method == http.MethodPost && request.URL.Path == "/api/table/kpi":
			if err := json.NewDecoder(request.Body).Decode(&createBody); err != nil {
				t.Fatal(err)
			}
			row = map[string]any{"_id": "kpi-1", "version": "v1", "owner": map[string]any{"_id": createBody["owner"]}, "year": createBody["year"], "type": createBody["type"], "block": createBody["block"], "name": createBody["name"], "score": createBody["score"], "remark": createBody["remark"], "info": createBody["info"]}
			_, _ = fmt.Fprint(writer, `{"code":200,"success":true,"data":{"_id":"kpi-1"}}`)
		case request.Method == http.MethodPut && request.URL.Path == "/api/table/kpi":
			_ = json.NewEncoder(writer).Encode(map[string]any{"code": 200, "success": true, "data": map[string]any{"total": 1, "records": []any{row}}})
		case request.Method == http.MethodPut && request.URL.Path == "/api/table/kpi/kpi-1":
			if err := json.NewDecoder(request.Body).Decode(&updateBody); err != nil {
				t.Fatal(err)
			}
			if updateBody["version"] != "v1" {
				t.Errorf("kpi version was not sent: %#v", updateBody)
			}
			for _, key := range []string{"owner", "year", "type", "block", "name", "score", "remark", "info"} {
				row[key] = updateBody[key]
			}
			row["owner"] = map[string]any{"_id": updateBody["owner"]}
			row["version"] = "v2"
			_, _ = fmt.Fprint(writer, `{"code":200,"success":true}`)
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	t.Setenv("CSUST_BASE_URL", server.URL)
	t.Setenv("CSUST_COOKIE_FILE", filepath.Join(t.TempDir(), "transport.cookies.txt"))

	created := runIssueJSON(t, "transport-mobile", "kpi-create", "--access-token", token, "--owner-id", "user-1", "--year", "2026", "--type", "本科教学", "--block", "J1", "--name", "课堂教学", "--score", "3.5", "--remark", "首条", "--info", `{"hours":2}`, "--yes")
	if created["submitted"] != true || created["confirmed"] != true || created["id"] != "kpi-1" {
		t.Fatalf("kpi create was not confirmed: %#v", created)
	}
	if createBody["owner"] != "user-1" || createBody["year"] != "2026" || createBody["score"] != 3.5 || createBody["info"].(map[string]any)["hours"] != float64(2) {
		t.Fatalf("kpi create entity was not mapped: %#v", createBody)
	}

	updated := runIssueJSON(t, "transport-mobile", "kpi-update", "--access-token", token, "--id", "kpi-1", "--score", "4", "--remark", "修改", "--yes")
	if updated["submitted"] != true || updated["confirmed"] != true || updated["id"] != "kpi-1" {
		t.Fatalf("kpi update was not confirmed: %#v", updated)
	}
	if updateBody["score"] != float64(4) || updateBody["remark"] != "修改" {
		t.Fatalf("kpi update entity was not mapped: %#v", updateBody)
	}
}

func TestTransportMobileNoticeCreateAndUpdate(t *testing.T) {
	const token = "notice-token"
	var row map[string]any
	var createBody, updateBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/api/user/getUserInfo":
			_, _ = fmt.Fprint(writer, `{"code":200,"success":true,"data":{"_id":"user-1"}}`)
		case request.Method == http.MethodPost && request.URL.Path == "/api/table/notice":
			if err := json.NewDecoder(request.Body).Decode(&createBody); err != nil {
				t.Fatal(err)
			}
			row = cloneJSONMap(createBody)
			row["_id"], row["version"] = "notice-1", "v1"
			_, _ = fmt.Fprint(writer, `{"code":200,"success":true,"data":{"_id":"notice-1"}}`)
		case request.Method == http.MethodPut && request.URL.Path == "/api/table/notice":
			_ = json.NewEncoder(writer).Encode(map[string]any{"code": 200, "success": true, "data": map[string]any{"total": 1, "records": []any{row}}})
		case request.Method == http.MethodPut && request.URL.Path == "/api/table/notice/notice-1":
			if err := json.NewDecoder(request.Body).Decode(&updateBody); err != nil {
				t.Fatal(err)
			}
			if updateBody["version"] != "v1" {
				t.Errorf("notice version was not sent: %#v", updateBody)
			}
			for key, value := range updateBody {
				row[key] = value
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

	created := runIssueJSON(t, "transport-mobile", "notice-create", "--access-token", token, "--title", "学院通知", "--type", "学院通知", "--content", "通知正文", "--private", "--recipient-id", "user-2,user-3", "--audience-group-id", "group-1", "--approver-id", "approver-1", "--url", "https://example.com", "--file-id", "file-1", "--need-approval", "--yes")
	if created["submitted"] != true || created["confirmed"] != true || created["id"] != "notice-1" {
		t.Fatalf("notice create was not confirmed: %#v", created)
	}
	if createBody["status"] != "暂存" || createBody["needApproval"] != true || createBody["title"] != "学院通知" {
		t.Fatalf("notice create entity was not mapped: %#v", createBody)
	}

	updated := runIssueJSON(t, "transport-mobile", "notice-update", "--access-token", token, "--id", "notice-1", "--title", "已发布通知", "--content", "新正文", "--public", "--no-approval", "--yes")
	if updated["submitted"] != true || updated["confirmed"] != true || updated["id"] != "notice-1" {
		t.Fatalf("notice update was not confirmed: %#v", updated)
	}
	if updateBody["status"] != "已办结" || updateBody["needApproval"] != false || updateBody["title"] != "已发布通知" {
		t.Fatalf("notice update entity was not mapped: %#v", updateBody)
	}
}

func cloneJSONMap(input map[string]any) map[string]any {
	result := make(map[string]any, len(input))
	for key, value := range input {
		result[key] = value
	}
	return result
}
