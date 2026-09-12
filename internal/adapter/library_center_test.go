package adapter

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestLibraryCenterResourceAvailabilityAndPersonalQueries(t *testing.T) {
	var deviceQuery, reserveAction string
	reserved := false
	contactPhone, contactEmail := "", "old@example.com"
	contactNotify := true
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/clientweb/xcus/ic2/Default.aspx":
			writer.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = writer.Write([]byte(`<li class="it" it="devcls" url="../a/dftdetail.aspx?classKind=1&amp;id=103235280&amp;name=%e5%9f%b9%e8%ae%ad%e5%ae%A4"><a><span>培训室</span></a></li><li class="it" it="lab_100455336" url="../a/roomdetail.aspx?classKind=8&amp;roomId=100455344&amp;roomName=%e9%98%85%e8%a7%88%e5%ae%A4%e4%b8%80B205"><a><span>阅览室一B205</span></a></li>`))
		case "/ClientWeb/pro/ajax/device.aspx":
			deviceQuery = request.URL.Query().Encode()
			writer.Header().Set("Content-Type", "application/json")
			_, _ = writer.Write([]byte(`{"ret":1,"act":"get_rsv_sta","msg":"ok","data":[{"id":"7_8","name":"测试座位","devId":"7","kindId":"6","roomId":"8","labName":"云塘馆","occupy":false,"freeTime":180,"ts":[],"ops":[{"state":"open"}]}],"ext":null}`))
		case "/ClientWeb/pro/ajax/reserve.aspx":
			reserveAction = request.URL.Query().Get("act")
			writer.Header().Set("Content-Type", "application/json")
			switch reserveAction {
			case "set_resv":
				if request.URL.Query().Get("dev_id") != "7" || request.URL.Query().Get("start") != "2026-09-12 09:00" || request.URL.Query().Get("mb_list") != "$202401150107,member2" {
					t.Errorf("reservation fields were not mapped: %s", request.URL.RawQuery)
				}
				reserved = true
				_, _ = writer.Write([]byte(`{"ret":1,"act":"set_resv","msg":"ok","data":{"id":"99"},"ext":null}`))
			case "del_resv":
				reserved = false
				_, _ = writer.Write([]byte(`{"ret":1,"act":"del_resv","msg":"ok","data":null,"ext":null}`))
			case "get_my_servertime":
				_, _ = writer.Write([]byte(`{"ret":1,"act":"get_my_servertime","msg":"ok","data":"2026-09-12 10:00:00","ext":null}`))
			case "get_my_resv":
				if reserved {
					_, _ = writer.Write([]byte(`{"ret":1,"act":"get_my_resv","msg":"ok","data":[{"id":"99"}],"ext":null}`))
				} else {
					_, _ = writer.Write([]byte(`{"ret":1,"act":"get_my_resv","msg":"ok","data":[],"ext":null}`))
				}
			default:
				http.NotFound(writer, request)
			}
		case "/ClientWeb/pro/ajax/login.aspx":
			if request.URL.Query().Get("act") != "init_acc" {
				http.NotFound(writer, request)
				return
			}
			writer.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(writer, `{"ret":1,"act":"init_acc","msg":"ok","data":{"id":"account","accno":"accno","name":"测试用户","phone":%q,"email":%q,"dept":"院系设置","receive":%t,"credit":[["研修间","300","300",""]]},"ext":null}`, contactPhone, contactEmail, contactNotify)
		case "/ClientWeb/pro/ajax/center.aspx":
			if request.URL.Query().Get("act") != "get_History_resv" {
				http.NotFound(writer, request)
				return
			}
			writer.Header().Set("Content-Type", "application/json")
			_, _ = writer.Write([]byte(`{"ret":1,"act":"get_History_resv","msg":"<tbody><tr><td>2026-09-11</td><td>研修间</td><td>取消</td><td>已处理</td><td>0</td><td>无</td></tr></tbody>","data":null,"ext":null}`))
		case "/ClientWeb/pro/ajax/account.aspx":
			if request.URL.Query().Get("act") != "update_contact" {
				http.NotFound(writer, request)
				return
			}
			contactPhone = request.URL.Query().Get("phone")
			contactEmail = request.URL.Query().Get("email")
			contactNotify = request.URL.Query().Get("note_alert") == "true"
			writer.Header().Set("Content-Type", "application/json")
			_, _ = writer.Write([]byte(`{"ret":1,"act":"update_contact","msg":"ok","data":null,"ext":null}`))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	t.Setenv("CSUST_BASE_URL", server.URL)
	t.Setenv("CSUST_COOKIE_FILE", filepath.Join(t.TempDir(), "library.cookies.txt"))

	resources := runIssueJSON(t, "library-center", "resources")
	if resources["total"] != float64(2) {
		t.Fatalf("resource directory was not parsed: %#v", resources)
	}

	availability := runIssueJSON(t, "library-center", "availability", "--resource", "座位", "--room", "阅览室一B205", "--date", "2026-09-12", "--from", "09:00", "--to", "10:00")
	if availability["total"] != float64(1) || availability["resource"].(map[string]any)["room_id"] != "100455344" {
		t.Fatalf("seat availability model failed: %#v", availability)
	}
	if !strings.Contains(deviceQuery, "room_id=100455344") || !strings.Contains(deviceQuery, "fr_start=09%3A00") || !strings.Contains(deviceQuery, "fr_end=10%3A00") {
		t.Fatalf("seat availability request mapping failed: %s", deviceQuery)
	}
	row := availability["data"].([]any)[0].(map[string]any)
	if row["name"] != "测试座位" || row["free_minutes"] != float64(180) || row["occupied"] != false {
		t.Fatalf("availability row model failed: %#v", row)
	}
	profile := runIssueJSON(t, "library-center", "profile")
	profileData := profile["data"].(map[string]any)
	if profileData["id"] != "account" || profileData["email"] != "old@example.com" || profileData["receive"] != true {
		t.Fatalf("profile query failed: %#v", profile)
	}
	history := runIssueJSON(t, "library-center", "credit-history", "--status", "history", "--days", "30")
	historyRows := history["data"].([]any)
	if history["status"] != "OVER" || history["days"] != float64(30) || len(historyRows) != 1 || historyRows[0].(map[string]any)["location"] != "研修间" {
		t.Fatalf("credit history query failed: %#v", history)
	}
	updated := runIssueJSON(t, "library-center", "update-contact", "--phone", "13800000000", "--email", "new@example.com", "--notify", "false", "--yes")
	updatedData := updated["data"].(map[string]any)
	if updated["submitted"] != true || updated["confirmed"] != true || updatedData["phone"] != "13800000000" || updatedData["email"] != "new@example.com" || updatedData["receive"] != false {
		t.Fatalf("contact update/readback failed: %#v", updated)
	}

	reservations := runIssueJSON(t, "library-center", "reservations")
	if reserveAction != "get_my_resv" || len(reservations["data"].([]any)) != 0 {
		t.Fatalf("personal reservation query failed: %#v action=%s", reservations, reserveAction)
	}
	serverTime := runIssueJSON(t, "library-center", "server-time")
	if serverTime["data"] != "2026-09-12 10:00:00" || reserveAction != "get_my_servertime" {
		t.Fatalf("server time query failed: %#v action=%s", serverTime, reserveAction)
	}
	reservedResult := runIssueJSON(t, "library-center", "reserve", "--resource", "座位", "--room", "阅览室一B205", "--item", "测试座位", "--date", "2026-09-12", "--from", "09:00", "--to", "10:00", "--member", "202401150107", "--member", "member2", "--yes")
	if reservedResult["submitted"] != true || reservedResult["confirmed"] != true || !reserved {
		t.Fatalf("reservation write/readback failed: %#v reserved=%v", reservedResult, reserved)
	}
	cancelled := runIssueJSON(t, "library-center", "cancel", "--id", "99", "--yes")
	if cancelled["submitted"] != true || cancelled["confirmed"] != true || reserved {
		t.Fatalf("reservation cancellation/readback failed: %#v reserved=%v", cancelled, reserved)
	}
}
