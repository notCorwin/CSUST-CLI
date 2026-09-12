package adapter

import (
	"context"
	"fmt"
	"strings"
)

const serviceHallService = "service-hall"

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
	case "status", "login", "logout":
		return a.executeSSOServiceCommand(ctx, args, serviceHallService, "融合服务大厅")
	default:
		return nil, &siteError{Code: "invalid_argument", Message: "service-hall 只支持 services、list、categories、labels、departments、catalog、status、login、logout"}
	}
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
