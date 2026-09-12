package adapter

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestTransportMobileExportsDownloadReturnedFile(t *testing.T) {
	const token = "export-access-token"
	var batchIDs []any
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/export/defense/d-1", "/api/export/finance/f-1":
			if request.Method != http.MethodGet || request.Header.Get("token") != token {
				t.Errorf("record export was not authenticated GET: method=%s token=%q", request.Method, request.Header.Get("token"))
			}
			_, _ = fmt.Fprint(writer, `{"code":200,"success":true,"data":{"token":"file-token"}}`)
		case "/api/defensesop":
			if request.Method != http.MethodPost || request.Header.Get("token") != token {
				t.Errorf("batch export was not authenticated POST: method=%s token=%q", request.Method, request.Header.Get("token"))
			}
			var body map[string]any
			if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
				t.Fatalf("batch export body is not JSON: %v", err)
			}
			batchIDs, _ = body["ids"].([]any)
			_, _ = fmt.Fprint(writer, `{"code":200,"success":true,"data":{"token":"file-token"}}`)
		case "/api/download/file-token":
			if request.Method != http.MethodGet || request.URL.Query().Get("token") != token || request.URL.Query().Get("t") == "" || request.Header.Get("token") != token {
				t.Errorf("download request was not authenticated: method=%s query=%s header=%q", request.Method, request.URL.RawQuery, request.Header.Get("token"))
			}
			writer.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
			writer.Header().Set("Content-Disposition", `attachment; filename="export.xlsx"`)
			_, _ = writer.Write([]byte("PK-export-bytes"))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	t.Setenv("CSUST_BASE_URL", server.URL)
	t.Setenv("CSUST_COOKIE_FILE", filepath.Join(t.TempDir(), "transport.cookies.txt"))

	root := t.TempDir()
	defenseOutput := filepath.Join(root, "defense.xlsx")
	financeOutput := filepath.Join(root, "finance.xlsx")
	batchOutput := filepath.Join(root, "defenses.xlsx")

	defense := runIssueJSON(t, "transport-mobile", "defense-export", "--access-token", token, "--id", "d-1", "--output", defenseOutput)
	finance := runIssueJSON(t, "transport-mobile", "finance-export", "--access-token", token, "--id", "f-1", "--output", financeOutput)
	batch := runIssueJSON(t, "transport-mobile", "defense-batch-export", "--access-token", token, "--id", "d-1,d-2", "--output", batchOutput)

	for _, result := range []map[string]any{defense, finance, batch} {
		if result["ok"] != true || result["downloaded"] != true || result["confirmed"] != true || result["bytes"] != float64(len("PK-export-bytes")) {
			t.Fatalf("export was not confirmed: %#v", result)
		}
	}
	if defense["operation"] != "defense-export" || finance["operation"] != "finance-export" || batch["operation"] != "defense-batch-export" {
		t.Fatalf("export operation was not mapped: defense=%#v finance=%#v batch=%#v", defense, finance, batch)
	}
	if len(batchIDs) != 2 || batchIDs[0] != "d-1" || batchIDs[1] != "d-2" {
		t.Fatalf("batch export IDs were not mapped: %#v", batchIDs)
	}
	for _, filename := range []string{defenseOutput, financeOutput, batchOutput} {
		content, err := os.ReadFile(filename)
		if err != nil || string(content) != "PK-export-bytes" {
			t.Fatalf("export file was not written: %s err=%v content=%q", filename, err, content)
		}
	}
}

func TestTransportMobileExportRequiresOutput(t *testing.T) {
	handled, _, _, code, err := (NativeSite{}).Run(t.Context(), []string{"transport-mobile", "finance-export", "--id", "f-1"}, false)
	if err != nil || !handled || code != 2 {
		t.Fatalf("finance export without output was not rejected: handled=%v code=%d err=%v", handled, code, err)
	}
}
