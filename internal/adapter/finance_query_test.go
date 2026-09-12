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

func TestFinanceQueryMapsSemanticFiltersAndRedacts(t *testing.T) {
	seen := make(map[string]map[string]string)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		if request.Method == http.MethodGet && request.URL.Path == "/AC/sso/index" {
			writer.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = fmt.Fprint(writer, `<title>智慧财务管理平台</title><div>已登录工作台</div>`)
			return
		}
		if request.Method != http.MethodPost {
			writer.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if err := request.ParseForm(); err != nil {
			t.Fatal(err)
		}
		values := make(map[string]string)
		for key := range request.Form {
			values[key] = request.Form.Get(key)
		}
		seen[request.URL.Path] = values
		switch request.URL.Path {
		case "/CWCX_V2/sfcx/xs/sfzz":
			_, _ = fmt.Fprint(writer, `{"total":1,"rows":[{"xh":"202401150107","xm":"测试用户","sfxmmc":"学费","qjje":100}],"userdata":{}}`)
		case "/CWCX_V2/sfcx/xs/sfmxz", "/CWCX_V2/sfcx/xs/jzjl", "/CWCX_V2/sfcx/xs/jmmxz", "/CWCX_V2/sfcx/xs/tfmxz", "/CWCX_V2/sfcx/xs/hjmxz", "/CWCX_V2/cwcx/gz/bwsrmx", "/CWCX_V2/cwcx/ggcx/dqrsr", "/CWCX_V2/cwcx/ggcx/cxzfkx", "/CWCX_V2/cwcx/ggcx/wqrsyddk":
			_, _ = fmt.Fprint(writer, `{"total":0,"rows":[],"userdata":{}}`)
		case "/CWCX_V2/cwcx/gz/grsr/chart":
			_, _ = fmt.Fprint(writer, `{"code":0,"success":true,"data":{"grsr":[]}}`)
		case "/CWCX_V2/cwcx/dm/module/msg/show/list":
			_, _ = fmt.Fprint(writer, `{"code":0,"success":true,"data":[]}`)
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	t.Setenv("CSUST_BASE_URL", server.URL)
	t.Setenv("CSUST_COOKIE_FILE", filepath.Join(t.TempDir(), "finance.cookies.txt"))

	status := runIssueJSON(t, "finance-query", "status")
	if status["logged_in"] != true {
		t.Fatalf("finance status did not confirm the workbench: %#v", status)
	}
	fees := runIssueJSON(t, "finance-query", "fees", "--year", "2026", "--status", "unpaid", "--page", "2", "--page-size", "25")
	if fees["year"] != "2026" || fees["status"] != "1" || fees["page"] != float64(2) || fees["page_size"] != float64(25) {
		t.Fatalf("semantic fee filters were not returned: %#v", fees)
	}
	feeData := fees["data"].(map[string]any)
	row := feeData["rows"].([]any)[0].(map[string]any)
	if row["xh"] != "<redacted>" || row["xm"] != "<redacted>" || row["sfxmmc"] != "学费" {
		t.Fatalf("financial private fields were not redacted: %#v", row)
	}
	if got := seen["/CWCX_V2/sfcx/xs/sfzz"]; got["search_s_sfqjdm"] != "26" || got["search_s_qjf"] != "1" || got["page.number"] != "2" || got["page.size"] != "25" {
		t.Fatalf("fee request mapping failed: %#v", got)
	}

	_ = runIssueJSON(t, "finance", "fee-details", "--page-size", "50")
	_ = runIssueJSON(t, "finance-query", "aid")
	_ = runIssueJSON(t, "finance-query", "exemptions")
	_ = runIssueJSON(t, "finance-query", "refunds")
	_ = runIssueJSON(t, "finance-query", "deferred")
	income := runIssueJSON(t, "finance-query", "income", "--year", "2026")
	if income["year"] != "2026" || seen["/CWCX_V2/cwcx/gz/bwsrmx"]["nian"] != "2026" {
		t.Fatalf("income year mapping failed: %#v seen=%#v", income, seen["/CWCX_V2/cwcx/gz/bwsrmx"])
	}
	_ = runIssueJSON(t, "finance-query", "income-chart")
	_ = runIssueJSON(t, "finance-query", "incoming", "--keyword", "摘要")
	loans := runIssueJSON(t, "finance-query", "unconfirmed-loans", "--keyword", "贷款")
	if loans["keyword"] != "贷款" || seen["/CWCX_V2/cwcx/ggcx/wqrsyddk"]["search_s_key"] != "贷款" {
		t.Fatalf("loan keyword mapping failed: %#v seen=%#v", loans, seen["/CWCX_V2/cwcx/ggcx/wqrsyddk"])
	}

	overview := runIssueJSON(t, "finance-query", "overview")
	if overview["operation"] != "overview" || overview["confirmed"] != true {
		t.Fatalf("finance overview was not confirmed: %#v", overview)
	}
	encoded, _ := json.Marshal(map[string]any{"fees": fees, "overview": overview})
	if strings.Contains(string(encoded), "202401150107") || strings.Contains(string(encoded), "测试用户") {
		t.Fatalf("financial private fields leaked: %s", encoded)
	}
}

func TestFinanceQueryReportsRemoteFailure(t *testing.T) {
	requestErr := financeQueryFailure(map[string]any{"success": false, "code": "401", "msg": "请先登录"})
	if requestErr == nil || requestErr.Code != "login_required" || requestErr.Message != "请先登录" {
		t.Fatalf("remote finance failure was not classified: %#v", requestErr)
	}
}
