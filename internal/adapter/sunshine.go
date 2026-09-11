package adapter

import (
	"context"
	"net/url"
	"regexp"
	"strings"
	"time"
)

var sunshinePhone = regexp.MustCompile(`^1[3-9][0-9]{9}$`)
var sunshineEmail = regexp.MustCompile(`^\w+([-.]\w+)*@\w+([-.]\w+)*\.\w+([-.]\w+)*$`)
var sunshineCode = regexp.MustCompile(`^[0-9]{6}$`)

func (a NativeSite) executeSunshine(ctx context.Context, args []string) (map[string]any, *siteError) {
	if len(args) == 0 || args[0] == "catalog" {
		return businessCatalogNames("sunshine"), nil
	}
	cookie, _, valueErr := businessValue(args[1:], "--cookie-file")
	if valueErr != nil {
		return nil, valueErr
	}
	switch args[0] {
	case "issues", "list":
		body, filters, parseErr := sunshineIssueQuery(args[1:])
		if parseErr != nil {
			return nil, parseErr
		}
		result, requestErr := a.sunshineJSON(ctx, "PUT", "/api/issues", body, cookie, true, true)
		if requestErr != nil {
			return nil, requestErr
		}
		result = businessResult(result, "sunshine", "issues")
		result["filters"] = filters
		return result, nil
	case "issue", "detail":
		id, requiredErr := businessRequired(args[1:], "--id", "sunshine issue 必须提供 --id")
		if requiredErr != nil {
			return nil, requiredErr
		}
		body := sunshineIssueDetailBody()
		result, requestErr := a.sunshineJSON(ctx, "POST", "/api/issues/"+url.PathEscape(id), body, cookie, true, true)
		if requestErr != nil {
			return nil, requestErr
		}
		return businessResult(result, "sunshine", "issue"), nil
	case "submit", "create", "suggestion", "complaint":
		defaultType := ""
		if args[0] == "suggestion" {
			defaultType = "建议咨询"
		} else if args[0] == "complaint" {
			defaultType = "服务投诉"
		}
		return a.sunshineCreate(ctx, args[1:], cookie, defaultType)
	case "departments":
		body := map[string]any{"paginator": map[string]any{"needAll": true}, "selector": []string{"name", "nickname"}, "sorter": map[string]any{"_p_nickname": 1}}
		result, requestErr := a.sunshineJSON(ctx, "PUT", "/api/departments", body, cookie, true, true)
		if requestErr != nil {
			return nil, requestErr
		}
		return businessResult(result, "sunshine", "departments"), nil
	case "stats":
		result, requestErr := a.sunshineJSON(ctx, "GET", "/api/issuestat", nil, cookie, true, true)
		if requestErr != nil {
			return nil, requestErr
		}
		return businessResult(result, "sunshine", "stats"), nil
	case "config":
		result, requestErr := a.sunshineJSON(ctx, "GET", "/api/systems", nil, cookie, true, true)
		if requestErr != nil {
			return nil, requestErr
		}
		return businessResult(result, "sunshine", "config"), nil
	case "send-code":
		if !businessBool(args[1:], "--yes") {
			return nil, &siteError{Code: "confirmation_required", Message: "发送阳光服务短信验证码会向手机号发短信，必须加 --yes"}
		}
		phone, requiredErr := businessRequired(args[1:], "--phone", "send-code 必须提供 --phone")
		if requiredErr != nil {
			return nil, requiredErr
		}
		if !sunshinePhone.MatchString(strings.TrimSpace(phone)) {
			return nil, &siteError{Code: "invalid_argument", Message: "--phone 必须是 11 位手机号码"}
		}
		result, requestErr := a.sunshineJSON(ctx, "POST", "/api/verifys", map[string]string{"tel": phone}, cookie, false, true)
		if requestErr != nil {
			return nil, requestErr
		}
		result = businessResult(result, "sunshine", "send-code")
		result["verified_by"] = "response-success"
		return result, nil
	default:
		return nil, &siteError{Code: "invalid_argument", Message: "sunshine 只支持 issues、issue、submit、suggestion、complaint、departments、stats、config、send-code、catalog"}
	}
}

func (a NativeSite) sunshineCreate(ctx context.Context, args []string, cookie, defaultType string) (map[string]any, *siteError) {
	if !businessBool(args, "--yes") {
		return nil, &siteError{Code: "confirmation_required", Message: "提交阳光服务诉求会改变远端状态，必须加 --yes"}
	}
	title, found, err := businessValue(args, "--title")
	if err != nil {
		return nil, err
	}
	if !found {
		title, found, err = businessValue(args, "--name")
		if err != nil {
			return nil, err
		}
	}
	title = strings.TrimSpace(title)
	if !found || title == "" {
		return nil, &siteError{Code: "invalid_argument", Message: "提交诉求必须提供 --title"}
	}
	if len([]rune(title)) < 2 {
		return nil, &siteError{Code: "invalid_argument", Message: "--title 至少需要 2 个字符"}
	}
	content, requiredErr := businessRequired(args, "--content", "提交诉求必须提供 --content")
	if requiredErr != nil {
		return nil, requiredErr
	}
	content = strings.TrimSpace(content)
	if len([]rune(content)) < 20 {
		return nil, &siteError{Code: "invalid_argument", Message: "--content 至少需要 20 个字符"}
	}
	typeName := strings.TrimSpace(flagValue(args, "--type"))
	if typeName == "" {
		typeName = defaultType
	}
	switch strings.ToLower(typeName) {
	case "suggestion", "咨询", "建议", "建议咨询":
		typeName = "建议咨询"
	case "complaint", "投诉", "服务投诉":
		typeName = "服务投诉"
	default:
		return nil, &siteError{Code: "invalid_argument", Message: "--type 必须是 suggestion 或 complaint"}
	}
	departmentID, departmentErr := a.sunshineDepartmentID(ctx, args, cookie)
	if departmentErr != nil {
		return nil, departmentErr
	}
	reporter, reporterErr := businessRequired(args, "--reporter", "匿名提交必须提供 --reporter")
	if reporterErr != nil {
		return nil, reporterErr
	}
	phone, phoneErr := businessRequired(args, "--phone", "匿名提交必须提供 --phone")
	if phoneErr != nil {
		return nil, phoneErr
	}
	phone = strings.TrimSpace(phone)
	if !sunshinePhone.MatchString(phone) {
		return nil, &siteError{Code: "invalid_argument", Message: "--phone 必须是 11 位手机号码"}
	}
	email, emailErr := businessRequired(args, "--email", "匿名提交必须提供 --email")
	if emailErr != nil {
		return nil, emailErr
	}
	email = strings.TrimSpace(email)
	if !sunshineEmail.MatchString(email) {
		return nil, &siteError{Code: "invalid_argument", Message: "--email 格式无效"}
	}
	role := strings.TrimSpace(flagValue(args, "--role"))
	switch strings.ToLower(role) {
	case "student", "学生", "本校学生":
		role = "本校学生"
	case "staff", "teacher", "教职工", "本校教职工":
		role = "本校教职工"
	case "external", "other", "校外", "校外人士":
		role = "校外人士"
	default:
		return nil, &siteError{Code: "invalid_argument", Message: "--role 必须是 student、staff 或 external"}
	}
	code, codeErr := businessRequired(args, "--code", "匿名提交必须提供 --code")
	if codeErr != nil {
		return nil, codeErr
	}
	code = strings.TrimSpace(code)
	if !sunshineCode.MatchString(code) {
		return nil, &siteError{Code: "invalid_argument", Message: "--code 必须是 6 位数字验证码"}
	}
	dateExpected, dateErr := sunshineExpectedDate(args)
	if dateErr != nil {
		return nil, dateErr
	}
	public, publicErr := sunshinePublicOption(args)
	if publicErr != nil {
		return nil, publicErr
	}
	body := map[string]any{
		"name": title, "department": departmentID, "content": content, "type": typeName,
		"dateExpected": dateExpected, "reporter": reporter, "phone": phone, "email": email,
		"role": role, "verifyCode": code, "needVerifyCode": true, "isPublic": public,
		"attachments": []any{},
	}
	path := "/api/issues"
	created, requestErr := a.sunshineJSON(ctx, "POST", path, body, cookie, false, true)
	if requestErr != nil {
		return nil, requestErr
	}
	payload, parseErr := businessJSONMap(created)
	if parseErr != nil {
		return nil, &siteError{Code: "mutation_unverified", Message: "诉求提交已发送但响应无法解析", Details: map[string]any{"api": path, "cause": parseErr.Code}}
	}
	data, ok := payload["data"].(map[string]any)
	issueID := ""
	if ok {
		issueID = nestedString(data, "_id")
	}
	if issueID == "" {
		return nil, &siteError{Code: "mutation_unverified", Message: "诉求提交已发送但响应缺少诉求编号", Details: map[string]any{"api": path}}
	}
	verificationPath := "/api/issues/" + url.PathEscape(issueID)
	verification, verifyErr := a.sunshineJSON(ctx, "POST", verificationPath, sunshineIssueDetailBody(), cookie, true, true)
	if verifyErr != nil {
		return nil, &siteError{Code: "mutation_unverified", Message: "诉求已提交但详情回读失败", Details: map[string]any{"api": path, "issue_id": issueID, "cause": verifyErr.Code}}
	}
	verificationPayload, verifyParseErr := businessJSONMap(verification)
	if verifyParseErr != nil {
		return nil, &siteError{Code: "mutation_unverified", Message: "诉求已提交但详情回读无法解析", Details: map[string]any{"api": verificationPath, "issue_id": issueID}}
	}
	verificationData, ok := verificationPayload["data"].(map[string]any)
	if !ok || nestedString(verificationData, "_id") != issueID || nestedString(verificationData, "name") != title {
		return nil, &siteError{Code: "mutation_unverified", Message: "诉求已提交但详情回读与提交内容不一致", Details: map[string]any{"api": verificationPath, "issue_id": issueID}}
	}
	result := businessResult(created, "sunshine", "submit")
	result["submitted"], result["confirmed"] = true, true
	result["issue_id"] = issueID
	result["verification"] = map[string]any{"api": verificationPath, "data": verificationData}
	result["evidence"] = "POST /api/issues success and detail readback"
	return result, nil
}

func sunshineIssueDetailBody() map[string]any {
	return map[string]any{"populator": []map[string]any{
		{"path": "department", "select": "name nickname"},
		{"path": "attachments", "select": "dateCreate size name fname fullname"},
	}}
}

func (a NativeSite) sunshineDepartmentID(ctx context.Context, args []string, cookie string) (string, *siteError) {
	departmentID, idFound, idErr := businessValue(args, "--department-id")
	if idErr != nil {
		return "", idErr
	}
	departmentName, nameFound, nameErr := businessValue(args, "--department")
	if nameErr != nil {
		return "", nameErr
	}
	if idFound && nameFound {
		return "", &siteError{Code: "invalid_argument", Message: "--department-id 与 --department 只能提供一个"}
	}
	if idFound {
		if strings.TrimSpace(departmentID) == "" {
			return "", &siteError{Code: "invalid_argument", Message: "--department-id 不能为空"}
		}
		return strings.TrimSpace(departmentID), nil
	}
	if !nameFound || strings.TrimSpace(departmentName) == "" {
		return "", &siteError{Code: "invalid_argument", Message: "提交诉求必须提供 --department 或 --department-id"}
	}
	body := map[string]any{
		"filter":    map[string]any{"acceptIssue": true},
		"paginator": map[string]any{"needAll": true},
		"selector":  []string{"name", "nickname", "contacts"},
		"populator": []map[string]any{{"path": "contacts", "select": "name phone"}},
		"sorter":    map[string]any{"priority": -1, "_p_name": 1},
	}
	result, requestErr := a.sunshineJSON(ctx, "PUT", "/api/departments", body, cookie, true, true)
	if requestErr != nil {
		return "", requestErr
	}
	payload, parseErr := businessJSONMap(result)
	if parseErr != nil {
		return "", parseErr
	}
	items, ok := payload["data"].([]any)
	if !ok {
		return "", &siteError{Code: "response_error", Message: "阳光服务部门列表格式无效"}
	}
	wanted := strings.TrimSpace(departmentName)
	found := ""
	for _, value := range items {
		item, ok := value.(map[string]any)
		if !ok {
			continue
		}
		name, _ := item["name"].(string)
		nickname, _ := item["nickname"].(string)
		if wanted != name && wanted != nickname {
			continue
		}
		id := nestedString(item, "_id")
		if id == "" {
			return "", &siteError{Code: "response_error", Message: "阳光服务部门缺少编号"}
		}
		if found != "" && found != id {
			return "", &siteError{Code: "invalid_argument", Message: "--department 匹配到多个部门，请改用 --department-id"}
		}
		found = id
	}
	if found == "" {
		return "", &siteError{Code: "invalid_argument", Message: "未找到可受理该诉求的部门: " + wanted}
	}
	return found, nil
}

func sunshineExpectedDate(args []string) (string, *siteError) {
	value := strings.TrimSpace(flagValue(args, "--expected-date"))
	if value == "" {
		value = strings.TrimSpace(flagValue(args, "--date-expected"))
	}
	location := time.Local
	today := time.Now().In(location)
	date := today.AddDate(0, 0, 7)
	if value != "" {
		parsed, err := time.ParseInLocation("2006-01-02", value, location)
		if err != nil {
			return "", &siteError{Code: "invalid_argument", Message: "--expected-date 必须是 YYYY-MM-DD"}
		}
		date = parsed
	}
	start := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, location)
	if !date.After(start) {
		return "", &siteError{Code: "invalid_argument", Message: "期待解决日期必须晚于今天"}
	}
	return date.UTC().Format("2006-01-02T15:04:05.000Z"), nil
}

func sunshinePublicOption(args []string) (bool, *siteError) {
	public, private := businessBool(args, "--public"), businessBool(args, "--private")
	if public && private {
		return false, &siteError{Code: "invalid_argument", Message: "--public 与 --private 不能同时提供"}
	}
	return !private, nil
}

func sunshineIssueQuery(args []string) (map[string]any, map[string]any, *siteError) {
	page, pageErr := businessInt(args, "--page", 1)
	if pageErr != nil {
		return nil, nil, pageErr
	}
	pageSize, pageSizeErr := businessInt(args, "--page-size", 30)
	if pageSizeErr != nil {
		return nil, nil, pageSizeErr
	}
	statuses, statusErr := businessValues(args, "--status")
	if statusErr != nil {
		return nil, nil, statusErr
	}
	filter := map[string]any{"isPublic": true}
	if len(statuses) > 0 {
		filter["status"] = map[string]any{"$in": statuses}
	} else if !businessBool(args, "--include-retracted") {
		filter["status"] = map[string]any{"$ne": "已撤销"}
	}
	body := map[string]any{
		"paginator": map[string]any{"page": page, "pages": 0, "count": 0, "pageSize": pageSize, "needAll": false},
		"filter":    filter,
		"sorter":    map[string]any{"version": -1},
		"populator": []map[string]any{{"path": "department", "select": "name nickname"}},
	}
	return body, map[string]any{"page": page, "page_size": pageSize, "status": statuses, "include_retracted": businessBool(args, "--include-retracted")}, nil
}

func (a NativeSite) sunshineJSON(ctx context.Context, method, path string, body any, cookie string, readOnly, yes bool) (map[string]any, *siteError) {
	request := siteRequest{Service: "sunshine", Method: method, Path: path, CookieFile: cookie, RawJSON: true, ReadOnly: readOnly, Yes: yes}
	if body != nil {
		request.JSON, request.HasJSON = body, true
	}
	return a.execute(ctx, request)
}
