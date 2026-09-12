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

func TestTransportMobileProtocolAndSemanticModels(t *testing.T) {
	const token = "transport-token-secret"
	var loginBody map[string]any
	var tableBodies []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/authenticate":
			if err := json.NewDecoder(request.Body).Decode(&loginBody); err != nil {
				t.Errorf("login body is not JSON: %v", err)
			}
			_, _ = fmt.Fprint(writer, `{"code":200,"data":{"token":"`+token+`","loginType":"account"},"message":"请求成功","success":true}`)
		case "/api/user/getUserInfo":
			if request.Header.Get("token") != token {
				t.Errorf("missing transport token: %q", request.Header.Get("token"))
			}
			_, _ = fmt.Fprint(writer, `{"code":200,"data":{"_id":"u-1","code":"T001","name":"交通老师","jobNumber":"T001","title":["讲师"],"phone":"13800138000","numPending":3,"roles":["teacher"],"permissions":["defense:view"]},"success":true}`)
		case "/api/user/getPendingCount":
			_, _ = fmt.Fprint(writer, `{"code":200,"data":{"numPending":3},"success":true}`)
		case "/api/user/changepwd":
			if request.Method != http.MethodPost || request.Header.Get("token") != token {
				t.Errorf("change-password request was not authenticated POST: method=%s token=%q", request.Method, request.Header.Get("token"))
			}
			var body map[string]any
			if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
				t.Errorf("change-password body is not JSON: %v", err)
			}
			if body["loginType"] != "account" || body["old"] != "old-secret" || body["new"] != "new-secret" {
				t.Errorf("change-password body was not mapped: %#v", body)
			}
			_, _ = fmt.Fprint(writer, `{"code":200,"message":"修改成功","success":true}`)
		case "/api/system/dict":
			_, _ = fmt.Fprint(writer, `{"code":200,"data":[{"code":"Finance.Type","desc":"财务项目类别","value":[{"key":"research","title":"科学研究"}]},{"code":"Other","desc":"其他","value":[]}],"success":true}`)
		case "/api/table/defense", "/api/table/finance":
			if request.Method != http.MethodPut {
				t.Errorf("table request used %s", request.Method)
			}
			var body map[string]any
			if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
				t.Errorf("table body is not JSON: %v", err)
			}
			tableBodies = append(tableBodies, body)
			if request.URL.Path == "/api/table/defense" {
				_, _ = fmt.Fprint(writer, `{"code":200,"data":{"total":1,"records":[{"_id":"d-1","name":"博士答辩组","isGenerated":true,"stat":{"file":2,"expert":3,"student":1},"creater":{"name":"交通老师"},"version":"2026-09-12"}]},"success":true}`)
			} else {
				_, _ = fmt.Fprint(writer, `{"code":200,"data":{"total":1,"records":[{"_id":"f-1","code":"F-1","name":"科研项目","ownname":"交通老师","ficode":"R-1","stage":"开题","status":"报账中","stat":{"total":1000,"fee":200,"left":800,"allow":900,"allowleft":700,"feerate":20,"allowrate":22.2}}]},"success":true}`)
			}
		case "/api/table/fitem":
			if request.Method != http.MethodPut || request.Header.Get("token") != token {
				t.Errorf("finance-item request was not authenticated PUT: method=%s token=%q", request.Method, request.Header.Get("token"))
			}
			var body map[string]any
			if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
				t.Errorf("finance-item body is not JSON: %v", err)
			}
			tableBodies = append(tableBodies, body)
			_, _ = fmt.Fprint(writer, `{"code":200,"data":{"total":1,"records":[{"_id":"fi-1","project":{"_id":"f-1","name":"科研项目","fitype":"纵向","status":"执行中"},"money":120.5,"type":"差旅","dir":"支出","remark":"现场调研","creater":{"name":"交通老师","code":"T001"},"dateCreate":"2026-09-12","file":[{"name":"票据","size":12}]}]},"success":true}`)
		case "/api/defensesop/d-1":
			_, _ = fmt.Fprint(writer, `{"code":200,"data":{"_id":"d-1","name":"博士答辩组","expert":[{"name":"专家一","type":"主席"}],"student":[{"code":"S-1","name":"学生一","title":"论文"}],"stat":{"expert":1,"student":1}},"success":true}`)
		case "/api/verifys":
			var body map[string]any
			if err := json.NewDecoder(request.Body).Decode(&body); err != nil || body["phone"] != "13800138000" {
				t.Errorf("unexpected verification body: %#v %v", body, err)
			}
			_, _ = fmt.Fprint(writer, `{"code":200,"message":"发送成功","success":true}`)
		case "/api/user/logout":
			_, _ = fmt.Fprint(writer, `{"code":200,"message":"退出成功","success":true}`)
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	root := t.TempDir()
	cookieFile := filepath.Join(root, "transport.cookies.txt")
	t.Setenv("CSUST_BASE_URL", server.URL)
	t.Setenv("CSUST_COOKIE_FILE", cookieFile)

	login := runIssueJSON(t, "transport-mobile", "login", "--username", "T001", "--password", "secret")
	tokenFile := strings.TrimSuffix(cookieFile, ".cookies.txt") + ".token"
	if login["confirmed"] != true || login["token_file"] != tokenFile || strings.Contains(string(mustMarshalIssue(login)), "secret") || strings.Contains(string(mustMarshalIssue(login)), token) {
		t.Fatalf("login evidence or redaction failed: %#v", login)
	}
	if loginBody["code"] != "T001" || loginBody["pwd"] != "secret" || loginBody["rememberMe"] != false {
		t.Fatalf("login payload was not mapped: %#v", loginBody)
	}

	profile := runIssueJSON(t, "transport-mobile", "profile")
	user := profile["data"].(map[string]any)
	if user["account"] != "T001" || user["name"] != "交通老师" || profile["confirmed"] != true {
		t.Fatalf("profile model was not mapped: %#v", profile)
	}
	pending := runIssueJSON(t, "transport-mobile", "pending")
	if pending["pending_count"] != float64(3) {
		t.Fatalf("pending count was not mapped: %#v", pending)
	}
	dictionaries := runIssueJSON(t, "transport-mobile", "dictionaries", "--code", "Finance.Type")
	if len(dictionaries["data"].([]any)) != 1 || dictionaries["data"].([]any)[0].(map[string]any)["code"] != "Finance.Type" {
		t.Fatalf("dictionary filter/model failed: %#v", dictionaries)
	}

	defenses := runIssueJSON(t, "transport-mobile", "defenses", "--keyword", "博士", "--page", "2", "--page-size", "5")
	defense := defenses["data"].([]any)[0].(map[string]any)
	if defense["id"] != "d-1" || defense["is_generated"] != true || defense["expert_count"] != float64(3) || defenses["total"] != float64(1) {
		t.Fatalf("defense model failed: %#v", defenses)
	}
	detail := runIssueJSON(t, "transport-mobile", "defense", "--id", "d-1")
	if len(detail["defense"].(map[string]any)["experts"].([]any)) != 1 || len(detail["defense"].(map[string]any)["students"].([]any)) != 1 {
		t.Fatalf("defense detail model failed: %#v", detail)
	}

	finances := runIssueJSON(t, "transport-mobile", "finances", "--keyword", "科研")
	finance := finances["data"].([]any)[0].(map[string]any)
	if finance["id"] != "f-1" || finance["project_code"] != "R-1" || finance["balance"] != float64(800) {
		t.Fatalf("finance model failed: %#v", finances)
	}
	financeDetail := runIssueJSON(t, "transport-mobile", "finance", "--id", "f-1")
	if financeDetail["finance"].(map[string]any)["owner"] != "交通老师" {
		t.Fatalf("finance detail model failed: %#v", financeDetail)
	}
	financeItems := runIssueJSON(t, "transport-mobile", "finance-items", "--keyword", "现场")
	financeItem := financeItems["data"].([]any)[0].(map[string]any)
	if financeItem["id"] != "fi-1" || financeItem["project_name"] != "科研项目" || financeItem["amount"] != 120.5 || financeItem["direction"] != "支出" {
		t.Fatalf("finance-item model failed: %#v", financeItems)
	}
	financeItemDetail := runIssueJSON(t, "transport-mobile", "finance-item", "--id", "fi-1")
	if financeItemDetail["finance_item"].(map[string]any)["remark"] != "现场调研" {
		t.Fatalf("finance-item detail model failed: %#v", financeItemDetail)
	}
	changedPassword := runIssueJSON(t, "transport-mobile", "change-password", "--current-password", "old-secret", "--new-password", "new-secret", "--password-confirm", "new-secret", "--yes")
	if changedPassword["operation"] != "change-password" || changedPassword["submitted"] != true || changedPassword["confirmed"] != true || strings.Contains(string(mustMarshalIssue(changedPassword)), "secret") {
		t.Fatalf("change-password result or redaction failed: %#v", changedPassword)
	}

	code := runIssueJSON(t, "transport-mobile", "send-code", "--phone", "13800138000", "--yes")
	if code["phone"] != "13800138000" || code["submitted"] != true || code["confirmed"] != true {
		t.Fatalf("verification-code result failed: %#v", code)
	}
	if len(tableBodies) != 5 {
		t.Fatalf("unexpected table calls: %d", len(tableBodies))
	}
	defenseBody := tableBodies[0]
	if defenseBody["paginator"].(map[string]any)["page"] != float64(2) || defenseBody["fuzzyFilter"].([]any)[0].(map[string]any)["name"].(map[string]any)["$regex"] != "博士" {
		t.Fatalf("defense filters were not mapped: %#v", defenseBody)
	}
	financeItemBody := tableBodies[3]
	if financeItemBody["fuzzyFilter"].([]any)[1].(map[string]any)["remark"].(map[string]any)["$regex"] != "现场" {
		t.Fatalf("finance-item filters were not mapped: %#v", financeItemBody)
	}
	financeItemDetailBody := tableBodies[4]
	if financeItemDetailBody["filter"].(map[string]any)["_id"] != "fi-1" {
		t.Fatalf("finance-item detail filter was not mapped: %#v", financeItemDetailBody)
	}

	logout := runIssueJSON(t, "transport-mobile", "logout")
	if logout["submitted"] != true || logout["logged_out"] != true || logout["confirmed"] != true {
		t.Fatalf("logout result failed: %#v", logout)
	}
	if _, err := os.Stat(tokenFile); !os.IsNotExist(err) {
		t.Fatalf("transport token file was not removed: %v", err)
	}
}

func TestTransportMobileSendCodeRequiresConfirmation(t *testing.T) {
	handled, _, _, code, err := (NativeSite{}).Run(t.Context(), []string{"transport-mobile", "send-code", "--phone", "13800138000"}, false)
	if err != nil || !handled || code != 2 {
		t.Fatalf("send-code without confirmation was not rejected: handled=%v code=%d err=%v", handled, code, err)
	}
}

func TestTransportMobileChangePasswordRequiresConfirmation(t *testing.T) {
	handled, _, _, code, err := (NativeSite{}).Run(t.Context(), []string{"transport-mobile", "change-password", "--new-password", "new", "--password-confirm", "new"}, false)
	if err != nil || !handled || code != 2 {
		t.Fatalf("change-password without confirmation was not rejected: handled=%v code=%d err=%v", handled, code, err)
	}
}

func TestTransportMobileAdditionalSemanticModels(t *testing.T) {
	if note := transportMobileNote(map[string]any{"_id": "n-1", "name": "请示", "dateModified": "2026-09-12", "participants": []any{"u-1"}}); note["last_reply"] != "2026-09-12" {
		t.Fatalf("note model failed: %#v", note)
	}
	access := transportMobileAccessRecord(map[string]any{"employeeCode": "T001", "personName": "交通老师", "departmentName": "交通学院", "deviceAlias": "门禁一", "eventDescription": "进校", "eventTime": "2026-09-12", "verifyModeName": "刷卡"})
	if access["employee_code"] != "T001" || access["verify_mode"] != "刷卡" {
		t.Fatalf("access record model failed: %#v", access)
	}
	achievement := transportMobileAchievement(map[string]any{"student": map[string]any{"code": "S-1", "name": "学生一"}, "awardLevel": "校级"})
	if achievement["student_code"] != "S-1" || achievement["award_level"] != "校级" {
		t.Fatalf("achievement model failed: %#v", achievement)
	}
	kpi := transportMobileKPI(map[string]any{"owner": map[string]any{"name": "交通老师"}, "type": "教学"})
	if kpi["owner"].(map[string]any)["name"] != "交通老师" || kpi["kpi_type"] != "教学" {
		t.Fatalf("KPI model failed: %#v", kpi)
	}
	notice := transportMobileNotice(map[string]any{"code": "N-1", "title": "通知", "type": "校内"})
	if notice["title"] != "通知" || notice["notice_type"] != "校内" {
		t.Fatalf("notice model failed: %#v", notice)
	}
	workflow := transportMobileWorkflow(map[string]any{"current": map[string]any{"node": "院系审核", "user": map[string]any{"name": "审批人"}}})
	if workflow["current_node"] != "院系审核" || workflow["approver"].(map[string]any)["name"] != "审批人" {
		t.Fatalf("workflow model failed: %#v", workflow)
	}
	vacation := transportMobileVacation(map[string]any{"reason": "出差", "days": 2})
	if vacation["reason"] != "出差" || vacation["days"] != 2 {
		t.Fatalf("vacation model failed: %#v", vacation)
	}
}

func TestTransportMobileRoomsAndAttendance(t *testing.T) {
	const token = "rooms-test-token"
	var buildingBody, attendanceBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		if request.Header.Get("token") != token {
			t.Errorf("missing rooms token: %q", request.Header.Get("token"))
		}
		switch request.URL.Path {
		case "/api/table/building":
			if request.Method != http.MethodPut || json.NewDecoder(request.Body).Decode(&buildingBody) != nil {
				t.Fatalf("rooms query was not an authenticated JSON PUT")
			}
			_, _ = fmt.Fprint(writer, `{"code":200,"success":true,"data":{"total":1,"records":[{"_id":"b-1","code":"实验楼","name":"实验楼","floors":[{"_id":"f-1","code":"3","block":"A","rooms":[{"_id":"r-1","code":"301","seats":[{"code":"01","status":"在用"},{"code":"02","status":"可用"}]}]}]}]}}`)
		case "/api/accessrecordsop":
			if request.Method != http.MethodPut || json.NewDecoder(request.Body).Decode(&attendanceBody) != nil {
				t.Fatalf("attendance query was not an authenticated JSON PUT")
			}
			_, _ = fmt.Fprint(writer, `{"code":200,"success":true,"data":{"T002":"2026-09-13T09:02:00Z","T001":"2026-09-13T09:01:00Z"}}`)
		case "/api/zklink":
			if request.Method != http.MethodGet {
				t.Fatalf("room sync used %s", request.Method)
			}
			_, _ = fmt.Fprint(writer, `{"code":200,"success":true,"data":{"totalCount":2,"inserted":1,"updated":1}}`)
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	t.Setenv("CSUST_BASE_URL", server.URL)
	t.Setenv("CSUST_COOKIE_FILE", filepath.Join(t.TempDir(), "transport.cookies.txt"))

	rooms := runIssueJSON(t, "transport-mobile", "rooms", "--access-token", token, "--keyword", "实验", "--page", "2", "--page-size", "5")
	room := rooms["data"].([]any)[0].(map[string]any)
	if room["id"] != "b-1" || room["floor_count"] != float64(1) || room["room_count"] != float64(1) || room["seat_count"] != float64(2) {
		t.Fatalf("room tree model failed: %#v", rooms)
	}
	if buildingBody["paginator"].(map[string]any)["page"] != float64(2) || buildingBody["fuzzyFilter"].([]any)[0].(map[string]any)["name"].(map[string]any)["$regex"] != "实验" {
		t.Fatalf("room filters were not mapped: %#v", buildingBody)
	}

	attendance := runIssueJSON(t, "transport-mobile", "room-attendance", "--access-token", token, "--date", "2026-09-13")
	if attendanceBody["date"] != "2026-09-13" {
		t.Fatalf("attendance date was not mapped: %#v", attendanceBody)
	}
	records := attendance["records"].([]any)
	if len(records) != 2 || records[0].(map[string]any)["employee_code"] != "T001" || records[1].(map[string]any)["employee_code"] != "T002" {
		t.Fatalf("attendance records were not sorted or mapped: %#v", attendance)
	}

	if handled, _, _, code, err := (NativeSite{}).Run(t.Context(), []string{"transport-mobile", "room-sync", "--access-token", token}, false); err != nil || !handled || code != 2 {
		t.Fatalf("room sync without confirmation was not rejected: handled=%v code=%d err=%v", handled, code, err)
	}
	syncResult := runIssueJSON(t, "transport-mobile", "room-sync", "--access-token", token, "--yes")
	if syncResult["submitted"] != true || syncResult["confirmed"] != true || syncResult["data"].(map[string]any)["inserted"] != float64(1) {
		t.Fatalf("room sync was not confirmed: %#v", syncResult)
	}
}
