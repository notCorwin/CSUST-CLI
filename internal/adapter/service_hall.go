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
	case "catalog":
		return businessCatalogFilter(serviceHallService), nil
	case "status", "login", "logout":
		return a.executeSSOServiceCommand(ctx, args, serviceHallService, "融合服务大厅")
	default:
		return nil, &siteError{Code: "invalid_argument", Message: "service-hall 只支持 services、list、catalog、status、login、logout"}
	}
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
	categoryID, _, categoryErr := businessValue(args, "--category-id")
	if categoryErr != nil {
		return nil, categoryErr
	}
	labelID, _, labelErr := businessValue(args, "--label-id")
	if labelErr != nil {
		return nil, labelErr
	}
	departmentID, _, departmentErr := businessValue(args, "--department-id")
	if departmentErr != nil {
		return nil, departmentErr
	}
	serveType, serveTypeFound, serveTypeErr := businessValue(args, "--serve-type")
	if serveTypeErr != nil {
		return nil, serveTypeErr
	}
	sortType, _, sortErr := businessValue(args, "--sort")
	if sortErr != nil {
		return nil, sortErr
	}
	classifyID := firstNonEmpty(categoryID, "0")
	labelID = firstNonEmpty(labelID, "0")
	isCollect := 0
	if businessBool(args, "--favorites") {
		isCollect = 1
	}
	var serveTypeValue any
	if serveTypeFound {
		serveTypeValue = serveType
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
		"query": map[string]any{"keyword": keyword, "category_id": categoryID, "label_id": labelID, "department_id": departmentID, "favorites": isCollect == 1},
		"api":   "/handleHall/getApp",
	}, nil
}
