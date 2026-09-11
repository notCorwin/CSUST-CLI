package adapter

import (
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
)

func TestRecruitmentProtocolAndSemanticModels(t *testing.T) {
	ciphertext, err := recruitmentEncrypt([]byte(`{"functionId":"ZP1000000009"}`))
	if err != nil {
		t.Fatalf("SM2 encryption failed: %v", err)
	}
	decoded, err := hex.DecodeString(ciphertext)
	if err != nil || len(decoded) < 97 || decoded[0] != 4 {
		t.Fatalf("unexpected SM2 ciphertext: %d bytes, %v", len(decoded), err)
	}

	responses := []string{}
	responseIndex := 0
	var requests []url.Values
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != recruitmentEndpoint || request.Method != http.MethodPost {
			http.NotFound(writer, request)
			return
		}
		if err := request.ParseForm(); err != nil {
			t.Errorf("invalid recruitment form: %v", err)
			return
		}
		encoded := request.Form.Get("__xml")
		ciphertext, decodeErr := hex.DecodeString(encoded)
		if decodeErr != nil || len(ciphertext) < 97 || ciphertext[0] != 4 {
			t.Errorf("invalid recruitment SM2 form field: %d bytes, %v", len(ciphertext), decodeErr)
		}
		if request.Form.Get("__type") != "extTrans" {
			t.Errorf("unexpected recruitment form type: %q", request.Form.Get("__type"))
		}
		requests = append(requests, request.Form)
		if responseIndex >= len(responses) {
			writer.WriteHeader(http.StatusInternalServerError)
			_, _ = writer.Write([]byte(`{"succeed":false,"message":"unexpected call"}`))
			return
		}
		writer.Header().Set("Content-Type", "text/xml; charset=UTF-8")
		_, _ = fmt.Fprint(writer, responses[responseIndex])
		responseIndex++
	}))
	defer server.Close()
	t.Setenv("CSUST_BASE_URL", server.URL)
	t.Setenv("CSUST_COOKIE_FILE", filepath.Join(t.TempDir(), "cookies.txt"))
	setResponses := func(values ...string) {
		responses = values
		responseIndex = 0
	}

	setResponses(
		`{"functionId":"ZP1000000009","succeed":true,"return_data":{"portal":"ok"}}`,
		`{"functionId":"ZP1000000010","succeed":true,"return_data":{"channels":[{"id":"531","name":"专任教师自主招聘","hireChannel":"08","link":"/hire/faculty","params":{"tab":"all"}}]}}`,
		`{"functionId":"ZP1000000002","succeed":true,"return_data":{"pageTotal":1,"list":[{"id":"n1","title":"招聘公告","createtime":"2026-08-18","down":"false","days":-23,"content":"<p>公告正文</p>"}]}}`,
	)
	home := runIssueJSON(t, "recruitment", "home")
	homeData := home["data"].(map[string]any)
	channels := homeData["channels"].([]any)
	if home["service"] != recruitmentServiceName || len(channels) != 1 || channels[0].(map[string]any)["channel"] != "08" {
		t.Fatalf("unexpected recruitment home: %#v", home)
	}
	notices := homeData["notices"].([]any)
	if len(notices) != 1 || notices[0].(map[string]any)["title"] != "招聘公告" || notices[0].(map[string]any)["downloadable"] != false {
		t.Fatalf("recruitment notices were not mapped: %#v", homeData["notices"])
	}

	setResponses(`{"functionId":"ZP1000000001","succeed":true,"return_data":{"unitList":[{"name":"交通学院","value":"103E02","hidden":false}],"departmentList":[{"name":"交通工程系","value":"103E0201","hidden":false}]}}`)
	filters := runIssueJSON(t, "recruitment", "filters", "--channel", "faculty")
	filterData := filters["data"].(map[string]any)
	if filters["channel"] != "08" || filterData["units"].([]any)[0].(map[string]any)["id"] != "103E02" {
		t.Fatalf("recruitment filters were not mapped: %#v", filters)
	}

	positionList := `{"functionId":"ZP1000000001","succeed":true,"return_data":{"posTotal":1,"position_data":[{"z03a2":"202601001-ZJY01","z0321":"103E02","z0321Name":"交通学院","z0301":"hjev-position-1","z0351":"教学科研-202601001-ZJY01","z0383":"教学科研岗","z0315":"5","ypljl":"应聘","isNewPos":"1","isApplyedPos":"false"}],"columninfo":[{"itemid":"z0351","itemdesc":"岗位名称-岗位代码"}]}}`
	setResponses(
		`{"functionId":"ZP1000000001","succeed":true,"return_data":{"unitList":[{"name":"交通学院","value":"103E02","hidden":false}]}}`,
		positionList,
	)
	positions := runIssueJSON(t, "recruitment", "positions", "--channel", "faculty", "--unit", "交通学院", "--keyword", "教学科研", "--page", "2", "--page-size", "5")
	items := positions["data"].([]any)
	if positions["channel"] != "08" || positions["total"] != float64(1) || len(items) != 1 {
		t.Fatalf("unexpected recruitment positions: %#v", positions)
	}
	position := items[0].(map[string]any)
	if position["id"] != "hjev-position-1" || position["code"] != "202601001-ZJY01" || position["unit"] != "交通学院" || position["plan"] != float64(5) {
		t.Fatalf("recruitment position model was not mapped: %#v", position)
	}

	detailRow := `{"Z0101":"2026年自主招聘专任教师","Z0315":"5","Z0321":"交通学院","Z0351":"教学科研-202601001-ZJY01","total":"0","state":"0"}`
	setResponses(positionList, `{"functionId":"ZP1000000001","succeed":true,"return_data":{"hireChannel":"08","position_data":[`+detailRow+`],"columninfo":[]}}`)
	detail := runIssueJSON(t, "recruitment", "position", "--channel", "faculty", "--id", "202601001-ZJY01")
	detailPosition := detail["position"].(map[string]any)
	if detailPosition["id"] != "hjev-position-1" || detailPosition["batch"] != "2026年自主招聘专任教师" || detailPosition["plan"] != float64(5) {
		t.Fatalf("recruitment detail model was not mapped: %#v", detail)
	}
	if len(requests) != 8 || strings.Contains(string(mustMarshalIssue(home)), "__xml") {
		t.Fatalf("recruitment transport or public result leaked unexpectedly: calls=%d", len(requests))
	}
}
