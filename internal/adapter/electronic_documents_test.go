package adapter

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestElectronicDocumentsQueriesAndDelivery(t *testing.T) {
	const token = "token-secret"
	const userID = "student-secret"
	var downloadSeen, emailSeen bool
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			t.Errorf("%s used %s instead of POST", request.URL.Path, request.Method)
			writer.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if got := request.Header.Get("X-Authorization"); got != token {
			t.Errorf("%s missing bearer token: %q", request.URL.Path, got)
		}
		var body map[string]any
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Errorf("%s invalid JSON body: %v", request.URL.Path, err)
			writer.WriteHeader(http.StatusBadRequest)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/engine-dzpz/ElectronicFile/getFilePrintType":
			if body["userId"] != userID {
				t.Errorf("file type user id was not mapped: %#v", body)
			}
			_, _ = fmt.Fprint(writer, `{"errcode":"0","errmsg":"成功","result":{"data":[{"vcPrintTypeId":"file-1","printType":"中文成绩单","fileProerty":"0","printerId":"printer-1","svEnable":"OFF"}]}}`)
		case "/api/engine-dzpz/ElectronicFile/userOrderList":
			if body["size"] != float64(10) || body["page"] != float64(2) || body["moduleType"] != float64(1) {
				t.Errorf("application list params were not mapped: %#v", body)
			}
			_, _ = fmt.Fprint(writer, `{"errcode":"0","errmsg":"成功","result":{"total":1,"data":[{"orderId":"order-1","studentId":"student-secret","toEmail":"mail@example.com"}]}}`)
		case "/api/engine-dzpz/ElectronicFile/userOrderDetails":
			if body["orderId"] != "order-1" {
				t.Errorf("application id was not mapped: %#v", body)
			}
			_, _ = fmt.Fprint(writer, `{"errcode":"0","errmsg":"成功","result":{"data":[{"orderId":"order-1","status":"已完成"}]}}`)
		case "/api/engine-dzpz/ElectronicFile/getPrintFilePictures":
			_, _ = fmt.Fprint(writer, `{"errcode":"0","errmsg":"成功","result":{"data":[{"pdfSerialId":"serial-1","fileUrl":"https://signed.example/file.pdf","smallImageList":"base64-preview"}]}}`)
		case "/api/engine-dzpz/ElectronicFile/getPayProductInfoEx":
			if body["userId"] != userID {
				t.Errorf("product user id was not mapped: %#v", body)
			}
			_, _ = fmt.Fprint(writer, `{"errcode":"0","errmsg":"成功","result":{"data":[{"fileName":"transcript.pdf","products":[{"printerId":"printer-1"}]}]}}`)
		case "/api/engine-dzpz/Pay/webDownloadDoc":
			downloadSeen = true
			writer.Header().Set("Content-Type", "application/pdf")
			_, _ = writer.Write([]byte("%PDF-test"))
		case "/api/engine-dzpz/Pay/webWechatPay":
			emailSeen = true
			if body["toEmail"] != "mail@example.com" {
				t.Errorf("email target was not mapped: %#v", body)
			}
			_, _ = fmt.Fprint(writer, `{"errcode":"0","errmsg":"成功","result":{"data":[]}}`)
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	root := t.TempDir()
	cookie := filepath.Join(root, "electronic.cookies.txt")
	t.Setenv("CSUST_BASE_URL", server.URL)
	t.Setenv("CSUST_COOKIE_FILE", cookie)
	if err := os.WriteFile(electronicDocumentsTokenPath(cookie), []byte(token+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(electronicDocumentsUserPath(cookie), []byte(userID+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	types := runIssueJSON(t, "electronic-documents", "types")
	if types["count"] != float64(1) || types["data"].([]any)[0].(map[string]any)["printType"] != "中文成绩单" {
		t.Fatalf("file types were not mapped: %#v", types)
	}
	applications := runIssueJSON(t, "electronic-documents", "applications", "--kind", "transcript", "--page", "2", "--page-size", "10")
	if applications["kind"] != "transcript" || applications["total"] != float64(1) {
		t.Fatalf("applications were not mapped: %#v", applications)
	}
	application := runIssueJSON(t, "electronic-documents", "application", "--id", "order-1")
	if application["order_id"] != "order-1" || application["data"].([]any)[0].(map[string]any)["status"] != "已完成" {
		t.Fatalf("application detail was not mapped: %#v", application)
	}

	output := filepath.Join(root, "transcript.pdf")
	download := runIssueJSON(t, "electronic-documents", "apply", "--type", "chinese-transcript", "--delivery", "download", "--output", output, "--yes")
	content, err := os.ReadFile(output)
	if err != nil || string(content) != "%PDF-test" || !downloadSeen || download["confirmed"] != true {
		t.Fatalf("download was not verified: result=%#v content=%q seen=%v err=%v", download, content, downloadSeen, err)
	}

	email := runIssueJSON(t, "electronic-documents", "apply", "--type", "chinese-transcript", "--delivery", "email", "--email", "mail@example.com", "--yes")
	if !emailSeen || email["submitted"] != true || email["confirmed"] != true {
		t.Fatalf("email delivery was not verified: %#v", email)
	}
	encoded, _ := json.Marshal(map[string]any{"types": types, "applications": applications, "application": application, "email": email})
	if strings.Contains(string(encoded), token) || strings.Contains(string(encoded), userID) || strings.Contains(string(encoded), "mail@example.com") || strings.Contains(string(encoded), "base64-preview") {
		t.Fatalf("sensitive electronic document data leaked: %s", encoded)
	}
}
