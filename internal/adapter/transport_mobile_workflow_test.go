package adapter

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func TestTransportMobileWorkflowAction(t *testing.T) {
	const token = "workflow-token"
	row := map[string]any{
		"_id": "workflow-1", "version": "v1", "status": "办理中",
		"current": map[string]any{"index": 0.0}, "steps": []any{"node-1", "node-2"},
		"edges": []any{
			map[string]any{"id": "edge-approve", "source": "node-1", "text": "批准", "isMain": true},
			map[string]any{"id": "edge-reject", "source": "node-1", "text": "不批准", "isMain": false},
		},
	}
	var actionBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		if request.Header.Get("token") != token {
			t.Errorf("missing workflow token: %q", request.Header.Get("token"))
		}
		switch {
		case request.Method == http.MethodPut && request.URL.Path == "/api/table/realworkflow":
			_ = json.NewEncoder(writer).Encode(map[string]any{"code": 200, "success": true, "data": map[string]any{"total": 1, "records": []any{row}}})
		case request.Method == http.MethodPut && request.URL.Path == "/api/realworkflowsop/workflow-1":
			if err := json.NewDecoder(request.Body).Decode(&actionBody); err != nil {
				t.Fatal(err)
			}
			if actionBody["edge"] != "edge-approve" || actionBody["reason"] != "同意" || actionBody["version"] != "v1" {
				t.Fatalf("workflow action body was not mapped: %#v", actionBody)
			}
			row["version"], row["status"] = "v2", "已办结"
			row["current"] = map[string]any{"index": 1.0}
			_, _ = fmt.Fprint(writer, `{"code":200,"success":true}`)
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	t.Setenv("CSUST_BASE_URL", server.URL)
	t.Setenv("CSUST_COOKIE_FILE", filepath.Join(t.TempDir(), "transport.cookies.txt"))

	result := runIssueJSON(t, "transport-mobile", "workflow-action", "--access-token", token, "--id", "workflow-1", "--action", "approve", "--reason", "同意", "--yes")
	if result["submitted"] != true || result["confirmed"] != true || result["action"] != "approve" {
		t.Fatalf("workflow action was not confirmed: %#v", result)
	}
}
