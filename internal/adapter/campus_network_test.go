package adapter

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCampusNetworkQueriesAndMutations(t *testing.T) {
	state := struct {
		phone  string
		email  string
		macs   []string
		online bool
	}{phone: "13800138000", email: "old@example.com", macs: []string{"AABBCCDDEEFF", ""}, online: true}
	var periodBodies []string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		switch request.URL.Path {
		case "/Self/nav_main":
			_, _ = fmt.Fprint(writer, `<html><body><h1>用户自助服务系统</h1></body></html>`)
		case "/Self/nav_getUserInfo":
			_, _ = fmt.Fprintf(writer, `<html><body><table><tr><td>账号：</td><td>student</td></tr><tr><td>套餐：</td><td>B3</td></tr><tr><td>联系电话</td><td>%s</td></tr><tr><td>电子邮箱</td><td>%s</td></tr><tr><td>状态</td><td>正常 在线</td></tr></table></body></html>`, state.phone, state.email)
		case "/Self/nav_changeUserInfo":
			_, _ = fmt.Fprintf(writer, `<html><body><form action="ChangeUserMessageAction.action" method="post"><input name="user.flduserrealname" value="测试用户"><input name="user.tblregisteconfig.fldcheckcode" value="safe"><input name="user.flduserphone" value="%s"><input name="user.flduseremail" value="%s"><input name="user.fldinstalllocal" value="宿舍"></form></body></html>`, state.phone, state.email)
		case "/Self/ChangeUserMessageAction.action":
			if err := request.ParseForm(); err != nil {
				t.Fatal(err)
			}
			state.phone = request.Form.Get("user.flduserphone")
			state.email = request.Form.Get("user.flduseremail")
			_, _ = fmt.Fprint(writer, `<html><script>alert('修改成功')</script></html>`)
		case "/Self/MonthPayAction", "/Self/UserLoginLogAction", "/Self/AdminOpLogAction", "/Self/UserPayAction":
			if err := request.ParseForm(); err != nil {
				t.Fatal(err)
			}
			periodBodies = append(periodBodies, request.URL.Path+"?"+url.Values(request.Form).Encode())
			if request.URL.Path == "/Self/MonthPayAction" {
				_, _ = fmt.Fprint(writer, `<html><table><tr><th>账单开始时间</th><th>使用流量(MB)</th></tr><tr><td>2026-01-01</td><td>12</td></tr></table></html>`)
			} else {
				_, _ = fmt.Fprint(writer, `<html><table><tr><th>时间</th><th>金额</th></tr><tr><td>2026-09-12</td><td>0</td></tr></table></html>`)
			}
		case "/Self/nav_SetMac":
			_, _ = fmt.Fprintf(writer, `<html><body><form action="setMacAction" method="post"><input name="macs" value="%s"><input name="macs" value="%s"><input name="macs" value=""><input name="macs" value=""><input name="macs" value=""></form></body></html>`, state.macs[0], state.macs[1])
		case "/Self/setMacAction":
			if err := request.ParseForm(); err != nil {
				t.Fatal(err)
			}
			state.macs = append([]string(nil), request.Form["macs"]...)
			_, _ = fmt.Fprint(writer, `<html><script>alert('设置成功')</script></html>`)
		case "/Self/nav_servicedefaultbook":
			_, _ = fmt.Fprint(writer, campusNetworkTestPackagePage())
		case "/Self/selfservicebookAction.action":
			if err := request.ParseForm(); err != nil {
				t.Fatal(err)
			}
			if request.Form.Get("fldname") != "1" || request.Form.Get("fldcooperation") != "2" || request.Form.Get("fldbandwidth") != "5" {
				t.Fatalf("package fields were not mapped: %v", request.Form)
			}
			_, _ = fmt.Fprint(writer, `<html><script>alert('预约成功')</script></html>`)
		case "/Self/nav_changePsw":
			_, _ = fmt.Fprint(writer, `<html><body><form id="ChangePswAction"><input name="user.flduserpassword" type="password" value="server-current"><input name="user.fldmd5hehai" type="password"><input name="user.fldextend" type="password"></form></body></html>`)
		case "/Self/ChangePswAction.action":
			if err := request.ParseForm(); err != nil {
				t.Fatal(err)
			}
			if request.Form.Get("user.flduserpassword") != "server-current" || request.Form.Get("user.fldmd5hehai") != request.Form.Get("user.fldextend") {
				t.Fatalf("password fields were not mapped: %v", request.Form)
			}
			writer.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(writer, `{"outmessage":"true"}`)
		case "/Self/nav_offLine":
			if state.online {
				_, _ = fmt.Fprint(writer, `<html><table><tr><th>在线IPv4</th><th>在线IPv6</th><th>MAC</th><th>主机名</th><th>终端类型</th><th>终端型号</th><th>操作</th></tr><tr><td>10.0.0.1</td><td></td><td>AABBCCDDEEFF</td><td style="display:none;">session-1</td><td>host</td><td>PC端</td><td>Unknown</td><td><a>强制离线</a></td></tr></table></html>`)
			} else {
				_, _ = fmt.Fprint(writer, `<html><table><tr><th>在线IPv4</th><th>在线IPv6</th><th>MAC</th><th>主机名</th><th>终端类型</th><th>终端型号</th><th>操作</th></tr></table></html>`)
			}
		case "/Self/tooffline":
			state.online = false
			writer.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(writer, `{"outmessage":"true"}`)
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	root := t.TempDir()
	t.Setenv("CSUST_BASE_URL", server.URL)
	t.Setenv("CSUST_COOKIE_FILE", filepath.Join(root, "cookies.txt"))

	profile := runIssueJSON(t, "campus-network", "profile")
	encoded := fmt.Sprint(profile)
	if strings.Contains(encoded, state.phone) || strings.Contains(encoded, state.email) {
		t.Fatalf("profile response leaked private fields: %s", encoded)
	}
	bills := runIssueJSON(t, "campus-network", "bills", "--year", "2026")
	if bills["operation"] != "bills" {
		t.Fatalf("bill query failed: %#v", bills)
	}
	usage := runIssueJSON(t, "campus-network", "usage", "--from", "2026-09-01", "--to", "2026-09-12")
	if usage["operation"] != "usage" {
		t.Fatalf("usage query failed: %#v", usage)
	}
	_ = runIssueJSON(t, "campus-network", "payments", "--month", "2026-09")
	_ = runIssueJSON(t, "campus-network", "operations", "--date", "2026-09-12")
	if len(periodBodies) != 4 || !strings.Contains(strings.Join(periodBodies, "\n"), "type=4") || !strings.Contains(strings.Join(periodBodies, "\n"), "type=3") {
		t.Fatalf("period protocol was not mapped: %v", periodBodies)
	}

	updated := runIssueJSON(t, "campus-network", "update-profile", "--phone", "13800000000", "--email", "new@example.com", "--yes")
	if updated["confirmed"] != true {
		t.Fatalf("profile update was not read back: %#v", updated)
	}
	changedPassword := runIssueJSON(t, "campus-network", "change-password", "--new-password", "NewSecret!1", "--password-confirm", "NewSecret!1", "--yes")
	if changedPassword["confirmed"] != true {
		t.Fatalf("password change was not confirmed: %#v", changedPassword)
	}
	packageResult := runIssueJSON(t, "campus-network", "package", "--network", "出口2", "--bandwidth", "50M", "--yes")
	if packageResult["confirmed"] != true {
		t.Fatalf("package reservation was not confirmed: %#v", packageResult)
	}
	devices := runIssueJSON(t, "campus-network", "devices")
	if devices["confirmed"] != true {
		t.Fatalf("device query failed: %#v", devices)
	}
	setDevices := runIssueJSON(t, "campus-network", "set-devices", "--mac", "11:2233:445566", "--yes")
	if setDevices["confirmed"] != true || state.macs[0] != "112233445566" {
		t.Fatalf("device binding was not read back: %#v state=%v", setDevices, state.macs)
	}
	if unbound := runIssueJSON(t, "campus-network", "unbind-device", "--index", "1", "--yes"); unbound["confirmed"] != true || state.macs[0] != "" {
		t.Fatalf("device unbind was not confirmed: %#v state=%v", unbound, state.macs)
	}
	if disconnected := runIssueJSON(t, "campus-network", "disconnect", "--index", "1", "--yes"); disconnected["confirmed"] != true {
		t.Fatalf("online device disconnect was not confirmed: %#v", disconnected)
	}
	if _, err := os.Stat(filepath.Join(root, "cookies.txt")); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
}

func campusNetworkTestPackagePage() string {
	return `<html><body><form id="form1" action="selfservicebookAction.action" method="post"><select id="userpackageid" name="fldname"><option value="1" selected>学生</option></select><select id="cooid" name="fldcooperation"><option value="2" selected>出口2</option></select><select id="bandwidthid" name="fldbandwidth"><option value="6" selected>20M</option><option value="5">50M</option></select><select id="loginwayid" name="fldloginway"><option value="3" selected>有线+无线</option></select><select id="typeid" name="fldtype"><option value="2" selected>包学期</option></select><select id="loginnumbersid" name="fldloginnumber"><option value="1" selected>1终端</option></select><select id="costsid" name="fldcost"><option value="16" selected>200元</option></select><input type="hidden" name="name" value="学生出口220M有线+无线包学期1终端200元"></form></body></html>`
}
