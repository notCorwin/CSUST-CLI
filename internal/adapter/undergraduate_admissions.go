package adapter

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const undergraduateAdmissionsService = "undergraduate-admissions"

func (a NativeSite) executeUndergraduateAdmissions(ctx context.Context, args []string) (map[string]any, *siteError) {
	if len(args) == 0 || args[0] == "catalog" {
		return businessCatalogFilter(undergraduateAdmissionsService), nil
	}
	cookie, _, valueErr := businessValue(args[1:], "--cookie-file")
	if valueErr != nil {
		return nil, valueErr
	}
	switch args[0] {
	case "filters":
		return a.undergraduateAdmissionFilters(ctx, cookie)
	case "plans", "scores", "progress":
		query, queryErr := undergraduateAdmissionQuery(args[1:], args[0] == "scores")
		if queryErr != nil {
			return nil, queryErr
		}
		token, referer, openErr := a.undergraduateAdmissionOpen(ctx, undergraduateAdmissionPage(args[0]), cookie)
		if openErr != nil {
			return nil, openErr
		}
		data := []pair{{"ssmc", query.province}, {"zsnf", query.year}, {"klmc", query.category}, {"zslx", query.admissionType}}
		if query.major != "" {
			data = append(data, pair{"zyz", query.major})
		}
		path := map[string]string{"plans": "/f/ajax_zsjh", "scores": "/f/ajax_lnfs", "progress": "/f/ajax_lqjc"}[args[0]]
		payload, requestErr := a.undergraduateAdmissionAPI(ctx, path, data, token, referer, cookie)
		if requestErr != nil {
			return nil, requestErr
		}
		return undergraduateAdmissionResult(args[0], query, payload), nil
	case "lookup":
		return a.undergraduateAdmissionLookup(ctx, args[1:], cookie)
	default:
		return nil, &siteError{Code: "invalid_argument", Message: "undergraduate-admissions 只支持 filters、plans、scores、progress、lookup、catalog"}
	}
}

type undergraduateAdmissionSelection struct {
	province      string
	year          string
	category      string
	admissionType string
	major         string
}

func undergraduateAdmissionQuery(args []string, withMajor bool) (undergraduateAdmissionSelection, *siteError) {
	query := undergraduateAdmissionSelection{}
	for _, field := range []struct {
		flag  string
		value *string
		label string
	}{
		{"--province", &query.province, "省份"},
		{"--year", &query.year, "年份"},
		{"--category", &query.category, "科类"},
		{"--type", &query.admissionType, "招生类型"},
	} {
		value, err := businessRequired(args, field.flag, "查询必须提供 "+field.flag+"（"+field.label+"）")
		if err != nil {
			return undergraduateAdmissionSelection{}, err
		}
		*field.value = value
	}
	if _, err := strconv.Atoi(query.year); err != nil || len(query.year) != 4 {
		return undergraduateAdmissionSelection{}, &siteError{Code: "invalid_argument", Message: "--year 必须是四位年份"}
	}
	if withMajor {
		query.major = flagValue(args, "--major")
	}
	return query, nil
}

func undergraduateAdmissionPage(operation string) string {
	if operation == "scores" {
		return "/static/front/csust/basic/html_web/lnfs.html"
	}
	if operation == "progress" {
		return "/static/front/csust/basic/html_web/lqjc.html"
	}
	return "/static/front/csust/basic/html_web/zsjh.html"
}

func (a NativeSite) undergraduateAdmissionOpen(ctx context.Context, pagePath, cookie string) (string, string, *siteError) {
	page, requestErr := a.businessGet(ctx, undergraduateAdmissionsService, pagePath, nil, businessRequestOptions{cookieFile: cookie})
	if requestErr != nil {
		return "", "", requestErr
	}
	referer := safeResponseURL(page)
	csrf, requestErr := a.undergraduateAdmissionRequest(ctx, "/f/ajax_get_csrfToken", []pair{{"n", "3"}}, "", referer, cookie)
	if requestErr != nil {
		return "", "", requestErr
	}
	payload, parseErr := businessJSONMap(csrf)
	if parseErr != nil {
		return "", "", parseErr
	}
	if !undergraduateAdmissionState(payload) {
		return "", "", undergraduateAdmissionRejected(payload, "获取招生网会话令牌失败")
	}
	rawToken, ok := payload["data"].(string)
	if !ok || strings.TrimSpace(rawToken) == "" {
		return "", "", &siteError{Code: "parse_error", Message: "招生网 CSRF 响应缺少令牌"}
	}
	token := strings.TrimSpace(strings.Split(rawToken, ",")[0])
	if token == "" {
		return "", "", &siteError{Code: "parse_error", Message: "招生网 CSRF 令牌为空"}
	}
	return token, referer, nil
}

func (a NativeSite) undergraduateAdmissionRequest(ctx context.Context, path string, data []pair, token, referer, cookie string) (map[string]any, *siteError) {
	now := strconv.FormatInt(time.Now().UnixMilli(), 10)
	headers := []pair{{"X-Requested-With", "XMLHttpRequest"}, {"X-Requested-Time", now}}
	if referer != "" {
		headers = append(headers, pair{"Referer", referer})
	}
	if token != "" {
		headers = append(headers, pair{"Csrf-Token", token})
	}
	return businessRequest(ctx, undergraduateAdmissionsService, "POST", path, []pair{{"ts", now}}, data, headers, businessRequestOptions{cookieFile: cookie, allowBusinessFailure: true}, true, true)
}

func (a NativeSite) undergraduateAdmissionAPI(ctx context.Context, path string, data []pair, token, referer, cookie string) (map[string]any, *siteError) {
	result, requestErr := a.undergraduateAdmissionRequest(ctx, path, data, token, referer, cookie)
	if requestErr != nil {
		return nil, requestErr
	}
	payload, parseErr := businessJSONMap(result)
	if parseErr != nil {
		return nil, parseErr
	}
	if !undergraduateAdmissionState(payload) {
		return nil, undergraduateAdmissionRejected(payload, "招生网业务接口返回失败")
	}
	return payload, nil
}

func undergraduateAdmissionState(payload map[string]any) bool {
	value, exists := payload["state"]
	if !exists {
		return true
	}
	switch typed := value.(type) {
	case float64:
		return typed == 1
	case int:
		return typed == 1
	case string:
		switch strings.ToLower(strings.TrimSpace(typed)) {
		case "1", "success", "ok", "true":
			return true
		default:
			return false
		}
	default:
		return false
	}
}

func undergraduateAdmissionRejected(payload map[string]any, fallback string) *siteError {
	message := strings.TrimSpace(fmt.Sprint(payload["msg"]))
	if message == "" || message == "<nil>" {
		message = fallback
	}
	return &siteError{Code: "business_rejected", Message: message, Details: map[string]any{"submitted": false, "confirmed": false, "evidence": "undergraduate-admissions-api-state"}}
}

func (a NativeSite) undergraduateAdmissionFilters(ctx context.Context, cookie string) (map[string]any, *siteError) {
	token, referer, openErr := a.undergraduateAdmissionOpen(ctx, "/static/front/csust/basic/html_web/zsjh.html", cookie)
	if openErr != nil {
		return nil, openErr
	}
	filters := map[string]any{}
	for name, path := range map[string]string{
		"plans":    "/f/ajax_zsjh_param",
		"scores":   "/f/ajax_lnfs_param",
		"progress": "/f/ajax_lqjc_param",
		"lookup":   "/f/ajax_lqcx_param",
	} {
		payload, requestErr := a.undergraduateAdmissionAPI(ctx, path, nil, token, referer, cookie)
		if requestErr != nil {
			return nil, requestErr
		}
		filters[name] = payload["data"]
	}
	return map[string]any{"ok": true, "submitted": false, "confirmed": true, "evidence": "招生网筛选参数 JSON API", "service": undergraduateAdmissionsService, "operation": "filters", "filters": filters}, nil
}

func undergraduateAdmissionResult(operation string, query undergraduateAdmissionSelection, payload map[string]any) map[string]any {
	data, _ := payload["data"].(map[string]any)
	result := map[string]any{
		"ok": true, "submitted": false, "confirmed": true, "evidence": "本科招生网结构化 JSON API",
		"service": undergraduateAdmissionsService, "operation": operation,
		"filters": map[string]any{"province": query.province, "year": query.year, "category": query.category, "type": query.admissionType},
	}
	if query.major != "" {
		result["filters"].(map[string]any)["major"] = query.major
	}
	switch operation {
	case "plans":
		result["summary"] = undergraduateAdmissionRows(data, "zsjhTotal", undergraduatePlanRecord)
		result["items"] = undergraduateAdmissionRows(data, "zsjhList", undergraduatePlanRecord)
	case "scores":
		result["items"] = append(undergraduateAdmissionRows(data, "zsSsgradeList", undergraduateScoreRecord), undergraduateAdmissionRows(data, "sszyzgradeList", undergraduateScoreRecord)...)
	case "progress":
		result["items"] = undergraduateAdmissionRows(payload["data"], "", undergraduateProgressRecord)
	}
	return result
}

func undergraduateAdmissionRows(data any, key string, convert func(map[string]any) map[string]any) []map[string]any {
	value := data
	if key != "" {
		object, ok := data.(map[string]any)
		if !ok {
			return []map[string]any{}
		}
		value = object[key]
	}
	items, _ := value.([]any)
	result := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if row, ok := item.(map[string]any); ok {
			result = append(result, convert(row))
		}
	}
	return result
}

func undergraduateValue(row map[string]any, keys ...string) any {
	for _, key := range keys {
		value, ok := row[key]
		if ok && value != nil && strings.TrimSpace(fmt.Sprint(value)) != "" {
			return value
		}
	}
	return nil
}

func undergraduatePlanRecord(row map[string]any) map[string]any {
	return map[string]any{
		"province": row["ssmc"], "year": row["nf"], "category": row["klmc"], "type": undergraduateValue(row, "zslx", "zylx"),
		"major": undergraduateValue(row, "zymc", "zydhmc"), "major_code": row["zydm"], "plan_count": row["zsjhs"],
		"batch": row["zycc"], "college": row["yxdm"], "subject_requirement": row["xkkm"],
		"study_length": undergraduateValue(row, "xz", "zyxz"), "tuition": undergraduateValue(row, "xf", "zyxf"), "raw": row,
	}
}

func undergraduateScoreRecord(row map[string]any) map[string]any {
	return map[string]any{
		"province": row["ssmc"], "year": row["nf"], "category": row["klmc"], "type": undergraduateValue(row, "zslx", "zylx"),
		"major": row["zymc"], "max_score": row["maxScore"], "min_score": row["minScore"], "average_score": row["avgScore"],
		"max_rank": row["maxRank"], "min_rank": row["minRank"], "average_rank": row["avgRank"], "batch": row["pcmc"],
		"count": row["rs"], "raw": row,
	}
}

func undergraduateProgressRecord(row map[string]any) map[string]any {
	return map[string]any{
		"province": row["province"], "year": row["zsnf"], "category": row["klmc"], "type": undergraduateValue(row, "zslx", "tjlx"),
		"status": row["lqzt"], "end_date": row["jssj"], "max_score": row["maxScore"], "min_score": row["minScore"],
		"average_score": row["avgScore"], "count": row["count"], "min_rank": row["minRank"], "average_rank": row["avgRank"], "max_rank": row["maxRank"], "raw": row,
	}
}

func (a NativeSite) undergraduateAdmissionLookup(ctx context.Context, args []string, cookie string) (map[string]any, *siteError) {
	candidate, candidateErr := businessRequired(args, "--candidate-number", "lookup 必须提供 --candidate-number")
	if candidateErr != nil {
		return nil, candidateErr
	}
	idCard, idErr := businessRequired(args, "--id-card", "lookup 必须提供 --id-card")
	if idErr != nil {
		return nil, idErr
	}
	token, referer, openErr := a.undergraduateAdmissionOpen(ctx, "/static/front/csust/basic/html_web/lqcx.html", cookie)
	if openErr != nil {
		return nil, openErr
	}
	param, requestErr := a.undergraduateAdmissionAPI(ctx, "/f/ajax_lqcx_param", nil, token, referer, cookie)
	if requestErr != nil {
		return nil, requestErr
	}
	captcha := flagValue(args, "--captcha")
	if captcha == "" {
		return nil, a.undergraduateAdmissionCaptcha(ctx, args, cookie)
	}
	valid, requestErr := a.undergraduateAdmissionRequest(ctx, "/servlet/validateCodeServlet", []pair{{"validateCode", captcha}}, token, referer, cookie)
	if requestErr != nil {
		return nil, requestErr
	}
	if !strings.EqualFold(strings.TrimSpace(businessBody(valid)), "true") {
		return nil, &siteError{Code: "captcha_invalid", Message: "录取查询验证码错误", Details: map[string]any{"submitted": false, "confirmed": false, "evidence": "validateCodeServlet"}}
	}
	result, requestErr := businessRequest(ctx, undergraduateAdmissionsService, "POST", "/f/lqcx_jg", nil, []pair{{"Csrf-Token", token}, {"ksh", candidate}, {"sfzh", idCard}, {"validateCode", captcha}}, []pair{{"Csrf-Token", token}, {"Referer", referer}}, businessRequestOptions{cookieFile: cookie, allowBusinessFailure: true}, true, true)
	if requestErr != nil {
		return nil, requestErr
	}
	body := businessBody(result)
	if strings.TrimSpace(body) == "" {
		return nil, &siteError{Code: "parse_error", Message: "录取查询结果为空"}
	}
	page, parseErr := pageInspect(body, safeResponseURL(result))
	if parseErr != nil {
		return nil, parseErr
	}
	return map[string]any{
		"ok": true, "submitted": false, "confirmed": true, "evidence": "录取查询验证码校验和结果页",
		"service": undergraduateAdmissionsService, "operation": "lookup", "query_window": param["data"], "page": page,
	}, nil
}

func (a NativeSite) undergraduateAdmissionCaptcha(ctx context.Context, args []string, cookie string) *siteError {
	_, cookiePath, resolveErr := resolveSite(siteRequest{Service: undergraduateAdmissionsService, CookieFile: cookie})
	if resolveErr != nil {
		return resolveErr
	}
	imagePath := flagValue(args, "--captcha-image")
	if imagePath == "" {
		imagePath = filepath.Join(filepath.Dir(cookiePath), "undergraduate-admissions-captcha.png")
	}
	imagePath = expandUserPath(imagePath)
	if _, imageErr := (NativeSite{}).execute(ctx, siteRequest{Service: undergraduateAdmissionsService, Method: "GET", Path: "/servlet/validateCodeServlet", Params: []pair{{"ts", strconv.FormatInt(time.Now().UnixMilli(), 10)}}, CookieFile: cookie, Output: imagePath, ReadOnly: true, Yes: true}); imageErr != nil {
		return imageErr
	}
	return &siteError{Code: "captcha_required", Message: "录取查询需要验证码，请查看图片后提供 --captcha", Details: map[string]any{"captcha_image": imagePath, "submitted": false, "confirmed": false, "evidence": "validateCodeServlet-image"}}
}
