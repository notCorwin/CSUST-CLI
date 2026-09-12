package adapter

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func TestTransportMobileVacationCreateAndUpdate(t *testing.T) {
	const token, userID, groupID = "vacation-token", "user-1", "group-1"
	var row map[string]any
	var createBody, updateBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		if request.Header.Get("token") != token {
			t.Errorf("missing vacation token: %q", request.Header.Get("token"))
		}
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/api/user/getUserInfo":
			_, _ = fmt.Fprint(writer, `{"code":200,"success":true,"data":{"_id":"user-1","group":[{"_id":"group-1"}]}}`)
		case request.Method == http.MethodPost && request.URL.Path == "/api/table/vacation":
			if err := json.NewDecoder(request.Body).Decode(&createBody); err != nil {
				t.Fatal(err)
			}
			row = map[string]any{"_id": "vac-1", "version": "v1", "dateBegin": createBody["dateBegin"], "dateEnd": createBody["dateEnd"], "numDay": createBody["numDay"], "type": createBody["type"], "reason": createBody["reason"], "address": createBody["address"], "department": createBody["department"]}
			_, _ = fmt.Fprint(writer, `{"code":200,"success":true,"data":{"_id":"vac-1","version":"v1"}}`)
		case request.Method == http.MethodPut && request.URL.Path == "/api/table/vacation":
			_ = json.NewEncoder(writer).Encode(map[string]any{"code": 200, "success": true, "data": map[string]any{"total": 1, "records": []any{row}}})
		case request.Method == http.MethodPut && request.URL.Path == "/api/table/vacation/vac-1":
			if err := json.NewDecoder(request.Body).Decode(&updateBody); err != nil {
				t.Fatal(err)
			}
			if updateBody["version"] != "v1" {
				t.Errorf("vacation version was not sent: %#v", updateBody)
			}
			for _, key := range []string{"dateBegin", "dateEnd", "numDay", "type", "reason", "address"} {
				row[key] = updateBody[key]
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

	created := runIssueJSON(t, "transport-mobile", "vacation-create", "--access-token", token, "--from", "2026-09-15", "--to", "2026-09-16", "--type", "出差", "--reason", "项目调研", "--address", "长沙", "--yes")
	if created["submitted"] != true || created["confirmed"] != true || created["id"] != "vac-1" {
		t.Fatalf("vacation create was not confirmed: %#v", created)
	}
	if createBody["code"] != "__auto__vacation" || createBody["status"] != "暂存" || createBody["creater"] != userID || createBody["department"] != groupID || createBody["numDay"] != float64(2) {
		t.Fatalf("vacation create entity was not mapped: %#v", createBody)
	}

	updated := runIssueJSON(t, "transport-mobile", "vacation-update", "--access-token", token, "--id", "vac-1", "--from", "2026-09-20", "--to", "2026-09-20", "--days", "0.5", "--type", "事假", "--reason", "个人事务", "--address", "家中", "--yes")
	if updated["submitted"] != true || updated["confirmed"] != true || updated["id"] != "vac-1" {
		t.Fatalf("vacation update was not confirmed: %#v", updated)
	}
	if updateBody["numDay"] != 0.5 || updateBody["type"] != "事假" || updateBody["reason"] != "个人事务" || updateBody["address"] != "家中" {
		t.Fatalf("vacation update entity was not mapped: %#v", updateBody)
	}
}
