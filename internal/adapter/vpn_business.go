package adapter

import (
	"context"
	"crypto/aes"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

func (a NativeSite) vpnBusinessJSON(ctx context.Context, path, method string, body any, readOnly bool) (map[string]any, *siteError) {
	base, cookie, session, connErr := vpnConnection(false)
	if connErr != nil {
		return nil, connErr
	}
	result, runErr := a.vpnJSONRequest(ctx, base, cookie, session, path, method, body, readOnly)
	if runErr != nil {
		return nil, runErr
	}
	code := vpnResponseCode(result)
	if code == "3010" {
		return nil, &siteError{Code: "login_required", Message: "VPN 会话已失效"}
	}
	if !readOnly && code == "" {
		return nil, &siteError{Code: "mutation_unverified", Message: "VPN 写操作未返回 code=200，无法确认成功", Details: map[string]any{"api": path}}
	}
	if code != "" && code != "200" {
		return nil, &siteError{Code: "business_rejected", Message: "VPN 业务请求失败", Details: map[string]any{"api": path, "remote_code": code}}
	}
	payload := resultJSON(result)
	if payload == nil {
		return nil, &siteError{Code: "response_error", Message: "VPN 业务响应不是 JSON 对象", Details: map[string]any{"api": path}}
	}
	return payload, nil
}

func vpnBusinessResult(operation, path string, data any, extra map[string]any) map[string]any {
	result := map[string]any{
		"ok":        true,
		"submitted": false,
		"confirmed": true,
		"evidence":  "VPN API code 200",
		"vpn":       true,
		"operation": operation,
		"api":       path,
		"data":      data,
	}
	for key, value := range extra {
		result[key] = value
	}
	return result
}

func vpnOptionValue(args []string, index *int, arg, value string, inline bool) (string, *siteError) {
	if inline {
		return value, nil
	}
	if *index+1 >= len(args) || strings.HasPrefix(args[*index+1], "--") {
		return "", &siteError{Code: "invalid_argument", Message: arg + " 缺少参数值"}
	}
	*index = *index + 1
	return args[*index], nil
}

type vpnAppsOptions struct {
	tab, search, group, order string
	page, pageSize            int
}

func parseVPNAppsOptions(args []string) (vpnAppsOptions, *siteError) {
	options := vpnAppsOptions{tab: "all", order: "desc", page: 1, pageSize: 200}
	for index := 0; index < len(args); index++ {
		arg, value, inline := splitInline(args[index])
		switch arg {
		case "--json":
			continue
		case "recent", "all", "temporary", "group", "web-source":
			if inline || options.tab != "all" {
				return vpnAppsOptions{}, &siteError{Code: "invalid_argument", Message: "vpn apps 视图只能指定一次"}
			}
			options.tab = arg
		case "--tab":
			var err *siteError
			value, err = vpnOptionValue(args, &index, arg, value, inline)
			if err != nil {
				return vpnAppsOptions{}, err
			}
			options.tab = value
		case "--search":
			var err *siteError
			options.search, err = vpnOptionValue(args, &index, arg, value, inline)
			if err != nil {
				return vpnAppsOptions{}, err
			}
		case "--group":
			var err *siteError
			options.group, err = vpnOptionValue(args, &index, arg, value, inline)
			if err != nil {
				return vpnAppsOptions{}, err
			}
		case "--order":
			var err *siteError
			options.order, err = vpnOptionValue(args, &index, arg, value, inline)
			if err != nil {
				return vpnAppsOptions{}, err
			}
		case "--page", "--page-size":
			var err *siteError
			value, err = vpnOptionValue(args, &index, arg, value, inline)
			if err != nil {
				return vpnAppsOptions{}, err
			}
			parsed, parseErr := strconv.Atoi(value)
			if parseErr != nil || parsed < 1 {
				return vpnAppsOptions{}, &siteError{Code: "invalid_argument", Message: arg + " 必须是正整数"}
			}
			if arg == "--page" {
				options.page = parsed
			} else {
				options.pageSize = parsed
			}
		default:
			return vpnAppsOptions{}, &siteError{Code: "invalid_argument", Message: "vpn apps 参数无效: " + arg}
		}
	}
	if options.tab != "recent" && options.tab != "all" && options.tab != "temporary" && options.tab != "group" && options.tab != "web-source" {
		return vpnAppsOptions{}, &siteError{Code: "invalid_argument", Message: "--tab 必须是 recent、all、group、temporary 或 web-source"}
	}
	if options.tab == "group" && strings.TrimSpace(options.group) == "" {
		return vpnAppsOptions{}, &siteError{Code: "invalid_argument", Message: "group 视图必须提供 --group"}
	}
	if options.tab == "web-source" && strings.TrimSpace(options.group) != "" {
		return vpnAppsOptions{}, &siteError{Code: "invalid_argument", Message: "web-source 视图不接受 --group"}
	}
	if options.order != "asc" && options.order != "desc" && options.order != "custom" {
		return vpnAppsOptions{}, &siteError{Code: "invalid_argument", Message: "--order 必须是 asc、desc 或 custom"}
	}
	return options, nil
}

func (a NativeSite) runVPNApps(ctx context.Context, args []string) (map[string]any, *siteError) {
	options, parseErr := parseVPNAppsOptions(args)
	if parseErr != nil {
		return nil, parseErr
	}
	var groups any
	if options.tab == "all" || options.tab == "group" {
		groupPayload, groupErr := a.vpnBusinessJSON(ctx, "/api/client/users/service/grouping", "GET", nil, true)
		if groupErr != nil {
			return nil, groupErr
		}
		groups = groupPayload["data"]
	}

	var payload map[string]any
	var apiPath string
	var requestBody any
	switch options.tab {
	case "recent":
		apiPath = "/api/users/service/visit/list"
		requestBody = map[string]any{"orderValue": options.order}
		payload, parseErr = a.vpnBusinessJSON(ctx, apiPath, "POST", requestBody, true)
	case "temporary":
		apiPath = "/api/users/service/getAllTemporaryService?orderValue=" + url.QueryEscape(options.order)
		payload, parseErr = a.vpnBusinessJSON(ctx, apiPath, "GET", nil, true)
	case "web-source":
		apiPath = "/api/client/user/service/pageUserService?endlessType=browser"
		requestBody = map[string]any{
			"pageIndex":     options.page,
			"pageSize":      options.pageSize,
			"serviceName":   options.search,
			"groupIds":      []any{},
			"orderValue":    options.order,
			"serviceSource": "2",
		}
		payload, parseErr = a.vpnBusinessJSON(ctx, apiPath, "POST", requestBody, true)
	default:
		apiPath = "/api/client/user/service/pageUserService?endlessType=browser"
		groupIDs := []any{}
		if options.group != "" {
			groupIDs = append(groupIDs, vpnIdentifier(options.group))
		}
		requestBody = map[string]any{
			"pageIndex":   options.page,
			"pageSize":    options.pageSize,
			"serviceName": options.search,
			"groupIds":    groupIDs,
			"orderValue":  options.order,
		}
		payload, parseErr = a.vpnBusinessJSON(ctx, apiPath, "POST", requestBody, true)
	}
	if parseErr != nil {
		return nil, parseErr
	}
	data := payload["data"]
	result := vpnBusinessResult("apps", apiPath, data, map[string]any{
		"view":    options.tab,
		"filters": map[string]any{"search": options.search, "group": options.group, "order": options.order, "page": options.page, "page_size": options.pageSize},
		"groups":  groups,
	})
	result["applications"] = data
	if items, total, page, pageSize, ok := vpnPageData(data); ok {
		result["applications"], result["total"], result["page"], result["page_size"] = items, total, page, pageSize
	}
	return result, nil
}

type vpnGroupOptions struct {
	groupID, groupName string
	groupIDs           []string
	groupNames         []string
	serviceIDs         []string
	serviceNames       []string
	yes                bool
}

func parseVPNGroupOptions(command string, args []string) (vpnGroupOptions, *siteError) {
	options := vpnGroupOptions{}
	for index := 0; index < len(args); index++ {
		arg, value, inline := splitInline(args[index])
		switch arg {
		case "--json":
		case "--group-id":
			var err *siteError
			value, err = vpnOptionValue(args, &index, arg, value, inline)
			if err != nil {
				return vpnGroupOptions{}, err
			}
			if command == "sort" {
				options.groupIDs = append(options.groupIDs, value)
			} else {
				options.groupID = value
			}
		case "--name", "--group-name":
			var err *siteError
			value, err = vpnOptionValue(args, &index, arg, value, inline)
			if err != nil {
				return vpnGroupOptions{}, err
			}
			if command == "sort" {
				options.groupNames = append(options.groupNames, value)
			} else {
				options.groupName = value
			}
		case "--service-id":
			var err *siteError
			value, err = vpnOptionValue(args, &index, arg, value, inline)
			if err != nil {
				return vpnGroupOptions{}, err
			}
			if strings.TrimSpace(value) == "" {
				return vpnGroupOptions{}, &siteError{Code: "invalid_argument", Message: "--service-id 不能为空"}
			}
			options.serviceIDs = append(options.serviceIDs, value)
		case "--service-name":
			var err *siteError
			value, err = vpnOptionValue(args, &index, arg, value, inline)
			if err != nil {
				return vpnGroupOptions{}, err
			}
			if strings.TrimSpace(value) == "" {
				return vpnGroupOptions{}, &siteError{Code: "invalid_argument", Message: "--service-name 不能为空"}
			}
			options.serviceNames = append(options.serviceNames, value)
		case "--yes":
			if inline {
				return vpnGroupOptions{}, &siteError{Code: "invalid_argument", Message: "布尔参数不接受 =VALUE"}
			}
			options.yes = true
		default:
			return vpnGroupOptions{}, &siteError{Code: "invalid_argument", Message: "vpn groups " + command + " 参数无效: " + arg}
		}
	}
	if command == "sort" {
		if len(options.groupIDs) == 0 || len(options.groupIDs) != len(options.groupNames) {
			return vpnGroupOptions{}, &siteError{Code: "invalid_argument", Message: "groups sort 的 --group-id 与 --group-name 必须成对提供"}
		}
	} else if command != "create" && strings.TrimSpace(options.groupID) == "" {
		return vpnGroupOptions{}, &siteError{Code: "invalid_argument", Message: "必须提供 --group-id"}
	}
	if command != "sort" && command != "delete" && command != "sort-services" && strings.TrimSpace(options.groupName) == "" {
		return vpnGroupOptions{}, &siteError{Code: "invalid_argument", Message: "必须提供 --name"}
	}
	if command == "save" && len(options.serviceIDs) != len(options.serviceNames) {
		return vpnGroupOptions{}, &siteError{Code: "invalid_argument", Message: "save 的 --service-id 与 --service-name 必须成对提供"}
	}
	if command == "sort-services" && len(options.serviceIDs) != len(options.serviceNames) {
		return vpnGroupOptions{}, &siteError{Code: "invalid_argument", Message: "sort-services 的 --service-id 与 --service-name 必须成对提供"}
	}
	if command != "create" && command != "delete" && command != "sort" && len(options.serviceIDs) == 0 {
		return vpnGroupOptions{}, &siteError{Code: "invalid_argument", Message: "必须至少提供一个 --service-id"}
	}
	if command == "save" && len(options.serviceIDs) == 0 {
		return vpnGroupOptions{}, &siteError{Code: "invalid_argument", Message: "save 必须至少提供一个服务"}
	}
	if command == "sort-services" && len(options.serviceIDs) == 0 {
		return vpnGroupOptions{}, &siteError{Code: "invalid_argument", Message: "sort-services 必须至少提供一个服务"}
	}
	if command == "sort" && len(options.serviceIDs) > 0 {
		return vpnGroupOptions{}, &siteError{Code: "invalid_argument", Message: "groups sort 不接受 --service-id"}
	}
	if command == "sort" && len(options.serviceNames) > 0 {
		return vpnGroupOptions{}, &siteError{Code: "invalid_argument", Message: "groups sort 不接受 --service-name"}
	}
	switch command {
	case "create":
		if strings.TrimSpace(options.groupID) != "" || len(options.serviceIDs) > 0 || len(options.serviceNames) > 0 {
			return vpnGroupOptions{}, &siteError{Code: "invalid_argument", Message: "groups create 只接受 --name"}
		}
	case "delete":
		if len(options.serviceIDs) > 0 || len(options.serviceNames) > 0 {
			return vpnGroupOptions{}, &siteError{Code: "invalid_argument", Message: "groups delete 不接受服务参数"}
		}
	case "update", "remove-service":
		if len(options.serviceNames) > 0 {
			return vpnGroupOptions{}, &siteError{Code: "invalid_argument", Message: "groups " + command + " 不接受 --service-name"}
		}
	case "sort-services":
		if strings.TrimSpace(options.groupName) != "" {
			return vpnGroupOptions{}, &siteError{Code: "invalid_argument", Message: "groups sort-services 不接受 --name"}
		}
	}
	if command == "create" && strings.TrimSpace(options.groupName) == "" {
		return vpnGroupOptions{}, &siteError{Code: "invalid_argument", Message: "创建分组必须提供 --name"}
	}
	if command != "sort" && len([]rune(strings.TrimSpace(options.groupName))) > 32 {
		return vpnGroupOptions{}, &siteError{Code: "invalid_argument", Message: "--name 不能超过 32 个字符"}
	}
	if !options.yes {
		return vpnGroupOptions{}, &siteError{Code: "confirmation_required", Message: "VPN 应用分组操作会改变远端状态，请加 --yes"}
	}
	return options, nil
}

func (a NativeSite) runVPNGroups(ctx context.Context, args []string) (map[string]any, *siteError) {
	if len(args) == 0 || args[0] == "list" {
		if len(args) > 0 {
			args = args[1:]
		}
		if err := onlyJSONArgs(args); err != nil {
			return nil, err
		}
		path := "/api/client/users/service/grouping"
		payload, runErr := a.vpnBusinessJSON(ctx, path, "GET", nil, true)
		if runErr != nil {
			return nil, runErr
		}
		return vpnBusinessResult("groups", path, payload["data"], map[string]any{"groups": payload["data"]}), nil
	}
	command := args[0]
	if command != "create" && command != "delete" && command != "update" && command != "save" && command != "remove-service" && command != "sort" && command != "sort-services" {
		return nil, &siteError{Code: "invalid_argument", Message: "未知 vpn groups 子命令: " + command}
	}
	options, parseErr := parseVPNGroupOptions(command, args[1:])
	if parseErr != nil {
		return nil, parseErr
	}

	path := ""
	var body any
	operation := "group-" + command
	switch command {
	case "create":
		path = "/api/client/user/addServiceGroup"
		body = map[string]any{"groupName": options.groupName}
	case "delete":
		path = "/api/client/user/deleteServiceGroup"
		body = map[string]any{"groupId": options.groupID, "groupNames": options.groupName}
	case "update", "remove-service":
		path = "/api/client/user/updateServiceGroup"
		body = map[string]any{"groupId": options.groupID, "groupName": options.groupName, "serviceIds": options.serviceIDs}
	case "save":
		path = "/api/client/user/service/addToCustomGroup"
		body = map[string]any{"groupId": options.groupID, "groupName": options.groupName, "serviceIds": options.serviceIDs, "serviceNames": options.serviceNames}
	case "sort":
		path = "/api/client/user/updateServiceGroupSort"
		body = map[string]any{"groupIds": options.groupIDs, "groupNames": options.groupNames}
	case "sort-services":
		path = "/api/client/user/updateServiceSort"
		body = map[string]any{"groupId": options.groupID, "serviceIds": options.serviceIDs, "serviceNames": options.serviceNames}
	}
	payload, runErr := a.vpnBusinessJSON(ctx, path, "POST", body, false)
	if runErr != nil {
		return nil, runErr
	}
	return map[string]any{"ok": true, "submitted": true, "confirmed": true, "evidence": "VPN API code 200", "vpn": true, "operation": operation, "api": path, "data": payload["data"]}, nil
}

type vpnMessagesOptions struct {
	typeName, readStatus, search string
	page, pageSize               int
}

var vpnMessageTypes = map[string]int{"notice": 0, "announcement": 0, "login": 1, "approve": 2, "system": 5, "safe": 7}

func parseVPNMessagesOptions(args []string) (vpnMessagesOptions, *siteError) {
	options := vpnMessagesOptions{typeName: "all", readStatus: "all", page: 1, pageSize: 10}
	for index := 0; index < len(args); index++ {
		arg, value, inline := splitInline(args[index])
		if arg == "--json" {
			continue
		}
		var err *siteError
		switch arg {
		case "--type", "--read-status", "--search", "--page", "--page-size":
			value, err = vpnOptionValue(args, &index, arg, value, inline)
			if err != nil {
				return vpnMessagesOptions{}, err
			}
			switch arg {
			case "--type":
				options.typeName = value
			case "--read-status":
				options.readStatus = value
			case "--search":
				options.search = value
			default:
				parsed, parseErr := strconv.Atoi(value)
				if parseErr != nil || parsed < 1 {
					return vpnMessagesOptions{}, &siteError{Code: "invalid_argument", Message: arg + " 必须是正整数"}
				}
				if arg == "--page" {
					options.page = parsed
				} else {
					options.pageSize = parsed
				}
			}
		default:
			return vpnMessagesOptions{}, &siteError{Code: "invalid_argument", Message: "vpn messages 参数无效: " + arg}
		}
	}
	if options.typeName != "all" {
		if _, ok := vpnMessageTypes[options.typeName]; !ok {
			return vpnMessagesOptions{}, &siteError{Code: "invalid_argument", Message: "--type 必须是 all、notice、announcement、login、approve、system 或 safe"}
		}
	}
	if options.readStatus != "all" && options.readStatus != "unread" && options.readStatus != "read" {
		return vpnMessagesOptions{}, &siteError{Code: "invalid_argument", Message: "--read-status 必须是 all、unread 或 read"}
	}
	return options, nil
}

func (a NativeSite) runVPNMessages(ctx context.Context, args []string) (map[string]any, *siteError) {
	if len(args) > 0 && (args[0] == "read-all" || args[0] == "mark-all-read") {
		return a.runVPNMarkAllMessagesRead(ctx, args[1:])
	}
	if len(args) > 0 && args[0] == "count" {
		if err := onlyJSONArgs(args[1:]); err != nil {
			return nil, err
		}
		path := "/api/users/message/count"
		payload, runErr := a.vpnBusinessJSON(ctx, path, "GET", nil, true)
		if runErr != nil {
			return nil, runErr
		}
		return vpnBusinessResult("message-count", path, payload["data"], map[string]any{"counts": payload["data"]}), nil
	}
	options, parseErr := parseVPNMessagesOptions(args)
	if parseErr != nil {
		return nil, parseErr
	}
	condition := map[string]any{}
	if options.typeName != "all" {
		condition["type"] = vpnMessageTypes[options.typeName]
	}
	conditionLike := map[string]any{}
	if options.search != "" {
		conditionLike = map[string]any{"create_time": options.search, "content": options.search, "title": options.search}
	}
	body := map[string]any{
		"pageVo":     map[string]any{"condition": condition, "conditionLike": conditionLike, "notCondition": map[string]any{}, "pageIndex": options.page, "pageSize": options.pageSize, "sorts": map[string]any{}},
		"readStatus": map[string]string{"all": "ALL_MESSAGE", "unread": "NOT_READ", "read": "HAD_READ"}[options.readStatus],
	}
	path := "/api/users/message/page"
	payload, runErr := a.vpnBusinessJSON(ctx, path, "POST", body, true)
	if runErr != nil {
		return nil, runErr
	}
	data := payload["data"]
	result := vpnBusinessResult("messages", path, data, map[string]any{
		"filters": map[string]any{"type": options.typeName, "read_status": options.readStatus, "search": options.search, "page": options.page, "page_size": options.pageSize},
	})
	if items, total, page, pageSize, ok := vpnPageData(data); ok {
		result["messages"], result["total"], result["page"], result["page_size"] = items, total, page, pageSize
	} else {
		result["messages"] = data
	}
	return result, nil
}

func (a NativeSite) runVPNMessage(ctx context.Context, args []string) (map[string]any, *siteError) {
	id := ""
	for index := 0; index < len(args); index++ {
		arg, value, inline := splitInline(args[index])
		if arg == "get" {
			if inline {
				return nil, &siteError{Code: "invalid_argument", Message: "message get 不接受 =VALUE"}
			}
			continue
		}
		if arg == "--json" {
			continue
		}
		if arg != "--id" {
			return nil, &siteError{Code: "invalid_argument", Message: "vpn message 参数无效: " + arg}
		}
		var err *siteError
		id, err = vpnOptionValue(args, &index, arg, value, inline)
		if err != nil {
			return nil, err
		}
	}
	if strings.TrimSpace(id) == "" {
		return nil, &siteError{Code: "invalid_argument", Message: "vpn message get 必须提供 --id"}
	}
	path := "/api/users/message/get?messageId=" + url.QueryEscape(id)
	payload, runErr := a.vpnBusinessJSON(ctx, path, "GET", nil, true)
	if runErr != nil {
		return nil, runErr
	}
	return vpnBusinessResult("message", path, payload["data"], map[string]any{"message_id": id, "message": payload["data"]}), nil
}

func (a NativeSite) runVPNMarkAllMessagesRead(ctx context.Context, args []string) (map[string]any, *siteError) {
	yes := false
	for _, arg := range args {
		switch arg {
		case "--json":
		case "--yes":
			yes = true
		default:
			return nil, &siteError{Code: "invalid_argument", Message: "vpn messages read-all 参数无效: " + arg}
		}
	}
	if !yes {
		return nil, &siteError{Code: "confirmation_required", Message: "标记 VPN 消息为已读会改变远端状态，请加 --yes"}
	}
	path := "/api/users/message/allRead?type=99"
	payload, runErr := a.vpnBusinessJSON(ctx, path, "GET", nil, false)
	if runErr != nil {
		return nil, runErr
	}
	return map[string]any{"ok": true, "submitted": true, "confirmed": true, "evidence": "VPN message/allRead code 200", "vpn": true, "operation": "messages-read-all", "api": path, "read_all": true, "data": payload["data"]}, nil
}

type vpnApprovalsOptions struct {
	view, status, search string
	page, pageSize       int
}

var vpnApprovalStatuses = map[string][]string{"pending": {"10", "11"}, "passed": {"20"}, "rejected": {"30"}, "undone": {"40"}}

func parseVPNApprovalsOptions(args []string) (vpnApprovalsOptions, *siteError) {
	options := vpnApprovalsOptions{view: "pending", status: "all", page: 1, pageSize: 10}
	for index := 0; index < len(args); index++ {
		arg, value, inline := splitInline(args[index])
		if arg == "--json" {
			continue
		}
		if arg != "--view" && arg != "--tab" && arg != "--status" && arg != "--search" && arg != "--page" && arg != "--page-size" {
			return vpnApprovalsOptions{}, &siteError{Code: "invalid_argument", Message: "vpn approvals 参数无效: " + arg}
		}
		var err *siteError
		value, err = vpnOptionValue(args, &index, arg, value, inline)
		if err != nil {
			return vpnApprovalsOptions{}, err
		}
		switch arg {
		case "--view", "--tab":
			options.view = value
		case "--status":
			options.status = value
		case "--search":
			options.search = value
		default:
			parsed, parseErr := strconv.Atoi(value)
			if parseErr != nil || parsed < 1 {
				return vpnApprovalsOptions{}, &siteError{Code: "invalid_argument", Message: arg + " 必须是正整数"}
			}
			if arg == "--page" {
				options.page = parsed
			} else {
				options.pageSize = parsed
			}
		}
	}
	if options.view != "pending" && options.view != "processed" && options.view != "initiated" && options.view != "all" {
		return vpnApprovalsOptions{}, &siteError{Code: "invalid_argument", Message: "--view 必须是 pending、processed、initiated 或 all"}
	}
	if options.status != "all" {
		if _, ok := vpnApprovalStatuses[options.status]; !ok {
			return vpnApprovalsOptions{}, &siteError{Code: "invalid_argument", Message: "--status 必须是 all、pending、passed、rejected 或 undone"}
		}
	}
	return options, nil
}

func vpnApprovalQuery(options vpnApprovalsOptions) map[string]any {
	body := map[string]any{"pageIndex": options.page, "pageSize": options.pageSize, "applyTimeZoneOffset": vpnTimezoneOffset()}
	if options.search != "" {
		body["vagueLike"] = options.search
	}
	if options.status != "all" {
		body["approveResultList"] = vpnApprovalStatuses[options.status]
	}
	return body
}

func (a NativeSite) runVPNApprovals(ctx context.Context, args []string) (map[string]any, *siteError) {
	options, parseErr := parseVPNApprovalsOptions(args)
	if parseErr != nil {
		return nil, parseErr
	}
	countPath := "/api/users/approve/center/groupCount"
	countPayload, runErr := a.vpnBusinessJSON(ctx, countPath, "POST", map[string]any{}, true)
	if runErr != nil {
		return nil, runErr
	}
	paths := map[string]string{"pending": "/api/users/approve/center/waitingHandle", "processed": "/api/users/approve/center/myHandle", "initiated": "/api/users/approve/center/applyPage"}
	views := map[string]any{}
	viewNames := []string{options.view}
	if options.view == "all" {
		viewNames = []string{"pending", "processed", "initiated"}
	}
	for _, view := range viewNames {
		path := paths[view]
		payload, requestErr := a.vpnBusinessJSON(ctx, path, "POST", vpnApprovalQuery(options), true)
		if requestErr != nil {
			return nil, requestErr
		}
		data := payload["data"]
		viewResult := map[string]any{"data": data}
		if items, total, page, pageSize, ok := vpnPageData(data); ok {
			viewResult["items"], viewResult["total"], viewResult["page"], viewResult["page_size"] = items, total, page, pageSize
		}
		views[view] = viewResult
	}
	result := vpnBusinessResult("approvals", paths[viewNames[0]], nil, map[string]any{
		"view":    options.view,
		"filters": map[string]any{"status": options.status, "search": options.search, "page": options.page, "page_size": options.pageSize},
		"counts":  countPayload["data"],
		"views":   views,
	})
	if options.view != "all" {
		view := views[options.view].(map[string]any)
		result["data"], result["items"], result["total"], result["page"], result["page_size"] = view["data"], view["items"], view["total"], view["page"], view["page_size"]
	}
	return result, nil
}

func (a NativeSite) runVPNApproval(ctx context.Context, args []string) (map[string]any, *siteError) {
	if len(args) == 0 {
		return nil, &siteError{Code: "invalid_argument", Message: "vpn approval 必须提供 get、approve 或 reject"}
	}
	switch args[0] {
	case "get":
		return a.runVPNApprovalDetail(ctx, args[1:])
	case "approve", "reject", "deal":
		return a.runVPNApprovalDeal(ctx, args[0], args[1:])
	default:
		return nil, &siteError{Code: "invalid_argument", Message: "未知 vpn approval 子命令: " + args[0]}
	}
}

func parseVPNApprovalID(args []string, allowFlow bool) (string, bool, *siteError) {
	id, flow := "", false
	for index := 0; index < len(args); index++ {
		arg, value, inline := splitInline(args[index])
		switch arg {
		case "--json":
		case "--flow":
			if inline {
				return "", false, &siteError{Code: "invalid_argument", Message: "布尔参数不接受 =VALUE"}
			}
			flow = true
		case "--id":
			var err *siteError
			id, err = vpnOptionValue(args, &index, arg, value, inline)
			if err != nil {
				return "", false, err
			}
		default:
			return "", false, &siteError{Code: "invalid_argument", Message: "vpn approval get 参数无效: " + arg}
		}
	}
	if !allowFlow && flow {
		return "", false, &siteError{Code: "invalid_argument", Message: "当前审批动作不支持 --flow"}
	}
	if strings.TrimSpace(id) == "" {
		return "", false, &siteError{Code: "invalid_argument", Message: "vpn approval 必须提供 --id"}
	}
	return id, flow, nil
}

func (a NativeSite) runVPNApprovalDetail(ctx context.Context, args []string) (map[string]any, *siteError) {
	id, withFlow, parseErr := parseVPNApprovalID(args, true)
	if parseErr != nil {
		return nil, parseErr
	}
	idValue := vpnIdentifier(id)
	path := "/api/users/approve/center/applyDetail"
	payload, runErr := a.vpnBusinessJSON(ctx, path, "POST", map[string]any{"applicationId": idValue, "applyTimeZoneOffset": vpnTimezoneOffset()}, true)
	if runErr != nil {
		return nil, runErr
	}
	data := payload["data"]
	result := vpnBusinessResult("approval-get", path, data, map[string]any{"approval_id": id, "approval": data})
	if !withFlow {
		return result, nil
	}
	detail, ok := data.(map[string]any)
	if !ok || strings.TrimSpace(fmt.Sprint(detail["applyType"])) == "" {
		return nil, &siteError{Code: "response_error", Message: "审批详情缺少 applyType，无法取得流程图"}
	}
	flowPath := "/api/users/approve/center/flowImage"
	flowPayload, flowErr := a.vpnBusinessJSON(ctx, flowPath, "POST", map[string]any{"applicationId": idValue, "applyType": detail["applyType"]}, true)
	if flowErr != nil {
		return nil, flowErr
	}
	result["flow"] = flowPayload["data"]
	result["flow_api"] = flowPath
	return result, nil
}

func (a NativeSite) runVPNApprovalDeal(ctx context.Context, command string, args []string) (map[string]any, *siteError) {
	id, reason := "", ""
	yes, action := false, command
	if command == "deal" {
		action = ""
	}
	for index := 0; index < len(args); index++ {
		arg, value, inline := splitInline(args[index])
		switch arg {
		case "--json":
		case "--id":
			var err *siteError
			id, err = vpnOptionValue(args, &index, arg, value, inline)
			if err != nil {
				return nil, err
			}
		case "--yes":
			if inline {
				return nil, &siteError{Code: "invalid_argument", Message: "布尔参数不接受 =VALUE"}
			}
			yes = true
		case "--action":
			var err *siteError
			action, err = vpnOptionValue(args, &index, arg, value, inline)
			if err != nil {
				return nil, err
			}
		case "--reason", "--comment":
			var err *siteError
			reason, err = vpnOptionValue(args, &index, arg, value, inline)
			if err != nil {
				return nil, err
			}
		default:
			return nil, &siteError{Code: "invalid_argument", Message: "vpn approval deal 参数无效: " + arg}
		}
	}
	if strings.TrimSpace(id) == "" {
		return nil, &siteError{Code: "invalid_argument", Message: "vpn approval 必须提供 --id"}
	}
	if action != "approve" && action != "reject" {
		return nil, &siteError{Code: "invalid_argument", Message: "审批动作必须是 approve 或 reject"}
	}
	if action == "reject" && strings.TrimSpace(reason) == "" {
		return nil, &siteError{Code: "invalid_argument", Message: "reject 必须提供 --reason"}
	}
	if !yes {
		return nil, &siteError{Code: "confirmation_required", Message: "审批处理会改变远端状态，请加 --yes"}
	}
	infoPayload, infoErr := a.vpnBusinessJSON(ctx, "/api/users/info", "GET", nil, true)
	if infoErr != nil {
		return nil, infoErr
	}
	user, ok := infoPayload["data"].(map[string]any)
	if !ok || user["userId"] == nil {
		return nil, &siteError{Code: "response_error", Message: "VPN 用户信息缺少 userId，无法提交审批处理"}
	}
	remoteResult := "20"
	if action == "reject" {
		remoteResult = "30"
	}
	path := "/api/users/approve/center/deal"
	payload, requestErr := a.vpnBusinessJSON(ctx, path, "POST", map[string]any{
		"approveComment": reason,
		"approveResult":  remoteResult,
		"approveUser":    user["userId"],
		"ids":            []any{vpnIdentifier(id)},
	}, false)
	if requestErr != nil {
		return nil, requestErr
	}
	return map[string]any{"ok": true, "submitted": true, "confirmed": true, "evidence": "VPN approve/center/deal code 200", "vpn": true, "operation": "approval-" + action, "api": path, "approval_id": id, "action": action, "data": payload["data"]}, nil
}

func vpnPageData(data any) (items, total, page, pageSize any, ok bool) {
	pageData, isPage := data.(map[string]any)
	if !isPage {
		return nil, nil, nil, nil, false
	}
	items, hasItems := pageData["data"]
	if !hasItems {
		return nil, nil, nil, nil, false
	}
	return items, pageData["total"], pageData["pageIndex"], pageData["pageSize"], true
}

func vpnIdentifier(value string) any {
	if parsed, err := strconv.ParseInt(value, 10, 64); err == nil {
		return parsed
	}
	return value
}

func vpnTimezoneOffset() int {
	_, offset := time.Now().Zone()
	return offset / 60
}

type vpnDevicesOptions struct {
	page, pageSize int
}

func parseVPNDevicesOptions(args []string) (vpnDevicesOptions, *siteError) {
	options := vpnDevicesOptions{page: 1, pageSize: 10}
	for index := 0; index < len(args); index++ {
		arg, value, inline := splitInline(args[index])
		if arg == "--json" {
			continue
		}
		if arg != "--page" && arg != "--page-size" {
			return vpnDevicesOptions{}, &siteError{Code: "invalid_argument", Message: "vpn devices 参数无效: " + arg}
		}
		var err *siteError
		value, err = vpnOptionValue(args, &index, arg, value, inline)
		if err != nil {
			return vpnDevicesOptions{}, err
		}
		parsed, parseErr := strconv.Atoi(value)
		if parseErr != nil || parsed < 1 {
			return vpnDevicesOptions{}, &siteError{Code: "invalid_argument", Message: arg + " 必须是正整数"}
		}
		if arg == "--page" {
			options.page = parsed
		} else {
			options.pageSize = parsed
		}
	}
	return options, nil
}

func (a NativeSite) runVPNDevices(ctx context.Context, args []string) (map[string]any, *siteError) {
	if len(args) > 0 && args[0] == "list" {
		args = args[1:]
	}
	options, parseErr := parseVPNDevicesOptions(args)
	if parseErr != nil {
		return nil, parseErr
	}
	path := "/api/users/device/list/page"
	body := map[string]any{"pageIndex": options.page, "pageSize": options.pageSize, "condition": map[string]any{}}
	payload, runErr := a.vpnBusinessJSON(ctx, path, "POST", body, true)
	if runErr != nil {
		return nil, runErr
	}
	data := payload["data"]
	result := vpnBusinessResult("devices", path, data, map[string]any{
		"filters": map[string]any{"page": options.page, "page_size": options.pageSize},
	})
	if items, total, page, pageSize, ok := vpnPageData(data); ok {
		result["devices"], result["total"], result["page"], result["page_size"] = items, total, page, pageSize
	} else {
		result["devices"] = data
	}
	return result, nil
}

type vpnDeviceActionOptions struct {
	deviceID, userID, useMode, offlineKey string
	yes                                   bool
}

func parseVPNDeviceActionOptions(command string, args []string) (vpnDeviceActionOptions, *siteError) {
	options := vpnDeviceActionOptions{}
	for index := 0; index < len(args); index++ {
		arg, value, inline := splitInline(args[index])
		switch arg {
		case "--json":
		case "--device-id", "--feature-code":
			var err *siteError
			options.deviceID, err = vpnOptionValue(args, &index, arg, value, inline)
			if err != nil {
				return vpnDeviceActionOptions{}, err
			}
		case "--user-id":
			var err *siteError
			options.userID, err = vpnOptionValue(args, &index, arg, value, inline)
			if err != nil {
				return vpnDeviceActionOptions{}, err
			}
		case "--use-mode":
			var err *siteError
			options.useMode, err = vpnOptionValue(args, &index, arg, value, inline)
			if err != nil {
				return vpnDeviceActionOptions{}, err
			}
		case "--offline-key":
			var err *siteError
			options.offlineKey, err = vpnOptionValue(args, &index, arg, value, inline)
			if err != nil {
				return vpnDeviceActionOptions{}, err
			}
		case "--yes":
			if inline {
				return vpnDeviceActionOptions{}, &siteError{Code: "invalid_argument", Message: "布尔参数不接受 =VALUE"}
			}
			options.yes = true
		default:
			return vpnDeviceActionOptions{}, &siteError{Code: "invalid_argument", Message: "vpn device " + command + " 参数无效: " + arg}
		}
	}
	if strings.TrimSpace(options.deviceID) == "" {
		return vpnDeviceActionOptions{}, &siteError{Code: "invalid_argument", Message: "vpn device " + command + " 必须提供 --device-id"}
	}
	if command == "remove-user" && strings.TrimSpace(options.userID) == "" {
		return vpnDeviceActionOptions{}, &siteError{Code: "invalid_argument", Message: "移除设备用户必须提供 --user-id"}
	}
	if command != "offline" && (options.useMode != "" || options.offlineKey != "") {
		return vpnDeviceActionOptions{}, &siteError{Code: "invalid_argument", Message: "--use-mode 和 --offline-key 只适用于 device offline"}
	}
	if options.useMode != "" && options.useMode != "noTerminal" {
		return vpnDeviceActionOptions{}, &siteError{Code: "invalid_argument", Message: "--use-mode 目前只能是 noTerminal"}
	}
	if options.offlineKey != "" && options.useMode == "" {
		return vpnDeviceActionOptions{}, &siteError{Code: "invalid_argument", Message: "提供 --offline-key 时必须同时指定 --use-mode noTerminal"}
	}
	if !options.yes {
		return vpnDeviceActionOptions{}, &siteError{Code: "confirmation_required", Message: "VPN 设备操作会改变远端状态，请加 --yes"}
	}
	return options, nil
}

func (a NativeSite) runVPNDevice(ctx context.Context, args []string) (map[string]any, *siteError) {
	if len(args) == 0 {
		return nil, &siteError{Code: "invalid_argument", Message: "vpn device 必须提供 offline、unbind 或 remove-user"}
	}
	command := args[0]
	if command != "offline" && command != "unbind" && command != "remove-user" {
		return nil, &siteError{Code: "invalid_argument", Message: "未知 vpn device 子命令: " + command}
	}
	options, parseErr := parseVPNDeviceActionOptions(command, args[1:])
	if parseErr != nil {
		return nil, parseErr
	}
	if command == "remove-user" {
		path := "/api/users/device/unbindUser"
		payload, runErr := a.vpnBusinessJSON(ctx, path, "POST", map[string]any{"userId": options.userID, "deviceId": options.deviceID}, false)
		if runErr != nil {
			return nil, runErr
		}
		return map[string]any{"ok": true, "submitted": true, "confirmed": true, "evidence": "VPN device/unbindUser code 200", "vpn": true, "operation": "device-remove-user", "api": path, "device_id": options.deviceID, "user_id": options.userID, "data": payload["data"]}, nil
	}

	infoPayload, infoErr := a.vpnBusinessJSON(ctx, "/api/users/info", "GET", nil, true)
	if infoErr != nil {
		return nil, infoErr
	}
	user, ok := infoPayload["data"].(map[string]any)
	if !ok || user["userId"] == nil {
		return nil, &siteError{Code: "response_error", Message: "VPN 用户信息缺少 userId，无法操作设备"}
	}
	body := map[string]any{"featureCode": options.deviceID, "userId": user["userId"]}
	if command == "offline" && options.useMode == "noTerminal" {
		body["useMode"] = options.useMode
		body["offlineKey"] = options.offlineKey
		delete(body, "featureCode")
	}
	path := "/api/users/center/device/" + command
	payload, runErr := a.vpnBusinessJSON(ctx, path, "POST", body, false)
	if runErr != nil {
		return nil, runErr
	}
	return map[string]any{"ok": true, "submitted": true, "confirmed": true, "evidence": "VPN device/" + command + " code 200", "vpn": true, "operation": "device-" + command, "api": path, "device_id": options.deviceID, "data": payload["data"]}, nil
}

func (a NativeSite) runVPNApply(ctx context.Context, args []string) (map[string]any, *siteError) {
	if len(args) == 0 || strings.HasPrefix(args[0], "--") {
		return a.runVPNApplyList(ctx, args)
	}
	switch args[0] {
	case "list", "services":
		return a.runVPNApplyList(ctx, args[1:])
	case "flow":
		return a.runVPNApplyFlow(ctx, args[1:])
	case "request", "submit":
		return a.runVPNApplyRequest(ctx, args[1:])
	case "cancel", "cancel-account", "account-cancel":
		return a.runVPNApplyCancelAccount(ctx, args[1:])
	default:
		return nil, &siteError{Code: "invalid_argument", Message: "未知 vpn apply 子命令: " + args[0]}
	}
}

func (a NativeSite) runVPNApplyList(ctx context.Context, args []string) (map[string]any, *siteError) {
	search := ""
	for index := 0; index < len(args); index++ {
		arg, value, inline := splitInline(args[index])
		if arg == "--json" {
			continue
		}
		if arg != "--search" {
			return nil, &siteError{Code: "invalid_argument", Message: "vpn apply list 参数无效: " + arg}
		}
		var err *siteError
		search, err = vpnOptionValue(args, &index, arg, value, inline)
		if err != nil {
			return nil, err
		}
	}
	path := "/api/users/service/getAllApplicabilityService"
	payload, runErr := a.vpnBusinessJSON(ctx, path, "POST", map[string]any{"serviceType": "ALL", "serviceName": search}, true)
	if runErr != nil {
		return nil, runErr
	}
	data := payload["data"]
	applications := vpnVisibleApplications(data)
	return vpnBusinessResult("apply-services", path, data, map[string]any{
		"filters":          map[string]any{"search": search},
		"applications":     applications,
		"visible_count":    len(applications),
		"application_data": data,
	}), nil
}

func vpnVisibleApplications(data any) []any {
	items, ok := data.([]any)
	if !ok {
		if page, pageOK := data.(map[string]any); pageOK {
			items, _ = page["data"].([]any)
		}
	}
	visible := make([]any, 0, len(items))
	for _, item := range items {
		candidate, ok := item.(map[string]any)
		if !ok {
			continue
		}
		switch show := candidate["ifShow"].(type) {
		case bool:
			if show {
				visible = append(visible, item)
			}
		case string:
			if show == "1" || strings.EqualFold(show, "true") {
				visible = append(visible, item)
			}
		case float64:
			if show != 0 {
				visible = append(visible, item)
			}
		}
	}
	return visible
}

func (a NativeSite) runVPNApplyFlow(ctx context.Context, args []string) (map[string]any, *siteError) {
	if err := onlyJSONArgs(args); err != nil {
		return nil, err
	}
	path := "/api/users/approve/center/getFlow"
	payload, runErr := a.vpnBusinessJSON(ctx, path, "GET", nil, true)
	if runErr != nil {
		return nil, runErr
	}
	return vpnBusinessResult("apply-flow", path, payload["data"], map[string]any{"flow": payload["data"]}), nil
}

type vpnApplyRequestOptions struct {
	serviceID, serviceName, reason, start, end string
	applyTimeType                              string
	yes                                        bool
}

func parseVPNApplyRequestOptions(args []string) (vpnApplyRequestOptions, *siteError) {
	options := vpnApplyRequestOptions{applyTimeType: "1"}
	modeSet := false
	for index := 0; index < len(args); index++ {
		arg, value, inline := splitInline(args[index])
		switch arg {
		case "--json":
		case "--service-id", "--service-name", "--reason", "--start", "--start-time", "--end", "--end-time":
			var err *siteError
			value, err = vpnOptionValue(args, &index, arg, value, inline)
			if err != nil {
				return vpnApplyRequestOptions{}, err
			}
			switch arg {
			case "--service-id":
				options.serviceID = value
			case "--service-name":
				options.serviceName = value
			case "--reason":
				options.reason = value
			case "--start", "--start-time":
				options.start = value
			default:
				options.end = value
			}
		case "--temporary", "--perpetual":
			if inline {
				return vpnApplyRequestOptions{}, &siteError{Code: "invalid_argument", Message: "布尔参数不接受 =VALUE"}
			}
			if modeSet {
				return vpnApplyRequestOptions{}, &siteError{Code: "invalid_argument", Message: "--temporary 和 --perpetual 只能选择一个"}
			}
			modeSet = true
			if arg == "--perpetual" {
				options.applyTimeType = "0"
			}
		case "--yes":
			if inline {
				return vpnApplyRequestOptions{}, &siteError{Code: "invalid_argument", Message: "布尔参数不接受 =VALUE"}
			}
			options.yes = true
		default:
			return vpnApplyRequestOptions{}, &siteError{Code: "invalid_argument", Message: "vpn apply request 参数无效: " + arg}
		}
	}
	if strings.TrimSpace(options.serviceID) == "" || strings.TrimSpace(options.serviceName) == "" {
		return vpnApplyRequestOptions{}, &siteError{Code: "invalid_argument", Message: "申请服务必须提供 --service-id 和 --service-name"}
	}
	if options.applyTimeType == "0" && (strings.TrimSpace(options.start) != "" || strings.TrimSpace(options.end) != "") {
		return vpnApplyRequestOptions{}, &siteError{Code: "invalid_argument", Message: "永久申请不能提供起止时间"}
	}
	if (strings.TrimSpace(options.start) == "") != (strings.TrimSpace(options.end) == "") {
		return vpnApplyRequestOptions{}, &siteError{Code: "invalid_argument", Message: "起止时间必须同时提供"}
	}
	if !options.yes {
		return vpnApplyRequestOptions{}, &siteError{Code: "confirmation_required", Message: "提交 VPN 服务申请会改变远端状态，请加 --yes"}
	}
	return options, nil
}

func vpnApplyTimeValue(label, value string) (string, *siteError) {
	value = strings.TrimSpace(value)
	for _, layout := range []string{"2006-01-02 15:04:05", "2006-01-02 15:04"} {
		parsed, err := time.Parse(layout, value)
		if err == nil {
			return parsed.Format("2006-01-02 15:04:05"), nil
		}
	}
	return "", &siteError{Code: "invalid_argument", Message: label + " 必须是 YYYY-MM-DD HH:MM 或 YYYY-MM-DD HH:MM:SS"}
}

func (a NativeSite) runVPNApplyRequest(ctx context.Context, args []string) (map[string]any, *siteError) {
	options, parseErr := parseVPNApplyRequestOptions(args)
	if parseErr != nil {
		return nil, parseErr
	}
	start, end := "", ""
	if options.start != "" {
		var err *siteError
		start, err = vpnApplyTimeValue("--start", options.start)
		if err != nil {
			return nil, err
		}
		end, err = vpnApplyTimeValue("--end", options.end)
		if err != nil {
			return nil, err
		}
	}
	contentSummary, _ := json.Marshal(map[string]string{"businessName": options.serviceName})
	path := "/api/users/service/createServiceApply"
	body := map[string]any{
		"serviceId":           vpnIdentifier(options.serviceID),
		"applyTimeType":       options.applyTimeType,
		"applyStartTime":      start,
		"applyEndTime":        end,
		"applyReason":         options.reason,
		"contentSummary":      string(contentSummary),
		"applyTimeZoneOffset": vpnTimezoneOffset(),
	}
	payload, runErr := a.vpnBusinessJSON(ctx, path, "POST", body, false)
	if runErr != nil {
		return nil, runErr
	}
	return map[string]any{"ok": true, "submitted": true, "confirmed": true, "evidence": "VPN service/createServiceApply code 200", "vpn": true, "operation": "apply-request", "api": path, "service_id": options.serviceID, "service_name": options.serviceName, "apply_time_type": options.applyTimeType, "apply_start_time": start, "apply_end_time": end, "data": payload["data"]}, nil
}

func (a NativeSite) runVPNApplyCancelAccount(ctx context.Context, args []string) (map[string]any, *siteError) {
	reason, yes := "", false
	for index := 0; index < len(args); index++ {
		arg, value, inline := splitInline(args[index])
		switch arg {
		case "--json":
		case "--reason", "--comment":
			var err *siteError
			reason, err = vpnOptionValue(args, &index, arg, value, inline)
			if err != nil {
				return nil, err
			}
		case "--yes":
			if inline {
				return nil, &siteError{Code: "invalid_argument", Message: "布尔参数不接受 =VALUE"}
			}
			yes = true
		default:
			return nil, &siteError{Code: "invalid_argument", Message: "vpn apply cancel-account 参数无效: " + arg}
		}
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return nil, &siteError{Code: "invalid_argument", Message: "账号注销申请必须提供 --reason"}
	}
	if len([]rune(reason)) > 200 {
		return nil, &siteError{Code: "invalid_argument", Message: "--reason 不能超过 200 个字符"}
	}
	if !yes {
		return nil, &siteError{Code: "confirmation_required", Message: "提交账号注销申请会改变远端状态，请加 --yes"}
	}
	path := "/api/users/service/cancleServiceApply"
	body := map[string]any{"applyType": "WriteOffAccount", "applyReason": reason, "applyTimeZoneOffset": vpnTimezoneOffset()}
	payload, runErr := a.vpnBusinessJSON(ctx, path, "POST", body, false)
	if runErr != nil {
		return nil, runErr
	}
	return map[string]any{"ok": true, "submitted": true, "confirmed": true, "evidence": "VPN service/cancleServiceApply code 200", "vpn": true, "operation": "apply-account-cancel", "api": path, "apply_type": "WriteOffAccount", "reason": reason, "data": payload["data"]}, nil
}

type vpnShareListOptions struct {
	view, search, searchField string
	page, pageSize            int
}

func parseVPNShareListOptions(command string, args []string) (vpnShareListOptions, *siteError) {
	options := vpnShareListOptions{view: "sent", searchField: "filename", page: 1, pageSize: 10}
	for index := 0; index < len(args); index++ {
		arg, value, inline := splitInline(args[index])
		if arg == "--json" {
			continue
		}
		if command == "links" && (arg == "--view" || arg == "--tab") {
			return vpnShareListOptions{}, &siteError{Code: "invalid_argument", Message: "vpn links 不支持 --view"}
		}
		if arg != "--view" && arg != "--tab" && arg != "--search" && arg != "--search-field" && arg != "--page" && arg != "--page-size" {
			return vpnShareListOptions{}, &siteError{Code: "invalid_argument", Message: "vpn " + command + " 参数无效: " + arg}
		}
		var err *siteError
		value, err = vpnOptionValue(args, &index, arg, value, inline)
		if err != nil {
			return vpnShareListOptions{}, err
		}
		switch arg {
		case "--view", "--tab":
			options.view = value
		case "--search":
			options.search = value
		case "--search-field":
			options.searchField = value
		default:
			parsed, parseErr := strconv.Atoi(value)
			if parseErr != nil || parsed < 1 {
				return vpnShareListOptions{}, &siteError{Code: "invalid_argument", Message: arg + " 必须是正整数"}
			}
			if arg == "--page" {
				options.page = parsed
			} else {
				options.pageSize = parsed
			}
		}
	}
	if command == "shares" && options.view != "sent" && options.view != "received" && options.view != "my-share" && options.view != "my-receive" {
		return vpnShareListOptions{}, &siteError{Code: "invalid_argument", Message: "--view 必须是 sent 或 received"}
	}
	if command == "links" && options.searchField != "filename" {
		return vpnShareListOptions{}, &siteError{Code: "invalid_argument", Message: "vpn links 只支持按文件名搜索"}
	}
	if options.searchField != "filename" && options.searchField != "sender" {
		return vpnShareListOptions{}, &siteError{Code: "invalid_argument", Message: "--search-field 必须是 filename 或 sender"}
	}
	return options, nil
}

func (a NativeSite) runVPNShares(ctx context.Context, args []string) (map[string]any, *siteError) {
	if len(args) > 0 && args[0] == "list" {
		args = args[1:]
	}
	options, parseErr := parseVPNShareListOptions("shares", args)
	if parseErr != nil {
		return nil, parseErr
	}
	infoPayload, infoErr := a.vpnBusinessJSON(ctx, "/api/users/info", "GET", nil, true)
	if infoErr != nil {
		return nil, infoErr
	}
	user, ok := infoPayload["data"].(map[string]any)
	if !ok || user["userId"] == nil {
		return nil, &siteError{Code: "response_error", Message: "VPN 用户信息缺少 userId，无法查询文件分享"}
	}
	conditionKey := "fromId"
	view := options.view
	if view == "my-share" {
		view = "sent"
	}
	if view == "my-receive" {
		view = "received"
	}
	if view == "received" {
		conditionKey = "objectId"
	}
	conditionLike := map[string]any{"filename": ""}
	if options.searchField == "sender" {
		conditionLike = map[string]any{"userLike": options.search}
	} else {
		conditionLike["filename"] = options.search
	}
	body := map[string]any{
		"pageIndex":     options.page,
		"pageSize":      options.pageSize,
		"condition":     map[string]any{conditionKey: user["userId"]},
		"conditionLike": conditionLike,
	}
	path := "/api/users/safeSpace/getShareFilePage"
	payload, runErr := a.vpnBusinessJSON(ctx, path, "POST", body, true)
	if runErr != nil {
		return nil, runErr
	}
	data := payload["data"]
	result := vpnBusinessResult("shares", path, data, map[string]any{
		"view":    view,
		"filters": map[string]any{"search": options.search, "search_field": options.searchField, "page": options.page, "page_size": options.pageSize},
	})
	if items, total, page, pageSize, pageOK := vpnPageData(data); pageOK {
		result["shares"], result["total"], result["page"], result["page_size"] = items, total, page, pageSize
	} else {
		result["shares"] = data
	}
	return result, nil
}

func (a NativeSite) runVPNLinks(ctx context.Context, args []string) (map[string]any, *siteError) {
	if len(args) > 0 && (args[0] == "delete" || args[0] == "remove") {
		return a.runVPNLinkDelete(ctx, args[1:])
	}
	if len(args) > 0 && args[0] == "list" {
		args = args[1:]
	}
	options, parseErr := parseVPNShareListOptions("links", args)
	if parseErr != nil {
		return nil, parseErr
	}
	conditionLike := map[string]any{}
	if options.search != "" {
		conditionLike["filename"] = options.search
	}
	body := map[string]any{
		"condition":     map[string]any{},
		"conditionLike": conditionLike,
		"notCondition":  map[string]any{},
		"pageIndex":     options.page,
		"pageSize":      options.pageSize,
	}
	path := "/api/client/share/link/page"
	payload, runErr := a.vpnBusinessJSON(ctx, path, "POST", body, true)
	if runErr != nil {
		return nil, runErr
	}
	data := payload["data"]
	result := vpnBusinessResult("links", path, data, map[string]any{
		"filters": map[string]any{"search": options.search, "page": options.page, "page_size": options.pageSize},
	})
	if items, total, page, pageSize, pageOK := vpnPageData(data); pageOK {
		result["links"], result["total"], result["page"], result["page_size"] = items, total, page, pageSize
	} else {
		result["links"] = data
	}
	return result, nil
}

func (a NativeSite) runVPNLinkDelete(ctx context.Context, args []string) (map[string]any, *siteError) {
	id, yes := "", false
	for index := 0; index < len(args); index++ {
		arg, value, inline := splitInline(args[index])
		switch arg {
		case "--json":
		case "--id":
			var err *siteError
			id, err = vpnOptionValue(args, &index, arg, value, inline)
			if err != nil {
				return nil, err
			}
		case "--yes":
			if inline {
				return nil, &siteError{Code: "invalid_argument", Message: "布尔参数不接受 =VALUE"}
			}
			yes = true
		default:
			return nil, &siteError{Code: "invalid_argument", Message: "vpn links delete 参数无效: " + arg}
		}
	}
	if strings.TrimSpace(id) == "" {
		return nil, &siteError{Code: "invalid_argument", Message: "删除分享链接必须提供 --id"}
	}
	if !yes {
		return nil, &siteError{Code: "confirmation_required", Message: "删除 VPN 分享链接会改变远端状态，请加 --yes"}
	}
	path := "/api/client/share/link/delete/" + url.PathEscape(id)
	payload, runErr := a.vpnBusinessJSON(ctx, path, "GET", nil, false)
	if runErr != nil {
		return nil, runErr
	}
	return map[string]any{"ok": true, "submitted": true, "confirmed": true, "evidence": "VPN share/link/delete code 200", "vpn": true, "operation": "link-delete", "api": path, "link_id": id, "data": payload["data"]}, nil
}

func (a NativeSite) runVPNProfile(ctx context.Context, args []string) (map[string]any, *siteError) {
	if len(args) == 0 || args[0] == "get" {
		if len(args) > 0 {
			args = args[1:]
		}
		if err := onlyJSONArgs(args); err != nil {
			return nil, err
		}
		path := "/api/users/info"
		payload, runErr := a.vpnBusinessJSON(ctx, path, "GET", nil, true)
		if runErr != nil {
			return nil, runErr
		}
		return vpnBusinessResult("profile", path, payload["data"], map[string]any{"profile": payload["data"]}), nil
	}
	if args[0] == "bindings" || args[0] == "bind-status" {
		if err := onlyJSONArgs(args[1:]); err != nil {
			return nil, err
		}
		path := "/api/users/person/getBindInfos"
		payload, runErr := a.vpnBusinessJSON(ctx, path, "GET", nil, true)
		if runErr != nil {
			return nil, runErr
		}
		return vpnBusinessResult("profile-bindings", path, payload["data"], map[string]any{"bindings": payload["data"]}), nil
	}
	if args[0] == "bind-code" || args[0] == "bind" {
		return a.runVPNProfileBinding(ctx, args[0], args[1:])
	}
	if args[0] == "unbind-code" || args[0] == "unbind" {
		return a.runVPNProfileUnbinding(ctx, args[0], args[1:])
	}
	if args[0] == "password" || args[0] == "change-password" {
		return a.runVPNProfilePassword(ctx, args[1:])
	}
	if args[0] == "common-locations" || args[0] == "locations" {
		return a.runVPNCommonLocations(ctx, args[1:])
	}
	if args[0] != "rename" && args[0] != "name" {
		return nil, &siteError{Code: "invalid_argument", Message: "未知 vpn profile 子命令: " + args[0]}
	}
	name, yes := "", false
	for index := 1; index < len(args); index++ {
		arg, value, inline := splitInline(args[index])
		switch arg {
		case "--json":
		case "--name", "--new-name":
			var err *siteError
			name, err = vpnOptionValue(args, &index, arg, value, inline)
			if err != nil {
				return nil, err
			}
		case "--yes":
			if inline {
				return nil, &siteError{Code: "invalid_argument", Message: "布尔参数不接受 =VALUE"}
			}
			yes = true
		default:
			return nil, &siteError{Code: "invalid_argument", Message: "vpn profile rename 参数无效: " + arg}
		}
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, &siteError{Code: "invalid_argument", Message: "修改账号名称必须提供 --name"}
	}
	if len([]rune(name)) > 32 {
		return nil, &siteError{Code: "invalid_argument", Message: "--name 不能超过 32 个字符"}
	}
	if !yes {
		return nil, &siteError{Code: "confirmation_required", Message: "修改 VPN 账号名称会改变远端状态，请加 --yes"}
	}
	path := "/api/users/person/restName"
	payload, runErr := a.vpnBusinessJSON(ctx, path, "POST", map[string]any{"name": name}, false)
	if runErr != nil {
		return nil, runErr
	}
	return map[string]any{"ok": true, "submitted": true, "confirmed": true, "evidence": "VPN person/restName code 200", "vpn": true, "operation": "profile-rename", "api": path, "name": name, "data": payload["data"]}, nil
}

type vpnCommonLocation struct {
	province, city string
}

func parseVPNCommonLocations(args []string) ([]vpnCommonLocation, bool, *siteError) {
	locations := []vpnCommonLocation{}
	yes := false
	for index := 0; index < len(args); index++ {
		arg, value, inline := splitInline(args[index])
		switch arg {
		case "--json":
		case "--location":
			var err *siteError
			value, err = vpnOptionValue(args, &index, arg, value, inline)
			if err != nil {
				return nil, false, err
			}
			separator := "/"
			if !strings.Contains(value, separator) {
				separator = ","
			}
			parts := strings.SplitN(value, separator, 2)
			if len(parts) != 2 {
				return nil, false, &siteError{Code: "invalid_argument", Message: "--location 必须使用 省/市 或 省,市 格式"}
			}
			province, city := strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
			if province == "" || city == "" || strings.ContainsAny(province+city, "\r\n") || len([]rune(province)) > 128 || len([]rune(city)) > 128 {
				return nil, false, &siteError{Code: "invalid_argument", Message: "--location 的省、市不能为空，且不能超过 128 个字符或包含换行"}
			}
			locations = append(locations, vpnCommonLocation{province: province, city: city})
		case "--yes":
			if inline {
				return nil, false, &siteError{Code: "invalid_argument", Message: "布尔参数不接受 =VALUE"}
			}
			yes = true
		default:
			return nil, false, &siteError{Code: "invalid_argument", Message: "vpn profile common-locations 参数无效: " + arg}
		}
	}
	if len(locations) > 10 {
		return nil, false, &siteError{Code: "invalid_argument", Message: "常用地区最多只能设置 10 个"}
	}
	return locations, yes, nil
}

func vpnCommonLocationData(value any) ([]map[string]string, bool) {
	items, ok := value.([]any)
	if !ok {
		return nil, false
	}
	locations := make([]map[string]string, 0, len(items))
	for _, item := range items {
		entry, ok := item.(map[string]any)
		if !ok {
			return nil, false
		}
		province, provinceOK := entry["province"].(string)
		city, cityOK := entry["city"].(string)
		if !provinceOK || !cityOK {
			return nil, false
		}
		locations = append(locations, map[string]string{"province": province, "city": city})
	}
	return locations, true
}

func (a NativeSite) runVPNCommonLocations(ctx context.Context, args []string) (map[string]any, *siteError) {
	command := "list"
	if len(args) > 0 && !strings.HasPrefix(args[0], "--") {
		command, args = args[0], args[1:]
	}
	if command != "list" && command != "get" && command != "update" && command != "set" {
		return nil, &siteError{Code: "invalid_argument", Message: "vpn profile common-locations 只支持 list 或 update"}
	}
	if command == "list" || command == "get" {
		if err := onlyJSONArgs(args); err != nil {
			return nil, err
		}
		path := "/api/users/info"
		payload, runErr := a.vpnBusinessJSON(ctx, path, "GET", nil, true)
		if runErr != nil {
			return nil, runErr
		}
		user, ok := payload["data"].(map[string]any)
		if !ok {
			return nil, &siteError{Code: "response_error", Message: "VPN 用户信息格式无效"}
		}
		locations, ok := vpnCommonLocationData(user["commonLocationList"])
		if !ok {
			return nil, &siteError{Code: "response_error", Message: "VPN 用户信息缺少可解析的 commonLocationList"}
		}
		return vpnBusinessResult("profile-common-locations", path, locations, map[string]any{"common_locations": locations}), nil
	}
	locations, yes, parseErr := parseVPNCommonLocations(args)
	if parseErr != nil {
		return nil, parseErr
	}
	if len(locations) == 0 {
		return nil, &siteError{Code: "invalid_argument", Message: "更新常用地区至少需要提供一个 --location"}
	}
	if !yes {
		return nil, &siteError{Code: "confirmation_required", Message: "更新 VPN 常用地区会改变远端状态，请加 --yes"}
	}
	commonList := make([]map[string]string, 0, len(locations))
	for _, location := range locations {
		commonList = append(commonList, map[string]string{"province": location.province, "city": location.city})
	}
	updatePath := "/api/users/updateCommonLocation"
	if _, runErr := a.vpnBusinessJSON(ctx, updatePath, "POST", map[string]any{"commonList": commonList}, false); runErr != nil {
		return nil, runErr
	}
	infoPath := "/api/users/info"
	infoPayload, infoErr := a.vpnBusinessJSON(ctx, infoPath, "GET", nil, true)
	if infoErr != nil {
		return nil, &siteError{Code: "mutation_unverified", Message: "常用地区更新已返回成功，但回读用户信息失败", Details: map[string]any{"api": updatePath, "cause": infoErr.Code}}
	}
	user, ok := infoPayload["data"].(map[string]any)
	if !ok {
		return nil, &siteError{Code: "mutation_unverified", Message: "常用地区更新已返回成功，但回读用户信息格式无效", Details: map[string]any{"api": updatePath}}
	}
	readback, ok := vpnCommonLocationData(user["commonLocationList"])
	if !ok || len(readback) != len(commonList) {
		return nil, &siteError{Code: "mutation_unverified", Message: "常用地区更新已返回成功，但回读结果无法确认", Details: map[string]any{"api": updatePath, "readback": readback}}
	}
	for index := range commonList {
		if readback[index]["province"] != commonList[index]["province"] || readback[index]["city"] != commonList[index]["city"] {
			return nil, &siteError{Code: "mutation_unverified", Message: "常用地区更新已返回成功，但回读结果与提交内容不一致", Details: map[string]any{"api": updatePath, "readback": readback}}
		}
	}
	return vpnBusinessResult("profile-common-locations-update", updatePath, readback, map[string]any{"common_locations": readback, "readback_api": infoPath}), nil
}

type vpnProfilePasswordOptions struct {
	currentPassword, newPassword string
	yes                          bool
}

func parseVPNProfilePasswordOptions(args []string) (vpnProfilePasswordOptions, *siteError) {
	options := vpnProfilePasswordOptions{}
	for index := 0; index < len(args); index++ {
		arg, value, inline := splitInline(args[index])
		switch arg {
		case "--json":
		case "--current-password", "--new-password":
			var err *siteError
			_, err = vpnOptionValue(args, &index, arg, value, inline)
			if err != nil {
				return vpnProfilePasswordOptions{}, err
			}
		case "--current-password-stdin":
			if inline {
				return vpnProfilePasswordOptions{}, &siteError{Code: "invalid_argument", Message: "布尔参数不接受 =VALUE"}
			}
		case "--yes":
			if inline {
				return vpnProfilePasswordOptions{}, &siteError{Code: "invalid_argument", Message: "布尔参数不接受 =VALUE"}
			}
			options.yes = true
		default:
			return vpnProfilePasswordOptions{}, &siteError{Code: "invalid_argument", Message: "vpn profile password 参数无效: " + arg}
		}
	}
	if !options.yes {
		return vpnProfilePasswordOptions{}, &siteError{Code: "confirmation_required", Message: "修改 VPN 密码会改变远端认证状态，请加 --yes"}
	}
	return options, nil
}

func (a NativeSite) runVPNProfilePassword(ctx context.Context, args []string) (map[string]any, *siteError) {
	options, parseErr := parseVPNProfilePasswordOptions(args)
	if parseErr != nil {
		return nil, parseErr
	}
	var err *siteError
	options.currentPassword, err = businessSecret(args, "--current-password", "CSUST_VPN_CURRENT_PASSWORD")
	if err != nil {
		return nil, err
	}
	options.newPassword, err = businessSecret(args, "--new-password", "CSUST_VPN_NEW_PASSWORD")
	if err != nil {
		return nil, err
	}
	if options.currentPassword == "" || options.newPassword == "" {
		return nil, &siteError{Code: "credentials_required", Message: "当前密码和新密码不能为空"}
	}
	if strings.ContainsAny(options.newPassword, "\r\n") || len([]rune(options.newPassword)) > 32 {
		return nil, &siteError{Code: "invalid_argument", Message: "新密码不能超过 32 个字符或包含换行"}
	}

	_, cookie, session, connErr := vpnConnection(false)
	if connErr != nil {
		return nil, connErr
	}
	infoPayload, infoErr := a.vpnBusinessJSON(ctx, "/api/users/info", "GET", nil, true)
	if infoErr != nil {
		return nil, infoErr
	}
	user, ok := infoPayload["data"].(map[string]any)
	if !ok || user["userId"] == nil {
		return nil, &siteError{Code: "response_error", Message: "VPN 用户信息缺少 userId，无法修改密码"}
	}
	token, _ := session["token"].(string)
	if token == "" {
		token, _ = user["token"].(string)
	}
	parts := strings.Split(token, "-")
	if len(parts) < 3 || len([]byte(parts[2])) != aes.BlockSize {
		return nil, &siteError{Code: "vpn_protocol_error", Message: "VPN 会话缺少可用于修改密码的 AES 密钥"}
	}
	key := parts[2]
	oldPassword, encryptErr := encryptVPNPassword(options.currentPassword, key)
	if encryptErr != nil {
		return nil, encryptErr
	}
	newPassword, encryptErr := encryptVPNPassword(options.newPassword, key)
	if encryptErr != nil {
		return nil, encryptErr
	}
	path := "/api/client/user/center/resetUserPassword"
	payload, runErr := a.vpnBusinessJSON(ctx, path, "POST", map[string]any{
		"id":          user["userId"],
		"oldPassword": oldPassword,
		"password":    newPassword,
	}, false)
	if runErr != nil {
		return nil, runErr
	}
	cleanupError := ""
	if removeErr := removeCookieFile(cookie); removeErr != nil {
		cleanupError = removeErr.Error()
	} else if removeErr := removeCookieFile(sessionPath(false)); removeErr != nil {
		cleanupError = removeErr.Error()
	}
	result := map[string]any{"ok": true, "submitted": true, "confirmed": true, "evidence": "VPN user/center/resetUserPassword code 200", "vpn": true, "operation": "profile-password", "api": path, "session_cleared": cleanupError == "", "data": payload["data"]}
	if cleanupError != "" {
		result["session_cleanup_error"] = cleanupError
	}
	return result, nil
}

type vpnProfileBindingOptions struct {
	typeName, address, code string
	yes                     bool
}

func parseVPNProfileBindingOptions(command string, args []string) (vpnProfileBindingOptions, *siteError) {
	options := vpnProfileBindingOptions{}
	for index := 0; index < len(args); index++ {
		arg, value, inline := splitInline(args[index])
		switch arg {
		case "--json":
		case "--type":
			var err *siteError
			options.typeName, err = vpnOptionValue(args, &index, arg, value, inline)
			if err != nil {
				return vpnProfileBindingOptions{}, err
			}
		case "--address", "--mobile", "--email":
			var err *siteError
			options.address, err = vpnOptionValue(args, &index, arg, value, inline)
			if err != nil {
				return vpnProfileBindingOptions{}, err
			}
		case "--code":
			var err *siteError
			options.code, err = vpnOptionValue(args, &index, arg, value, inline)
			if err != nil {
				return vpnProfileBindingOptions{}, err
			}
		case "--yes":
			if inline {
				return vpnProfileBindingOptions{}, &siteError{Code: "invalid_argument", Message: "布尔参数不接受 =VALUE"}
			}
			options.yes = true
		default:
			return vpnProfileBindingOptions{}, &siteError{Code: "invalid_argument", Message: "vpn profile " + command + " 参数无效: " + arg}
		}
	}
	if options.typeName != "phone" && options.typeName != "email" {
		return vpnProfileBindingOptions{}, &siteError{Code: "invalid_argument", Message: "--type 必须是 phone 或 email"}
	}
	if strings.TrimSpace(options.address) == "" {
		return vpnProfileBindingOptions{}, &siteError{Code: "invalid_argument", Message: "绑定联系方式必须提供 --address"}
	}
	if command == "bind-code" && strings.TrimSpace(options.code) != "" {
		return vpnProfileBindingOptions{}, &siteError{Code: "invalid_argument", Message: "bind-code 不接受 --code"}
	}
	if command == "bind" {
		if strings.TrimSpace(options.code) == "" {
			return vpnProfileBindingOptions{}, &siteError{Code: "invalid_argument", Message: "绑定联系方式必须提供 --code"}
		}
		if !vpnVerificationCode(options.code) {
			return vpnProfileBindingOptions{}, &siteError{Code: "invalid_argument", Message: "--code 必须是 4 位字母或数字验证码"}
		}
	}
	if !options.yes {
		return vpnProfileBindingOptions{}, &siteError{Code: "confirmation_required", Message: "VPN 绑定操作会发送验证码或改变远端状态，请加 --yes"}
	}
	return options, nil
}

func vpnVerificationCode(value string) bool {
	value = strings.TrimSpace(value)
	if len([]rune(value)) != 4 {
		return false
	}
	for _, char := range value {
		if !((char >= '0' && char <= '9') || (char >= 'A' && char <= 'Z') || (char >= 'a' && char <= 'z')) {
			return false
		}
	}
	return true
}

func (a NativeSite) runVPNProfileBinding(ctx context.Context, command string, args []string) (map[string]any, *siteError) {
	options, parseErr := parseVPNProfileBindingOptions(command, args)
	if parseErr != nil {
		return nil, parseErr
	}
	path := "/api/users/bindMobileEmail/send/code"
	body := map[string]any{"type": options.typeName, "loginNum": options.address}
	operation := "profile-bind-code"
	if command == "bind" {
		path = "/api/users/person/bindMobileEmail"
		body = map[string]any{"type": options.typeName, "bindNumber": options.address, "validCode": options.code}
		operation = "profile-bind"
	}
	payload, runErr := a.vpnBusinessJSON(ctx, path, "POST", body, false)
	if runErr != nil {
		return nil, runErr
	}
	return map[string]any{"ok": true, "submitted": true, "confirmed": true, "evidence": "VPN API code 200", "vpn": true, "operation": operation, "api": path, "type": options.typeName, "address": options.address, "data": payload["data"]}, nil
}

type vpnProfileUnbindOptions struct {
	typeName, address, code, authConfigID string
	yes                                   bool
}

func parseVPNProfileUnbindOptions(command string, args []string) (vpnProfileUnbindOptions, *siteError) {
	options := vpnProfileUnbindOptions{}
	for index := 0; index < len(args); index++ {
		arg, value, inline := splitInline(args[index])
		switch arg {
		case "--json":
		case "--type":
			var err *siteError
			options.typeName, err = vpnOptionValue(args, &index, arg, value, inline)
			if err != nil {
				return vpnProfileUnbindOptions{}, err
			}
		case "--address", "--mobile", "--email":
			var err *siteError
			options.address, err = vpnOptionValue(args, &index, arg, value, inline)
			if err != nil {
				return vpnProfileUnbindOptions{}, err
			}
		case "--code":
			var err *siteError
			options.code, err = vpnOptionValue(args, &index, arg, value, inline)
			if err != nil {
				return vpnProfileUnbindOptions{}, err
			}
		case "--auth-config-id":
			var err *siteError
			options.authConfigID, err = vpnOptionValue(args, &index, arg, value, inline)
			if err != nil {
				return vpnProfileUnbindOptions{}, err
			}
		case "--yes":
			if inline {
				return vpnProfileUnbindOptions{}, &siteError{Code: "invalid_argument", Message: "布尔参数不接受 =VALUE"}
			}
			options.yes = true
		default:
			return vpnProfileUnbindOptions{}, &siteError{Code: "invalid_argument", Message: "vpn profile " + command + " 参数无效: " + arg}
		}
	}
	if options.typeName != "phone" && options.typeName != "email" {
		return vpnProfileUnbindOptions{}, &siteError{Code: "invalid_argument", Message: "--type 必须是 phone 或 email"}
	}
	if command == "unbind-code" && strings.TrimSpace(options.address) == "" {
		return vpnProfileUnbindOptions{}, &siteError{Code: "invalid_argument", Message: "解绑验证码必须提供 --address"}
	}
	if command == "unbind" && !vpnVerificationCode(options.code) {
		return vpnProfileUnbindOptions{}, &siteError{Code: "invalid_argument", Message: "--code 必须是 4 位字母或数字验证码"}
	}
	if !options.yes {
		return vpnProfileUnbindOptions{}, &siteError{Code: "confirmation_required", Message: "VPN 解绑操作会发送验证码或改变远端状态，请加 --yes"}
	}
	return options, nil
}

func (a NativeSite) runVPNProfileUnbinding(ctx context.Context, command string, args []string) (map[string]any, *siteError) {
	options, parseErr := parseVPNProfileUnbindOptions(command, args)
	if parseErr != nil {
		return nil, parseErr
	}
	path := "/api/users/unbind/send/code"
	body := map[string]any{"loginNum": options.address, "type": options.typeName}
	operation := "profile-unbind-code"
	if command == "unbind" {
		path = "/api/users/unbind/check/code"
		body = map[string]any{"type": options.typeName, "validCode": options.code}
		if options.authConfigID != "" {
			body["authConfigId"] = options.authConfigID
		}
		operation = "profile-unbind"
	}
	payload, runErr := a.vpnBusinessJSON(ctx, path, "POST", body, false)
	if runErr != nil {
		return nil, runErr
	}
	return map[string]any{"ok": true, "submitted": true, "confirmed": true, "evidence": "VPN API code 200", "vpn": true, "operation": operation, "api": path, "type": options.typeName, "data": payload["data"]}, nil
}

func (a NativeSite) runVPNOTP(ctx context.Context, args []string) (map[string]any, *siteError) {
	if len(args) == 0 || args[0] != "generate" {
		return nil, &siteError{Code: "invalid_argument", Message: "vpn otp 必须提供 generate"}
	}
	username, yes := "", false
	for index := 1; index < len(args); index++ {
		arg, value, inline := splitInline(args[index])
		switch arg {
		case "--json":
		case "--username", "--user":
			var err *siteError
			username, err = vpnOptionValue(args, &index, arg, value, inline)
			if err != nil {
				return nil, err
			}
		case "--yes":
			if inline {
				return nil, &siteError{Code: "invalid_argument", Message: "布尔参数不接受 =VALUE"}
			}
			yes = true
		default:
			return nil, &siteError{Code: "invalid_argument", Message: "vpn otp generate 参数无效: " + arg}
		}
	}
	if strings.TrimSpace(username) == "" {
		return nil, &siteError{Code: "invalid_argument", Message: "生成 OTP 密钥必须提供 --username"}
	}
	if !yes {
		return nil, &siteError{Code: "confirmation_required", Message: "生成 OTP 密钥会改变远端认证状态，请加 --yes"}
	}
	path := "/api/client/totp/login/generateSecretQRContent"
	payload, runErr := a.vpnBusinessJSON(ctx, path, "POST", map[string]any{"userName": username}, false)
	if runErr != nil {
		return nil, runErr
	}
	return map[string]any{"ok": true, "submitted": true, "confirmed": true, "evidence": "VPN totp/login/generateSecretQRContent code 200", "vpn": true, "operation": "otp-generate", "api": path, "username": username, "otp": payload["data"], "data": payload["data"]}, nil
}

func (a NativeSite) runVPNSafeSpace(ctx context.Context, args []string) (map[string]any, *siteError) {
	if len(args) > 0 && args[0] == "status" {
		args = args[1:]
	}
	if err := onlyJSONArgs(args); err != nil {
		return nil, err
	}
	path := "/api/users/safeSpace/getService"
	payload, runErr := a.vpnBusinessJSON(ctx, path, "GET", nil, true)
	if runErr != nil {
		return nil, runErr
	}
	return vpnBusinessResult("safe-space", path, payload["data"], map[string]any{"space_service": payload["data"]}), nil
}

type vpnUSBRequestOptions struct {
	name, identification, reason string
	yes                          bool
}

func parseVPNUSBRequestOptions(args []string) (vpnUSBRequestOptions, *siteError) {
	options := vpnUSBRequestOptions{}
	for index := 0; index < len(args); index++ {
		arg, value, inline := splitInline(args[index])
		switch arg {
		case "--json":
		case "--name", "--device-name", "--identification", "--device-id", "--reason":
			var err *siteError
			value, err = vpnOptionValue(args, &index, arg, value, inline)
			if err != nil {
				return vpnUSBRequestOptions{}, err
			}
			switch arg {
			case "--name", "--device-name":
				options.name = value
			case "--identification", "--device-id":
				options.identification = value
			default:
				options.reason = value
			}
		case "--yes":
			if inline {
				return vpnUSBRequestOptions{}, &siteError{Code: "invalid_argument", Message: "布尔参数不接受 =VALUE"}
			}
			options.yes = true
		default:
			return vpnUSBRequestOptions{}, &siteError{Code: "invalid_argument", Message: "vpn usb request 参数无效: " + arg}
		}
	}
	if strings.TrimSpace(options.name) == "" || strings.TrimSpace(options.identification) == "" || strings.TrimSpace(options.reason) == "" {
		return vpnUSBRequestOptions{}, &siteError{Code: "invalid_argument", Message: "USB 申请必须提供 --name、--identification 和 --reason"}
	}
	if len([]rune(options.name)) > 100 || len([]rune(options.reason)) > 100 || len([]rune(options.identification)) > 255 {
		return vpnUSBRequestOptions{}, &siteError{Code: "invalid_argument", Message: "USB 申请字段长度超出网页表单限制"}
	}
	if !options.yes {
		return vpnUSBRequestOptions{}, &siteError{Code: "confirmation_required", Message: "提交 USB 存储设备申请会改变远端状态，请加 --yes"}
	}
	return options, nil
}

func (a NativeSite) runVPNUSB(ctx context.Context, args []string) (map[string]any, *siteError) {
	if len(args) == 0 || args[0] != "request" {
		return nil, &siteError{Code: "invalid_argument", Message: "vpn usb 必须提供 request"}
	}
	options, parseErr := parseVPNUSBRequestOptions(args[1:])
	if parseErr != nil {
		return nil, parseErr
	}
	path := "/api/users/approve/center/addApply"
	body := map[string]any{
		"extendParamMap": map[string]any{
			"type":           "PERIPHERAL_APPLY_PAGE",
			"name":           options.name,
			"identification": options.identification,
		},
		"applyReason":         options.reason,
		"applyType":           "Peripherals",
		"applyTimeZoneOffset": vpnTimezoneOffset(),
	}
	payload, runErr := a.vpnBusinessJSON(ctx, path, "POST", body, false)
	if runErr != nil {
		return nil, runErr
	}
	return map[string]any{"ok": true, "submitted": true, "confirmed": true, "evidence": "VPN approve/center/addApply code 200", "vpn": true, "operation": "usb-request", "api": path, "name": options.name, "identification": options.identification, "reason": options.reason, "apply_type": "Peripherals", "data": payload["data"]}, nil
}
