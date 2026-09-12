package adapter

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const libraryPersonalService = "library-personal"

func (a NativeSite) executeLibraryCenter(ctx context.Context, args []string, cookie string) (map[string]any, *siteError) {
	switch args[0] {
	case "resources":
		return a.libraryCenterResources(ctx, cookie)
	case "profile":
		return a.libraryCenterProfile(ctx, cookie)
	case "credit-history":
		return a.libraryCenterCreditHistory(ctx, args[1:], cookie)
	case "availability":
		return a.libraryCenterAvailability(ctx, args[1:], cookie)
	case "reservations":
		return a.libraryCenterReservations(ctx, cookie)
	case "server-time":
		return a.libraryCenterServerTime(ctx, cookie)
	case "update-contact":
		return a.libraryCenterUpdateContact(ctx, args[1:], cookie)
	case "reserve":
		return a.libraryCenterReserve(ctx, args[1:], cookie)
	case "cancel":
		return a.libraryCenterCancel(ctx, args[1:], cookie)
	default:
		return nil, &siteError{Code: "invalid_argument", Message: "library-center 只支持 status、catalog、login、logout、resources、profile、credit-history、availability、reservations、server-time、update-contact、reserve、cancel"}
	}
}

func (a NativeSite) libraryCenterPage(ctx context.Context, cookie string) (*pageNode, *siteError) {
	result, requestErr := a.businessGet(ctx, libraryPersonalService, "/clientweb/xcus/ic2/Default.aspx", []pair{{"page", "center"}}, businessRequestOptions{cookieFile: cookie, require: true})
	if requestErr != nil {
		return nil, requestErr
	}
	body, bodyErr := libraryRemoteBody(result)
	if bodyErr != nil {
		return nil, bodyErr
	}
	document, parseErr := parsePage(body)
	if parseErr != nil {
		return nil, &siteError{Code: "parse_error", Message: "图书馆个人中心资源目录解析失败: " + parseErr.Error()}
	}
	return document, nil
}

func (a NativeSite) libraryCenterResources(ctx context.Context, cookie string) (map[string]any, *siteError) {
	document, pageErr := a.libraryCenterPage(ctx, cookie)
	if pageErr != nil {
		return nil, pageErr
	}
	resources := libraryCenterResourceRows(document)
	return map[string]any{
		"ok": true, "submitted": false, "confirmed": true,
		"evidence": "图书馆个人中心服务端资源目录返回",
		"service":  libraryPersonalService, "operation": "resources",
		"data": resources, "total": len(resources),
	}, nil
}

func (a NativeSite) libraryCenterProfile(ctx context.Context, cookie string) (map[string]any, *siteError) {
	result, requestErr := a.businessGet(ctx, libraryPersonalService, "/ClientWeb/pro/ajax/login.aspx", []pair{{"act", "init_acc"}}, businessRequestOptions{cookieFile: cookie, require: true})
	if requestErr != nil {
		return nil, requestErr
	}
	payload, payloadErr := libraryCenterPayload(result)
	if payloadErr != nil {
		return nil, payloadErr
	}
	if fmt.Sprint(payload["ret"]) != "1" {
		return nil, libraryCenterAPIError(payload, "图书馆账户资料查询失败")
	}
	data, ok := payload["data"].(map[string]any)
	if !ok {
		return nil, &siteError{Code: "parse_error", Message: "图书馆账户资料响应结构无效"}
	}
	return map[string]any{
		"ok": true, "submitted": false, "confirmed": true,
		"evidence": "图书馆 login.aspx init_acc 返回",
		"service":  libraryPersonalService, "operation": "profile",
		"data": data, "raw": redactSiteJSON(payload),
	}, nil
}

func (a NativeSite) libraryCenterCreditHistory(ctx context.Context, args []string, cookie string) (map[string]any, *siteError) {
	days, daysErr := businessInt(args, "--days", 90)
	if daysErr != nil {
		return nil, daysErr
	}
	status, _, statusErr := businessValue(args, "--status")
	if statusErr != nil {
		return nil, statusErr
	}
	statusValue, statusErr := libraryCenterHistoryStatus(status)
	if statusErr != nil {
		return nil, statusErr
	}
	result, requestErr := a.businessGet(ctx, libraryPersonalService, "/ClientWeb/pro/ajax/center.aspx", []pair{
		{"act", "get_History_resv"}, {"strat", strconv.Itoa(days)}, {"StatFlag", statusValue},
	}, businessRequestOptions{cookieFile: cookie, require: true})
	if requestErr != nil {
		return nil, requestErr
	}
	payload, payloadErr := libraryCenterPayload(result)
	if payloadErr != nil {
		return nil, payloadErr
	}
	if fmt.Sprint(payload["ret"]) != "1" {
		return nil, libraryCenterAPIError(payload, "图书馆信用记录查询失败")
	}
	html := strings.TrimSpace(fmt.Sprint(payload["msg"]))
	rows := libraryCenterHistoryRows(html)
	return map[string]any{
		"ok": true, "submitted": false, "confirmed": true,
		"evidence": "图书馆 center.aspx get_History_resv 返回",
		"service":  libraryPersonalService, "operation": "credit-history",
		"days": days, "status": statusValue, "data": rows, "total": len(rows),
		"raw": redactSiteJSON(payload),
	}, nil
}

func libraryCenterHistoryStatus(value string) (string, *siteError) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "new", "new-reservation", "新预约记录":
		return "New", nil
	case "over", "history", "historical", "历史", "历史记录":
		return "OVER", nil
	case "default", "违约", "违约记录":
		return "DEFAULT", nil
	case "cancel", "cancelled", "canceled", "取消", "取消记录":
		return "CANCEL", nil
	default:
		return "", &siteError{Code: "invalid_argument", Message: "--status 只能是 new、history、default 或 cancel"}
	}
}

func libraryCenterHistoryRows(source string) []map[string]any {
	// The endpoint returns a <tbody> fragment, which HTML parsers may discard
	// when it is parsed outside a table.
	document, err := parsePage("<table>" + source + "</table>")
	if err != nil {
		return []map[string]any{}
	}
	rows := make([]map[string]any, 0)
	for _, row := range document.findAll("tr") {
		cells := make([]string, 0)
		for _, cell := range row.findAll("td") {
			cells = append(cells, strings.TrimSpace(pageDisplayText(cell)))
		}
		if len(cells) < 6 || strings.Contains(strings.Join(cells, " "), "没有数据") {
			continue
		}
		rows = append(rows, map[string]any{
			"date": cells[0], "location": cells[1], "type": cells[2],
			"state": cells[3], "deduction": cells[4], "penalty": cells[5], "cells": cells,
		})
	}
	return rows
}

func libraryCenterResourceRows(document *pageNode) []map[string]any {
	rows := make([]map[string]any, 0)
	seen := make(map[string]bool)
	for _, node := range document.findAll("li") {
		rawURL := strings.TrimSpace(node.attr("url"))
		if rawURL == "" {
			continue
		}
		target, err := url.Parse(rawURL)
		if err != nil {
			continue
		}
		query := target.Query()
		name := strings.TrimSpace(query.Get("name"))
		if name == "" {
			name = strings.TrimSpace(query.Get("roomName"))
		}
		if name == "" {
			name = strings.TrimSpace(pageDisplayText(node))
		}
		classKind := query.Get("classKind")
		if node.attr("it") == "devcls" {
			id := query.Get("id")
			if id == "" || name == "" {
				continue
			}
			key := "space:" + id
			if seen[key] {
				continue
			}
			seen[key] = true
			rows = append(rows, map[string]any{
				"type": "space", "name": name, "kind_id": id, "class_kind": classKind,
			})
			continue
		}
		if !strings.HasPrefix(node.attr("it"), "lab_") {
			continue
		}
		roomID := query.Get("roomId")
		if roomID == "" || name == "" {
			continue
		}
		key := "seat:" + roomID
		if seen[key] {
			continue
		}
		seen[key] = true
		rows = append(rows, map[string]any{
			"type": "seat", "name": name, "room_id": roomID,
			"lab_id": strings.TrimPrefix(node.attr("it"), "lab_"), "class_kind": classKind,
		})
	}
	return rows
}

func (a NativeSite) libraryCenterAvailability(ctx context.Context, args []string, cookie string) (map[string]any, *siteError) {
	resourceName, requiredErr := businessRequired(args, "--resource", "availability 必须提供 --resource；先运行 resources 查看可用资源")
	if requiredErr != nil {
		return nil, requiredErr
	}
	document, pageErr := a.libraryCenterPage(ctx, cookie)
	if pageErr != nil {
		return nil, pageErr
	}
	resources := libraryCenterResourceRows(document)
	resource, resourceErr := libraryCenterSelectResource(resources, resourceName, flagValue(args, "--room"))
	if resourceErr != nil {
		return nil, resourceErr
	}
	date, dateErr := libraryCenterDate(args)
	if dateErr != nil {
		return nil, dateErr
	}
	start, end, timeErr := libraryCenterTimeRange(args)
	if timeErr != nil {
		return nil, timeErr
	}

	params := []pair{}
	if resource["type"] == "space" {
		params = []pair{
			{"dev_order", ""}, {"kind_order", ""}, {"classkind", fmt.Sprint(resource["class_kind"])},
			{"display", "cld"}, {"md", "d"}, {"kind_id", fmt.Sprint(resource["kind_id"])},
			{"purpose", ""}, {"selectOpenAty", ""}, {"cld_name", "default"},
			{"date", date}, {"act", "get_rsv_sta"},
		}
	} else {
		params = []pair{
			{"byType", "devcls"}, {"classkind", fmt.Sprint(resource["class_kind"])},
			{"display", "fp"}, {"md", "d"}, {"room_id", fmt.Sprint(resource["room_id"])},
			{"purpose", ""}, {"selectOpenAty", ""}, {"cld_name", "default"}, {"date", date},
		}
		if start != "" {
			params = append(params, pair{"fr_start", start}, pair{"fr_end", end})
		}
		params = append(params, pair{"act", "get_rsv_sta"})
	}
	result, requestErr := a.businessGet(ctx, libraryPersonalService, "/ClientWeb/pro/ajax/device.aspx", params, businessRequestOptions{cookieFile: cookie, require: true})
	if requestErr != nil {
		return nil, requestErr
	}
	payload, payloadErr := libraryCenterPayload(result)
	if payloadErr != nil {
		return nil, payloadErr
	}
	if ret := fmt.Sprint(payload["ret"]); ret != "1" {
		return nil, libraryCenterAPIError(payload, "图书馆空间状态查询失败")
	}
	rows := libraryCenterAvailabilityRows(payload["data"])
	return map[string]any{
		"ok": true, "submitted": false, "confirmed": true,
		"evidence": "图书馆 device.aspx get_rsv_sta 返回",
		"service":  libraryPersonalService, "operation": "availability",
		"resource": resource, "date": date, "from": start, "to": end,
		"data": rows, "total": len(rows), "raw": redactSiteJSON(payload),
	}, nil
}

func libraryCenterSelectResource(resources []map[string]any, value, room string) (map[string]any, *siteError) {
	wanted := strings.ToLower(strings.TrimSpace(value))
	if wanted == "seat" || wanted == "seats" || wanted == "座位" || wanted == "seat-room" {
		if strings.TrimSpace(room) == "" {
			return nil, &siteError{Code: "invalid_argument", Message: "座位查询必须提供 --room"}
		}
		wanted = strings.ToLower(strings.TrimSpace(room))
	}
	aliases := map[string]string{
		"training-room": "培训室", "training": "培训室",
		"meeting-room": "会议室", "meeting": "会议室",
		"discussion-room": "研讨间", "discussion": "研讨间",
	}
	if alias, ok := aliases[wanted]; ok {
		wanted = strings.ToLower(alias)
	}
	for _, resource := range resources {
		if strings.ToLower(strings.TrimSpace(fmt.Sprint(resource["name"]))) == wanted {
			return resource, nil
		}
	}
	available := make([]string, 0, len(resources))
	for _, resource := range resources {
		available = append(available, fmt.Sprint(resource["name"]))
	}
	return nil, &siteError{Code: "invalid_argument", Message: "--resource 未找到图书馆资源", Details: map[string]any{"resource": value, "available": available}}
}

func libraryCenterDate(args []string) (string, *siteError) {
	value, _, valueErr := businessValue(args, "--date")
	if valueErr != nil {
		return "", valueErr
	}
	if value == "" || strings.EqualFold(strings.TrimSpace(value), "today") {
		return time.Now().Format("2006-01-02"), nil
	}
	value = strings.TrimSpace(value)
	if _, err := time.Parse("2006-01-02", value); err != nil {
		return "", &siteError{Code: "invalid_argument", Message: "--date 必须是 YYYY-MM-DD 或 today"}
	}
	return value, nil
}

func libraryCenterTimeRange(args []string) (string, string, *siteError) {
	start, _, startErr := businessValue(args, "--from")
	if startErr != nil {
		return "", "", startErr
	}
	end, _, endErr := businessValue(args, "--to")
	if endErr != nil {
		return "", "", endErr
	}
	if (start == "") != (end == "") {
		return "", "", &siteError{Code: "invalid_argument", Message: "--from 和 --to 必须同时提供"}
	}
	if start == "" {
		return "", "", nil
	}
	for flag, value := range map[string]string{"--from": start, "--to": end} {
		if _, err := time.Parse("15:04", strings.TrimSpace(value)); err != nil {
			return "", "", &siteError{Code: "invalid_argument", Message: flag + " 必须是 HH:MM"}
		}
	}
	if start >= end {
		return "", "", &siteError{Code: "invalid_argument", Message: "--from 必须早于 --to"}
	}
	return strings.TrimSpace(start), strings.TrimSpace(end), nil
}

func (a NativeSite) libraryCenterReservations(ctx context.Context, cookie string) (map[string]any, *siteError) {
	return a.libraryCenterReservationRequest(ctx, "get_my_resv", "reservations", cookie)
}

func (a NativeSite) libraryCenterServerTime(ctx context.Context, cookie string) (map[string]any, *siteError) {
	return a.libraryCenterReservationRequest(ctx, "get_my_servertime", "server-time", cookie)
}

func (a NativeSite) libraryCenterUpdateContact(ctx context.Context, args []string, cookie string) (map[string]any, *siteError) {
	if !businessBool(args, "--yes") {
		return nil, &siteError{Code: "confirmation_required", Message: "更新图书馆联系方式会修改远端数据，请加 --yes"}
	}
	phone, phoneFound, phoneErr := businessValue(args, "--phone")
	if phoneErr != nil {
		return nil, phoneErr
	}
	email, emailFound, emailErr := businessValue(args, "--email")
	if emailErr != nil {
		return nil, emailErr
	}
	notify, notifyFound, notifyErr := businessValue(args, "--notify")
	if notifyErr != nil {
		return nil, notifyErr
	}
	if !phoneFound && !emailFound && !notifyFound {
		return nil, &siteError{Code: "invalid_argument", Message: "update-contact 至少需要 --phone、--email 或 --notify"}
	}
	profile, profileErr := a.libraryCenterProfile(ctx, cookie)
	if profileErr != nil {
		return nil, profileErr
	}
	current, ok := profile["data"].(map[string]any)
	if !ok {
		return nil, &siteError{Code: "parse_error", Message: "图书馆账户资料结构无效"}
	}
	if !phoneFound {
		phone = fmt.Sprint(current["phone"])
	}
	if !emailFound {
		email = fmt.Sprint(current["email"])
	}
	if strings.TrimSpace(email) == "" {
		return nil, &siteError{Code: "invalid_argument", Message: "图书馆账户邮箱不能为空，请提供 --email"}
	}
	params := []pair{{"act", "update_contact"}, {"phone", phone}, {"email", email}}
	var notifyValue bool
	if notifyFound {
		var parseOK bool
		notifyValue, parseOK = libraryCenterBool(notify)
		if !parseOK {
			return nil, &siteError{Code: "invalid_argument", Message: "--notify 只能是 true、false、on 或 off"}
		}
		params = append(params, pair{"note_alert", strconv.FormatBool(notifyValue)})
	}
	result, requestErr := businessRequest(ctx, libraryPersonalService, "GET", "/ClientWeb/pro/ajax/account.aspx", params, nil, nil, businessRequestOptions{cookieFile: cookie, require: true}, false, true)
	if requestErr != nil {
		return nil, requestErr
	}
	payload, payloadErr := libraryCenterPayload(result)
	if payloadErr != nil {
		return nil, payloadErr
	}
	if fmt.Sprint(payload["ret"]) != "1" {
		return nil, libraryCenterAPIError(payload, "图书馆联系方式更新失败")
	}
	after, afterErr := a.libraryCenterProfile(ctx, cookie)
	if afterErr != nil {
		return nil, &siteError{Code: "mutation_unverified", Message: "联系方式更新返回成功，但账户资料回读失败", Details: map[string]any{"submitted": true, "confirmed": false, "evidence": "update_contact ret=1", "cause": afterErr.Code}}
	}
	afterData, ok := after["data"].(map[string]any)
	if !ok || fmt.Sprint(afterData["phone"]) != phone || fmt.Sprint(afterData["email"]) != email || (notifyFound && libraryCenterBoolValue(afterData["receive"]) != notifyValue) {
		return nil, &siteError{Code: "mutation_unverified", Message: "联系方式更新返回成功，但账户资料回读不匹配", Details: map[string]any{"submitted": true, "confirmed": false, "evidence": "update_contact ret=1 but init_acc readback mismatched"}}
	}
	return map[string]any{
		"ok": true, "submitted": true, "confirmed": true,
		"evidence": "update_contact ret=1 且 init_acc 回读匹配",
		"service":  libraryPersonalService, "operation": "update-contact",
		"data": afterData, "response": redactSiteJSON(payload),
	}, nil
}

func libraryCenterBool(value string) (bool, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "true", "1", "on", "yes":
		return true, true
	case "false", "0", "off", "no":
		return false, true
	default:
		return false, false
	}
}

func libraryCenterBoolValue(value any) bool {
	if typed, ok := value.(bool); ok {
		return typed
	}
	parsed, _ := libraryCenterBool(fmt.Sprint(value))
	return parsed
}

func (a NativeSite) libraryCenterReserve(ctx context.Context, args []string, cookie string) (map[string]any, *siteError) {
	itemName, requiredErr := businessRequired(args, "--item", "reserve 必须提供 --item；先运行 availability 查看资源项")
	if requiredErr != nil {
		return nil, requiredErr
	}
	if !businessBool(args, "--yes") {
		return nil, &siteError{Code: "confirmation_required", Message: "图书馆预约会修改远端数据，请加 --yes"}
	}
	start, end, timeErr := libraryCenterTimeRange(args)
	if timeErr != nil {
		return nil, timeErr
	}
	if start == "" {
		return nil, &siteError{Code: "invalid_argument", Message: "reserve 必须提供 --from 和 --to"}
	}
	date, dateErr := libraryCenterDate(args)
	if dateErr != nil {
		return nil, dateErr
	}
	availability, availabilityErr := a.libraryCenterAvailability(ctx, args, cookie)
	if availabilityErr != nil {
		return nil, availabilityErr
	}
	rows, rowsOK := availability["data"].([]map[string]any)
	if !rowsOK {
		return nil, &siteError{Code: "parse_error", Message: "图书馆预约资源状态结构无效"}
	}
	var selected map[string]any
	for _, row := range rows {
		if strings.EqualFold(strings.TrimSpace(fmt.Sprint(row["name"])), strings.TrimSpace(itemName)) {
			selected = row
			break
		}
	}
	if selected == nil {
		return nil, &siteError{Code: "invalid_argument", Message: "--item 未找到可预约资源项", Details: map[string]any{"item": itemName}}
	}
	resource := availability["resource"].(map[string]any)
	theme, _, themeErr := businessValue(args, "--theme")
	if themeErr != nil {
		return nil, themeErr
	}
	memo, _, memoErr := businessValue(args, "--memo")
	if memoErr != nil {
		return nil, memoErr
	}
	fields := []pair{
		{"dialogid", ""}, {"dev_id", fmt.Sprint(selected["resource_id"])},
		{"lab_id", libraryCenterLabID(selected)}, {"kind_id", fmt.Sprint(selected["kind_id"])},
		{"room_id", fmt.Sprint(selected["room_id"])}, {"type", "dev"}, {"prop", ""},
		{"test_id", ""}, {"term", ""}, {"number", ""},
		{"classkind", fmt.Sprint(resource["class_kind"])},
		{"start", date + " " + start}, {"end", date + " " + end},
		{"test_name", theme}, {"up_file", ""}, {"memo", memo},
	}
	members, membersErr := businessValues(args, "--member")
	if membersErr != nil {
		return nil, membersErr
	}
	groupID, groupFound, groupErr := businessValue(args, "--group-id")
	if groupErr != nil {
		return nil, groupErr
	}
	if groupFound && strings.TrimSpace(groupID) == "" {
		return nil, &siteError{Code: "invalid_argument", Message: "--group-id 不能为空"}
	}
	minUsers := libraryCenterNumber(selected["min_users"])
	if minUsers > 0 && groupID == "" && len(members) < minUsers {
		return nil, &siteError{Code: "invalid_argument", Message: "该资源需要满足最少参与人数，请提供足够的 --member 或 --group-id", Details: map[string]any{"min_users": minUsers}}
	}
	if groupFound {
		fields = append(fields, pair{"group_id", strings.TrimSpace(groupID)})
	}
	if len(members) > 0 {
		fields = append(fields, pair{"mb_list", "$" + strings.Join(members, ",")})
	}
	result, requestErr := businessRequest(ctx, libraryPersonalService, "GET", "/ClientWeb/pro/ajax/reserve.aspx", append(fields, pair{"act", "set_resv"}), nil, nil, businessRequestOptions{cookieFile: cookie, require: true}, false, true)
	if requestErr != nil {
		return nil, requestErr
	}
	payload, payloadErr := libraryCenterPayload(result)
	if payloadErr != nil {
		return nil, payloadErr
	}
	if fmt.Sprint(payload["ret"]) != "1" {
		return nil, libraryCenterAPIError(payload, "图书馆预约提交失败")
	}
	verified, verifyErr := a.libraryCenterReservationReadback(ctx, payload, selected, date, start, end, cookie)
	if verifyErr != nil {
		return nil, verifyErr
	}
	if !verified {
		return nil, &siteError{Code: "mutation_unverified", Message: "预约请求返回成功，但个人预约列表未确认新预约", Details: map[string]any{"submitted": true, "confirmed": false, "evidence": "set_resv ret=1 but get_my_resv did not match"}}
	}
	return map[string]any{
		"ok": true, "submitted": true, "confirmed": true,
		"evidence": "set_resv ret=1 且 get_my_resv 回读匹配",
		"service":  libraryPersonalService, "operation": "reserve", "item": selected,
		"date": date, "from": start, "to": end, "response": redactSiteJSON(payload),
	}, nil
}

func (a NativeSite) libraryCenterCancel(ctx context.Context, args []string, cookie string) (map[string]any, *siteError) {
	id, requiredErr := businessRequired(args, "--id", "cancel 必须提供 --id")
	if requiredErr != nil {
		return nil, requiredErr
	}
	if !businessBool(args, "--yes") {
		return nil, &siteError{Code: "confirmation_required", Message: "取消图书馆预约会修改远端数据，请加 --yes"}
	}
	result, requestErr := businessRequest(ctx, libraryPersonalService, "GET", "/ClientWeb/pro/ajax/reserve.aspx", []pair{{"id", strings.TrimSpace(id)}, {"act", "del_resv"}}, nil, nil, businessRequestOptions{cookieFile: cookie, require: true}, false, true)
	if requestErr != nil {
		return nil, requestErr
	}
	payload, payloadErr := libraryCenterPayload(result)
	if payloadErr != nil {
		return nil, payloadErr
	}
	if fmt.Sprint(payload["ret"]) != "1" {
		return nil, libraryCenterAPIError(payload, "图书馆预约取消失败")
	}
	reservations, reservationsErr := a.libraryCenterReservationRequest(ctx, "get_my_resv", "reservations", cookie)
	if reservationsErr != nil {
		return nil, &siteError{Code: "mutation_unverified", Message: "取消请求返回成功，但无法回读个人预约列表", Details: map[string]any{"submitted": true, "confirmed": false, "evidence": "del_resv ret=1", "cause": reservationsErr.Code}}
	}
	if libraryCenterReservationContains(reservations["data"], id) {
		return nil, &siteError{Code: "mutation_unverified", Message: "取消请求返回成功，但个人预约列表仍包含该预约", Details: map[string]any{"submitted": true, "confirmed": false, "evidence": "del_resv ret=1 but get_my_resv still contains id"}}
	}
	return map[string]any{
		"ok": true, "submitted": true, "confirmed": true,
		"evidence": "del_resv ret=1 且 get_my_resv 确认已移除",
		"service":  libraryPersonalService, "operation": "cancel", "id": id,
		"response": redactSiteJSON(payload),
	}, nil
}

func libraryCenterLabID(row map[string]any) string {
	id := strings.TrimSpace(fmt.Sprint(row["id"]))
	if parts := strings.SplitN(id, "_", 2); len(parts) == 2 {
		return parts[1]
	}
	return ""
}

func libraryCenterNumber(value any) int {
	switch number := value.(type) {
	case float64:
		return int(number)
	case int:
		return number
	default:
		parsed, _ := strconv.Atoi(strings.TrimSpace(fmt.Sprint(value)))
		return parsed
	}
}

func (a NativeSite) libraryCenterReservationRequest(ctx context.Context, action, operation, cookie string) (map[string]any, *siteError) {
	result, requestErr := a.businessGet(ctx, libraryPersonalService, "/ClientWeb/pro/ajax/reserve.aspx", []pair{{"act", action}}, businessRequestOptions{cookieFile: cookie, require: true})
	if requestErr != nil {
		return nil, requestErr
	}
	payload, payloadErr := libraryCenterPayload(result)
	if payloadErr != nil {
		return nil, payloadErr
	}
	if ret := fmt.Sprint(payload["ret"]); ret != "1" {
		return nil, libraryCenterAPIError(payload, "图书馆预约查询失败")
	}
	return map[string]any{
		"ok": true, "submitted": false, "confirmed": true,
		"evidence": "图书馆 reserve.aspx " + action + " 返回",
		"service":  libraryPersonalService, "operation": operation,
		"data": payload["data"], "raw": redactSiteJSON(payload),
	}, nil
}

func (a NativeSite) libraryCenterReservationReadback(ctx context.Context, submitted map[string]any, selected map[string]any, date, start, end, cookie string) (bool, *siteError) {
	reservations, reservationsErr := a.libraryCenterReservationRequest(ctx, "get_my_resv", "reservations", cookie)
	if reservationsErr != nil {
		return false, &siteError{Code: "mutation_unverified", Message: "预约请求返回成功，但无法回读个人预约列表", Details: map[string]any{"submitted": true, "confirmed": false, "evidence": "set_resv ret=1", "cause": reservationsErr.Code}}
	}
	if id := libraryCenterFindID(submitted["data"]); id != "" && libraryCenterReservationContains(reservations["data"], id) {
		return true, nil
	}
	return libraryCenterReservationMatches(reservations["data"], fmt.Sprint(selected["resource_id"]), date+" "+start, date+" "+end), nil
}

func libraryCenterReservationMatches(value any, resourceID, start, end string) bool {
	matched := false
	var visit func(any)
	visit = func(value any) {
		if matched {
			return
		}
		switch item := value.(type) {
		case []any:
			for _, child := range item {
				visit(child)
			}
		case map[string]any:
			device := libraryCenterText(item, "dev_id", "devId", "device_id", "deviceId")
			actualStart := libraryCenterText(item, "start", "start_time", "startTime")
			actualEnd := libraryCenterText(item, "end", "end_time", "endTime")
			if device == resourceID && strings.Contains(actualStart, start) && strings.Contains(actualEnd, end) {
				matched = true
				return
			}
			for _, child := range item {
				visit(child)
			}
		}
	}
	visit(value)
	return matched
}

func libraryCenterFindID(value any) string {
	found := ""
	var visit func(any)
	visit = func(value any) {
		if found != "" {
			return
		}
		switch item := value.(type) {
		case []any:
			for _, child := range item {
				visit(child)
			}
		case map[string]any:
			found = libraryCenterText(item, "id", "resv_id", "resvId", "ResvID")
			if found != "" {
				return
			}
			for _, child := range item {
				visit(child)
			}
		}
	}
	visit(value)
	return found
}

func libraryCenterReservationContains(value any, id string) bool {
	needle := strings.TrimSpace(id)
	if needle == "" {
		return false
	}
	found := false
	var visit func(any)
	visit = func(value any) {
		if found {
			return
		}
		switch item := value.(type) {
		case []any:
			for _, child := range item {
				visit(child)
			}
		case map[string]any:
			if libraryCenterText(item, "id", "resv_id", "resvId", "ResvID") == needle {
				found = true
				return
			}
			for _, child := range item {
				visit(child)
			}
		}
	}
	visit(value)
	return found
}

func libraryCenterPayload(result map[string]any) (map[string]any, *siteError) {
	payload, payloadErr := businessJSONMap(result)
	if payloadErr != nil {
		return nil, payloadErr
	}
	if _, ok := payload["ret"]; !ok {
		return nil, &siteError{Code: "parse_error", Message: "图书馆个人中心业务响应缺少 ret"}
	}
	return payload, nil
}

func libraryCenterAPIError(payload map[string]any, message string) *siteError {
	apiMessage := strings.TrimSpace(fmt.Sprint(payload["msg"]))
	if strings.Contains(apiMessage, "登录") || strings.Contains(apiMessage, "登陆") {
		return &siteError{Code: "login_required", Message: apiMessage, Details: map[string]any{"evidence": "library API ret != 1"}}
	}
	if apiMessage == "" || apiMessage == "<nil>" {
		apiMessage = message
	}
	return &siteError{Code: "business_rejected", Message: apiMessage, Details: map[string]any{"evidence": "library API ret != 1", "response": redactSiteJSON(payload)}}
}

func libraryCenterAvailabilityRows(value any) []map[string]any {
	rows, ok := value.([]any)
	if !ok {
		return []map[string]any{}
	}
	items := make([]map[string]any, 0, len(rows))
	for _, value := range rows {
		row, ok := value.(map[string]any)
		if !ok {
			continue
		}
		item := map[string]any{
			"id":           libraryCenterText(row, "id", "devId"),
			"name":         libraryCenterText(row, "name", "title", "devName", "roomName"),
			"resource_id":  libraryCenterText(row, "devId"),
			"kind_id":      libraryCenterText(row, "kindId"),
			"room_id":      libraryCenterText(row, "roomId"),
			"location":     libraryCenterText(row, "labName"),
			"min_users":    row["minUser"],
			"max_users":    row["maxUser"],
			"state":        row["state"],
			"reservations": row["ts"],
			"operations":   row["ops"],
		}
		if occupy, ok := row["occupy"]; ok {
			item["occupied"] = occupy
		}
		if freeTime, ok := row["freeTime"]; ok {
			item["free_minutes"] = freeTime
		}
		if open, ok := row["open"]; ok {
			item["open_hours"] = open
		}
		items = append(items, item)
	}
	return items
}

func libraryCenterText(row map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := row[key]; ok && value != nil {
			text := strings.TrimSpace(fmt.Sprint(value))
			if text != "" && text != "<nil>" {
				return text
			}
		}
	}
	return ""
}
