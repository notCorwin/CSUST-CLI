package adapter

import (
	"context"
	"fmt"
	"math"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
)

const transportMobileService = "transport-mobile"

var transportMobilePhone = regexp.MustCompile(`^1[3-9]\d{9}$`)

type transportMobileTable struct {
	table      string
	sortColumn string
	fuzzy      []string
	populator  []map[string]any
	model      func(map[string]any) map[string]any
}

var transportMobileTables = map[string]transportMobileTable{
	"defenses": {
		table: "defense", sortColumn: "version", fuzzy: []string{"name"},
		populator: []map[string]any{{"path": "file.file", "select": "name fname fullname extension size"}, {"path": "creater", "select": "name"}},
		model:     transportMobileDefense,
	},
	"finances": {
		table: "finance", sortColumn: "code", fuzzy: []string{"code", "name", "ownname", "remark", "ficode"},
		populator: []map[string]any{{"path": "group", "select": "name"}, {"path": "creater", "select": "name code"}},
		model:     transportMobileFinance,
	},
	"finance-items": {
		table: "fitem", sortColumn: "dateCreate", fuzzy: []string{"money", "remark"},
		populator: []map[string]any{{"path": "creater", "select": "name code"}, {"path": "project", "select": "name fitype status creater"}, {"path": "file", "select": "name fullname fname size"}},
		model:     transportMobileFinanceItem,
	},
	"notes": {
		table: "note", sortColumn: "dateModified", fuzzy: []string{"name"},
		populator: []map[string]any{{"path": "creater participants.user", "select": "name code"}},
		model:     transportMobileNote,
	},
	"access-records": {
		table: "accessrecord", sortColumn: "eventTime", fuzzy: []string{"employeeCode", "personName", "departmentName", "deviceAlias", "eventDescription", "verifyModeName"},
		model: transportMobileAccessRecord,
	},
	"achievements": {
		table: "achievement", sortColumn: "code", fuzzy: []string{"name", "code", "remark"},
		populator: []map[string]any{{"path": "creater teacher.department student.department", "select": "name"}},
		model:     transportMobileAchievement,
	},
	"kpis": {
		table: "kpi", sortColumn: "version", fuzzy: []string{"code", "name", "remark"},
		populator: []map[string]any{{"path": "creater owner batch", "select": "name"}},
		model:     transportMobileKPI,
	},
	"notices": {
		table: "notice", sortColumn: "code", fuzzy: []string{"code", "title", "content"},
		populator: []map[string]any{{"path": "creater user approver public.user public.role public.group", "select": "name"}},
		model:     transportMobileNotice,
	},
	"workflows": {
		table: "realworkflow", sortColumn: "code", fuzzy: []string{"code"},
		populator: []map[string]any{{"path": "creater current.user history.user", "select": "name"}},
		model:     transportMobileWorkflow,
	},
	"vacations": {
		table: "vacation", sortColumn: "dateCreate", fuzzy: []string{"code"},
		populator: []map[string]any{{"path": "creater department", "select": "name"}},
		model:     transportMobileVacation,
	},
}

func (a NativeSite) executeTransportMobile(ctx context.Context, args []string) (map[string]any, *siteError) {
	if len(args) == 0 || args[0] == "catalog" {
		return businessCatalogNames(transportMobileService), nil
	}
	cookie, _, valueErr := businessValue(args, "--cookie-file")
	if valueErr != nil {
		return nil, valueErr
	}
	switch args[0] {
	case "login":
		return a.transportMobileLogin(ctx, args[1:], cookie)
	case "send-code":
		return a.transportMobileSendCode(ctx, args[1:], cookie)
	case "change-password":
		return a.transportMobileChangePassword(ctx, args[1:], cookie)
	case "logout":
		return a.transportMobileLogout(ctx, args[1:], cookie)
	case "profile":
		return a.transportMobileProfile(ctx, args[1:], cookie)
	case "pending":
		return a.transportMobilePending(ctx, args[1:], cookie)
	case "dictionaries", "dict":
		return a.transportMobileDictionaries(ctx, args[1:], cookie)
	case "defenses":
		return a.transportMobileTableList(ctx, args[1:], cookie, transportMobileTables["defenses"], "defenses")
	case "notes":
		return a.transportMobileTableList(ctx, args[1:], cookie, transportMobileTables["notes"], "notes")
	case "access-records":
		return a.transportMobileTableList(ctx, args[1:], cookie, transportMobileTables["access-records"], "access-records")
	case "achievements":
		return a.transportMobileTableList(ctx, args[1:], cookie, transportMobileTables["achievements"], "achievements")
	case "kpis":
		return a.transportMobileTableList(ctx, args[1:], cookie, transportMobileTables["kpis"], "kpis")
	case "notices":
		return a.transportMobileTableList(ctx, args[1:], cookie, transportMobileTables["notices"], "notices")
	case "workflows":
		return a.transportMobileTableList(ctx, args[1:], cookie, transportMobileTables["workflows"], "workflows")
	case "vacations":
		return a.transportMobileTableList(ctx, args[1:], cookie, transportMobileTables["vacations"], "vacations")
	case "defense":
		return a.transportMobileDefenseDetail(ctx, args[1:], cookie)
	case "finances":
		return a.transportMobileTableList(ctx, args[1:], cookie, transportMobileTables["finances"], "finances")
	case "finance":
		return a.transportMobileFinanceDetail(ctx, args[1:], cookie)
	case "finance-items":
		return a.transportMobileTableList(ctx, args[1:], cookie, transportMobileTables["finance-items"], "finance-items")
	case "finance-item":
		return a.transportMobileFinanceItemDetail(ctx, args[1:], cookie)
	case "finance-item-create":
		return a.transportMobileFinanceItemSave(ctx, args[1:], cookie, "create")
	case "finance-item-update":
		return a.transportMobileFinanceItemSave(ctx, args[1:], cookie, "update")
	case "finance-item-delete":
		return a.transportMobileFinanceItemDelete(ctx, args[1:], cookie)
	default:
		return nil, &siteError{Code: "invalid_argument", Message: "transport-mobile 只支持 login、send-code、change-password、logout、profile、pending、dictionaries、defenses、notes、access-records、achievements、kpis、notices、workflows、vacations、defense、finances、finance、finance-items、finance-item、finance-item-create、finance-item-update、finance-item-delete、catalog"}
	}
}

func (a NativeSite) transportMobileRequest(ctx context.Context, method, path string, body any, hasJSON bool, token, cookie string, readOnly, yes bool) (map[string]any, *siteError) {
	headers := []pair{}
	if strings.TrimSpace(token) != "" {
		headers = append(headers, pair{"token", token})
	}
	return a.execute(ctx, siteRequest{
		Service: transportMobileService, Method: method, Path: path, JSON: body, HasJSON: hasJSON,
		Headers: headers, CookieFile: cookie, AllowBusinessFailure: true, RawJSON: true,
		ReadOnly: readOnly, Yes: yes,
	})
}

func transportMobileEnvelope(result map[string]any) (map[string]any, *siteError) {
	value, ok := businessData(result)
	if !ok {
		return nil, &siteError{Code: "parse_error", Message: "交通移动端响应不是 JSON 对象"}
	}
	payload, ok := value.(map[string]any)
	if !ok {
		return nil, &siteError{Code: "parse_error", Message: "交通移动端响应 JSON 结构无效"}
	}
	return payload, nil
}

func (a NativeSite) transportMobileCall(ctx context.Context, method, path string, body any, hasJSON bool, token, cookie string, readOnly, yes bool) (map[string]any, *siteError) {
	result, requestErr := a.transportMobileRequest(ctx, method, path, body, hasJSON, token, cookie, readOnly, yes)
	if requestErr != nil {
		return nil, requestErr
	}
	payload, parseErr := transportMobileEnvelope(result)
	if parseErr != nil {
		return nil, parseErr
	}
	if !transportMobileSuccess(payload) {
		return nil, transportMobileRejected(payload)
	}
	return payload, nil
}

func transportMobileSuccess(payload map[string]any) bool {
	if success, ok := payload["success"].(bool); ok {
		return success
	}
	return fmt.Sprint(payload["code"]) == "200"
}

func transportMobileRejected(payload map[string]any) *siteError {
	code := strings.TrimSpace(fmt.Sprint(payload["code"]))
	message := transportMobileMessage(payload)
	if code == "401" {
		return &siteError{Code: "login_required", Message: "交通移动端会话已失效，请重新登录", Details: map[string]any{"remote_code": payload["code"], "remote_message": message}}
	}
	if message == "" {
		message = "交通移动端接口拒绝请求"
	}
	return &siteError{Code: "business_rejected", Message: message, Details: map[string]any{"remote_code": payload["code"], "remote_message": message}}
}

func transportMobileMessage(payload map[string]any) string {
	for _, key := range []string{"message", "msg", "error"} {
		value, ok := payload[key]
		if !ok || value == nil {
			continue
		}
		if message := strings.TrimSpace(fmt.Sprint(value)); message != "" {
			return message
		}
	}
	return ""
}

func (a NativeSite) transportMobileSession(args []string, cookie string) (string, string, *siteError) {
	token, found, valueErr := businessValue(args, "--access-token")
	if valueErr != nil {
		return "", "", valueErr
	}
	if !found || strings.TrimSpace(token) == "" {
		token = os.Getenv("CSUST_TRANSPORT_MOBILE_TOKEN")
	}
	_, cookiePath, resolveErr := resolveSite(siteRequest{Service: transportMobileService, CookieFile: cookie})
	if resolveErr != nil {
		return "", "", resolveErr
	}
	tokenPath := strings.TrimSuffix(cookiePath, ".cookies.txt") + ".token"
	if strings.TrimSpace(token) == "" {
		info, statErr := os.Lstat(tokenPath)
		if statErr == nil {
			if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
				return "", tokenPath, &siteError{Code: "session_error", Message: "交通移动端令牌文件必须是普通文件且不能是符号链接"}
			}
			content, readErr := os.ReadFile(tokenPath)
			if readErr != nil {
				return "", tokenPath, &siteError{Code: "session_error", Message: "无法读取交通移动端令牌: " + readErr.Error()}
			}
			token = strings.TrimSpace(string(content))
		} else if !os.IsNotExist(statErr) {
			return "", tokenPath, &siteError{Code: "session_error", Message: "无法读取交通移动端令牌: " + statErr.Error()}
		}
	}
	return strings.TrimSpace(token), tokenPath, nil
}

func transportMobileResult(operation, evidence string) map[string]any {
	return map[string]any{"ok": true, "submitted": false, "confirmed": true, "evidence": evidence, "service": transportMobileService, "operation": operation}
}

func (a NativeSite) transportMobileLogin(ctx context.Context, args []string, cookie string) (map[string]any, *siteError) {
	phone, phoneFound, valueErr := businessValue(args, "--phone")
	if valueErr != nil {
		return nil, valueErr
	}
	smsCode, smsFound, valueErr := businessValue(args, "--sms-code")
	if valueErr != nil {
		return nil, valueErr
	}
	body := map[string]any{"rememberMe": businessBool(args, "--remember")}
	identity := ""
	if phoneFound {
		if !transportMobilePhone.MatchString(strings.TrimSpace(phone)) {
			return nil, &siteError{Code: "invalid_argument", Message: "--phone 必须是有效的 11 位手机号"}
		}
		if !smsFound || strings.TrimSpace(smsCode) == "" {
			return nil, &siteError{Code: "invalid_argument", Message: "短信登录必须提供 --sms-code"}
		}
		if len([]rune(strings.TrimSpace(smsCode))) != 6 {
			return nil, &siteError{Code: "invalid_argument", Message: "--sms-code 必须是 6 位验证码"}
		}
		body["phone"], body["sms"] = strings.TrimSpace(phone), strings.TrimSpace(smsCode)
		identity = strings.TrimSpace(phone)
	} else {
		if smsFound {
			return nil, &siteError{Code: "invalid_argument", Message: "--sms-code 必须与 --phone 一起使用"}
		}
		account, password, credentialErr := businessCredentials(args, "CSUST_TRANSPORT_MOBILE_PASSWORD")
		if credentialErr != nil {
			return nil, credentialErr
		}
		body["code"], body["pwd"] = account, password
		identity = account
	}
	result, requestErr := a.transportMobileRequest(ctx, "POST", "/api/authenticate", body, true, "", cookie, true, true)
	if requestErr != nil {
		return nil, requestErr
	}
	payload, parseErr := transportMobileEnvelope(result)
	if parseErr != nil {
		return nil, parseErr
	}
	if !transportMobileSuccess(payload) {
		message := transportMobileMessage(payload)
		if message == "" {
			message = "交通移动端登录失败"
		}
		return nil, &siteError{Code: "authentication_failed", Message: message, Details: map[string]any{"submitted": false, "confirmed": false, "evidence": "rejected", "remote_code": payload["code"]}}
	}
	data, ok := payload["data"].(map[string]any)
	if !ok {
		return nil, &siteError{Code: "authentication_failed", Message: "登录成功响应缺少令牌数据", Details: map[string]any{"submitted": false, "confirmed": false, "evidence": "success-without-token"}}
	}
	token, ok := data["token"].(string)
	if !ok || strings.TrimSpace(token) == "" {
		return nil, &siteError{Code: "authentication_failed", Message: "登录成功响应缺少访问令牌", Details: map[string]any{"submitted": false, "confirmed": false, "evidence": "success-without-token"}}
	}
	_, tokenPath, sessionErr := a.transportMobileSession(nil, cookie)
	if sessionErr != nil {
		return nil, sessionErr
	}
	if writeErr := atomicWrite(tokenPath, []byte(strings.TrimSpace(token)+"\n")); writeErr != nil {
		return nil, &siteError{Code: "session_error", Message: "登录成功但令牌保存失败: " + writeErr.Error(), Details: map[string]any{"submitted": false, "confirmed": true, "evidence": "remote-login-token"}}
	}
	profile, profileErr := a.transportMobileCall(ctx, "GET", "/api/user/getUserInfo", nil, false, token, cookie, true, true)
	if profileErr != nil {
		return nil, &siteError{Code: "authentication_unverified", Message: "登录已返回令牌，但用户信息验证失败: " + profileErr.Error(), Details: map[string]any{"submitted": false, "confirmed": false, "evidence": "remote-login-token-and-profile-check", "token_file": tokenPath}}
	}
	resultValue := transportMobileResult("login", "远端登录返回令牌并通过用户信息回读验证")
	resultValue["username"], resultValue["token_file"] = identity, tokenPath
	resultValue["login_type"] = data["loginType"]
	if user, ok := profile["data"].(map[string]any); ok {
		resultValue["user"] = transportMobileUser(user)
	}
	return resultValue, nil
}

func (a NativeSite) transportMobileSendCode(ctx context.Context, args []string, cookie string) (map[string]any, *siteError) {
	if !businessBool(args, "--yes") {
		return nil, &siteError{Code: "confirmation_required", Message: "发送短信验证码会产生外部副作用，请加 --yes"}
	}
	phone, requiredErr := businessRequired(args, "--phone", "send-code 必须提供 --phone")
	if requiredErr != nil {
		return nil, requiredErr
	}
	phone = strings.TrimSpace(phone)
	if !transportMobilePhone.MatchString(phone) {
		return nil, &siteError{Code: "invalid_argument", Message: "--phone 必须是有效的 11 位手机号"}
	}
	payload, requestErr := a.transportMobileCall(ctx, "POST", "/api/verifys", map[string]any{"phone": phone}, true, "", cookie, false, true)
	if requestErr != nil {
		return nil, requestErr
	}
	result := transportMobileResult("send-code", "验证码接口返回 success=true")
	result["phone"] = phone
	result["api_code"] = payload["code"]
	return result, nil
}

func (a NativeSite) transportMobileChangePassword(ctx context.Context, args []string, cookie string) (map[string]any, *siteError) {
	if !businessBool(args, "--yes") {
		return nil, &siteError{Code: "confirmation_required", Message: "修改交通移动端密码会改变远端认证状态，请加 --yes"}
	}
	loginType := strings.ToLower(strings.TrimSpace(flagValue(args, "--login-type")))
	if loginType == "" {
		loginType = "account"
	}
	if loginType != "account" && loginType != "sms" {
		return nil, &siteError{Code: "invalid_argument", Message: "--login-type 必须是 account 或 sms"}
	}
	oldPassword, oldErr := businessSecret(args, "--current-password", "CSUST_TRANSPORT_MOBILE_CURRENT_PASSWORD")
	if oldErr != nil && loginType == "account" {
		return nil, oldErr
	}
	newPassword, newErr := businessSecret(args, "--new-password", "CSUST_TRANSPORT_MOBILE_NEW_PASSWORD")
	if newErr != nil {
		return nil, newErr
	}
	if strings.TrimSpace(newPassword) == "" || strings.ContainsAny(newPassword, "\r\n") {
		return nil, &siteError{Code: "invalid_argument", Message: "新密码不能为空且不能包含换行"}
	}
	confirmation, confirmationErr := businessRequired(args, "--password-confirm", "change-password 必须提供 --password-confirm")
	if confirmationErr != nil {
		return nil, confirmationErr
	}
	if newPassword != confirmation {
		return nil, &siteError{Code: "invalid_argument", Message: "新密码两次输入不一致"}
	}
	token, _, sessionErr := a.transportMobileSession(args, cookie)
	if sessionErr != nil {
		return nil, sessionErr
	}
	if token == "" {
		return nil, &siteError{Code: "login_required", Message: "请先运行 transport-mobile login 或提供 --access-token"}
	}
	payload, requestErr := a.transportMobileCall(ctx, "POST", "/api/user/changepwd", map[string]any{"loginType": loginType, "old": oldPassword, "new": newPassword}, true, token, cookie, false, true)
	if requestErr != nil {
		return nil, requestErr
	}
	result := transportMobileResult("change-password", "修改密码接口返回 success=true")
	result["submitted"] = true
	result["login_type"], result["api_code"] = loginType, payload["code"]
	return result, nil
}

func (a NativeSite) transportMobileLogout(ctx context.Context, args []string, cookie string) (map[string]any, *siteError) {
	token, tokenPath, sessionErr := a.transportMobileSession(args, cookie)
	if sessionErr != nil {
		return nil, sessionErr
	}
	if token == "" {
		if removeErr := removeCookieFile(tokenPath); removeErr != nil {
			return nil, &siteError{Code: "session_error", Message: "无法删除交通移动端令牌: " + removeErr.Error()}
		}
		result := transportMobileResult("logout", "本地令牌已删除")
		result["logged_out"], result["token_file"] = true, tokenPath
		return result, nil
	}
	_, requestErr := a.transportMobileCall(ctx, "POST", "/api/user/logout", map[string]any{}, true, token, cookie, true, true)
	if requestErr != nil && requestErr.Code != "login_required" {
		return nil, requestErr
	}
	if removeErr := removeCookieFile(tokenPath); removeErr != nil {
		return nil, &siteError{Code: "session_error", Message: "远端退出后令牌删除失败: " + removeErr.Error()}
	}
	result := transportMobileResult("logout", "远端退出并删除本地令牌")
	result["logged_out"], result["token_file"] = true, tokenPath
	return result, nil
}

func (a NativeSite) transportMobileProfile(ctx context.Context, args []string, cookie string) (map[string]any, *siteError) {
	token, _, sessionErr := a.transportMobileSession(args, cookie)
	if sessionErr != nil {
		return nil, sessionErr
	}
	if token == "" {
		return nil, &siteError{Code: "login_required", Message: "请先运行 transport-mobile login 或提供 --access-token"}
	}
	payload, requestErr := a.transportMobileCall(ctx, "GET", "/api/user/getUserInfo", nil, false, token, cookie, true, true)
	if requestErr != nil {
		return nil, requestErr
	}
	data, ok := payload["data"].(map[string]any)
	if !ok {
		return nil, &siteError{Code: "parse_error", Message: "用户信息响应缺少 data 对象"}
	}
	result := transportMobileResult("profile", "用户信息接口返回 success=true")
	result["data"] = transportMobileUser(data)
	result["raw"] = redactSiteJSON(payload)
	return result, nil
}

func (a NativeSite) transportMobilePending(ctx context.Context, args []string, cookie string) (map[string]any, *siteError) {
	token, _, sessionErr := a.transportMobileSession(args, cookie)
	if sessionErr != nil {
		return nil, sessionErr
	}
	if token == "" {
		return nil, &siteError{Code: "login_required", Message: "请先运行 transport-mobile login 或提供 --access-token"}
	}
	payload, requestErr := a.transportMobileCall(ctx, "GET", "/api/user/getPendingCount", nil, false, token, cookie, true, true)
	if requestErr != nil {
		return nil, requestErr
	}
	result := transportMobileResult("pending", "待办数量接口返回 success=true")
	result["data"] = payload["data"]
	result["pending_count"] = transportMobileValue(payload["data"], "numPending", "pending", "count")
	result["raw"] = redactSiteJSON(payload)
	return result, nil
}

func (a NativeSite) transportMobileDictionaries(ctx context.Context, args []string, cookie string) (map[string]any, *siteError) {
	code, _, valueErr := businessValue(args, "--code")
	if valueErr != nil {
		return nil, valueErr
	}
	payload, requestErr := a.transportMobileCall(ctx, "GET", "/api/system/dict", nil, false, "", cookie, true, true)
	if requestErr != nil {
		return nil, requestErr
	}
	items := make([]map[string]any, 0)
	for _, row := range transportMobileMaps(payload["data"]) {
		rowCode := transportMobileText(row, "code")
		if strings.TrimSpace(code) != "" && rowCode != strings.TrimSpace(code) {
			continue
		}
		items = append(items, map[string]any{"code": rowCode, "description": transportMobileText(row, "desc", "description"), "values": row["value"], "raw": redactSiteJSON(row)})
	}
	result := transportMobileResult("dictionaries", "公开系统字典接口返回 success=true")
	result["data"], result["filters"], result["raw"] = items, map[string]any{"code": strings.TrimSpace(code)}, redactSiteJSON(payload)
	return result, nil
}

func (a NativeSite) transportMobileTableList(ctx context.Context, args []string, cookie string, spec transportMobileTable, operation string) (map[string]any, *siteError) {
	body, filters, bodyErr := transportMobileTableBody(args, spec)
	if bodyErr != nil {
		return nil, bodyErr
	}
	token, _, sessionErr := a.transportMobileSession(args, cookie)
	if sessionErr != nil {
		return nil, sessionErr
	}
	if token == "" {
		return nil, &siteError{Code: "login_required", Message: "请先运行 transport-mobile login 或提供 --access-token"}
	}
	payload, requestErr := a.transportMobileCall(ctx, "PUT", "/api/table/"+spec.table, body, true, token, cookie, true, true)
	if requestErr != nil {
		return nil, requestErr
	}
	data, ok := payload["data"].(map[string]any)
	if !ok {
		return nil, &siteError{Code: "parse_error", Message: "交通移动端列表响应缺少 data 对象"}
	}
	items := make([]map[string]any, 0)
	for _, row := range transportMobileMaps(data["records"]) {
		items = append(items, spec.model(row))
	}
	total := data["total"]
	if total == nil {
		total = len(items)
	}
	result := transportMobileResult(operation, "登录后列表接口返回 success=true")
	result["data"], result["total"], result["filters"], result["raw"] = items, total, filters, redactSiteJSON(payload)
	return result, nil
}

func transportMobileTableBody(args []string, spec transportMobileTable) (map[string]any, map[string]any, *siteError) {
	page, pageErr := businessInt(args, "--page", 1)
	if pageErr != nil {
		return nil, nil, pageErr
	}
	pageSize, pageSizeErr := businessInt(args, "--page-size", 10)
	if pageSizeErr != nil {
		return nil, nil, pageSizeErr
	}
	keyword, _, valueErr := businessValue(args, "--keyword")
	if valueErr != nil {
		return nil, nil, valueErr
	}
	keyword = strings.TrimSpace(keyword)
	body := map[string]any{
		"paginator": map[string]any{"page": page, "pageSize": pageSize, "needAll": false, "pages": 0},
		"sorter":    map[string]any{spec.sortColumn: -1},
		"filter":    map[string]any{},
		"selector":  []string{},
		"populator": spec.populator,
	}
	filters := map[string]any{"keyword": keyword, "page": page, "page_size": pageSize}
	if keyword != "" {
		fuzzy := make([]map[string]any, 0, len(spec.fuzzy))
		for _, field := range spec.fuzzy {
			fuzzy = append(fuzzy, map[string]any{field: map[string]any{"$regex": regexp.QuoteMeta(keyword)}})
		}
		body["fuzzyFilter"] = fuzzy
	}
	return body, filters, nil
}

func (a NativeSite) transportMobileDefenseDetail(ctx context.Context, args []string, cookie string) (map[string]any, *siteError) {
	id, requiredErr := businessRequired(args, "--id", "defense detail 必须提供 --id")
	if requiredErr != nil {
		return nil, requiredErr
	}
	token, _, sessionErr := a.transportMobileSession(args, cookie)
	if sessionErr != nil {
		return nil, sessionErr
	}
	if token == "" {
		return nil, &siteError{Code: "login_required", Message: "请先运行 transport-mobile login 或提供 --access-token"}
	}
	payload, requestErr := a.transportMobileCall(ctx, "GET", "/api/defensesop/"+url.PathEscape(strings.TrimSpace(id)), nil, false, token, cookie, true, true)
	if requestErr != nil {
		return nil, requestErr
	}
	data, ok := payload["data"].(map[string]any)
	if !ok {
		return nil, &siteError{Code: "parse_error", Message: "答辩详情响应缺少 data 对象"}
	}
	result := transportMobileResult("defense", "答辩详情接口返回 success=true")
	result["id"], result["data"], result["defense"], result["raw"] = strings.TrimSpace(id), transportMobileDefense(data), transportMobileDefense(data), redactSiteJSON(payload)
	return result, nil
}

func (a NativeSite) transportMobileFinanceDetail(ctx context.Context, args []string, cookie string) (map[string]any, *siteError) {
	id, requiredErr := businessRequired(args, "--id", "finance detail 必须提供 --id")
	if requiredErr != nil {
		return nil, requiredErr
	}
	spec := transportMobileTables["finances"]
	body := map[string]any{
		"paginator": map[string]any{"page": 1, "pageSize": 1, "needAll": false, "pages": 0},
		"sorter":    map[string]any{spec.sortColumn: -1},
		"filter":    map[string]any{"_id": strings.TrimSpace(id)},
		"selector":  []string{},
		"populator": spec.populator,
	}
	token, _, sessionErr := a.transportMobileSession(args, cookie)
	if sessionErr != nil {
		return nil, sessionErr
	}
	if token == "" {
		return nil, &siteError{Code: "login_required", Message: "请先运行 transport-mobile login 或提供 --access-token"}
	}
	payload, requestErr := a.transportMobileCall(ctx, "PUT", "/api/table/finance", body, true, token, cookie, true, true)
	if requestErr != nil {
		return nil, requestErr
	}
	data, ok := payload["data"].(map[string]any)
	if !ok {
		return nil, &siteError{Code: "parse_error", Message: "财务项目详情响应缺少 data 对象"}
	}
	rows := transportMobileMaps(data["records"])
	if len(rows) == 0 {
		return nil, &siteError{Code: "not_found", Message: "未找到该财务项目", Details: map[string]any{"id": strings.TrimSpace(id)}}
	}
	result := transportMobileResult("finance", "财务项目详情通过列表接口精确回读")
	result["id"], result["data"], result["finance"], result["raw"] = strings.TrimSpace(id), spec.model(rows[0]), spec.model(rows[0]), redactSiteJSON(payload)
	return result, nil
}

func (a NativeSite) transportMobileFinanceItemDetail(ctx context.Context, args []string, cookie string) (map[string]any, *siteError) {
	id, requiredErr := businessRequired(args, "--id", "finance-item detail 必须提供 --id")
	if requiredErr != nil {
		return nil, requiredErr
	}
	token, _, sessionErr := a.transportMobileSession(args, cookie)
	if sessionErr != nil {
		return nil, sessionErr
	}
	if token == "" {
		return nil, &siteError{Code: "login_required", Message: "请先运行 transport-mobile login 或提供 --access-token"}
	}
	row, payload, rowErr := a.transportMobileFinanceItemRow(ctx, strings.TrimSpace(id), token, cookie)
	if rowErr != nil {
		return nil, rowErr
	}
	item := transportMobileFinanceItem(row)
	result := transportMobileResult("finance-item", "财务明细通过列表接口精确回读")
	result["id"], result["data"], result["finance_item"], result["raw"] = strings.TrimSpace(id), item, item, redactSiteJSON(payload)
	return result, nil
}

func transportMobileFinanceItemBody(id string) map[string]any {
	spec := transportMobileTables["finance-items"]
	body := map[string]any{
		"paginator": map[string]any{"page": 1, "pageSize": 1, "needAll": false, "pages": 0},
		"sorter":    map[string]any{spec.sortColumn: -1},
		"filter":    map[string]any{},
		"selector":  []string{},
		"populator": spec.populator,
	}
	if strings.TrimSpace(id) != "" {
		body["filter"] = map[string]any{"_id": strings.TrimSpace(id)}
	}
	return body
}

func (a NativeSite) transportMobileFinanceItemRow(ctx context.Context, id, token, cookie string) (map[string]any, map[string]any, *siteError) {
	payload, requestErr := a.transportMobileCall(ctx, "PUT", "/api/table/fitem", transportMobileFinanceItemBody(id), true, token, cookie, true, true)
	if requestErr != nil {
		return nil, nil, requestErr
	}
	data, ok := payload["data"].(map[string]any)
	if !ok {
		return nil, nil, &siteError{Code: "parse_error", Message: "财务明细响应缺少 data 对象"}
	}
	rows := transportMobileMaps(data["records"])
	if len(rows) == 0 {
		return nil, nil, &siteError{Code: "not_found", Message: "未找到该财务明细", Details: map[string]any{"id": strings.TrimSpace(id)}}
	}
	return rows[0], payload, nil
}

func (a NativeSite) transportMobileRequiredToken(args []string, cookie string) (string, *siteError) {
	token, _, sessionErr := a.transportMobileSession(args, cookie)
	if sessionErr != nil {
		return "", sessionErr
	}
	if token == "" {
		return "", &siteError{Code: "login_required", Message: "请先运行 transport-mobile login 或提供 --access-token"}
	}
	return token, nil
}

func transportMobileFinanceItemMoney(value string) (float64, *siteError) {
	money, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil || math.IsNaN(money) || math.IsInf(money, 0) || money < -999999 || money > 999999 {
		return 0, &siteError{Code: "invalid_argument", Message: "--money 必须是 -999999 到 999999 之间的数字"}
	}
	return money, nil
}

func transportMobileFinanceItemMoneyArg(args []string, current any, required bool) (float64, *siteError) {
	value, found, valueErr := businessValue(args, "--money")
	if valueErr != nil {
		return 0, valueErr
	}
	if !found {
		if required {
			return 0, &siteError{Code: "invalid_argument", Message: "finance-item-create 必须提供 --money"}
		}
		value = strings.TrimSpace(fmt.Sprint(current))
	}
	if strings.TrimSpace(value) == "" {
		return 0, &siteError{Code: "invalid_argument", Message: "--money 不能为空"}
	}
	return transportMobileFinanceItemMoney(value)
}

func transportMobileFinanceItemDirection(value string) (int, *siteError) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "in", "income", "收入":
		return 1, nil
	case "-1", "out", "expense", "支出":
		return -1, nil
	default:
		return 0, &siteError{Code: "invalid_argument", Message: "--direction 必须是 income/收入 或 expense/支出"}
	}
}

func transportMobileFinanceItemDirectionValue(value any) (int, *siteError) {
	if direction, ok := value.(bool); ok {
		if direction {
			return 1, nil
		}
		return -1, nil
	}
	return transportMobileFinanceItemDirection(fmt.Sprint(value))
}

func transportMobileFinanceItemDirectionArg(args []string, current any, required bool) (int, *siteError) {
	value, found, valueErr := businessValue(args, "--direction")
	if valueErr != nil {
		return 0, valueErr
	}
	if found {
		return transportMobileFinanceItemDirection(value)
	}
	if current == nil {
		if required {
			return 0, &siteError{Code: "invalid_argument", Message: "finance-item-create 必须提供 --direction"}
		}
		return 0, &siteError{Code: "protocol_unconfirmed", Message: "财务明细缺少收支方向"}
	}
	return transportMobileFinanceItemDirectionValue(current)
}

func transportMobileFinanceItemTypeArg(args []string, current any) (any, *siteError) {
	value, found, valueErr := businessValue(args, "--type")
	if valueErr != nil {
		return nil, valueErr
	}
	if !found {
		return current, nil
	}
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	return strings.TrimSpace(value), nil
}

func transportMobileFinanceItemRemarkArg(args []string, current string) (string, *siteError) {
	value, found, valueErr := businessValue(args, "--remark")
	if valueErr != nil {
		return "", valueErr
	}
	if !found {
		return current, nil
	}
	return value, nil
}

func transportMobileFinanceItemFileIDs(value any) []string {
	ids := []string{}
	add := func(item any) {
		switch typed := item.(type) {
		case map[string]any:
			if id := transportMobileText(typed, "_id", "id"); id != "" {
				ids = append(ids, id)
			}
		case string:
			if strings.TrimSpace(typed) != "" {
				ids = append(ids, strings.TrimSpace(typed))
			}
		}
	}
	switch typed := value.(type) {
	case []any:
		for _, item := range typed {
			add(item)
		}
	case []map[string]any:
		for _, item := range typed {
			add(item)
		}
	case []string:
		for _, item := range typed {
			add(item)
		}
	}
	return ids
}

func (a NativeSite) transportMobileUploadFinanceFiles(ctx context.Context, args []string, token, cookie string) ([]string, *siteError) {
	paths, valueErr := businessValues(args, "--file")
	if valueErr != nil {
		return nil, valueErr
	}
	ids := make([]string, 0, len(paths))
	for _, path := range paths {
		file, fileErr := siteFilePart("file", path, "交通移动端附件")
		if fileErr != nil {
			return nil, fileErr
		}
		if file.size == 0 {
			return nil, &siteError{Code: "invalid_argument", Message: "交通移动端附件不能为空"}
		}
		result, requestErr := a.execute(ctx, siteRequest{
			Service: transportMobileService, Method: "POST", Path: "/api/file/upload", CookieFile: cookie,
			Headers: []pair{{"token", token}, {"X-Encoded-Filename", "1"}}, Files: []filePart{file},
			ReadOnly: false, Yes: true, RawJSON: true, AllowBusinessFailure: true,
		})
		if requestErr != nil {
			return nil, requestErr
		}
		payload, parseErr := transportMobileEnvelope(result)
		if parseErr != nil {
			return nil, &siteError{Code: "mutation_unverified", Message: "附件上传已发送但响应无法解析", Details: map[string]any{"submitted": true, "confirmed": false, "cause": parseErr.Code}}
		}
		if !transportMobileSuccess(payload) {
			return nil, transportMobileRejected(payload)
		}
		data, ok := payload["data"].(map[string]any)
		if !ok {
			return nil, &siteError{Code: "mutation_unverified", Message: "附件上传成功但响应缺少文件数据", Details: map[string]any{"submitted": true, "confirmed": false}}
		}
		id := transportMobileText(data, "_id", "id")
		if id == "" {
			return nil, &siteError{Code: "mutation_unverified", Message: "附件上传成功但响应缺少文件编号", Details: map[string]any{"submitted": true, "confirmed": false}}
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func (a NativeSite) transportMobileFinanceItemSave(ctx context.Context, args []string, cookie, mode string) (map[string]any, *siteError) {
	if !businessBool(args, "--yes") {
		return nil, &siteError{Code: "confirmation_required", Message: "保存交通移动端财务明细会改变远端数据，请加 --yes"}
	}
	token, tokenErr := a.transportMobileRequiredToken(args, cookie)
	if tokenErr != nil {
		return nil, tokenErr
	}
	entity := map[string]any{}
	projectID := ""
	id := ""
	var existing map[string]any
	if mode == "create" {
		var projectErr *siteError
		projectID, projectErr = businessRequired(args, "--project-id", "finance-item-create 必须提供 --project-id")
		if projectErr != nil {
			return nil, projectErr
		}
		money, moneyErr := transportMobileFinanceItemMoneyArg(args, nil, true)
		if moneyErr != nil {
			return nil, moneyErr
		}
		direction, directionErr := transportMobileFinanceItemDirectionArg(args, nil, true)
		if directionErr != nil {
			return nil, directionErr
		}
		typeValue, typeErr := transportMobileFinanceItemTypeArg(args, nil)
		if typeErr != nil {
			return nil, typeErr
		}
		remark, remarkErr := transportMobileFinanceItemRemarkArg(args, "")
		if remarkErr != nil {
			return nil, remarkErr
		}
		entity["project"], entity["money"], entity["dir"] = strings.TrimSpace(projectID), money, direction
		entity["type"], entity["remark"], entity["file"] = typeValue, remark, []string{}
	} else {
		var idErr *siteError
		id, idErr = businessRequired(args, "--id", "finance-item-update 必须提供 --id")
		if idErr != nil {
			return nil, idErr
		}
		var rowErr *siteError
		existing, _, rowErr = a.transportMobileFinanceItemRow(ctx, strings.TrimSpace(id), token, cookie)
		if rowErr != nil {
			return nil, rowErr
		}
		projectID = transportMobileText(existing, "project._id", "project.id", "projectId", "project_id")
		if projectID == "" {
			return nil, &siteError{Code: "protocol_unconfirmed", Message: "财务明细缺少所属财务项目编号"}
		}
		money, moneyErr := transportMobileFinanceItemMoneyArg(args, existing["money"], false)
		if moneyErr != nil {
			return nil, moneyErr
		}
		direction, directionErr := transportMobileFinanceItemDirectionArg(args, existing["dir"], false)
		if directionErr != nil {
			return nil, directionErr
		}
		typeValue, typeErr := transportMobileFinanceItemTypeArg(args, existing["type"])
		if typeErr != nil {
			return nil, typeErr
		}
		remark, remarkErr := transportMobileFinanceItemRemarkArg(args, transportMobileText(existing, "remark"))
		if remarkErr != nil {
			return nil, remarkErr
		}
		version := existing["version"]
		if version == nil || strings.TrimSpace(fmt.Sprint(version)) == "" {
			return nil, &siteError{Code: "protocol_unconfirmed", Message: "财务明细缺少并发版本号，无法安全修改"}
		}
		entity["_id"], entity["version"] = strings.TrimSpace(id), version
		entity["project"], entity["money"], entity["dir"] = projectID, money, direction
		entity["type"], entity["remark"] = typeValue, remark
		fileIDs := transportMobileFinanceItemFileIDs(existing["file"])
		explicitFileIDs, fileIDErr := businessValues(args, "--file-id")
		if fileIDErr != nil {
			return nil, fileIDErr
		}
		if businessBool(args, "--clear-files") && len(explicitFileIDs) > 0 {
			return nil, &siteError{Code: "invalid_argument", Message: "--clear-files 不能与 --file-id 一起使用"}
		}
		if businessBool(args, "--clear-files") {
			fileIDs = []string{}
		} else if len(explicitFileIDs) > 0 {
			fileIDs = explicitFileIDs
		}
		entity["file"] = fileIDs
	}
	uploaded, uploadErr := a.transportMobileUploadFinanceFiles(ctx, args, token, cookie)
	if uploadErr != nil {
		return nil, uploadErr
	}
	entityFiles := transportMobileFinanceItemFileIDs(entity["file"])
	entityFiles = append(entityFiles, uploaded...)
	entity["file"] = entityFiles
	payload, requestErr := a.transportMobileCall(ctx, "POST", "/api/financesop/"+url.PathEscape(strings.TrimSpace(projectID)), map[string]any{"type": mode, "entity": entity}, true, token, cookie, false, true)
	if requestErr != nil {
		return nil, requestErr
	}
	operation := "finance-item-" + mode
	result := transportMobileResult(operation, "财务明细保存接口返回 success=true")
	result["submitted"], result["project_id"], result["api_code"] = true, strings.TrimSpace(projectID), payload["code"]
	if id != "" {
		result["id"] = strings.TrimSpace(id)
	}
	result["files_uploaded"] = len(uploaded)
	if data, ok := payload["data"]; ok {
		result["data"] = redactSiteJSON(data)
	}
	if mode == "update" {
		row, _, readbackErr := a.transportMobileFinanceItemRow(ctx, strings.TrimSpace(id), token, cookie)
		if readbackErr != nil {
			return nil, &siteError{Code: "mutation_unverified", Message: "财务明细保存成功反馈已返回，但回读失败", Details: map[string]any{"submitted": true, "confirmed": false, "id": strings.TrimSpace(id), "cause": readbackErr.Code}}
		}
		if !transportMobileFinanceItemMatches(row, entity) {
			return nil, &siteError{Code: "mutation_unverified", Message: "财务明细保存成功反馈已返回，但回读内容不一致", Details: map[string]any{"submitted": true, "confirmed": false, "id": strings.TrimSpace(id)}}
		}
		result["evidence"] = "server-success-and-item-readback"
	}
	return result, nil
}

func transportMobileFinanceItemMatches(row, entity map[string]any) bool {
	if transportMobileText(row, "project._id", "project.id", "projectId", "project_id") != strings.TrimSpace(fmt.Sprint(entity["project"])) {
		return false
	}
	money, moneyErr := transportMobileFinanceItemMoney(fmt.Sprint(entity["money"]))
	actualMoney, actualErr := transportMobileFinanceItemMoney(fmt.Sprint(row["money"]))
	if moneyErr != nil || actualErr != nil || math.Abs(money-actualMoney) > 1e-9 {
		return false
	}
	direction, directionErr := transportMobileFinanceItemDirectionValue(row["dir"])
	if directionErr != nil || direction != int(entity["dir"].(int)) {
		return false
	}
	if fmt.Sprint(row["type"]) != fmt.Sprint(entity["type"]) || transportMobileText(row, "remark") != fmt.Sprint(entity["remark"]) {
		return false
	}
	actualFiles := transportMobileFinanceItemFileIDs(row["file"])
	expectedFiles := transportMobileFinanceItemFileIDs(entity["file"])
	if len(actualFiles) != len(expectedFiles) {
		return false
	}
	for index := range actualFiles {
		if actualFiles[index] != expectedFiles[index] {
			return false
		}
	}
	return true
}

func (a NativeSite) transportMobileFinanceItemDelete(ctx context.Context, args []string, cookie string) (map[string]any, *siteError) {
	if !businessBool(args, "--yes") {
		return nil, &siteError{Code: "confirmation_required", Message: "删除交通移动端财务明细会删除远端数据，请加 --yes"}
	}
	id, idErr := businessRequired(args, "--id", "finance-item-delete 必须提供 --id")
	if idErr != nil {
		return nil, idErr
	}
	token, tokenErr := a.transportMobileRequiredToken(args, cookie)
	if tokenErr != nil {
		return nil, tokenErr
	}
	row, _, rowErr := a.transportMobileFinanceItemRow(ctx, strings.TrimSpace(id), token, cookie)
	if rowErr != nil {
		return nil, rowErr
	}
	projectID := transportMobileText(row, "project._id", "project.id", "projectId", "project_id")
	if projectID == "" {
		return nil, &siteError{Code: "protocol_unconfirmed", Message: "财务明细缺少所属财务项目编号，无法刷新项目汇总"}
	}
	payload, requestErr := a.transportMobileCall(ctx, "DELETE", "/api/table/fitem/"+url.PathEscape(strings.TrimSpace(id)), nil, false, token, cookie, false, true)
	if requestErr != nil {
		return nil, requestErr
	}
	if _, patchErr := a.transportMobileCall(ctx, "PATCH", "/api/financesop/"+url.PathEscape(projectID), map[string]any{}, true, token, cookie, false, true); patchErr != nil {
		return nil, &siteError{Code: "mutation_unverified", Message: "财务明细删除成功反馈已返回，但项目汇总刷新失败", Details: map[string]any{"submitted": true, "confirmed": false, "id": strings.TrimSpace(id), "cause": patchErr.Code}}
	}
	if _, _, readbackErr := a.transportMobileFinanceItemRow(ctx, strings.TrimSpace(id), token, cookie); readbackErr == nil || readbackErr.Code != "not_found" {
		return nil, &siteError{Code: "mutation_unverified", Message: "财务明细删除成功反馈已返回，但删除结果无法确认", Details: map[string]any{"submitted": true, "confirmed": false, "id": strings.TrimSpace(id)}}
	}
	result := transportMobileResult("finance-item-delete", "删除接口成功、项目汇总刷新成功且详情回读为不存在")
	result["submitted"], result["id"], result["project_id"], result["api_code"] = true, strings.TrimSpace(id), projectID, payload["code"]
	return result, nil
}

func transportMobileUser(row map[string]any) map[string]any {
	return map[string]any{
		"id":            transportMobileText(row, "_id", "ID", "id"),
		"account":       transportMobileText(row, "code"),
		"name":          transportMobileText(row, "name"),
		"nickname":      transportMobileText(row, "nickname"),
		"job_number":    transportMobileText(row, "jobNumber"),
		"title":         row["title"],
		"gender":        row["gender"],
		"phone":         row["phone"],
		"group":         row["group"],
		"pending_count": row["numPending"],
		"roles":         row["roles"],
		"permissions":   row["permissions"],
		"raw":           redactSiteJSON(row),
	}
}

func transportMobileRecord(row map[string]any) map[string]any {
	return map[string]any{
		"id":         transportMobileText(row, "_id", "id"),
		"code":       transportMobileText(row, "code"),
		"name":       transportMobileText(row, "name"),
		"status":     row["status"],
		"creator":    transportMobileText(row, "creater.name", "creator.name", "creater"),
		"created_at": transportMobileValue(row, "dateCreate", "created_at"),
		"updated_at": transportMobileValue(row, "version", "dateModified", "updated_at"),
		"raw":        redactSiteJSON(row),
	}
}

func transportMobileDefense(row map[string]any) map[string]any {
	result := transportMobileRecord(row)
	result["is_generated"] = transportMobileValue(row, "isGenerated")
	result["file_count"] = transportMobileValue(row, "stat.file")
	result["expert_count"] = transportMobileValue(row, "stat.expert")
	result["student_count"] = transportMobileValue(row, "stat.student")
	if experts := transportMobileMaps(transportMobileValue(row, "expert", "experts")); len(experts) > 0 {
		result["experts"] = experts
	}
	if students := transportMobileMaps(transportMobileValue(row, "student", "students")); len(students) > 0 {
		result["students"] = students
	}
	return result
}

func transportMobileFinance(row map[string]any) map[string]any {
	result := transportMobileRecord(row)
	result["owner"] = transportMobileText(row, "ownname")
	result["project_code"] = transportMobileText(row, "ficode")
	result["stage"] = transportMobileText(row, "stage")
	result["total_amount"] = transportMobileValue(row, "stat.total")
	result["expense_amount"] = transportMobileValue(row, "stat.fee")
	result["balance"] = transportMobileValue(row, "stat.left")
	result["allowed_amount"] = transportMobileValue(row, "stat.allow")
	result["allowed_balance"] = transportMobileValue(row, "stat.allowleft")
	result["expense_rate"] = transportMobileValue(row, "stat.feerate")
	result["allowed_rate"] = transportMobileValue(row, "stat.allowrate")
	return result
}

func transportMobileFinanceItem(row map[string]any) map[string]any {
	result := transportMobileRecord(row)
	result["project_id"] = transportMobileText(row, "project._id", "project.id", "projectId", "project_id")
	result["project_name"] = transportMobileText(row, "project.name")
	result["project"] = row["project"]
	result["amount"] = row["money"]
	result["direction"] = transportMobileValue(row, "dir", "direction")
	result["item_type"] = row["type"]
	result["remark"] = transportMobileText(row, "remark")
	result["creator_detail"] = transportMobileValue(row, "creater", "creator")
	result["files"] = row["file"]
	return result
}

func transportMobileNote(row map[string]any) map[string]any {
	result := transportMobileRecord(row)
	result["participants"] = row["participants"]
	result["last_reply"] = transportMobileValue(row, "dateModified")
	return result
}

func transportMobileAccessRecord(row map[string]any) map[string]any {
	result := transportMobileRecord(row)
	result["employee_code"] = transportMobileText(row, "employeeCode")
	result["person_name"] = transportMobileText(row, "personName")
	result["department_name"] = transportMobileText(row, "departmentName")
	result["device_alias"] = transportMobileText(row, "deviceAlias")
	result["event_description"] = transportMobileText(row, "eventDescription")
	result["event_time"] = transportMobileValue(row, "eventTime")
	result["verify_mode"] = transportMobileText(row, "verifyModeName")
	return result
}

func transportMobileAchievement(row map[string]any) map[string]any {
	result := transportMobileRecord(row)
	result["achievement_type"] = transportMobileValue(row, "type")
	result["event"] = transportMobileValue(row, "event")
	result["level"] = transportMobileValue(row, "level")
	result["student_code"] = transportMobileText(row, "studentCode", "student.code")
	result["student_name"] = transportMobileText(row, "studentName", "student.name")
	result["authorization_number"] = transportMobileText(row, "authorizationNo")
	result["first_inventor"] = transportMobileText(row, "firstInventor")
	result["journal"] = transportMobileText(row, "journal")
	result["indexing"] = transportMobileValue(row, "indexing")
	result["competition"] = transportMobileValue(row, "competition")
	result["award_level"] = transportMobileValue(row, "awardLevel")
	result["completed_at"] = transportMobileValue(row, "dateComplete", "completed_at")
	result["applicant"] = transportMobileValue(row, "applicant")
	result["applied_at"] = transportMobileValue(row, "dateApply", "applied_at")
	return result
}

func transportMobileKPI(row map[string]any) map[string]any {
	result := transportMobileRecord(row)
	result["kpi_type"] = transportMobileValue(row, "type")
	result["owner"] = transportMobileValue(row, "owner")
	result["block"] = transportMobileValue(row, "block")
	result["batch"] = transportMobileValue(row, "batch")
	return result
}

func transportMobileNotice(row map[string]any) map[string]any {
	result := transportMobileRecord(row)
	result["title"] = transportMobileText(row, "title")
	result["notice_type"] = transportMobileValue(row, "type")
	result["file"] = row["file"]
	result["audience"] = row["public"]
	result["publisher"] = transportMobileValue(row, "user", "creater")
	result["published_at"] = transportMobileValue(row, "dateCreate")
	return result
}

func transportMobileWorkflow(row map[string]any) map[string]any {
	result := transportMobileRecord(row)
	result["workflow_type"] = transportMobileValue(row, "type")
	result["applicant"] = transportMobileValue(row, "user", "applicant")
	result["sms_notified"] = transportMobileValue(row, "sms", "sendSms")
	result["current_node"] = transportMobileValue(row, "current.node")
	result["approver"] = transportMobileValue(row, "current.user")
	result["submitted_at"] = transportMobileValue(row, "dateCreate")
	result["completed_at"] = transportMobileValue(row, "dateComplete")
	return result
}

func transportMobileVacation(row map[string]any) map[string]any {
	result := transportMobileRecord(row)
	result["applicant"] = transportMobileValue(row, "user", "applicant")
	result["department"] = row["department"]
	result["period"] = transportMobileValue(row, "period", "time")
	result["days"] = transportMobileValue(row, "days")
	result["leave_type"] = transportMobileValue(row, "type")
	result["reason"] = transportMobileText(row, "reason")
	result["applied_at"] = transportMobileValue(row, "dateCreate")
	return result
}

func transportMobileValue(value any, paths ...string) any {
	for _, path := range paths {
		current := value
		found := true
		for _, part := range strings.Split(path, ".") {
			object, ok := current.(map[string]any)
			if !ok {
				found = false
				break
			}
			current, ok = object[part]
			if !ok {
				found = false
				break
			}
		}
		if found && current != nil {
			return current
		}
	}
	return nil
}

func transportMobileText(value map[string]any, paths ...string) string {
	item := transportMobileValue(value, paths...)
	if item == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(item))
}

func transportMobileMaps(value any) []map[string]any {
	result := make([]map[string]any, 0)
	switch typed := value.(type) {
	case []any:
		for _, item := range typed {
			if row, ok := item.(map[string]any); ok {
				result = append(result, row)
			}
		}
	case []map[string]any:
		result = append(result, typed...)
	}
	return result
}
