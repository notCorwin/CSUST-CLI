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

func TestTransportMobileFinanceItemMutations(t *testing.T) {
	const token = "finance-item-token"
	currentRow := map[string]any{
		"_id": "fi-1", "project": map[string]any{"_id": "p-1", "name": "科研项目"},
		"money": 100.0, "dir": -1, "type": "travel", "remark": "旧备注",
		"version": "v1", "file": []any{map[string]any{"_id": "old-file"}},
	}
	detailExists := true
	var savedBody map[string]any
	uploads, deletes, refreshes := 0, 0, 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		if request.Header.Get("token") != token {
			t.Errorf("missing transport token for %s %s", request.Method, request.URL.Path)
		}
		switch request.URL.Path {
		case "/api/file/upload":
			if request.Method != http.MethodPost || request.Header.Get("X-Encoded-Filename") != "1" {
				t.Errorf("upload protocol mismatch: method=%s encoded=%q", request.Method, request.Header.Get("X-Encoded-Filename"))
			}
			if err := request.ParseMultipartForm(1 << 20); err != nil {
				t.Errorf("upload body was not multipart: %v", err)
			}
			file, header, err := request.FormFile("file")
			if err != nil {
				t.Errorf("uploaded file missing: %v", err)
			} else {
				_ = file.Close()
				if header.Filename != "receipt.txt" {
					t.Errorf("unexpected uploaded filename: %q", header.Filename)
				}
			}
			uploads++
			_, _ = fmt.Fprint(writer, `{"code":200,"data":{"_id":"file-1","name":"receipt.txt"},"success":true}`)
		case "/api/table/fitem":
			if request.Method != http.MethodPut {
				t.Errorf("fitem query used %s", request.Method)
			}
			if detailExists {
				_ = json.NewEncoder(writer).Encode(map[string]any{"code": 200, "data": map[string]any{"total": 1, "records": []any{currentRow}}, "success": true})
			} else {
				_, _ = fmt.Fprint(writer, `{"code":200,"data":{"total":0,"records":[]},"success":true}`)
			}
		case "/api/financesop/p-1":
			if request.Method == http.MethodPatch {
				refreshes++
				_, _ = fmt.Fprint(writer, `{"code":200,"success":true}`)
				return
			}
			if request.Method != http.MethodPost {
				t.Errorf("finance save used %s", request.Method)
			}
			if err := json.NewDecoder(request.Body).Decode(&savedBody); err != nil {
				t.Errorf("finance save body was not JSON: %v", err)
			}
			entity, _ := savedBody["entity"].(map[string]any)
			if savedBody["type"] == "update" {
				currentRow = map[string]any{
					"_id": "fi-1", "project": map[string]any{"_id": "p-1", "name": "科研项目"},
					"money": entity["money"], "dir": entity["dir"], "type": entity["type"], "remark": entity["remark"],
					"version": "v1", "file": []any{map[string]any{"_id": "retained-file"}, map[string]any{"_id": "file-1"}},
				}
			}
			_, _ = fmt.Fprint(writer, `{"code":200,"data":{"saved":true},"success":true}`)
		case "/api/table/fitem/fi-1":
			if request.Method != http.MethodDelete {
				t.Errorf("finance delete used %s", request.Method)
			}
			deletes++
			detailExists = false
			_, _ = fmt.Fprint(writer, `{"code":200,"success":true}`)
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	root := t.TempDir()
	filePath := filepath.Join(root, "receipt.txt")
	if err := os.WriteFile(filePath, []byte("receipt"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CSUST_BASE_URL", server.URL)
	t.Setenv("CSUST_COOKIE_FILE", filepath.Join(root, "transport.cookies.txt"))

	created := runIssueJSON(t, "transport-mobile", "finance-item-create", "--access-token", token, "--project-id", "p-1", "--money", "120.5", "--direction", "expense", "--type", "travel", "--remark", "现场调研", "--file", filePath, "--yes")
	if created["submitted"] != true || created["confirmed"] != true || created["files_uploaded"] != float64(1) {
		t.Fatalf("finance-item create was not confirmed: %#v", created)
	}

	updated := runIssueJSON(t, "transport-mobile", "finance-item-update", "--access-token", token, "--id", "fi-1", "--money", "150", "--direction", "income", "--remark", "新备注", "--file-id", "retained-file", "--file", filePath, "--yes")
	if updated["submitted"] != true || updated["confirmed"] != true || updated["evidence"] != "server-success-and-item-readback" {
		t.Fatalf("finance-item update was not confirmed: %#v", updated)
	}
	entity := savedBody["entity"].(map[string]any)
	if entity["project"] != "p-1" || entity["money"] != float64(150) || entity["dir"] != float64(1) || entity["remark"] != "新备注" {
		t.Fatalf("finance-item update body was not mapped: %#v", savedBody)
	}
	files := entity["file"].([]any)
	if len(files) != 2 || files[0] != "retained-file" || files[1] != "file-1" {
		t.Fatalf("finance-item files were not mapped: %#v", entity["file"])
	}

	deleted := runIssueJSON(t, "transport-mobile", "finance-item-delete", "--access-token", token, "--id", "fi-1", "--yes")
	if deleted["submitted"] != true || deleted["confirmed"] != true || deletes != 1 || refreshes != 1 || uploads != 2 {
		t.Fatalf("finance-item delete or mutation sequence was not confirmed: result=%#v deletes=%d refreshes=%d uploads=%d", deleted, deletes, refreshes, uploads)
	}
}
