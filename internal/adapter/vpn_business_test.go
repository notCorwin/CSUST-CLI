package adapter

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNativeVPNWorkbenchCommands(t *testing.T) {
	calls := map[string]int{}
	bodies := map[string]map[string]any{}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		calls[request.URL.Path]++
		if request.Method != http.MethodGet {
			var body map[string]any
			if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
				http.Error(writer, err.Error(), http.StatusBadRequest)
				return
			}
			bodies[request.URL.Path] = body
		}
		writer.Header().Set("Content-Type", "application/json")
		var response string
		switch request.URL.Path {
		case "/enclient/api/client/users/service/grouping":
			response = `{"code":200,"data":{"groupInfos":[{"groupId":"g1","groupName":"常用"}]}}`
		case "/enclient/api/client/user/addServiceGroup":
			response = `{"code":200,"data":{"groupId":"g2"}}`
		case "/enclient/api/client/user/deleteServiceGroup":
			response = `{"code":200,"data":{"deleted":true}}`
		case "/enclient/api/client/user/updateServiceGroup":
			response = `{"code":200,"data":{"updated":true}}`
		case "/enclient/api/client/user/updateServiceGroupSort":
			response = `{"code":200,"data":{"sorted":true}}`
		case "/enclient/api/client/user/updateServiceSort":
			response = `{"code":200,"data":{"sorted":true}}`
		case "/enclient/api/client/user/service/addToCustomGroup":
			response = `{"code":200,"data":{"saved":true}}`
		case "/enclient/api/client/user/service/pageUserService":
			response = `{"code":200,"data":{"total":1,"pageIndex":1,"pageSize":20,"data":[{"serviceId":"s1","serviceName":"教务系统"}]}}`
		case "/enclient/api/users/service/visit/list":
			response = `{"code":200,"data":[{"serviceId":"s1","serviceName":"教务系统"}]}`
		case "/enclient/api/users/device/list/page":
			response = `{"code":200,"data":{"total":1,"pageIndex":1,"pageSize":20,"data":[{"featureCode":"d1","name":"MacBook","online":true,"bindStatus":"1"}]}}`
		case "/enclient/api/users/service/getAllApplicabilityService":
			response = `{"code":200,"data":[{"id":"s1","name":"教务系统","ifShow":true},{"id":"s2","name":"隐藏服务","ifShow":false}]}`
		case "/enclient/api/users/approve/center/getFlow":
			response = `{"code":200,"data":[{"nodeName":"申请"}]}`
		case "/enclient/api/users/service/createServiceApply":
			response = `{"code":200,"data":{"applicationId":"a1"}}`
		case "/enclient/api/users/service/cancleServiceApply":
			response = `{"code":200,"data":{"applicationId":"cancel-1"}}`
		case "/enclient/api/users/approve/center/addApply":
			response = `{"code":200,"data":{"applicationId":"usb-1"}}`
		case "/enclient/api/users/safeSpace/getShareFilePage":
			response = `{"code":200,"data":{"total":1,"pageIndex":1,"pageSize":10,"data":[{"shareMpId":"file-1","fileName":"报告.pdf","spaceName":"我的空间","shareReason":"课程资料"}]}}`
		case "/enclient/api/client/share/link/page":
			response = `{"code":200,"data":{"total":1,"pageIndex":1,"pageSize":10,"data":[{"id":"link-1","filename":"报告.pdf","valid":"0","link":"/share/link-1"}]}}`
		case "/enclient/api/client/share/link/delete/link-1":
			response = `{"code":200,"data":{"deleted":true}}`
		case "/enclient/api/users/center/device/offline":
			response = `{"code":200,"data":{"offline":true}}`
		case "/enclient/api/users/center/device/unbind":
			response = `{"code":200,"data":{"unbind":true}}`
		case "/enclient/api/users/device/unbindUser":
			response = `{"code":200,"data":{"removed":true}}`
		case "/enclient/api/users/message/page":
			response = `{"code":200,"data":{"total":1,"pageIndex":1,"pageSize":10,"data":[{"id":"m1","type":2,"title":"审批","readStatus":0}]}}`
		case "/enclient/api/users/message/count":
			response = `{"code":200,"data":{"total":2,"unread":1}}`
		case "/enclient/api/users/message/allRead":
			response = `{"code":200,"data":{"read":true}}`
		case "/enclient/api/users/approve/center/groupCount":
			response = `{"code":200,"data":{"waitingHandle":1,"myHandle":0,"mySubmit":0}}`
		case "/enclient/api/users/approve/center/waitingHandle":
			response = `{"code":200,"data":{"total":1,"pageIndex":1,"pageSize":10,"data":[{"id":"123","applicationId":"123","applyType":"Service","approveResult":"11"}]}}`
		case "/enclient/api/users/approve/center/applyDetail":
			response = `{"code":200,"data":{"applicationId":"123","applyType":"Service","approveResult":"11"}}`
		case "/enclient/api/users/approve/center/flowImage":
			response = `{"code":200,"data":[{"nodeName":"审批"}]}`
		case "/enclient/api/users/info":
			response = `{"code":200,"data":{"userId":"u1","username":"student"}}`
		case "/enclient/api/users/person/getBindInfos":
			response = `{"code":200,"data":{"phone":"","email":"student@example.edu"}}`
		case "/enclient/api/users/bindMobileEmail/send/code":
			response = `{"code":200,"data":{"sent":true}}`
		case "/enclient/api/users/person/bindMobileEmail":
			response = `{"code":200,"data":{"bound":true}}`
		case "/enclient/api/users/unbind/send/code":
			response = `{"code":200,"data":{"sent":true}}`
		case "/enclient/api/users/unbind/check/code":
			response = `{"code":200,"data":{"unbound":true}}`
		case "/enclient/api/client/totp/login/generateSecretQRContent":
			response = `{"code":200,"data":{"secret":"otp-secret","secretQrContent":"qr-data"}}`
		case "/enclient/api/users/person/restName":
			response = `{"code":200,"data":{"name":"新名称"}}`
		case "/enclient/api/client/user/center/resetUserPassword":
			response = `{"code":200,"data":{"changed":true}}`
		case "/enclient/api/users/safeSpace/getService":
			response = `{"code":200,"data":{"desktop":{"status":true},"spaces":[{"desktopId":"space-1","UseSpace":10,"totalSpace":100}]}}`
		case "/enclient/api/users/approve/center/deal":
			response = `{"code":200,"data":{"accepted":true}}`
		default:
			http.NotFound(writer, request)
			return
		}
		_, _ = writer.Write([]byte(response))
	}))
	defer server.Close()
	root := t.TempDir()
	t.Setenv("CSUST_VPN_BASE_URL", server.URL)
	t.Setenv("CSUST_VPN_COOKIE_FILE", filepath.Join(root, "vpn.cookies"))
	t.Setenv("CSUST_VPN_SESSION_FILE", filepath.Join(root, "vpn.json"))

	run := func(args ...string) map[string]any {
		handled, stdout, _, code, err := (NativeSite{}).Run(context.Background(), append(args, "--json"), true)
		if err != nil || !handled || code != 0 {
			t.Fatalf("%v: handled=%v code=%d err=%v output=%s", args, handled, code, err, stdout)
		}
		var payload map[string]any
		if err := json.Unmarshal(stdout, &payload); err != nil {
			t.Fatalf("%v: %v", args, err)
		}
		return payload
	}

	apps := run("vpn", "apps", "--search", "教务", "--page-size", "20")
	if apps["view"] != "all" || len(apps["applications"].([]any)) != 1 || apps["groups"] == nil {
		t.Fatalf("unexpected apps result: %#v", apps)
	}
	if bodies["/enclient/api/client/user/service/pageUserService"]["serviceName"] != "教务" {
		t.Fatalf("search was not mapped to serviceName: %#v", bodies)
	}
	groupCalls := calls["/enclient/api/client/users/service/grouping"]
	recent := run("vpn", "apps", "recent")
	if recent["view"] != "recent" || len(recent["applications"].([]any)) != 1 {
		t.Fatalf("unexpected recent apps result: %#v", recent)
	}
	if calls["/enclient/api/client/users/service/grouping"] != groupCalls {
		t.Fatalf("recent apps should not require grouping: %#v", calls)
	}
	webSource := run("vpn", "apps", "web-source", "--search", "教务")
	if webSource["view"] != "web-source" || len(webSource["applications"].([]any)) != 1 {
		t.Fatalf("unexpected web-source apps result: %#v", webSource)
	}
	webSourceBody := bodies["/enclient/api/client/user/service/pageUserService"]
	if webSourceBody["serviceSource"] != "2" || webSourceBody["serviceName"] != "教务" {
		t.Fatalf("web-source query was not mapped: %#v", webSourceBody)
	}

	devices := run("vpn", "devices", "--page-size", "20")
	if len(devices["devices"].([]any)) != 1 || devices["total"].(float64) != 1 {
		t.Fatalf("unexpected devices result: %#v", devices)
	}
	deviceBody := bodies["/enclient/api/users/device/list/page"]
	if deviceBody["pageSize"].(float64) != 20 || deviceBody["condition"].(map[string]any) == nil {
		t.Fatalf("device query was not mapped: %#v", deviceBody)
	}

	applications := run("vpn", "apply", "list", "--search", "教务")
	if len(applications["applications"].([]any)) != 1 || applications["visible_count"].(float64) != 1 {
		t.Fatalf("unexpected applicable services result: %#v", applications)
	}
	serviceBody := bodies["/enclient/api/users/service/getAllApplicabilityService"]
	if serviceBody["serviceType"] != "ALL" || serviceBody["serviceName"] != "教务" {
		t.Fatalf("applicable service query was not mapped: %#v", serviceBody)
	}

	flow := run("vpn", "apply", "flow")
	if len(flow["flow"].([]any)) != 1 {
		t.Fatalf("unexpected apply flow result: %#v", flow)
	}

	space := run("vpn", "safe-space", "status")
	if space["space_service"].(map[string]any)["desktop"].(map[string]any)["status"] != true {
		t.Fatalf("unexpected safe-space result: %#v", space)
	}

	_, _, _, code, err := (NativeSite{}).Run(context.Background(), []string{"vpn", "usb", "request", "--name", "U盘", "--identification", "usb-id", "--reason", "数据交换", "--json"}, true)
	if err != nil || code != 2 || calls["/enclient/api/users/approve/center/addApply"] != 0 {
		t.Fatalf("USB request should require confirmation: code=%d err=%v calls=%#v", code, err, calls)
	}
	usb := run("vpn", "usb", "request", "--name", "U盘", "--identification", "usb-id", "--reason", "数据交换", "--yes")
	if usb["submitted"] != true || usb["confirmed"] != true || usb["apply_type"] != "Peripherals" {
		t.Fatalf("unexpected USB request result: %#v", usb)
	}
	usbBody := bodies["/enclient/api/users/approve/center/addApply"]
	if usbBody["applyType"] != "Peripherals" || usbBody["applyReason"] != "数据交换" || usbBody["extendParamMap"].(map[string]any)["identification"] != "usb-id" {
		t.Fatalf("USB request payload was not mapped: %#v", usbBody)
	}

	_, _, _, code, err = (NativeSite{}).Run(context.Background(), []string{"vpn", "device", "offline", "--device-id", "d1", "--json"}, true)
	if err != nil || code != 2 || calls["/enclient/api/users/center/device/offline"] != 0 {
		t.Fatalf("device offline should require confirmation: code=%d err=%v calls=%#v", code, err, calls)
	}
	offline := run("vpn", "device", "offline", "--device-id", "d1", "--yes")
	if offline["submitted"] != true || offline["confirmed"] != true || offline["operation"] != "device-offline" {
		t.Fatalf("unexpected device offline result: %#v", offline)
	}
	offlineBody := bodies["/enclient/api/users/center/device/offline"]
	if offlineBody["featureCode"] != "d1" || offlineBody["userId"] != "u1" {
		t.Fatalf("device offline payload was not mapped: %#v", offlineBody)
	}
	unbound := run("vpn", "device", "unbind", "--device-id", "d1", "--yes")
	if unbound["confirmed"] != true || unbound["operation"] != "device-unbind" {
		t.Fatalf("unexpected device unbind result: %#v", unbound)
	}
	removed := run("vpn", "device", "remove-user", "--device-id", "d1", "--user-id", "u2", "--yes")
	if removed["confirmed"] != true || removed["operation"] != "device-remove-user" {
		t.Fatalf("unexpected device user removal result: %#v", removed)
	}
	removeBody := bodies["/enclient/api/users/device/unbindUser"]
	if removeBody["deviceId"] != "d1" || removeBody["userId"] != "u2" {
		t.Fatalf("device user removal payload was not mapped: %#v", removeBody)
	}

	_, _, _, code, err = (NativeSite{}).Run(context.Background(), []string{"vpn", "apply", "request", "--service-id", "s1", "--service-name", "教务系统", "--json"}, true)
	if err != nil || code != 2 || calls["/enclient/api/users/service/createServiceApply"] != 0 {
		t.Fatalf("apply request should require confirmation: code=%d err=%v calls=%#v", code, err, calls)
	}
	requested := run("vpn", "apply", "request", "--service-id", "s1", "--service-name", "教务系统", "--reason", "课程需要", "--start", "2026-09-12 09:00", "--end", "2026-09-13 18:00", "--yes")
	if requested["submitted"] != true || requested["confirmed"] != true || requested["service_id"] != "s1" {
		t.Fatalf("unexpected apply request result: %#v", requested)
	}
	requestBody := bodies["/enclient/api/users/service/createServiceApply"]
	if requestBody["serviceId"] != "s1" || requestBody["applyStartTime"] != "2026-09-12 09:00:00" || requestBody["applyEndTime"] != "2026-09-13 18:00:00" {
		t.Fatalf("apply request payload was not mapped: %#v", requestBody)
	}
	var summary map[string]string
	if err := json.Unmarshal([]byte(requestBody["contentSummary"].(string)), &summary); err != nil || summary["businessName"] != "教务系统" {
		t.Fatalf("apply content summary was not mapped: %#v", requestBody)
	}

	shares := run("vpn", "shares", "--view", "received", "--search", "报告")
	if len(shares["shares"].([]any)) != 1 || shares["view"] != "received" {
		t.Fatalf("unexpected shares result: %#v", shares)
	}
	shareBody := bodies["/enclient/api/users/safeSpace/getShareFilePage"]
	if shareBody["condition"].(map[string]any)["objectId"] != "u1" || shareBody["conditionLike"].(map[string]any)["filename"] != "报告" {
		t.Fatalf("share query was not mapped: %#v", shareBody)
	}

	links := run("vpn", "links", "--search", "报告")
	if len(links["links"].([]any)) != 1 {
		t.Fatalf("unexpected links result: %#v", links)
	}
	linkBody := bodies["/enclient/api/client/share/link/page"]
	if linkBody["condition"] == nil || linkBody["notCondition"] == nil || linkBody["conditionLike"].(map[string]any)["filename"] != "报告" {
		t.Fatalf("link query was not mapped: %#v", linkBody)
	}
	_, _, _, code, err = (NativeSite{}).Run(context.Background(), []string{"vpn", "links", "delete", "--id", "link-1", "--json"}, true)
	if err != nil || code != 2 || calls["/enclient/api/client/share/link/delete/link-1"] != 0 {
		t.Fatalf("link deletion should require confirmation: code=%d err=%v calls=%#v", code, err, calls)
	}
	deleted := run("vpn", "links", "delete", "--id", "link-1", "--yes")
	if deleted["submitted"] != true || deleted["confirmed"] != true || deleted["link_id"] != "link-1" {
		t.Fatalf("unexpected link deletion result: %#v", deleted)
	}

	profile := run("vpn", "profile")
	if profile["profile"].(map[string]any)["username"] != "student" {
		t.Fatalf("unexpected profile result: %#v", profile)
	}
	_, _, _, code, err = (NativeSite{}).Run(context.Background(), []string{"vpn", "profile", "rename", "--name", "新名称", "--json"}, true)
	if err != nil || code != 2 || calls["/enclient/api/users/person/restName"] != 0 {
		t.Fatalf("profile rename should require confirmation: code=%d err=%v calls=%#v", code, err, calls)
	}
	rename := run("vpn", "profile", "rename", "--name", "新名称", "--yes")
	if rename["submitted"] != true || rename["confirmed"] != true || rename["name"] != "新名称" {
		t.Fatalf("unexpected profile rename result: %#v", rename)
	}
	if bodies["/enclient/api/users/person/restName"]["name"] != "新名称" {
		t.Fatalf("profile rename payload was not mapped: %#v", bodies)
	}

	bindings := run("vpn", "profile", "bindings")
	if bindings["bindings"].(map[string]any)["email"] != "student@example.edu" {
		t.Fatalf("unexpected profile bindings result: %#v", bindings)
	}
	_, _, _, code, err = (NativeSite{}).Run(context.Background(), []string{"vpn", "profile", "bind-code", "--type", "phone", "--address", "13800138000", "--json"}, true)
	if err != nil || code != 2 || calls["/enclient/api/users/bindMobileEmail/send/code"] != 0 {
		t.Fatalf("binding code request should require confirmation: code=%d err=%v calls=%#v", code, err, calls)
	}
	sentCode := run("vpn", "profile", "bind-code", "--type", "phone", "--address", "13800138000", "--yes")
	if sentCode["operation"] != "profile-bind-code" || sentCode["confirmed"] != true {
		t.Fatalf("unexpected binding code result: %#v", sentCode)
	}
	codeBody := bodies["/enclient/api/users/bindMobileEmail/send/code"]
	if codeBody["type"] != "phone" || codeBody["loginNum"] != "13800138000" {
		t.Fatalf("binding code payload was not mapped: %#v", codeBody)
	}
	bound := run("vpn", "profile", "bind", "--type", "phone", "--address", "13800138000", "--code", "a1B2", "--yes")
	if bound["operation"] != "profile-bind" || bound["confirmed"] != true {
		t.Fatalf("unexpected profile binding result: %#v", bound)
	}
	bindBody := bodies["/enclient/api/users/person/bindMobileEmail"]
	if bindBody["type"] != "phone" || bindBody["bindNumber"] != "13800138000" || bindBody["validCode"] != "a1B2" {
		t.Fatalf("profile binding payload was not mapped: %#v", bindBody)
	}
	_, _, _, code, err = (NativeSite{}).Run(context.Background(), []string{"vpn", "profile", "unbind-code", "--type", "phone", "--address", "13800138000", "--json"}, true)
	if err != nil || code != 2 || calls["/enclient/api/users/unbind/send/code"] != 0 {
		t.Fatalf("unbinding code request should require confirmation: code=%d err=%v calls=%#v", code, err, calls)
	}
	unbindCode := run("vpn", "profile", "unbind-code", "--type", "phone", "--address", "13800138000", "--yes")
	if unbindCode["operation"] != "profile-unbind-code" || unbindCode["confirmed"] != true {
		t.Fatalf("unexpected unbinding code result: %#v", unbindCode)
	}
	unbindCodeBody := bodies["/enclient/api/users/unbind/send/code"]
	if unbindCodeBody["type"] != "phone" || unbindCodeBody["loginNum"] != "13800138000" {
		t.Fatalf("unbinding code payload was not mapped: %#v", unbindCodeBody)
	}
	profileUnbound := run("vpn", "profile", "unbind", "--type", "phone", "--code", "a1B2", "--auth-config-id", "auth-1", "--yes")
	if profileUnbound["operation"] != "profile-unbind" || profileUnbound["confirmed"] != true {
		t.Fatalf("unexpected profile unbinding result: %#v", profileUnbound)
	}
	unbindBody := bodies["/enclient/api/users/unbind/check/code"]
	if unbindBody["type"] != "phone" || unbindBody["validCode"] != "a1B2" || unbindBody["authConfigId"] != "auth-1" {
		t.Fatalf("profile unbinding payload was not mapped: %#v", unbindBody)
	}
	_, _, _, code, err = (NativeSite{}).Run(context.Background(), []string{"vpn", "otp", "generate", "--username", "student", "--json"}, true)
	if err != nil || code != 2 || calls["/enclient/api/client/totp/login/generateSecretQRContent"] != 0 {
		t.Fatalf("OTP generation should require confirmation: code=%d err=%v calls=%#v", code, err, calls)
	}
	otp := run("vpn", "otp", "generate", "--username", "student", "--yes")
	if otp["operation"] != "otp-generate" || otp["otp"].(map[string]any)["secret"] != "otp-secret" {
		t.Fatalf("unexpected OTP generation result: %#v", otp)
	}
	otpBody := bodies["/enclient/api/client/totp/login/generateSecretQRContent"]
	if otpBody["userName"] != "student" {
		t.Fatalf("OTP generation payload was not mapped: %#v", otpBody)
	}

	_, _, _, code, err = (NativeSite{}).Run(context.Background(), []string{"vpn", "apply", "cancel-account", "--reason", "不再使用", "--json"}, true)
	if err != nil || code != 2 || calls["/enclient/api/users/service/cancleServiceApply"] != 0 {
		t.Fatalf("account cancellation should require confirmation: code=%d err=%v calls=%#v", code, err, calls)
	}
	cancelled := run("vpn", "apply", "cancel-account", "--reason", "不再使用", "--yes")
	if cancelled["submitted"] != true || cancelled["confirmed"] != true || cancelled["apply_type"] != "WriteOffAccount" {
		t.Fatalf("unexpected account cancellation result: %#v", cancelled)
	}
	cancelBody := bodies["/enclient/api/users/service/cancleServiceApply"]
	if cancelBody["applyType"] != "WriteOffAccount" || cancelBody["applyReason"] != "不再使用" {
		t.Fatalf("account cancellation payload was not mapped: %#v", cancelBody)
	}

	messages := run("vpn", "messages", "--type", "approve", "--read-status", "unread")
	if len(messages["messages"].([]any)) != 1 || messages["filters"].(map[string]any)["read_status"] != "unread" {
		t.Fatalf("unexpected messages result: %#v", messages)
	}
	messageCounts := run("vpn", "messages", "count")
	if messageCounts["operation"] != "message-count" || messageCounts["counts"].(map[string]any)["unread"].(float64) != 1 {
		t.Fatalf("unexpected message counts result: %#v", messageCounts)
	}
	messageBody := bodies["/enclient/api/users/message/page"]
	if messageBody["readStatus"] != "NOT_READ" {
		t.Fatalf("read status was not mapped: %#v", messageBody)
	}

	approvals := run("vpn", "approvals", "--view", "pending", "--status", "pending")
	if len(approvals["items"].([]any)) != 1 || approvals["counts"].(map[string]any)["waitingHandle"].(float64) != 1 {
		t.Fatalf("unexpected approvals result: %#v", approvals)
	}

	detail := run("vpn", "approval", "get", "--id", "123", "--flow")
	if detail["approval"] == nil || len(detail["flow"].([]any)) != 1 {
		t.Fatalf("unexpected approval detail: %#v", detail)
	}

	_, _, _, code, err = (NativeSite{}).Run(context.Background(), []string{"vpn", "messages", "read-all", "--json"}, true)
	if err != nil || code != 2 || calls["/enclient/api/users/message/allRead"] != 0 {
		t.Fatalf("read-all should require confirmation: code=%d err=%v calls=%#v", code, err, calls)
	}
	readAll := run("vpn", "messages", "read-all", "--yes")
	if readAll["submitted"] != true || readAll["confirmed"] != true {
		t.Fatalf("unexpected read-all result: %#v", readAll)
	}

	approved := run("vpn", "approval", "approve", "--id", "123", "--yes")
	if approved["submitted"] != true || approved["confirmed"] != true || approved["action"] != "approve" {
		t.Fatalf("unexpected approval write result: %#v", approved)
	}
	dealBody := bodies["/enclient/api/users/approve/center/deal"]
	if dealBody["approveResult"] != "20" || len(dealBody["ids"].([]any)) != 1 {
		t.Fatalf("approval payload was not mapped: %#v", dealBody)
	}

	groups := run("vpn", "groups")
	if groups["groups"] == nil {
		t.Fatalf("unexpected groups result: %#v", groups)
	}
	_, _, _, code, err = (NativeSite{}).Run(context.Background(), []string{"vpn", "groups", "create", "--name", "工作", "--json"}, true)
	if err != nil || code != 2 || calls["/enclient/api/client/user/addServiceGroup"] != 0 {
		t.Fatalf("group creation should require confirmation: code=%d err=%v calls=%#v", code, err, calls)
	}
	created := run("vpn", "groups", "create", "--name", "工作", "--yes")
	if created["submitted"] != true || created["confirmed"] != true {
		t.Fatalf("unexpected group creation result: %#v", created)
	}
	if bodies["/enclient/api/client/user/addServiceGroup"]["groupName"] != "工作" {
		t.Fatalf("group creation payload was not mapped: %#v", bodies)
	}

	updated := run("vpn", "groups", "update", "--group-id", "g1", "--name", "常用新版", "--service-id", "s1", "--yes")
	if updated["operation"] != "group-update" {
		t.Fatalf("unexpected group update result: %#v", updated)
	}
	updateBody := bodies["/enclient/api/client/user/updateServiceGroup"]
	if updateBody["groupId"] != "g1" || updateBody["groupName"] != "常用新版" || len(updateBody["serviceIds"].([]any)) != 1 {
		t.Fatalf("group update payload was not mapped: %#v", updateBody)
	}

	saved := run("vpn", "groups", "save", "--group-id", "g1", "--name", "常用新版", "--service-id", "s1", "--service-name", "教务系统", "--yes")
	if saved["operation"] != "group-save" {
		t.Fatalf("unexpected group save result: %#v", saved)
	}
	saveBody := bodies["/enclient/api/client/user/service/addToCustomGroup"]
	if saveBody["groupId"] != "g1" || len(saveBody["serviceNames"].([]any)) != 1 {
		t.Fatalf("group save payload was not mapped: %#v", saveBody)
	}

	removedService := run("vpn", "groups", "remove-service", "--group-id", "g1", "--name", "常用新版", "--service-id", "s1", "--yes")
	if removedService["operation"] != "group-remove-service" {
		t.Fatalf("unexpected group service removal result: %#v", removedService)
	}

	sortedGroups := run("vpn", "groups", "sort", "--group-id", "g2", "--group-name", "工作", "--group-id", "g1", "--group-name", "常用新版", "--yes")
	if sortedGroups["operation"] != "group-sort" {
		t.Fatalf("unexpected group sort result: %#v", sortedGroups)
	}
	sortGroupBody := bodies["/enclient/api/client/user/updateServiceGroupSort"]
	if len(sortGroupBody["groupIds"].([]any)) != 2 || sortGroupBody["groupNames"].([]any)[0] != "工作" {
		t.Fatalf("group sort payload was not mapped: %#v", sortGroupBody)
	}

	sortedServices := run("vpn", "groups", "sort-services", "--group-id", "g1", "--service-id", "s1", "--service-name", "教务系统", "--yes")
	if sortedServices["operation"] != "group-sort-services" {
		t.Fatalf("unexpected service sort result: %#v", sortedServices)
	}
	serviceSortBody := bodies["/enclient/api/client/user/updateServiceSort"]
	if serviceSortBody["groupId"] != "g1" || serviceSortBody["serviceNames"].([]any)[0] != "教务系统" {
		t.Fatalf("service sort payload was not mapped: %#v", serviceSortBody)
	}

	_, _, _, code, err = (NativeSite{}).Run(context.Background(), []string{"vpn", "groups", "delete", "--group-id", "g1", "--name", "常用新版", "--json"}, true)
	if err != nil || code != 2 || calls["/enclient/api/client/user/deleteServiceGroup"] != 0 {
		t.Fatalf("group deletion should require confirmation: code=%d err=%v calls=%#v", code, err, calls)
	}
	deletedGroup := run("vpn", "groups", "delete", "--group-id", "g1", "--name", "常用新版", "--yes")
	if deletedGroup["operation"] != "group-delete" || deletedGroup["confirmed"] != true {
		t.Fatalf("unexpected group deletion result: %#v", deletedGroup)
	}
}

func TestNativeVPNProfilePasswordUsesPortalCipherAndClearsSession(t *testing.T) {
	var body map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/enclient/api/users/info" {
			writer.Header().Set("Content-Type", "application/json")
			_, _ = writer.Write([]byte(`{"code":200,"data":{"userId":"u1"}}`))
			return
		}
		if request.URL.Path != "/enclient/api/client/user/center/resetUserPassword" {
			http.NotFound(writer, request)
			return
		}
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			http.Error(writer, err.Error(), http.StatusBadRequest)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"code":200,"data":{"changed":true}}`))
	}))
	defer server.Close()
	root := t.TempDir()
	cookieFile := filepath.Join(root, "vpn.cookies")
	sessionFile := filepath.Join(root, "vpn.json")
	t.Setenv("CSUST_VPN_BASE_URL", server.URL)
	t.Setenv("CSUST_VPN_COOKIE_FILE", cookieFile)
	t.Setenv("CSUST_VPN_SESSION_FILE", sessionFile)
	if err := os.WriteFile(sessionFile, []byte(`{"token":"x-y-1234567890123456"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	handled, stdout, _, code, err := (NativeSite{}).Run(context.Background(), []string{"vpn", "profile", "password", "--current-password", "old-pass", "--new-password", "new-pass", "--yes", "--json"}, true)
	if err != nil || !handled || code != 0 {
		t.Fatalf("password change failed: handled=%v code=%d err=%v output=%s", handled, code, err, stdout)
	}
	if body["id"] != "u1" || body["oldPassword"] != mustVPNPasswordCipher(t, "old-pass", "1234567890123456") || body["password"] != mustVPNPasswordCipher(t, "new-pass", "1234567890123456") {
		t.Fatalf("password payload was not mapped: %#v", body)
	}
	if strings.Contains(string(stdout), "old-pass") || strings.Contains(string(stdout), "new-pass") {
		t.Fatalf("password leaked in command output: %s", stdout)
	}
	var result map[string]any
	if err := json.Unmarshal(stdout, &result); err != nil || result["confirmed"] != true || result["session_cleared"] != true {
		t.Fatalf("unexpected password change result: %#v err=%v", result, err)
	}
	if _, err := os.Stat(sessionFile); !os.IsNotExist(err) {
		t.Fatalf("VPN session should be cleared, stat err=%v", err)
	}
}

func mustVPNPasswordCipher(t *testing.T, password, key string) string {
	t.Helper()
	value, err := encryptVPNPassword(password, key)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func TestParseVPNGroupOptionsRejectsUnusedFields(t *testing.T) {
	cases := []struct {
		command string
		args    []string
	}{
		{"create", []string{"--name", "常用", "--service-id", "s1", "--yes"}},
		{"delete", []string{"--group-id", "g1", "--service-id", "s1", "--yes"}},
		{"update", []string{"--group-id", "g1", "--name", "常用", "--service-name", "教务", "--service-id", "s1", "--yes"}},
		{"sort-services", []string{"--group-id", "g1", "--name", "常用", "--service-id", "s1", "--service-name", "教务", "--yes"}},
	}
	for _, testCase := range cases {
		if _, err := parseVPNGroupOptions(testCase.command, testCase.args); err == nil {
			t.Fatalf("%s should reject unused fields", testCase.command)
		}
	}
}

func TestNativeVPNSecondAuthCompletesDynamicPortalFlow(t *testing.T) {
	var flowBody, sendBody, finishBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		decode := func(target *map[string]any) bool {
			if err := json.NewDecoder(request.Body).Decode(target); err != nil {
				http.Error(writer, err.Error(), http.StatusBadRequest)
				return false
			}
			return true
		}
		switch request.URL.Path {
		case "/enclient/api/users/custom/page/login/cfg/select":
			_, _ = writer.Write([]byte(`{"code":200,"data":{"defaultAuthType":"LOCAL"}}`))
		case "/enclient/api/users/client/auth/generateKey":
			_, _ = writer.Write([]byte(`{"code":200,"data":{"enToken":"a-b-1234567890abcdef"}}`))
		case "/enclient/api/users/auth/login":
			http.SetCookie(writer, &http.Cookie{Name: "vpn_session", Value: "pending", Path: "/"})
			_, _ = writer.Write([]byte(`{"code":2050,"data":{"entoken":"pending-token"}}`))
		case "/enclient/api/users/flow/path":
			if !decode(&flowBody) {
				return
			}
			if request.Header.Get("Authorization") != "Bearer pending-token" {
				writer.WriteHeader(http.StatusUnauthorized)
				_, _ = writer.Write([]byte(`{"code":3010,"messages":"missing pending token"}`))
				return
			}
			_, _ = writer.Write([]byte(`{"code":200,"data":[{"steps":[{"formConfig":{"formPage":[{"components":[{"refKey":"internalToken","hidden":true,"attributes":{"placeholder":"portal-internal"}},{"refKey":"loginNum","category":"RequiredInput","attributes":{"placeholder":"手机号"}},{"refKey":"code","category":"SendCodeBtn","attributes":{"placeholder":"验证码"}}]}]}}]}]}`))
		case "/enclient/api/users/second/send/code":
			if !decode(&sendBody) {
				return
			}
			_, _ = writer.Write([]byte(`{"code":200,"data":{"sent":true}}`))
		case "/enclient/api/users/auth/secondAuth":
			if !decode(&finishBody) {
				return
			}
			_, _ = writer.Write([]byte(`{"code":200,"data":{"token":"final-token","refreshToken":"refresh","username":"student"}}`))
		case "/enclient/api/users/info":
			if request.Header.Get("Authorization") != "Bearer final-token" {
				writer.WriteHeader(http.StatusUnauthorized)
				_, _ = writer.Write([]byte(`{"code":3010,"messages":"expired"}`))
				return
			}
			_, _ = writer.Write([]byte(`{"code":200,"data":{"userId":"u1","username":"student"}}`))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	root := t.TempDir()
	t.Setenv("CSUST_VPN_BASE_URL", server.URL)
	t.Setenv("CSUST_VPN_COOKIE_FILE", filepath.Join(root, "vpn.cookies"))
	t.Setenv("CSUST_VPN_SESSION_FILE", filepath.Join(root, "vpn.json"))
	t.Setenv("CSUST_USERNAME", "student")
	t.Setenv("CSUST_PASSWORD", "password")

	run := func(args ...string) map[string]any {
		handled, stdout, _, code, err := (NativeSite{}).Run(context.Background(), append(args, "--json"), true)
		if err != nil || !handled || code != 0 {
			t.Fatalf("%v: handled=%v code=%d err=%v output=%s", args, handled, code, err, stdout)
		}
		var result map[string]any
		if err := json.Unmarshal(stdout, &result); err != nil {
			t.Fatalf("%v: %v", args, err)
		}
		return result
	}

	pending := run("vpn", "login", "--auth", "local")
	if pending["pending"] != true || pending["next"] != "second-auth" {
		t.Fatalf("unexpected pending login result: %#v", pending)
	}
	session, err := os.ReadFile(os.Getenv("CSUST_VPN_SESSION_FILE"))
	if err != nil || !strings.Contains(string(session), `"token": "pending-token"`) {
		t.Fatalf("pending token was not persisted for second auth: %s err=%v", session, err)
	}

	sent := run("vpn", "login", "second-auth", "--method", "phone", "--login-number", "13800138000", "--send-code", "--yes")
	if sent["pending"] != true || sent["operation"] != "login-second-auth-send-code" {
		t.Fatalf("unexpected send-code result: %#v", sent)
	}
	if flowBody["scenes"] != "secondAuth" || flowBody["type"] != "phone" || sendBody["loginNum"] != "13800138000" || sendBody["internalToken"] != "portal-internal" {
		t.Fatalf("dynamic send payload was not mapped: flow=%#v send=%#v", flowBody, sendBody)
	}

	finished := run("vpn", "login", "second-auth", "--method", "phone", "--login-number", "13800138000", "--code", "123456")
	if finished["ok"] != true || finished["confirmed"] != true || finished["auth"] != "local" || finished["username"] != "student" {
		t.Fatalf("unexpected completed login result: %#v", finished)
	}
	if finishBody["code"] != "123456" || finishBody["loginNum"] != "13800138000" || finishBody["internalToken"] != "portal-internal" {
		t.Fatalf("dynamic finish payload was not mapped: %#v", finishBody)
	}
	finalSession, err := os.ReadFile(os.Getenv("CSUST_VPN_SESSION_FILE"))
	if err != nil || !strings.Contains(string(finalSession), `"token": "final-token"`) || strings.Contains(string(finalSession), "pendingToken") {
		t.Fatalf("completed session was not persisted: %s err=%v", finalSession, err)
	}
}

func TestParseVPNProfilePasswordRejectsSecondStdinSecret(t *testing.T) {
	if _, err := parseVPNProfilePasswordOptions([]string{"--new-password-stdin", "--yes"}); err == nil || err.Code != "invalid_argument" {
		t.Fatalf("new password stdin should be rejected as ambiguous: %#v", err)
	}
}

func TestNativeVPNResetPasswordUsesDynamicFlowAndClearsPendingState(t *testing.T) {
	bodies := map[string]map[string]any{}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		decode := func(path string) bool {
			var body map[string]any
			if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
				http.Error(writer, err.Error(), http.StatusBadRequest)
				return false
			}
			bodies[path] = body
			if request.Header.Get("Authorization") != "" {
				http.Error(writer, "reset flow must not use an authenticated bearer", http.StatusUnauthorized)
				return false
			}
			return true
		}
		switch request.URL.Path {
		case "/enclient/api/users/flow/path":
			if !decode(request.URL.Path) {
				return
			}
			_, _ = writer.Write([]byte(`{"code":200,"data":[{"steps":[{"page":"findUser","formConfig":{"formPage":[{"components":[{"refKey":"account","hidden":false,"attributes":{"placeholder":"账号"}}]}]}},{"page":"listVerifyType","formConfig":{"formPage":[{"components":[{"refKey":"mobile","hidden":false},{"refKey":"email","hidden":false}]}]}},{"page":"forgetCheckCode","formConfig":{"formPage":[{"components":[{"refKey":"loginNum","hidden":false},{"refKey":"code","hidden":false}]}]}},{"page":"modifyPassword","formConfig":{"formPage":[{"components":[{"refKey":"password","hidden":false}]}]}}]}]}`))
		case "/enclient/api/users/reset/password/find/verify/type":
			if !decode(request.URL.Path) {
				return
			}
			_, _ = writer.Write([]byte(`{"code":200,"data":{"reToken":"a-b-1234567890abcdef","phone":true}}`))
		case "/enclient/api/users/reset/password/send/code":
			if !decode(request.URL.Path) {
				return
			}
			_, _ = writer.Write([]byte(`{"code":200,"data":{"sent":true}}`))
		case "/enclient/api/users/reset/password/terminal/verify/code":
			if !decode(request.URL.Path) {
				return
			}
			_, _ = writer.Write([]byte(`{"code":200,"data":{"verified":true}}`))
		case "/enclient/api/users/reset/password/verify/code":
			if !decode(request.URL.Path) {
				return
			}
			_, _ = writer.Write([]byte(`{"code":200,"data":{"changed":true}}`))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	root := t.TempDir()
	sessionFile := filepath.Join(root, "vpn.json")
	t.Setenv("CSUST_VPN_BASE_URL", server.URL)
	t.Setenv("CSUST_VPN_COOKIE_FILE", filepath.Join(root, "vpn.cookies"))
	t.Setenv("CSUST_VPN_SESSION_FILE", sessionFile)
	if err := os.WriteFile(sessionFile, []byte(`{"token":"existing-session"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	handled, _, _, code, err := (NativeSite{}).Run(context.Background(), []string{"vpn", "login", "reset-password", "--account", "student", "--method", "phone", "--login-number", "13800138000", "--send-code", "--json"}, true)
	if err != nil || !handled || code != 2 || len(bodies) != 0 {
		t.Fatalf("reset password should require confirmation: handled=%v code=%d err=%v bodies=%#v", handled, code, err, bodies)
	}

	run := func(args ...string) map[string]any {
		handled, stdout, _, code, err := (NativeSite{}).Run(context.Background(), append(args, "--json"), true)
		if err != nil || !handled || code != 0 {
			t.Fatalf("%v: handled=%v code=%d err=%v output=%s", args, handled, code, err, stdout)
		}
		var result map[string]any
		if err := json.Unmarshal(stdout, &result); err != nil {
			t.Fatalf("%v: %v", args, err)
		}
		return result
	}

	sent := run("vpn", "login", "reset-password", "--account", "student", "--method", "phone", "--login-number", "13800138000", "--send-code", "--yes")
	if sent["pending"] != true || sent["operation"] != "login-reset-password-send-code" {
		t.Fatalf("unexpected reset send result: %#v", sent)
	}
	if body := bodies["/enclient/api/users/flow/path"]; body["scenes"] != "forgetPassword" || body["type"] != "forgetPassword" {
		t.Fatalf("flow payload was not mapped: %#v", body)
	}
	findBody := bodies["/enclient/api/users/reset/password/find/verify/type"]
	if findBody["account"] != "student" || findBody["userType"] != "1" {
		t.Fatalf("find payload was not mapped: %#v", findBody)
	}
	sendBody := bodies["/enclient/api/users/reset/password/send/code"]
	if sendBody["type"] != "phone" || sendBody["loginNum"] != "13800138000" || sendBody["reToken"] != "a-b-1234567890abcdef" {
		t.Fatalf("send payload was not mapped: %#v", sendBody)
	}
	session, err := os.ReadFile(sessionFile)
	if err != nil || !strings.Contains(string(session), `"resetPassword"`) || !strings.Contains(string(session), `"reToken": "a-b-1234567890abcdef"`) {
		t.Fatalf("pending reset state was not persisted: %s err=%v", session, err)
	}

	finished := run("vpn", "login", "reset-password", "--code", "123456", "--new-password", "new-pass", "--yes")
	if finished["ok"] != true || finished["confirmed"] != true || finished["operation"] != "login-reset-password" {
		t.Fatalf("unexpected reset completion result: %#v", finished)
	}
	terminalBody := bodies["/enclient/api/users/reset/password/terminal/verify/code"]
	if terminalBody["account"] != "student" || terminalBody["type"] != "phone" || terminalBody["code"] != "123456" || terminalBody["reToken"] != "a-b-1234567890abcdef" {
		t.Fatalf("terminal verification payload was not mapped: %#v", terminalBody)
	}
	finalBody := bodies["/enclient/api/users/reset/password/verify/code"]
	if finalBody["password"] != mustVPNPasswordCipher(t, "new-pass", "1234567890abcdef") || finalBody["newPassword"] != "new-pass" {
		t.Fatalf("final reset payload was not mapped: %#v", finalBody)
	}
	if strings.Contains(string(mustJSON(finalBody)), "existing-session") {
		t.Fatalf("reset payload leaked the stored bearer token: %#v", finalBody)
	}
	finalSession, err := os.ReadFile(sessionFile)
	if err != nil || !strings.Contains(string(finalSession), `"token": "existing-session"`) || strings.Contains(string(finalSession), `resetPassword`) {
		t.Fatalf("pending reset state was not cleared: %s err=%v", finalSession, err)
	}
}

func TestNativeVPNCommonLocationsUpdatesAndConfirmsByReadback(t *testing.T) {
	var updateBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/enclient/api/users/info":
			_, _ = writer.Write([]byte(`{"code":200,"data":{"commonLocationList":[{"province":"湖南省","city":"长沙市"},{"province":"广东省","city":"广州市"}]}}`))
		case "/enclient/api/users/updateCommonLocation":
			if err := json.NewDecoder(request.Body).Decode(&updateBody); err != nil {
				http.Error(writer, err.Error(), http.StatusBadRequest)
				return
			}
			_, _ = writer.Write([]byte(`{"code":200,"data":{"updated":true}}`))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	root := t.TempDir()
	t.Setenv("CSUST_VPN_BASE_URL", server.URL)
	t.Setenv("CSUST_VPN_COOKIE_FILE", filepath.Join(root, "vpn.cookies"))
	t.Setenv("CSUST_VPN_SESSION_FILE", filepath.Join(root, "vpn.json"))

	run := func(args ...string) map[string]any {
		handled, stdout, _, code, err := (NativeSite{}).Run(context.Background(), append(args, "--json"), true)
		if err != nil || !handled || code != 0 {
			t.Fatalf("%v: handled=%v code=%d err=%v output=%s", args, handled, code, err, stdout)
		}
		var result map[string]any
		if err := json.Unmarshal(stdout, &result); err != nil {
			t.Fatalf("%v: %v", args, err)
		}
		return result
	}

	listed := run("vpn", "profile", "common-locations")
	if listed["operation"] != "profile-common-locations" || len(listed["common_locations"].([]any)) != 2 {
		t.Fatalf("unexpected common location list: %#v", listed)
	}

	_, _, _, code, err := (NativeSite{}).Run(context.Background(), []string{"vpn", "profile", "common-locations", "update", "--location", "湖南省/长沙市", "--location", "广东省,广州市", "--json"}, true)
	if err != nil || code != 2 || updateBody != nil {
		t.Fatalf("common location update should require confirmation: code=%d err=%v body=%#v", code, err, updateBody)
	}
	updated := run("vpn", "profile", "common-locations", "update", "--location", "湖南省/长沙市", "--location", "广东省,广州市", "--yes")
	if updated["operation"] != "profile-common-locations-update" || updated["confirmed"] != true {
		t.Fatalf("unexpected common location update: %#v", updated)
	}
	items := updateBody["commonList"].([]any)
	if len(items) != 2 || items[0].(map[string]any)["province"] != "湖南省" || items[1].(map[string]any)["city"] != "广州市" {
		t.Fatalf("common location payload was not mapped: %#v", updateBody)
	}
}
