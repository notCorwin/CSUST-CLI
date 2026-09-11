package adapter

import (
	"context"
	"net/url"
	"regexp"
	"strings"
)

var sunshinePhone = regexp.MustCompile(`^1[3-9][0-9]{9}$`)

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
		body := map[string]any{"populator": []map[string]any{
			{"path": "department", "select": "name nickname"},
			{"path": "attachments", "select": "dateCreate size name fname fullname"},
		}}
		result, requestErr := a.sunshineJSON(ctx, "POST", "/api/issues/"+url.PathEscape(id), body, cookie, true, true)
		if requestErr != nil {
			return nil, requestErr
		}
		return businessResult(result, "sunshine", "issue"), nil
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
		return nil, &siteError{Code: "invalid_argument", Message: "sunshine 只支持 issues、issue、departments、stats、config、send-code、catalog"}
	}
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
