package adapter

import (
	"context"
	"fmt"
	"net/url"
	"strings"
)

const serviceHallService = "service-hall"

const (
	networkRepairAppID    = "19933721"
	networkRepairV1URL    = "https://v1.chaoxing.com/appInter/openPcApp?mappId=" + networkRepairAppID
	networkRepairFormHost = "office.csust.edu.cn"
)

func (a NativeSite) executeServiceHall(ctx context.Context, args []string) (map[string]any, *siteError) {
	if len(args) == 0 || strings.HasPrefix(args[0], "--") {
		return a.serviceHallServices(ctx, args)
	}
	switch args[0] {
	case "services", "list":
		return a.serviceHallServices(ctx, args[1:])
	case "categories", "labels", "departments":
		return a.serviceHallDictionary(ctx, args[0], args[1:])
	case "catalog":
		return businessCatalogFilter(serviceHallService), nil
	case "network-repair", "repair":
		if len(args) > 1 && args[1] != "schema" && args[1] != "form" {
			return nil, &siteError{Code: "invalid_argument", Message: "service-hall network-repair 只支持 schema、form"}
		}
		return a.serviceHallNetworkRepairSchema(ctx, args[1:])
	case "status", "login", "logout":
		return a.executeSSOServiceCommand(ctx, args, serviceHallService, "融合服务大厅")
	default:
		return nil, &siteError{Code: "invalid_argument", Message: "service-hall 只支持 services、list、categories、labels、departments、catalog、network-repair、status、login、logout"}
	}
}

func (a NativeSite) serviceHallNetworkRepairSchema(ctx context.Context, args []string) (map[string]any, *siteError) {
	cookie, _, valueErr := businessValue(args, "--cookie-file")
	if valueErr != nil {
		return nil, valueErr
	}
	target, urlErr := url.Parse(networkRepairV1URL)
	if urlErr != nil {
		return nil, &siteError{Code: "protocol_error", Message: "网络报修入口地址无效"}
	}
	sessionTarget, _, resolveErr := resolveSite(siteRequest{Service: serviceHallService, CookieFile: cookie})
	if resolveErr != nil {
		return nil, resolveErr
	}
	page, requestErr := a.execute(ctx, siteRequest{
		Target: target, SessionTarget: sessionTarget, Method: "GET", CookieFile: cookie,
		RequireLogin: true, ReadOnly: true, AllowSSO: true, Yes: true,
	})
	if requestErr != nil {
		return nil, requestErr
	}
	formURL, urlErr := url.Parse(safeResponseURL(page))
	if urlErr != nil || !strings.EqualFold(formURL.Scheme, "https") || !strings.EqualFold(formURL.Host, networkRepairFormHost) ||
		!strings.EqualFold(formURL.Path, "/apps/forms/web/apply.html") {
		return nil, &siteError{Code: "protocol_error", Message: "网络报修入口未返回 Office 表单", Details: map[string]any{
			"submitted": false, "confirmed": false, "evidence": "handoff-target-check",
		}}
	}
	query := formURL.Query()
	formID := firstNonEmpty(query.Get("formId"), query.Get("formid"), query.Get("id"))
	formParams := []pair{
		{"formId", formID}, {"aprvAppId", query.Get("aprvAppId")}, {"formUserId", query.Get("uid")},
		{"fidEnc", query.Get("fidEnc")}, {"newApply", firstNonEmpty(query.Get("newApply"), "0")}, {"pageEnc", query.Get("pageEnc")},
		{"manager", firstNonEmpty(query.Get("manager"), "0")}, {"uuid", query.Get("uuid")}, {"state", query.Get("state")},
	}
	for _, item := range formParams {
		if item.value == "" && item.name != "manager" {
			return nil, &siteError{Code: "protocol_error", Message: "网络报修表单缺少动态验证参数", Details: map[string]any{
				"submitted": false, "confirmed": false, "evidence": "handoff-query-check", "missing": item.name,
			}}
		}
	}
	verify, requestErr := a.execute(ctx, siteRequest{
		Target: &url.URL{Scheme: "https", Host: networkRepairFormHost, Path: "/data/approve/apps/forms/fore/user/verify/info"},
		Params: formParams, Method: "GET", CookieFile: cookie, RequireLogin: true, ReadOnly: true, Yes: true, RawJSON: true,
	})
	if requestErr != nil {
		return nil, requestErr
	}
	payload, parseErr := businessJSONMap(verify)
	if parseErr != nil {
		return nil, parseErr
	}
	if success, ok := payload["success"].(bool); !ok || !success {
		return nil, &siteError{Code: "business_rejected", Message: firstNonEmpty(fmt.Sprint(payload["msg"]), "网络报修表单验证失败"), Details: map[string]any{
			"submitted": false, "confirmed": false, "evidence": "verify-info-response",
		}}
	}
	data, ok := payload["data"].(map[string]any)
	if !ok {
		return nil, &siteError{Code: "parse_error", Message: "网络报修表单验证响应缺少 data"}
	}
	forms, ok := data["forms"].(map[string]any)
	if !ok {
		return nil, &siteError{Code: "parse_error", Message: "网络报修表单验证响应缺少 forms"}
	}
	config, ok := forms["formConfig"].([]any)
	if !ok {
		return nil, &siteError{Code: "parse_error", Message: "网络报修表单验证响应缺少 formConfig"}
	}
	fields := make([]map[string]any, 0, len(config))
	for _, item := range config {
		row, ok := item.(map[string]any)
		if !ok {
			continue
		}
		field := map[string]any{"id": row["id"], "component": serviceHallFormText(row, "compt", "component"), "label": serviceHallFormText(row, "name", "label", "caption", "title")}
		for _, key := range []string{"required", "isRequired", "readonly", "readOnly", "options"} {
			if value, exists := row[key]; exists {
				field[key] = value
			}
		}
		fields = append(fields, field)
	}
	return map[string]any{
		"ok": true, "submitted": false, "confirmed": true,
		"evidence": "office verify/info returned formConfig",
		"service":  serviceHallService, "operation": "network-repair-schema",
		"form_id": formID, "app_id": networkRepairAppID,
		"fields":          fields,
		"write_supported": false,
		"write_blocker":   "Office 保存请求依赖动态流程字段，待稳定协议和提交后回读接口确认",
	}, nil
}

func serviceHallFormText(row map[string]any, keys ...string) string {
	for _, key := range keys {
		value, ok := row[key]
		if !ok || value == nil {
			continue
		}
		if text := strings.TrimSpace(fmt.Sprint(value)); text != "" && text != "<nil>" {
			return text
		}
	}
	return ""
}

func (a NativeSite) serviceHallDictionary(ctx context.Context, operation string, args []string) (map[string]any, *siteError) {
	cookie, _, valueErr := businessValue(args, "--cookie-file")
	if valueErr != nil {
		return nil, valueErr
	}
	isCollect := 0
	if businessBool(args, "--favorites") {
		isCollect = 1
	}
	categoryID := "0"
	if operation == "departments" {
		value, found, categoryErr := businessValue(args, "--category-id")
		if categoryErr != nil {
			return nil, categoryErr
		}
		if found && strings.TrimSpace(value) != "" {
			categoryID = value
		}
	}
	items, itemsErr := a.serviceHallDictionaryItems(ctx, operation, categoryID, isCollect, cookie)
	if itemsErr != nil {
		return nil, itemsErr
	}
	path := "/handleHall/getAppClassify"
	if operation == "labels" {
		path = "/handleHall/getAppLabel"
	} else if operation == "departments" {
		path = "/handleHall/getAppDepart"
	}
	return map[string]any{
		"ok": true, "submitted": false, "confirmed": true,
		"evidence": "融合服务大厅字典接口返回 result=1",
		"service":  serviceHallService, "operation": operation,
		"data": items, "total": len(items), "api": path,
		"query": map[string]any{"category_id": categoryID, "favorites": isCollect == 1},
	}, nil
}

func (a NativeSite) serviceHallDictionaryItems(ctx context.Context, operation, categoryID string, isCollect int, cookie string) ([]any, *siteError) {
	path := "/handleHall/getAppClassify"
	body := map[string]any{"isCollect": isCollect, "noBusiness": 1}
	if operation == "labels" {
		path = "/handleHall/getAppLabel"
	}
	if operation == "departments" {
		path = "/handleHall/getAppDepart"
		body["classifyId"] = categoryID
	}
	result, requestErr := a.businessPostJSON(ctx, serviceHallService, path, body, businessRequestOptions{
		cookieFile: cookie, allowSSO: true, allowBusinessFailure: true, require: true,
		headers: []pair{{name: "X-Requested-With", value: "XMLHttpRequest"}},
	})
	if requestErr != nil {
		return nil, requestErr
	}
	payload, parseErr := businessJSONMap(result)
	if parseErr != nil {
		return nil, parseErr
	}
	if fmt.Sprint(payload["result"]) != "1" {
		message := strings.TrimSpace(fmt.Sprint(payload["msg"]))
		if message == "" {
			message = "融合服务大厅字典查询失败"
		}
		return nil, &siteError{Code: "business_rejected", Message: message, Details: map[string]any{"remote_result": payload["result"]}}
	}
	items, ok := payload["data"].([]any)
	if !ok {
		return nil, &siteError{Code: "parse_error", Message: "融合服务大厅字典响应不是列表"}
	}
	return items, nil
}

func serviceHallLookupID(items []any, wanted, idKey, nameKey, label string) (string, *siteError) {
	wanted = strings.TrimSpace(wanted)
	available := make([]string, 0, len(items))
	for _, item := range items {
		row, ok := item.(map[string]any)
		if !ok {
			continue
		}
		name := strings.TrimSpace(fmt.Sprint(row[nameKey]))
		if name != "" && name != "<nil>" {
			available = append(available, name)
		}
		if name == wanted {
			id := strings.TrimSpace(fmt.Sprint(row[idKey]))
			if id != "" && id != "<nil>" {
				return id, nil
			}
		}
	}
	return "", &siteError{Code: "invalid_argument", Message: wanted + " 未找到对应的" + label, Details: map[string]any{"available": available}}
}

func (a NativeSite) serviceHallServices(ctx context.Context, args []string) (map[string]any, *siteError) {
	cookie, _, valueErr := businessValue(args, "--cookie-file")
	if valueErr != nil {
		return nil, valueErr
	}
	page, pageErr := businessInt(args, "--page", 1)
	if pageErr != nil {
		return nil, pageErr
	}
	pageSize, pageSizeErr := businessInt(args, "--page-size", 12)
	if pageSizeErr != nil {
		return nil, pageSizeErr
	}
	keyword, _, keywordErr := businessValue(args, "--keyword")
	if keywordErr != nil {
		return nil, keywordErr
	}
	category, categoryFound, categoryErr := businessValue(args, "--category")
	if categoryErr != nil {
		return nil, categoryErr
	}
	categoryID, categoryIDFound, categoryErr := businessValue(args, "--category-id")
	if categoryErr != nil {
		return nil, categoryErr
	}
	if categoryFound && categoryIDFound {
		return nil, &siteError{Code: "invalid_argument", Message: "--category 和 --category-id 只能二选一"}
	}
	labelID, _, labelErr := businessValue(args, "--label-id")
	if labelErr != nil {
		return nil, labelErr
	}
	department, departmentFound, departmentErr := businessValue(args, "--department")
	if departmentErr != nil {
		return nil, departmentErr
	}
	departmentID, departmentIDFound, departmentErr := businessValue(args, "--department-id")
	if departmentErr != nil {
		return nil, departmentErr
	}
	if departmentFound && departmentIDFound {
		return nil, &siteError{Code: "invalid_argument", Message: "--department 和 --department-id 只能二选一"}
	}
	serveType, serveTypeFound, serveTypeErr := businessValue(args, "--serve-type")
	if serveTypeErr != nil {
		return nil, serveTypeErr
	}
	sortType, _, sortErr := businessValue(args, "--sort")
	if sortErr != nil {
		return nil, sortErr
	}
	labelID = firstNonEmpty(labelID, "0")
	isCollect := 0
	if businessBool(args, "--favorites") {
		isCollect = 1
	}
	if categoryFound && strings.TrimSpace(category) != "" {
		items, lookupErr := a.serviceHallDictionaryItems(ctx, "categories", "0", isCollect, cookie)
		if lookupErr != nil {
			return nil, lookupErr
		}
		categoryID, lookupErr = serviceHallLookupID(items, category, "id", "name", "服务大厅分类")
		if lookupErr != nil {
			return nil, lookupErr
		}
	}
	var serveTypeValue any
	if serveTypeFound {
		serveTypeValue = serveType
	}
	classifyID := firstNonEmpty(categoryID, "0")
	if departmentFound && strings.TrimSpace(department) != "" {
		items, lookupErr := a.serviceHallDictionaryItems(ctx, "departments", classifyID, isCollect, cookie)
		if lookupErr != nil {
			return nil, lookupErr
		}
		departmentID, lookupErr = serviceHallLookupID(items, department, "departId", "departName", "服务大厅部门")
		if lookupErr != nil {
			return nil, lookupErr
		}
	}
	body := map[string]any{
		"classifyId": classifyID,
		"labelId":    labelID,
		"departId":   departmentID,
		"page":       page,
		"row":        pageSize,
		"isCollect":  isCollect,
		"keyWord":    keyword,
		"type":       0,
		"sortType":   sortType,
		"noBusiness": 1,
		"serveType":  serveTypeValue,
	}
	result, requestErr := a.businessPostJSON(ctx, serviceHallService, "/handleHall/getApp", body, businessRequestOptions{
		cookieFile: cookie, allowSSO: true, allowBusinessFailure: true, require: true,
		headers: []pair{{name: "X-Requested-With", value: "XMLHttpRequest"}},
	})
	if requestErr != nil {
		return nil, requestErr
	}
	payload, parseErr := businessJSONMap(result)
	if parseErr != nil {
		return nil, parseErr
	}
	if fmt.Sprint(payload["result"]) != "1" {
		message := strings.TrimSpace(fmt.Sprint(payload["msg"]))
		if message == "" {
			message = "融合服务大厅服务目录查询失败"
		}
		return nil, &siteError{Code: "business_rejected", Message: message, Details: map[string]any{"remote_result": payload["result"]}}
	}
	data, ok := payload["data"].(map[string]any)
	if !ok {
		return nil, &siteError{Code: "parse_error", Message: "融合服务大厅响应缺少分页数据"}
	}
	records, ok := data["records"].([]any)
	if !ok {
		return nil, &siteError{Code: "parse_error", Message: "融合服务大厅响应缺少服务列表"}
	}
	return map[string]any{
		"ok": true, "submitted": false, "confirmed": true,
		"evidence": "融合服务大厅 /handleHall/getApp 返回 result=1",
		"service":  serviceHallService, "operation": "services",
		"page": data["current"], "page_size": pageSize, "total_pages": data["pages"],
		"total": data["total"], "services": records, "data": data,
		"query": map[string]any{"keyword": keyword, "category": category, "category_id": categoryID, "label_id": labelID, "department": department, "department_id": departmentID, "favorites": isCollect == 1},
		"api":   "/handleHall/getApp",
	}, nil
}
