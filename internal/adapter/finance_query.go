package adapter

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const financeQueryService = "finance-query"

func (a NativeSite) executeFinanceQuery(ctx context.Context, args []string) (map[string]any, *siteError) {
	if len(args) == 0 || args[0] == "catalog" {
		return businessCatalogFilter(financeQueryService), nil
	}
	if args[0] == "login" || args[0] == "logout" {
		return a.executeSSOServiceCommand(ctx, args, financeQueryService, "智慧财务查询")
	}
	cookie, _, valueErr := businessValue(args[1:], "--cookie-file")
	if valueErr != nil {
		return nil, valueErr
	}
	switch args[0] {
	case "status":
		return a.financeQueryStatus(ctx, cookie)
	case "overview", "dashboard":
		return a.financeQueryOverview(ctx, args[1:], cookie)
	case "fees", "fee-status":
		return a.financeQueryFees(ctx, args[1:], cookie)
	case "fee-details", "payments":
		return a.financeQueryPaged(ctx, args[1:], cookie, "fee-details", "/CWCX_V2/sfcx/xs/sfmxz", []pair{{"sort.s.sfdbh", "DESC"}, {"sort.s.bh", "asc"}})
	case "aid", "awards":
		return a.financeQueryPaged(ctx, args[1:], cookie, "aid", "/CWCX_V2/sfcx/xs/jzjl", nil)
	case "exemptions":
		return a.financeQueryPaged(ctx, args[1:], cookie, "exemptions", "/CWCX_V2/sfcx/xs/jmmxz", []pair{{"sort.s.sfqjdm", "DESC"}})
	case "refunds":
		return a.financeQueryPaged(ctx, args[1:], cookie, "refunds", "/CWCX_V2/sfcx/xs/tfmxz", []pair{{"sort.s.tfdbh", "desc"}})
	case "deferred", "deferred-payments":
		return a.financeQueryPaged(ctx, args[1:], cookie, "deferred", "/CWCX_V2/sfcx/xs/hjmxz", []pair{{"sort.tfdbh", "desc"}})
	case "income", "salary":
		return a.financeQueryIncome(ctx, args[1:], cookie)
	case "income-chart":
		return a.financeQueryPost(ctx, cookie, "income-chart", "/CWCX_V2/cwcx/gz/grsr/chart", []pair{{"key", "GRSR_LNTJ"}})
	case "incoming":
		return a.financeQueryIncoming(ctx, args[1:], cookie)
	case "bank-refunds":
		return a.financeQueryKeywordPage(ctx, args[1:], cookie, "bank-refunds", "/CWCX_V2/cwcx/ggcx/cxzfkx")
	case "unconfirmed-loans", "student-loans":
		return a.financeQueryKeywordPage(ctx, args[1:], cookie, "unconfirmed-loans", "/CWCX_V2/cwcx/ggcx/wqrsyddk")
	default:
		return nil, &siteError{Code: "invalid_argument", Message: "finance-query 不支持该子命令: " + args[0]}
	}
}

func financeQueryResult(operation string, data any, filters map[string]any) map[string]any {
	result := map[string]any{
		"ok": true, "submitted": false, "confirmed": true, "evidence": "finance_api_http_200",
		"service": financeQueryService, "operation": operation, "data": redactFinanceJSON(data),
	}
	for key, value := range filters {
		result[key] = value
	}
	return result
}

func redactFinanceJSON(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		result := make(map[string]any, len(typed))
		for key, item := range typed {
			switch strings.ToLower(key) {
			case "xh", "xm", "userid", "studentid", "studentno", "sfzjh", "idcard", "idnumber", "phone", "mobile", "tel", "email", "bankcard", "cardno", "accountno":
				result[key] = "<redacted>"
			default:
				result[key] = redactFinanceJSON(item)
			}
		}
		return result
	case []any:
		result := make([]any, len(typed))
		for index, item := range typed {
			result[index] = redactFinanceJSON(item)
		}
		return result
	default:
		return redactSiteJSON(value)
	}
}

func (a NativeSite) financeQueryStatus(ctx context.Context, cookie string) (map[string]any, *siteError) {
	result, requestErr := a.execute(ctx, siteRequest{
		Service: financeQueryService, Method: http.MethodGet, Path: "/AC/sso/index", CookieFile: cookie,
		RequireLogin: true, AllowSSO: true, ReadOnly: true, Yes: true,
	})
	if requestErr != nil {
		if requestErr.Code == "login_required" {
			return map[string]any{
				"ok": true, "submitted": false, "confirmed": true, "evidence": "finance_login_required",
				"service": financeQueryService, "operation": "status", "logged_in": false,
			}, nil
		}
		return nil, requestErr
	}
	body := businessBody(result)
	return map[string]any{
		"ok": true, "submitted": false, "confirmed": true, "evidence": "finance_workbench_page",
		"service": financeQueryService, "operation": "status", "logged_in": !loginPageBody(body),
		"url": safeResponseURL(result),
	}, nil
}

func financeQueryPage(args []string) (int, int, *siteError) {
	page, pageErr := businessInt(args, "--page", 1)
	if pageErr != nil {
		return 0, 0, pageErr
	}
	size, sizeErr := businessInt(args, "--page-size", 100)
	if sizeErr != nil {
		return 0, 0, sizeErr
	}
	return page, size, nil
}

func financeQueryYear(args []string, fallback int, optional bool) (string, *siteError) {
	year, found, valueErr := businessValue(args, "--year")
	if valueErr != nil {
		return "", valueErr
	}
	year = strings.TrimSpace(year)
	if !found || year == "" {
		if optional {
			return "", nil
		}
		year = strconv.Itoa(fallback)
	}
	if len(year) != 4 {
		return "", &siteError{Code: "invalid_argument", Message: "--year 必须使用 YYYY"}
	}
	if _, err := strconv.Atoi(year); err != nil {
		return "", &siteError{Code: "invalid_argument", Message: "--year 必须使用 YYYY"}
	}
	return year, nil
}

func financeQueryStatusFilter(args []string) (string, *siteError) {
	status, found, valueErr := businessValue(args, "--status")
	if valueErr != nil {
		return "", valueErr
	}
	if !found || strings.TrimSpace(status) == "" {
		return "", nil
	}
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "all", "全部":
		return "", nil
	case "unpaid", "unsettled", "未缴清":
		return "1", nil
	case "paid", "settled", "已缴清":
		return "0", nil
	default:
		return "", &siteError{Code: "invalid_argument", Message: "--status 只支持 all、unpaid 或 paid"}
	}
}

func financeQueryData(page, size int) []pair {
	return []pair{{"page.size", strconv.Itoa(size)}, {"page.number", strconv.Itoa(page)}}
}

func (a NativeSite) financeQueryPost(ctx context.Context, cookie, operation, path string, data []pair) (map[string]any, *siteError) {
	result, requestErr := a.execute(ctx, siteRequest{
		Service: financeQueryService, Method: http.MethodPost, Path: path, Data: data, CookieFile: cookie,
		RequireLogin: true, AllowSSO: true, AllowBusinessFailure: true, ReadOnly: true, Yes: true, RawJSON: true,
	})
	if requestErr != nil {
		return nil, requestErr
	}
	payload, ok := businessData(result)
	if !ok {
		return nil, &siteError{Code: "parse_error", Message: "智慧财务查询响应不是 JSON"}
	}
	if requestErr := financeQueryFailure(payload); requestErr != nil {
		return nil, requestErr
	}
	return financeQueryResult(operation, payload, map[string]any{"request_path": path}), nil
}

func financeQueryFailure(payload any) *siteError {
	state, known := jsonBusinessState(payload)
	if !known || state {
		return nil
	}
	code := "business_rejected"
	if jsonRequiresLogin(payload) {
		code = "login_required"
	}
	message := "智慧财务查询接口拒绝请求"
	details := map[string]any{"response": redactFinanceJSON(payload)}
	if envelope, ok := payload.(map[string]any); ok {
		if remoteCode, exists := envelope["code"]; exists {
			details["remote_code"] = remoteCode
		}
		for _, key := range []string{"message", "msg", "detailMsg"} {
			if value, ok := envelope[key].(string); ok && strings.TrimSpace(value) != "" {
				message = strings.TrimSpace(value)
				break
			}
		}
	}
	return &siteError{Code: code, Message: message, Details: details}
}

func (a NativeSite) financeQueryPaged(ctx context.Context, args []string, cookie, operation, path string, extra []pair) (map[string]any, *siteError) {
	page, size, pageErr := financeQueryPage(args)
	if pageErr != nil {
		return nil, pageErr
	}
	data := financeQueryData(page, size)
	data = append(data, extra...)
	result, requestErr := a.financeQueryPost(ctx, cookie, operation, path, data)
	if requestErr != nil {
		return nil, requestErr
	}
	result["page"], result["page_size"] = page, size
	return result, nil
}

func (a NativeSite) financeQueryFees(ctx context.Context, args []string, cookie string) (map[string]any, *siteError) {
	page, size, pageErr := financeQueryPage(args)
	if pageErr != nil {
		return nil, pageErr
	}
	year, yearErr := financeQueryYear(args, time.Now().Year(), true)
	if yearErr != nil {
		return nil, yearErr
	}
	status, statusErr := financeQueryStatusFilter(args)
	if statusErr != nil {
		return nil, statusErr
	}
	data := financeQueryData(page, size)
	data = append(data, pair{"sort.s.sfqjdm", "desc"}, pair{"search_s_qjf", status})
	if year != "" {
		data = append(data, pair{"search_s_sfqjdm", year[2:]})
	}
	result, requestErr := a.financeQueryPost(ctx, cookie, "fees", "/CWCX_V2/sfcx/xs/sfzz", data)
	if requestErr != nil {
		return nil, requestErr
	}
	result["page"], result["page_size"], result["year"], result["status"] = page, size, year, status
	return result, nil
}

func (a NativeSite) financeQueryIncome(ctx context.Context, args []string, cookie string) (map[string]any, *siteError) {
	page, size, pageErr := financeQueryPage(args)
	if pageErr != nil {
		return nil, pageErr
	}
	year, yearErr := financeQueryYear(args, time.Now().Year(), false)
	if yearErr != nil {
		return nil, yearErr
	}
	data := financeQueryData(page, size)
	data = append(data, pair{"nian", year})
	result, requestErr := a.financeQueryPost(ctx, cookie, "income", "/CWCX_V2/cwcx/gz/bwsrmx", data)
	if requestErr != nil {
		return nil, requestErr
	}
	result["page"], result["page_size"], result["year"] = page, size, year
	return result, nil
}

func (a NativeSite) financeQueryIncoming(ctx context.Context, args []string, cookie string) (map[string]any, *siteError) {
	page, size, pageErr := financeQueryPage(args)
	if pageErr != nil {
		return nil, pageErr
	}
	year, yearErr := financeQueryYear(args, time.Now().Year(), false)
	if yearErr != nil {
		return nil, yearErr
	}
	keyword, _, keywordErr := businessValue(args, "--keyword")
	if keywordErr != nil {
		return nil, keywordErr
	}
	data := financeQueryData(page, size)
	data = append(data,
		pair{"search_s_configKey", ""}, pair{"search_s_qsnd", year}, pair{"search_s_beginDzje", ""},
		pair{"search_s_endDzje", ""}, pair{"search_s_pzbh", ""}, pair{"search_s_wldwmc", ""}, pair{"search_s_key", keyword},
	)
	result, requestErr := a.financeQueryPost(ctx, cookie, "incoming", "/CWCX_V2/cwcx/ggcx/dqrsr", data)
	if requestErr != nil {
		return nil, requestErr
	}
	result["page"], result["page_size"], result["year"], result["keyword"] = page, size, year, keyword
	return result, nil
}

func (a NativeSite) financeQueryKeywordPage(ctx context.Context, args []string, cookie, operation, path string) (map[string]any, *siteError) {
	page, size, pageErr := financeQueryPage(args)
	if pageErr != nil {
		return nil, pageErr
	}
	keyword, _, keywordErr := businessValue(args, "--keyword")
	if keywordErr != nil {
		return nil, keywordErr
	}
	data := append(financeQueryData(page, size), pair{"search_s_key", keyword})
	result, requestErr := a.financeQueryPost(ctx, cookie, operation, path, data)
	if requestErr != nil {
		return nil, requestErr
	}
	result["page"], result["page_size"], result["keyword"] = page, size, keyword
	return result, nil
}

func (a NativeSite) financeQueryOverview(ctx context.Context, args []string, cookie string) (map[string]any, *siteError) {
	year := strconv.Itoa(time.Now().Year())
	calls := []struct {
		name string
		path string
		data []pair
	}{
		{"fees", "/CWCX_V2/sfcx/xs/sfzz", append(financeQueryData(1, 5), pair{"sort.s.sfqjdm", "desc"}, pair{"search_s_qjf", "1"})},
		{"income", "/CWCX_V2/cwcx/gz/bwsrmx", append(financeQueryData(1, 5), pair{"nian", year})},
		{"income_chart", "/CWCX_V2/cwcx/gz/grsr/chart", []pair{{"key", "GRSR_LNTJ"}}},
		{"messages", "/CWCX_V2/cwcx/dm/module/msg/show/list", nil},
		{"student_loan", "/CWCX_V2/cwcx/ggcx/wqrsyddk", append(financeQueryData(1, 100), pair{"search_s_key", ""})},
	}
	data := make(map[string]any, len(calls))
	errors := make(map[string]string)
	for _, call := range calls {
		payload, requestErr := a.financeQueryPost(ctx, cookie, call.name, call.path, call.data)
		if requestErr != nil {
			if requestErr.Code == "login_required" {
				return nil, requestErr
			}
			errors[call.name] = requestErr.Error()
			continue
		}
		data[call.name] = payload["data"]
	}
	if len(data) == 0 && len(errors) > 0 {
		return nil, &siteError{Code: "business_rejected", Message: "智慧财务总览接口均未返回数据", Details: map[string]any{"errors": errors}}
	}
	result := financeQueryResult("overview", data, map[string]any{"year": year})
	if len(errors) > 0 {
		result["partial_errors"] = errors
	}
	return result, nil
}
