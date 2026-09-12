package adapter

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const studentDigitalArchiveService = "student-digital-archive"

type studentDigitalArchiveSession struct {
	token     string
	userID    string
	cookie    string
	tokenPath string
	userPath  string
}

func (a NativeSite) executeStudentDigitalArchive(ctx context.Context, args []string) (map[string]any, *siteError) {
	if len(args) == 0 || args[0] == "catalog" {
		return businessCatalogFilter(studentDigitalArchiveService), nil
	}
	cookie, _, valueErr := businessValue(args[1:], "--cookie-file")
	if valueErr != nil {
		return nil, valueErr
	}
	switch args[0] {
	case "login":
		return a.studentDigitalArchiveLogin(ctx, args[1:], cookie)
	case "logout":
		return a.studentDigitalArchiveLogout(ctx, args[1:], cookie)
	}
	session, sessionErr := loadStudentDigitalArchiveSession(args[1:], cookie)
	if sessionErr != nil {
		return nil, sessionErr
	}
	if args[0] == "status" {
		return a.studentDigitalArchiveStatus(ctx, session)
	}
	if session.token == "" {
		return nil, &siteError{Code: "login_required", Message: "请先运行 student-digital-archive login，或提供 --access-token"}
	}
	switch args[0] {
	case "profile":
		return a.studentDigitalArchiveSimple(ctx, args[1:], session, "profile", "/tsa/shc/homepage/personInfo", true, false)
	case "overview", "dashboard":
		return a.studentDigitalArchiveOverview(ctx, args[1:], session)
	case "catalogs", "categories":
		return a.studentDigitalArchiveCatalogs(ctx, session)
	case "forms":
		return a.studentDigitalArchiveForms(ctx, args[1:], session)
	case "records":
		return a.studentDigitalArchiveRecords(ctx, args[1:], session)
	case "grades":
		return a.studentDigitalArchiveSimple(ctx, args[1:], session, "grades", "/tsa/shc/learnInfo/scoreCardList", true, false)
	case "credit", "credits":
		return a.studentDigitalArchiveSimple(ctx, args[1:], session, "credit", "/tsa/shc/learnInfo/learnSituation", true, false)
	case "rank", "grade-rank":
		return a.studentDigitalArchiveSimple(ctx, args[1:], session, "rank", "/tsa/shc/learnInfo/gradePointRanking", true, false)
	case "learning-gap":
		return a.studentDigitalArchiveSimple(ctx, args[1:], session, "learning-gap", "/tsa/shc/learnInfo/learningGap", true, false)
	case "schedule":
		return a.studentDigitalArchiveSchedule(ctx, args[1:], session)
	case "borrow":
		return a.studentDigitalArchiveBorrow(ctx, args[1:], session)
	case "recommendations", "book-recommendations":
		return a.studentDigitalArchiveSimple(ctx, args[1:], session, "recommendations", "/tsa/shc/borrow/bookRecommend", true, true)
	case "attendance":
		return a.studentDigitalArchiveSimple(ctx, args[1:], session, "attendance", "/tsa/shc/homepage/getMyAttendance", false, false)
	case "awards", "rewards":
		return a.studentDigitalArchiveSimple(ctx, args[1:], session, "awards", "/tsa/shc/homepage/bonusPenaltyList", true, false)
	case "labels":
		return a.studentDigitalArchiveSimple(ctx, args[1:], session, "labels", "/tsa/shc/homepage/getStuLableList", true, true)
	case "set-labels":
		return a.studentDigitalArchiveSetLabels(ctx, args[1:], session)
	case "timeline":
		return a.studentDigitalArchiveSimplePost(ctx, args[1:], session, "timeline", "/tsa/shc/personalInfo/personYearAchievements/personaltime", true)
	case "consumption":
		return a.studentDigitalArchiveSimple(ctx, args[1:], session, "consumption", "/tsa/shc/homepage/cardConsume", true, false)
	case "internet":
		return a.studentDigitalArchiveSimple(ctx, args[1:], session, "internet", "/tsa/shc/homepage/onlineState", true, false)
	case "internet-usage":
		return a.studentDigitalArchivePeriod(ctx, args[1:], session, "internet-usage", "/tsa/shc/onlineInfo/onlineDetailGrid")
	case "internet-flow":
		return a.studentDigitalArchivePeriod(ctx, args[1:], session, "internet-flow", "/tsa/shc/onlineInfo/flowDetailGrid")
	case "consumption-details":
		return a.studentDigitalArchivePeriod(ctx, args[1:], session, "consumption-details", "/tsa/shc/cardConsume/tradingDetailGrid")
	case "notes":
		return a.studentDigitalArchiveNotes(ctx, args[1:], session)
	case "note":
		return a.studentDigitalArchiveNote(ctx, args[1:], session)
	case "save-note":
		return a.studentDigitalArchiveSaveNote(ctx, args[1:], session)
	case "delete-note":
		return a.studentDigitalArchiveDeleteNote(ctx, args[1:], session)
	default:
		return nil, &siteError{Code: "invalid_argument", Message: "student-digital-archive 不支持该子命令: " + args[0]}
	}
}

func studentDigitalArchiveTokenPath(cookie string) string {
	return cookie + ".pdp-token"
}

func studentDigitalArchiveUserPath(cookie string) string {
	return cookie + ".pdp-user-id"
}

func loadStudentDigitalArchiveSession(args []string, cookie string) (studentDigitalArchiveSession, *siteError) {
	token, found, valueErr := businessValue(args, "--access-token")
	if valueErr != nil {
		return studentDigitalArchiveSession{}, valueErr
	}
	if !found || strings.TrimSpace(token) == "" {
		token = os.Getenv("CSUST_STUDENT_DIGITAL_ARCHIVE_TOKEN")
	}
	userID, found, valueErr := businessValue(args, "--user-id")
	if valueErr != nil {
		return studentDigitalArchiveSession{}, valueErr
	}
	if !found || strings.TrimSpace(userID) == "" {
		userID = os.Getenv("CSUST_STUDENT_DIGITAL_ARCHIVE_USER_ID")
	}
	_, cookiePath, resolveErr := resolveSite(siteRequest{Service: studentDigitalArchiveService, CookieFile: cookie})
	if resolveErr != nil {
		return studentDigitalArchiveSession{}, resolveErr
	}
	tokenPath := studentDigitalArchiveTokenPath(cookiePath)
	userPath := studentDigitalArchiveUserPath(cookiePath)
	if strings.TrimSpace(token) == "" {
		fileToken, readErr := electronicDocumentsReadSessionFile(tokenPath, "学生数字档案令牌文件")
		if readErr != nil {
			return studentDigitalArchiveSession{}, readErr
		}
		token = fileToken
	}
	if strings.TrimSpace(userID) == "" {
		fileUserID, readErr := electronicDocumentsReadSessionFile(userPath, "学生数字档案用户标识文件")
		if readErr != nil {
			return studentDigitalArchiveSession{}, readErr
		}
		userID = fileUserID
	}
	return studentDigitalArchiveSession{
		token: strings.TrimSpace(token), userID: strings.TrimSpace(userID), cookie: cookiePath,
		tokenPath: tokenPath, userPath: userPath,
	}, nil
}

func studentDigitalArchiveHeaders(token string) []pair {
	return []pair{{"Authentication", token}, {"Access-Control-Allow-Origin", "*"}}
}

func (a NativeSite) studentDigitalArchiveCall(ctx context.Context, session studentDigitalArchiveSession, method, path string, params []pair, body any, readOnly, yes bool) (map[string]any, *siteError) {
	return a.execute(ctx, siteRequest{
		Service: studentDigitalArchiveService, Method: method, Path: path, Params: params,
		JSON: body, HasJSON: body != nil, Headers: studentDigitalArchiveHeaders(session.token),
		CookieFile: session.cookie, RequireLogin: true, AllowBusinessFailure: true, ReadOnly: readOnly, Yes: yes, RawJSON: true,
	})
}

func studentDigitalArchivePayload(result map[string]any) (map[string]any, *siteError) {
	payload, parseErr := businessJSONMap(result)
	if parseErr != nil {
		return nil, parseErr
	}
	if fmt.Sprint(payload["code"]) != "2000" {
		message := strings.TrimSpace(fmt.Sprint(payload["message"]))
		if message == "" {
			message = "学生数字档案接口拒绝请求"
		}
		lower := strings.ToLower(message)
		code := "business_rejected"
		if strings.Contains(message, "登录") || strings.Contains(message, "认证") || strings.Contains(message, "权限") || strings.Contains(lower, "unauthorized") || strings.Contains(lower, "token") {
			code = "login_required"
		}
		return nil, &siteError{Code: code, Message: message, Details: map[string]any{"remote_code": payload["code"]}}
	}
	return payload, nil
}

func redactStudentDigitalArchive(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		result := make(map[string]any, len(typed))
		for key, item := range typed {
			switch strings.ToLower(key) {
			case "xh", "userid", "studentid", "username", "xm", "realname", "sfzjh", "idcard", "idnumber", "ksh", "dh", "tel", "phone", "email", "dzxx", "personphoto", "photo", "openid", "token", "signature":
				result[key] = "<redacted>"
			default:
				result[key] = redactStudentDigitalArchive(item)
			}
		}
		return result
	case []any:
		result := make([]any, len(typed))
		for index, item := range typed {
			result[index] = redactStudentDigitalArchive(item)
		}
		return result
	default:
		return redactSiteJSON(value)
	}
}

func studentDigitalArchiveResult(operation string, payload map[string]any, session studentDigitalArchiveSession) map[string]any {
	result := map[string]any{
		"ok": true, "submitted": false, "confirmed": true, "evidence": "学生数字档案接口返回 code=2000",
		"service": studentDigitalArchiveService, "operation": operation, "api_code": payload["code"],
		"api_message": payload["message"], "data": redactStudentDigitalArchive(payload["data"]),
		"raw": redactStudentDigitalArchive(payload), "token_file": session.tokenPath,
	}
	return result
}

func studentDigitalArchiveUser(args []string) (string, *siteError) {
	value, found, valueErr := businessValue(args, "--student-id")
	if valueErr != nil {
		return "", valueErr
	}
	if !found {
		value = os.Getenv("CSUST_STUDENT_DIGITAL_ARCHIVE_STUDENT_ID")
	}
	return strings.TrimSpace(value), nil
}

func studentDigitalArchiveParams(args []string, withUser, withTime bool) ([]pair, *siteError) {
	params := make([]pair, 0, 3)
	if withUser {
		user, userErr := studentDigitalArchiveUser(args)
		if userErr != nil {
			return nil, userErr
		}
		params = append(params, pair{"_USER", user})
	}
	if withTime {
		params = append(params, pair{"t", strconv.FormatInt(time.Now().UnixMilli(), 10)})
	}
	return params, nil
}

func (a NativeSite) studentDigitalArchiveSimple(ctx context.Context, args []string, session studentDigitalArchiveSession, operation, path string, withUser, withTime bool) (map[string]any, *siteError) {
	params, paramsErr := studentDigitalArchiveParams(args, withUser, withTime)
	if paramsErr != nil {
		return nil, paramsErr
	}
	result, requestErr := a.studentDigitalArchiveCall(ctx, session, "GET", path, params, nil, true, true)
	if requestErr != nil {
		return nil, requestErr
	}
	payload, payloadErr := studentDigitalArchivePayload(result)
	if payloadErr != nil {
		return nil, payloadErr
	}
	return studentDigitalArchiveResult(operation, payload, session), nil
}

func (a NativeSite) studentDigitalArchiveSimplePost(ctx context.Context, args []string, session studentDigitalArchiveSession, operation, path string, withUser bool) (map[string]any, *siteError) {
	params, paramsErr := studentDigitalArchiveParams(args, withUser, false)
	if paramsErr != nil {
		return nil, paramsErr
	}
	result, requestErr := a.studentDigitalArchiveCall(ctx, session, "POST", path, params, nil, true, true)
	if requestErr != nil {
		return nil, requestErr
	}
	payload, payloadErr := studentDigitalArchivePayload(result)
	if payloadErr != nil {
		return nil, payloadErr
	}
	return studentDigitalArchiveResult(operation, payload, session), nil
}

func (a NativeSite) studentDigitalArchiveSchedule(ctx context.Context, args []string, session studentDigitalArchiveSession) (map[string]any, *siteError) {
	page, pageErr := businessInt(args, "--page", 1)
	if pageErr != nil {
		return nil, pageErr
	}
	size, sizeErr := businessInt(args, "--page-size", 10)
	if sizeErr != nil {
		return nil, sizeErr
	}
	term, _, termErr := businessValue(args, "--term")
	if termErr != nil {
		return nil, termErr
	}
	params, paramsErr := studentDigitalArchiveParams(args, true, false)
	if paramsErr != nil {
		return nil, paramsErr
	}
	params = append(params, pair{"page", strconv.Itoa(page)}, pair{"pagesize", strconv.Itoa(size)}, pair{"termdm", strings.TrimSpace(term)})
	return a.studentDigitalArchivePagedGET(ctx, session, "schedule", "/tsa/shc/learnInfo/classScheduleGrid", params)
}

func (a NativeSite) studentDigitalArchiveBorrow(ctx context.Context, args []string, session studentDigitalArchiveSession) (map[string]any, *siteError) {
	page, pageErr := businessInt(args, "--page", 1)
	if pageErr != nil {
		return nil, pageErr
	}
	size, sizeErr := businessInt(args, "--page-size", 10)
	if sizeErr != nil {
		return nil, sizeErr
	}
	status, _, statusErr := businessValue(args, "--status")
	if statusErr != nil {
		return nil, statusErr
	}
	params, paramsErr := studentDigitalArchiveParams(args, true, true)
	if paramsErr != nil {
		return nil, paramsErr
	}
	params = append(params, pair{"page", strconv.Itoa(page)}, pair{"pagesize", strconv.Itoa(size)}, pair{"status", strings.TrimSpace(status)})
	return a.studentDigitalArchivePagedGET(ctx, session, "borrow", "/tsa/shc/borrow/borrowDetailGrid", params)
}

func (a NativeSite) studentDigitalArchivePagedGET(ctx context.Context, session studentDigitalArchiveSession, operation, path string, params []pair) (map[string]any, *siteError) {
	result, requestErr := a.studentDigitalArchiveCall(ctx, session, "GET", path, params, nil, true, true)
	if requestErr != nil {
		return nil, requestErr
	}
	payload, payloadErr := studentDigitalArchivePayload(result)
	if payloadErr != nil {
		return nil, payloadErr
	}
	return studentDigitalArchiveResult(operation, payload, session), nil
}

func studentDigitalArchivePeriod(args []string) (string, string, *siteError) {
	year, yearFound, yearErr := businessValue(args, "--year")
	if yearErr != nil {
		return "", "", yearErr
	}
	month, monthFound, monthErr := businessValue(args, "--month")
	if monthErr != nil {
		return "", "", monthErr
	}
	year, month = strings.TrimSpace(year), strings.TrimSpace(month)
	if monthFound && month != "" {
		parts := strings.Split(month, "-")
		if len(parts) != 2 || len(parts[0]) != 4 || len(parts[1]) != 2 {
			return "", "", &siteError{Code: "invalid_argument", Message: "--month 必须使用 YYYY-MM"}
		}
		if _, err := strconv.Atoi(parts[0]); err != nil {
			return "", "", &siteError{Code: "invalid_argument", Message: "--month 必须使用 YYYY-MM"}
		}
		monthNumber, err := strconv.Atoi(parts[1])
		if err != nil || monthNumber < 1 || monthNumber > 12 {
			return "", "", &siteError{Code: "invalid_argument", Message: "--month 必须使用 YYYY-MM"}
		}
		if year != "" && year != parts[0] {
			return "", "", &siteError{Code: "invalid_argument", Message: "--year 与 --month 的年份不一致"}
		}
		return parts[0], parts[1], nil
	}
	if yearFound && year != "" {
		if len(year) != 4 {
			return "", "", &siteError{Code: "invalid_argument", Message: "--year 必须使用 YYYY"}
		}
		if _, err := strconv.Atoi(year); err != nil {
			return "", "", &siteError{Code: "invalid_argument", Message: "--year 必须使用 YYYY"}
		}
	}
	return year, "", nil
}

func (a NativeSite) studentDigitalArchivePeriod(ctx context.Context, args []string, session studentDigitalArchiveSession, operation, path string) (map[string]any, *siteError) {
	year, month, periodErr := studentDigitalArchivePeriod(args)
	if periodErr != nil {
		return nil, periodErr
	}
	page, pageErr := businessInt(args, "--page", 1)
	if pageErr != nil {
		return nil, pageErr
	}
	size, sizeErr := businessInt(args, "--page-size", 10)
	if sizeErr != nil {
		return nil, sizeErr
	}
	params, paramsErr := studentDigitalArchiveParams(args, true, false)
	if paramsErr != nil {
		return nil, paramsErr
	}
	params = append(params, pair{"month", month}, pair{"page", strconv.Itoa(page)}, pair{"pagesize", strconv.Itoa(size)}, pair{"year", year})
	return a.studentDigitalArchivePagedGET(ctx, session, operation, path, params)
}

func studentDigitalArchiveRows(value any) []map[string]any {
	items, _ := value.([]any)
	rows := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if row, ok := item.(map[string]any); ok {
			rows = append(rows, row)
		}
	}
	return rows
}

func studentDigitalArchiveCatalogRows(payload map[string]any) []map[string]any {
	return studentDigitalArchiveRows(payload["data"])
}

func studentDigitalArchiveChildren(catalog map[string]any) []map[string]any {
	return studentDigitalArchiveRows(catalog["children"])
}

func studentDigitalArchiveCategoryAlias(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "basic", "basic-info", "profile", "基本信息":
		return "basicInfo"
	case "grades", "exams", "考试成绩":
		return "A08B3F7F6A621CA4E0537D00A8C0E5FF"
	case "teaching", "教学信息":
		return "A08B3F7F6A5E1CA4E0537D00A8C0E5FF"
	case "social-exam", "社考成绩":
		return "D3C8D389D393283AE05334C0FF0A7544"
	case "scholarships", "奖学助贷":
		return "A08B3F7F6A601CA4E0537D00A8C0E5FF"
	case "activities", "second-class", "校内活动":
		return "E683B2A446E7D786E05333C0FF0A683C"
	case "finance", "财务信息":
		return "D3C7BC80AB4762BAE05334C0FF0A45B8"
	case "status", "学籍信息":
		return "A08B3F7F6A611CA4E0537D00A8C0E5FF"
	case "discipline", "违纪信息":
		return "RewardAndPunishmentInformation"
	case "consumption", "生活消费":
		return "consumption"
	case "book", "books", "borrow", "图书借阅":
		return "bookRent"
	default:
		return strings.TrimSpace(value)
	}
}

func studentDigitalArchiveFindCategory(rows []map[string]any, wanted string) map[string]any {
	alias := studentDigitalArchiveCategoryAlias(wanted)
	for _, row := range rows {
		if fmt.Sprint(row["CATALOGID"]) == alias || strings.EqualFold(strings.TrimSpace(fmt.Sprint(row["CATALOGNAME"])), strings.TrimSpace(wanted)) {
			return row
		}
	}
	return nil
}

func (a NativeSite) studentDigitalArchiveCatalogs(ctx context.Context, session studentDigitalArchiveSession) (map[string]any, *siteError) {
	result, requestErr := a.studentDigitalArchiveCall(ctx, session, "GET", "/tsa/shc/personalInfo/formCatalogList", nil, nil, true, true)
	if requestErr != nil {
		return nil, requestErr
	}
	payload, payloadErr := studentDigitalArchivePayload(result)
	if payloadErr != nil {
		return nil, payloadErr
	}
	return studentDigitalArchiveResult("catalogs", payload, session), nil
}

func (a NativeSite) studentDigitalArchiveCatalogData(ctx context.Context, session studentDigitalArchiveSession) ([]map[string]any, *siteError) {
	result, requestErr := a.studentDigitalArchiveCall(ctx, session, "GET", "/tsa/shc/personalInfo/formCatalogList", nil, nil, true, true)
	if requestErr != nil {
		return nil, requestErr
	}
	payload, payloadErr := studentDigitalArchivePayload(result)
	if payloadErr != nil {
		return nil, payloadErr
	}
	return studentDigitalArchiveCatalogRows(payload), nil
}

func (a NativeSite) studentDigitalArchiveForms(ctx context.Context, args []string, session studentDigitalArchiveSession) (map[string]any, *siteError) {
	category, requiredErr := businessRequired(args, "--category", "forms 必须提供 --category")
	if requiredErr != nil {
		return nil, requiredErr
	}
	catalogs, catalogsErr := a.studentDigitalArchiveCatalogData(ctx, session)
	if catalogsErr != nil {
		return nil, catalogsErr
	}
	selected := studentDigitalArchiveFindCategory(catalogs, category)
	if selected == nil {
		return nil, &siteError{Code: "not_found", Message: "学生数字档案没有找到分类: " + category}
	}
	params := []pair{{"catalogId", fmt.Sprint(selected["CATALOGID"])}, {"t", strconv.FormatInt(time.Now().UnixMilli(), 10)}}
	result, requestErr := a.studentDigitalArchiveCall(ctx, session, "GET", "/tsa/shc/personalInfo/getFormListByCatalogId", params, nil, true, true)
	if requestErr != nil {
		return nil, requestErr
	}
	payload, payloadErr := studentDigitalArchivePayload(result)
	if payloadErr != nil {
		return nil, payloadErr
	}
	response := studentDigitalArchiveResult("forms", payload, session)
	response["category"] = selected["CATALOGNAME"]
	response["category_id"] = selected["CATALOGID"]
	return response, nil
}

func studentDigitalArchiveFormBody() map[string]any {
	return map[string]any{"headers": map[string]string{
		"Content-Type": "application/json;charset=utf-8", "Access-Control-Allow-Origin": "*", "Authentication": "",
	}}
}

func (a NativeSite) studentDigitalArchiveRecords(ctx context.Context, args []string, session studentDigitalArchiveSession) (map[string]any, *siteError) {
	category, requiredErr := businessRequired(args, "--category", "records 必须提供 --category")
	if requiredErr != nil {
		return nil, requiredErr
	}
	catalogs, catalogsErr := a.studentDigitalArchiveCatalogData(ctx, session)
	if catalogsErr != nil {
		return nil, catalogsErr
	}
	selected := studentDigitalArchiveFindCategory(catalogs, category)
	if selected == nil {
		return nil, &siteError{Code: "not_found", Message: "学生数字档案没有找到分类: " + category}
	}
	data, failures := map[string]any{}, map[string]any{}
	var firstErr *siteError
	for _, form := range studentDigitalArchiveChildren(selected) {
		formID := strings.TrimSpace(fmt.Sprint(form["formId"]))
		formName := strings.TrimSpace(fmt.Sprint(form["FORMNAME"]))
		if formID == "" {
			continue
		}
		result, requestErr := a.studentDigitalArchiveCall(ctx, session, "POST", "/tsa/shc/personalInfo/getCommonFormData/"+url.PathEscape(formID), nil, studentDigitalArchiveFormBody(), true, true)
		if requestErr != nil {
			if firstErr == nil {
				firstErr = requestErr
			}
			failures[formName] = map[string]any{"code": requestErr.Code, "error": requestErr.Message}
			continue
		}
		payload, payloadErr := studentDigitalArchivePayload(result)
		if payloadErr != nil {
			if firstErr == nil {
				firstErr = payloadErr
			}
			failures[formName] = map[string]any{"code": payloadErr.Code, "error": payloadErr.Message}
			continue
		}
		data[formName] = redactStudentDigitalArchive(payload["data"])
	}
	if len(data) == 0 && firstErr != nil {
		return nil, firstErr
	}
	return map[string]any{
		"ok": true, "submitted": false, "confirmed": true, "evidence": "学生数字档案分类表单接口已读取",
		"service": studentDigitalArchiveService, "operation": "records", "category": selected["CATALOGNAME"],
		"data": data, "errors": failures, "partial": len(failures) > 0, "token_file": session.tokenPath,
	}, nil
}

func (a NativeSite) studentDigitalArchiveOverview(ctx context.Context, args []string, session studentDigitalArchiveSession) (map[string]any, *siteError) {
	userParams, paramsErr := studentDigitalArchiveParams(args, true, false)
	if paramsErr != nil {
		return nil, paramsErr
	}
	endpoints := []struct {
		operation string
		method    string
		path      string
		params    []pair
	}{
		{"profile", "GET", "/tsa/shc/homepage/personInfo", userParams},
		{"credit", "GET", "/tsa/shc/learnInfo/learnSituation", userParams},
		{"grades", "GET", "/tsa/shc/learnInfo/scoreCardList", userParams},
		{"rank", "GET", "/tsa/shc/learnInfo/gradePointRanking", userParams},
		{"learning-gap", "GET", "/tsa/shc/learnInfo/learningGap", userParams},
		{"borrow", "GET", "/tsa/shc/borrow/borrowDetailGrid", append(append([]pair{}, userParams...), pair{"page", "1"}, pair{"pagesize", "10"}, pair{"status", ""}, pair{"t", strconv.FormatInt(time.Now().UnixMilli(), 10)})},
		{"consumption", "GET", "/tsa/shc/homepage/cardConsume", userParams},
		{"internet", "GET", "/tsa/shc/homepage/onlineState", userParams},
		{"attendance", "GET", "/tsa/shc/homepage/getMyAttendance", nil},
		{"awards", "GET", "/tsa/shc/homepage/bonusPenaltyList", userParams},
		{"labels", "GET", "/tsa/shc/homepage/getStuLableList", append(append([]pair{}, userParams...), pair{"t", strconv.FormatInt(time.Now().UnixMilli(), 10)})},
		{"timeline", "POST", "/tsa/shc/personalInfo/personYearAchievements/personaltime", userParams},
	}
	data, failures := map[string]any{}, map[string]any{}
	for _, endpoint := range endpoints {
		result, requestErr := a.studentDigitalArchiveCall(ctx, session, endpoint.method, endpoint.path, endpoint.params, nil, true, true)
		if requestErr != nil {
			if requestErr.Code == "login_required" {
				return nil, requestErr
			}
			failures[endpoint.operation] = map[string]any{"code": requestErr.Code, "error": requestErr.Message}
			continue
		}
		payload, payloadErr := studentDigitalArchivePayload(result)
		if payloadErr != nil {
			if payloadErr.Code == "login_required" {
				return nil, payloadErr
			}
			failures[endpoint.operation] = map[string]any{"code": payloadErr.Code, "error": payloadErr.Message}
			continue
		}
		data[endpoint.operation] = redactStudentDigitalArchive(payload["data"])
	}
	return map[string]any{
		"ok": true, "submitted": false, "confirmed": true, "evidence": "学生数字档案首页核心接口已读取",
		"service": studentDigitalArchiveService, "operation": "overview", "data": data, "errors": failures,
		"partial": len(failures) > 0, "token_file": session.tokenPath,
	}, nil
}

func (a NativeSite) studentDigitalArchiveNotes(ctx context.Context, args []string, session studentDigitalArchiveSession) (map[string]any, *siteError) {
	page, pageErr := businessInt(args, "--page", 1)
	if pageErr != nil {
		return nil, pageErr
	}
	size, sizeErr := businessInt(args, "--page-size", 10)
	if sizeErr != nil {
		return nil, sizeErr
	}
	year, _, yearErr := businessValue(args, "--year")
	if yearErr != nil {
		return nil, yearErr
	}
	title, _, titleErr := businessValue(args, "--title")
	if titleErr != nil {
		return nil, titleErr
	}
	start, _, startErr := businessValue(args, "--from")
	if startErr != nil {
		return nil, startErr
	}
	end, _, endErr := businessValue(args, "--to")
	if endErr != nil {
		return nil, endErr
	}
	body := map[string]any{"page": page, "year": strings.TrimSpace(year), "pageSize": size, "title": strings.TrimSpace(title), "startTime": strings.TrimSpace(start), "endTime": strings.TrimSpace(end)}
	result, requestErr := a.studentDigitalArchiveCall(ctx, session, "POST", "/tsa/pdp/notes/notesList", nil, body, true, true)
	if requestErr != nil {
		return nil, requestErr
	}
	payload, payloadErr := studentDigitalArchivePayload(result)
	if payloadErr != nil {
		return nil, payloadErr
	}
	response := studentDigitalArchiveResult("notes", payload, session)
	response["page"], response["page_size"] = page, size
	return response, nil
}

func (a NativeSite) studentDigitalArchiveNote(ctx context.Context, args []string, session studentDigitalArchiveSession) (map[string]any, *siteError) {
	wid, requiredErr := businessRequired(args, "--id", "note 必须提供 --id")
	if requiredErr != nil {
		return nil, requiredErr
	}
	result, requestErr := a.studentDigitalArchiveCall(ctx, session, "GET", "/tsa/pdp/notes/getNotesByWid", []pair{{"wid", wid}, {"t", strconv.FormatInt(time.Now().UnixMilli(), 10)}}, nil, true, true)
	if requestErr != nil {
		return nil, requestErr
	}
	payload, payloadErr := studentDigitalArchivePayload(result)
	if payloadErr != nil {
		return nil, payloadErr
	}
	response := studentDigitalArchiveResult("note", payload, session)
	response["id"] = wid
	return response, nil
}

func (a NativeSite) studentDigitalArchiveSaveNote(ctx context.Context, args []string, session studentDigitalArchiveSession) (map[string]any, *siteError) {
	if !businessBool(args, "--yes") {
		return nil, &siteError{Code: "confirmation_required", Message: "保存学生数字档案随手记需要 --yes"}
	}
	title, titleErr := businessRequired(args, "--title", "save-note 必须提供 --title")
	if titleErr != nil {
		return nil, titleErr
	}
	content, contentErr := businessRequired(args, "--content", "save-note 必须提供 --content")
	if contentErr != nil {
		return nil, contentErr
	}
	if utf8.RuneCountInString(title) > 50 {
		return nil, &siteError{Code: "invalid_argument", Message: "--title 不能超过 50 个字符"}
	}
	body := map[string]any{"title": title, "noteDesc": content, "wid": "", "imgList": []any{}}
	result, requestErr := a.studentDigitalArchiveCall(ctx, session, "POST", "/tsa/pdp/notes/saveNotes", []pair{{"t", strconv.FormatInt(time.Now().UnixMilli(), 10)}}, body, false, true)
	if requestErr != nil {
		return nil, requestErr
	}
	payload, payloadErr := studentDigitalArchivePayload(result)
	if payloadErr != nil {
		return nil, payloadErr
	}
	response := studentDigitalArchiveResult("save-note", payload, session)
	response["submitted"], response["evidence"] = true, "学生数字档案 saveNotes 返回 code=2000"
	return response, nil
}

func (a NativeSite) studentDigitalArchiveDeleteNote(ctx context.Context, args []string, session studentDigitalArchiveSession) (map[string]any, *siteError) {
	if !businessBool(args, "--yes") {
		return nil, &siteError{Code: "confirmation_required", Message: "删除学生数字档案随手记需要 --yes"}
	}
	wid, requiredErr := businessRequired(args, "--id", "delete-note 必须提供 --id")
	if requiredErr != nil {
		return nil, requiredErr
	}
	result, requestErr := a.studentDigitalArchiveCall(ctx, session, "POST", "/tsa/pdp/notes/deleteNotes", []pair{{"wid", wid}}, nil, false, true)
	if requestErr != nil {
		return nil, requestErr
	}
	payload, payloadErr := studentDigitalArchivePayload(result)
	if payloadErr != nil {
		return nil, payloadErr
	}
	response := studentDigitalArchiveResult("delete-note", payload, session)
	response["submitted"], response["id"] = true, wid
	response["evidence"] = "学生数字档案 deleteNotes 返回 code=2000"
	return response, nil
}

func studentDigitalArchiveSelectedLabels(payload map[string]any) []string {
	data, _ := payload["data"].(map[string]any)
	values, _ := data["selectedLable"].([]any)
	labels := make([]string, 0, len(values))
	for _, value := range values {
		labels = append(labels, fmt.Sprint(value))
	}
	return labels
}

func sameStudentDigitalArchiveLabels(want, got []string) bool {
	if len(want) != len(got) {
		return false
	}
	for index := range want {
		if strings.TrimSpace(want[index]) != strings.TrimSpace(got[index]) {
			return false
		}
	}
	return true
}

func (a NativeSite) studentDigitalArchiveSetLabels(ctx context.Context, args []string, session studentDigitalArchiveSession) (map[string]any, *siteError) {
	if !businessBool(args, "--yes") {
		return nil, &siteError{Code: "confirmation_required", Message: "设置学生数字档案标签需要 --yes"}
	}
	labels, valuesErr := businessValues(args, "--label")
	if valuesErr != nil {
		return nil, valuesErr
	}
	if len(labels) == 0 {
		return nil, &siteError{Code: "invalid_argument", Message: "set-labels 至少需要一个 --label"}
	}
	values := make([]any, len(labels))
	for index, label := range labels {
		values[index] = label
	}
	result, requestErr := a.studentDigitalArchiveCall(ctx, session, "POST", "/tsa/shc/homepage/saveSelectedLable", nil, values, false, true)
	if requestErr != nil {
		return nil, requestErr
	}
	payload, payloadErr := studentDigitalArchivePayload(result)
	if payloadErr != nil {
		return nil, payloadErr
	}
	check, checkErr := a.studentDigitalArchiveCall(ctx, session, "GET", "/tsa/shc/homepage/getStuLableList", []pair{{"_USER", ""}, {"t", strconv.FormatInt(time.Now().UnixMilli(), 10)}}, nil, true, true)
	if checkErr != nil {
		return nil, &siteError{Code: "mutation_unverified", Message: "标签设置成功反馈已返回，但标签回读失败", Details: map[string]any{"submitted": true, "confirmed": false, "cause": checkErr.Code}}
	}
	checkPayload, checkPayloadErr := studentDigitalArchivePayload(check)
	if checkPayloadErr != nil || !sameStudentDigitalArchiveLabels(labels, studentDigitalArchiveSelectedLabels(checkPayload)) {
		cause := "readback_mismatch"
		if checkPayloadErr != nil {
			cause = checkPayloadErr.Code
		}
		return nil, &siteError{Code: "mutation_unverified", Message: "标签设置成功反馈已返回，但标签回读结果不一致", Details: map[string]any{"submitted": true, "confirmed": false, "cause": cause}}
	}
	response := studentDigitalArchiveResult("set-labels", payload, session)
	response["submitted"], response["confirmed"], response["labels"] = true, true, labels
	response["evidence"] = "saveSelectedLable 返回 code=2000 且标签回读一致"
	return response, nil
}

func (a NativeSite) studentDigitalArchiveLogin(ctx context.Context, args []string, cookie string) (map[string]any, *siteError) {
	options, parseErr := parseLoginOptions(args)
	if parseErr != nil {
		return nil, parseErr
	}
	if options.auth != "auto" && options.auth != "sso" {
		return nil, &siteError{Code: "invalid_argument", Message: "student-digital-archive 只支持 --auth auto 或 sso"}
	}
	account, password, credentialErr := credentialsGo(options.username, options.password)
	if credentialErr != nil {
		return nil, credentialErr
	}
	target, cookiePath, resolveErr := resolveSite(siteRequest{Service: studentDigitalArchiveService, CookieFile: cookie})
	if resolveErr != nil {
		return nil, resolveErr
	}
	target.RawQuery, target.Fragment = "", ""
	sessionTarget := *target
	sessionTarget.Path, sessionTarget.RawQuery, sessionTarget.Fragment = "/", "", ""
	loginResult, loginErr := a.loginSSOPassword(ctx, &sessionTarget, target.String(), account, password, options, cookiePath)
	if loginErr != nil {
		return nil, loginErr
	}
	body, responseURL, bodyErr := loginBody(loginResult)
	if bodyErr != nil {
		return nil, bodyErr
	}
	callback, callbackErr := url.Parse(responseURL)
	if callbackErr != nil || callback == nil || !strings.EqualFold(callback.Host, target.Host) || callback.Path != "/tsa/validateLogin" {
		return nil, &siteError{Code: "authentication_failed", Message: "学生数字档案 CAS 回跳地址无效"}
	}
	var envelope map[string]any
	if json.Unmarshal([]byte(body), &envelope) != nil {
		return nil, &siteError{Code: "authentication_failed", Message: "学生数字档案 CAS 回跳响应不是 JSON"}
	}
	if fmt.Sprint(envelope["code"]) != "2000" {
		return nil, &siteError{Code: "authentication_failed", Message: strings.TrimSpace(fmt.Sprint(envelope["message"]))}
	}
	data, dataOK := envelope["data"].(map[string]any)
	if !dataOK {
		return nil, &siteError{Code: "authentication_failed", Message: "学生数字档案登录响应缺少 data"}
	}
	token := strings.TrimSpace(fmt.Sprint(data["token"]))
	userID := strings.TrimSpace(fmt.Sprint(data["userId"]))
	if token == "" || userID == "" {
		return nil, &siteError{Code: "authentication_failed", Message: "学生数字档案登录响应缺少令牌或用户标识"}
	}
	session := studentDigitalArchiveSession{token: token, userID: userID, cookie: cookiePath, tokenPath: studentDigitalArchiveTokenPath(cookiePath), userPath: studentDigitalArchiveUserPath(cookiePath)}
	if writeErr := atomicWrite(session.tokenPath, []byte(token+"\n")); writeErr != nil {
		return nil, &siteError{Code: "session_error", Message: "登录成功但学生数字档案令牌保存失败: " + writeErr.Error()}
	}
	if writeErr := atomicWrite(session.userPath, []byte(userID+"\n")); writeErr != nil {
		_ = removeCookieFile(session.tokenPath)
		return nil, &siteError{Code: "session_error", Message: "登录成功但学生数字档案用户标识保存失败: " + writeErr.Error()}
	}
	catalogs, verifyErr := a.studentDigitalArchiveCatalogData(ctx, session)
	if verifyErr != nil {
		return nil, &siteError{Code: "authentication_unverified", Message: "登录已返回令牌，但分类接口回读验证失败: " + verifyErr.Error(), Details: map[string]any{"token_file": session.tokenPath}}
	}
	return map[string]any{
		"ok": true, "submitted": false, "confirmed": true, "evidence": "CAS 回跳、validateLogin 令牌和分类接口回读均成功",
		"service": studentDigitalArchiveService, "operation": "login", "auth": "sso", "username": account,
		"token_file": session.tokenPath, "user_id_file": session.userPath, "category_count": len(catalogs),
	}, nil
}

func (a NativeSite) studentDigitalArchiveStatus(ctx context.Context, session studentDigitalArchiveSession) (map[string]any, *siteError) {
	if session.token == "" {
		return map[string]any{
			"ok": true, "submitted": false, "confirmed": true, "evidence": "本地没有学生数字档案令牌",
			"service": studentDigitalArchiveService, "operation": "status", "logged_in": false, "token_file": session.tokenPath,
		}, nil
	}
	result, requestErr := a.studentDigitalArchiveCall(ctx, session, "GET", "/tsa/shc/personalInfo/getDataNavigationConfig", nil, nil, true, true)
	if requestErr != nil {
		if requestErr.Code == "login_required" {
			return map[string]any{
				"ok": true, "submitted": false, "confirmed": true, "evidence": "分类配置接口报告学生数字档案会话失效",
				"service": studentDigitalArchiveService, "operation": "status", "logged_in": false, "token_file": session.tokenPath,
			}, nil
		}
		return nil, requestErr
	}
	payload, payloadErr := studentDigitalArchivePayload(result)
	if payloadErr != nil {
		return nil, payloadErr
	}
	response := studentDigitalArchiveResult("status", payload, session)
	response["logged_in"] = true
	return response, nil
}

func (a NativeSite) studentDigitalArchiveLogout(ctx context.Context, args []string, cookie string) (map[string]any, *siteError) {
	if !businessBool(args, "--yes") {
		return nil, &siteError{Code: "confirmation_required", Message: "退出学生数字档案会话需要 --yes"}
	}
	session, sessionErr := loadStudentDigitalArchiveSession(args, cookie)
	if sessionErr != nil {
		return nil, sessionErr
	}
	for _, path := range []string{session.tokenPath, session.userPath, session.cookie} {
		if removeErr := removeCookieFile(path); removeErr != nil {
			return nil, &siteError{Code: "session_error", Message: "学生数字档案会话删除失败: " + removeErr.Error()}
		}
	}
	return map[string]any{
		"ok": true, "submitted": false, "confirmed": true, "evidence": "删除学生数字档案本地令牌、用户标识和 Cookie",
		"service": studentDigitalArchiveService, "operation": "logout", "logged_out": true,
	}, nil
}
