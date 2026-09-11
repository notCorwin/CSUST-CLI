package adapter

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"regexp"
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
	case "defense":
		return a.transportMobileDefenseDetail(ctx, args[1:], cookie)
	case "finances":
		return a.transportMobileTableList(ctx, args[1:], cookie, transportMobileTables["finances"], "finances")
	case "finance":
		return a.transportMobileFinanceDetail(ctx, args[1:], cookie)
	default:
		return nil, &siteError{Code: "invalid_argument", Message: "transport-mobile 只支持 login、send-code、logout、profile、pending、dictionaries、defenses、defense、finances、finance、catalog"}
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
