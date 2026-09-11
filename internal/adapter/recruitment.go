package adapter

import (
	"context"
	cryptorand "crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/emmansun/gmsm/sm2"
)

const (
	recruitmentServiceName = "recruitment"
	recruitmentEndpoint    = "/ajax/ajaxService"
	recruitmentPublicKey   = "04ad821e0a07bc83627decc9d6fabc3665f8aa568156380b3475e364506271c179542604be51d7e4fe9d56212c23ad7ffce3a24edee632545977d20f1d6a48923b"
)

func (a NativeSite) executeRecruitment(ctx context.Context, args []string) (map[string]any, *siteError) {
	if len(args) == 0 || args[0] == "catalog" {
		return businessCatalogNames(recruitmentServiceName), nil
	}
	cookie, _, valueErr := businessValue(args[1:], "--cookie-file")
	if valueErr != nil {
		return nil, valueErr
	}
	switch args[0] {
	case "home":
		return a.recruitmentHome(ctx, cookie)
	case "notices":
		return a.recruitmentNotices(ctx, args[1:], cookie)
	case "filters", "organizations":
		return a.recruitmentFilters(ctx, args[1:], cookie)
	case "positions", "list":
		return a.recruitmentPositions(ctx, args[1:], cookie)
	case "position", "detail":
		return a.recruitmentPosition(ctx, args[1:], cookie)
	default:
		return nil, &siteError{Code: "invalid_argument", Message: "recruitment 只支持 home、notices、filters、positions、position、catalog"}
	}
}

func (a NativeSite) recruitmentHome(ctx context.Context, cookie string) (map[string]any, *siteError) {
	config, requestErr := a.recruitmentCall(ctx, cookie, "ZP1000000009", map[string]any{})
	if requestErr != nil {
		return nil, requestErr
	}
	channels, requestErr := a.recruitmentCall(ctx, cookie, "ZP1000000010", map[string]any{"type": "list", "state": "1"})
	if requestErr != nil {
		return nil, requestErr
	}
	notices, requestErr := a.recruitmentCall(ctx, cookie, "ZP1000000002", map[string]any{
		"type": "board_list", "hire_channel": "0", "pageNum": 1, "pageSize": 8,
	})
	if requestErr != nil {
		return nil, requestErr
	}
	configData := recruitmentReturnData(config)
	channelData := recruitmentReturnData(channels)
	noticeData := recruitmentReturnData(notices)
	result := recruitmentResult("home", "公开招聘首页配置、频道和公告接口均返回 succeed=true")
	result["data"] = map[string]any{
		"config":   configData,
		"channels": recruitmentChannels(channelData["channels"]),
		"notices":  recruitmentNotices(noticeData["list"]),
	}
	result["notice_total_pages"] = recruitmentNumber(noticeData["pageTotal"])
	result["raw"] = map[string]any{"config": config, "channels": channels, "notices": notices}
	return result, nil
}

func (a NativeSite) recruitmentNotices(ctx context.Context, args []string, cookie string) (map[string]any, *siteError) {
	channel, channelErr := recruitmentChannel(args, "0", false)
	if channelErr != nil {
		return nil, channelErr
	}
	page, pageErr := businessInt(args, "--page", 1)
	if pageErr != nil {
		return nil, pageErr
	}
	pageSize, pageSizeErr := businessInt(args, "--page-size", 8)
	if pageSizeErr != nil {
		return nil, pageSizeErr
	}
	envelope, requestErr := a.recruitmentCall(ctx, cookie, "ZP1000000002", map[string]any{
		"type": "board_list", "hire_channel": channel, "pageNum": page, "pageSize": pageSize,
	})
	if requestErr != nil {
		return nil, requestErr
	}
	data := recruitmentReturnData(envelope)
	result := recruitmentResult("notices", "公开公告接口返回 succeed=true")
	result["data"] = recruitmentNotices(data["list"])
	result["total_pages"] = recruitmentNumber(data["pageTotal"])
	result["page"], result["page_size"], result["channel"] = page, pageSize, channel
	result["raw"] = envelope
	return result, nil
}

func (a NativeSite) recruitmentFilters(ctx context.Context, args []string, cookie string) (map[string]any, *siteError) {
	channel, channelErr := recruitmentChannel(args, "", true)
	if channelErr != nil {
		return nil, channelErr
	}
	envelope, requestErr := a.recruitmentCall(ctx, cookie, "ZP1000000001", map[string]any{
		"type": "organization", "hire_channel": channel,
	})
	if requestErr != nil {
		return nil, requestErr
	}
	data := recruitmentReturnData(envelope)
	result := recruitmentResult("filters", "公开组织接口返回招聘单位和部门字典")
	result["channel"] = channel
	result["data"] = map[string]any{
		"units":       recruitmentOrganizations(data["unitList"]),
		"departments": recruitmentOrganizations(data["departmentList"]),
	}
	result["raw"] = envelope
	return result, nil
}

func (a NativeSite) recruitmentPositions(ctx context.Context, args []string, cookie string) (map[string]any, *siteError) {
	channel, channelErr := recruitmentChannel(args, "", true)
	if channelErr != nil {
		return nil, channelErr
	}
	page, pageErr := businessInt(args, "--page", 1)
	if pageErr != nil {
		return nil, pageErr
	}
	pageSize, pageSizeErr := businessInt(args, "--page-size", 20)
	if pageSizeErr != nil {
		return nil, pageSizeErr
	}
	keyword, _, keywordErr := businessValue(args, "--keyword")
	if keywordErr != nil {
		return nil, keywordErr
	}
	unit, unitFound, unitErr := businessValue(args, "--unit")
	if unitErr != nil {
		return nil, unitErr
	}
	queryItems := make([]any, 0, 2)
	if unitFound {
		unit = strings.TrimSpace(unit)
		if unit == "" {
			return nil, &siteError{Code: "invalid_argument", Message: "--unit 不能为空"}
		}
		unitCode, resolveErr := a.recruitmentUnitCode(ctx, cookie, channel, unit)
		if resolveErr != nil {
			return nil, resolveErr
		}
		queryItems = append(queryItems, map[string]any{"itemid": "z0321", "value": unitCode})
	}
	if strings.TrimSpace(keyword) != "" {
		queryItems = append(queryItems, map[string]any{"itemid": "z0351", "value": strings.TrimSpace(keyword)})
	}
	if len(queryItems) == 0 {
		queryItems = append(queryItems, map[string]any{"itemid": "z0351", "value": ""})
	}
	envelope, requestErr := a.recruitmentCall(ctx, cookie, "ZP1000000001", map[string]any{
		"type": "list", "hire_channel": channel, "queryitem": queryItems,
		"newHire": "true", "pageNum": page, "pageSize": pageSize,
	})
	if requestErr != nil {
		return nil, requestErr
	}
	data := recruitmentReturnData(envelope)
	items := make([]map[string]any, 0)
	for _, row := range recruitmentMaps(data["position_data"]) {
		items = append(items, recruitmentPosition(row))
	}
	result := recruitmentResult("positions", "公开岗位列表接口返回 succeed=true")
	result["channel"], result["page"], result["page_size"] = channel, page, pageSize
	result["data"] = items
	result["total"] = recruitmentNumber(data["posTotal"])
	result["columns"] = data["columninfo"]
	result["filters"] = map[string]any{"unit": unit, "keyword": strings.TrimSpace(keyword)}
	result["raw"] = envelope
	return result, nil
}

func (a NativeSite) recruitmentPosition(ctx context.Context, args []string, cookie string) (map[string]any, *siteError) {
	id, idErr := businessRequired(args, "--id", "position 必须提供 --id")
	if idErr != nil {
		return nil, idErr
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, &siteError{Code: "invalid_argument", Message: "position 的 --id 不能为空"}
	}
	channel, channelErr := recruitmentChannel(args, "", false)
	if channelErr != nil {
		return nil, channelErr
	}
	opaqueID := id
	if !strings.HasPrefix(strings.ToLower(id), "hjev-") {
		if channel == "" {
			return nil, &siteError{Code: "invalid_argument", Message: "使用岗位代码查询详情时必须提供 --channel"}
		}
		opaqueID, idErr = a.recruitmentResolvePosition(ctx, cookie, channel, id)
		if idErr != nil {
			return nil, idErr
		}
	}
	envelope, requestErr := a.recruitmentCall(ctx, cookie, "ZP1000000001", map[string]any{"Z0301": opaqueID})
	if requestErr != nil {
		return nil, requestErr
	}
	data := recruitmentReturnData(envelope)
	rows := recruitmentMaps(data["position_data"])
	if len(rows) == 0 {
		return nil, &siteError{Code: "not_found", Message: "未找到该招聘岗位", Details: map[string]any{"id": id}}
	}
	position := recruitmentPositionDetail(rows[0], opaqueID, data)
	result := recruitmentResult("position", "公开岗位详情接口返回 succeed=true")
	result["channel"] = recruitmentText(data["hireChannel"])
	if result["channel"] == "" {
		result["channel"] = channel
	}
	result["data"], result["position"], result["raw"] = position, position, envelope
	return result, nil
}

func (a NativeSite) recruitmentResolvePosition(ctx context.Context, cookie, channel, id string) (string, *siteError) {
	envelope, requestErr := a.recruitmentCall(ctx, cookie, "ZP1000000001", map[string]any{
		"type": "list", "hire_channel": channel,
		"queryitem": []any{map[string]any{"itemid": "z0351", "value": id}},
		"newHire":   "true", "pageNum": 1, "pageSize": 20,
	})
	if requestErr != nil {
		return "", requestErr
	}
	for _, row := range recruitmentMaps(recruitmentReturnData(envelope)["position_data"]) {
		position := recruitmentPosition(row)
		if id == position["id"] || id == position["code"] || id == position["title"] {
			return recruitmentText(position["id"]), nil
		}
	}
	return "", &siteError{Code: "not_found", Message: "未找到该招聘岗位", Details: map[string]any{"id": id, "channel": channel}}
}

func (a NativeSite) recruitmentUnitCode(ctx context.Context, cookie, channel, name string) (string, *siteError) {
	envelope, requestErr := a.recruitmentCall(ctx, cookie, "ZP1000000001", map[string]any{
		"type": "organization", "hire_channel": channel,
	})
	if requestErr != nil {
		return "", requestErr
	}
	for _, unit := range recruitmentOrganizations(recruitmentReturnData(envelope)["unitList"]) {
		if name == unit["name"] || name == unit["id"] {
			return recruitmentText(unit["id"]), nil
		}
	}
	return "", &siteError{Code: "not_found", Message: "未找到招聘单位: " + name, Details: map[string]any{"channel": channel, "unit": name}}
}

func (a NativeSite) recruitmentCall(ctx context.Context, cookie, functionID string, params map[string]any) (map[string]any, *siteError) {
	payload := make(map[string]any, len(params)+1)
	for key, value := range params {
		payload[key] = value
	}
	payload["functionId"] = functionID
	plain, marshalErr := json.Marshal(payload)
	if marshalErr != nil {
		return nil, &siteError{Code: "protocol_error", Message: "无法构造人才招聘请求: " + marshalErr.Error()}
	}
	encrypted, encryptErr := recruitmentEncrypt(plain)
	if encryptErr != nil {
		return nil, &siteError{Code: "protocol_error", Message: "无法加密人才招聘请求: " + encryptErr.Error()}
	}
	result, requestErr := a.execute(ctx, siteRequest{
		Service: recruitmentServiceName, Method: "POST", Path: recruitmentEndpoint,
		Data:       []pair{{"__xml", encrypted}, {"__type", "extTrans"}},
		CookieFile: cookie, RawJSON: true, ReadOnly: true, Yes: true,
	})
	if requestErr != nil {
		return nil, requestErr
	}
	value, ok := businessData(result)
	if !ok {
		return nil, &siteError{Code: "parse_error", Message: "人才招聘接口响应不是 JSON"}
	}
	envelope, ok := value.(map[string]any)
	if !ok {
		return nil, &siteError{Code: "parse_error", Message: "人才招聘接口响应结构无效"}
	}
	if !recruitmentSucceeded(envelope) {
		message := recruitmentText(envelope["message"])
		if message == "" {
			message = recruitmentText(envelope["return_msg"])
		}
		if message == "" {
			message = "远端返回失败"
		}
		return nil, &siteError{Code: "business_rejected", Message: "人才招聘接口 " + functionID + " 失败: " + message, Details: map[string]any{
			"api": functionID, "message": message,
		}}
	}
	return envelope, nil
}

func recruitmentEncrypt(value []byte) (string, error) {
	key, err := hex.DecodeString(recruitmentPublicKey)
	if err != nil {
		return "", err
	}
	publicKey, err := sm2.NewPublicKey(key)
	if err != nil {
		return "", err
	}
	ciphertext, err := sm2.Encrypt(cryptorand.Reader, publicKey, value, nil)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(ciphertext), nil
}

func recruitmentSucceeded(envelope map[string]any) bool {
	if value, ok := envelope["succeed"].(bool); ok {
		return value
	}
	if value := strings.ToLower(recruitmentText(envelope["succeed"])); value != "" {
		return value == "true" || value == "1" || value == "success"
	}
	if value := strings.ToLower(recruitmentText(envelope["return_code"])); value != "" {
		return value == "success" || value == "0"
	}
	return false
}

func recruitmentChannel(args []string, fallback string, required bool) (string, *siteError) {
	value, found, valueErr := businessValue(args, "--channel")
	if valueErr != nil {
		return "", valueErr
	}
	if !found {
		value = fallback
	}
	value = strings.TrimSpace(value)
	if required && value == "" {
		return "", &siteError{Code: "invalid_argument", Message: "招聘查询必须提供 --channel"}
	}
	if value == "" {
		return "", nil
	}
	aliases := map[string]string{
		"home": "0", "首页": "0", "招聘首页": "0", "0": "0",
		"faculty": "08", "teacher": "08", "专任教师": "08", "专任教师自主招聘": "08", "08": "08",
		"postdoc": "05", "博士后": "05", "博士后招聘": "05", "05": "05",
		"mental-health": "06", "psychological": "06", "心理健康教育教师": "06", "2026心理健康教育教师公开招聘": "06", "06": "06",
	}
	if channel, ok := aliases[strings.ToLower(value)]; ok {
		return channel, nil
	}
	return value, nil
}

func recruitmentReturnData(envelope map[string]any) map[string]any {
	data, _ := envelope["return_data"].(map[string]any)
	return data
}

func recruitmentChannels(value any) []map[string]any {
	result := make([]map[string]any, 0)
	for _, row := range recruitmentMaps(value) {
		result = append(result, map[string]any{
			"id":      recruitmentFirstText(row, "id", "ID"),
			"name":    recruitmentFirstText(row, "name", "NAME"),
			"channel": recruitmentFirstText(row, "hireChannel", "hire_channel", "channel"),
			"link":    recruitmentFirstText(row, "link", "url"),
			"params":  row["params"],
			"raw":     row,
		})
	}
	return result
}

func recruitmentNotices(value any) []map[string]any {
	result := make([]map[string]any, 0)
	for _, row := range recruitmentMaps(value) {
		result = append(result, map[string]any{
			"id":           recruitmentFirstText(row, "id", "ID"),
			"title":        recruitmentFirstText(row, "title", "TITLE"),
			"created_at":   recruitmentFirstText(row, "createtime", "createTime", "created_at"),
			"start_date":   recruitmentFirstText(row, "start_date", "startDate"),
			"days":         recruitmentNumber(row["days"]),
			"downloadable": recruitmentBool(row["down"]),
			"content_html": recruitmentFirstText(row, "content", "content_html"),
			"raw":          row,
		})
	}
	return result
}

func recruitmentOrganizations(value any) []map[string]any {
	result := make([]map[string]any, 0)
	for _, row := range recruitmentMaps(value) {
		result = append(result, map[string]any{
			"id":     recruitmentFirstText(row, "value", "id", "ID"),
			"name":   recruitmentFirstText(row, "name", "label", "NAME"),
			"hidden": recruitmentBool(row["hidden"]),
			"raw":    row,
		})
	}
	return result
}

func recruitmentPosition(row map[string]any) map[string]any {
	return map[string]any{
		"id":            recruitmentFirstText(row, "z0301", "Z0301"),
		"code":          recruitmentFirstText(row, "z03a2", "Z03A2"),
		"title":         recruitmentFirstText(row, "z0351", "Z0351"),
		"unit_id":       recruitmentFirstText(row, "z0321Id", "Z0321Id", "z0321Code", "Z0321Code"),
		"unit":          recruitmentFirstText(row, "z0321Name", "Z0321Name", "z0321", "Z0321"),
		"department_id": recruitmentFirstText(row, "z0325Id", "Z0325Id", "z0325Code", "Z0325Code"),
		"department":    recruitmentFirstText(row, "z0325Name", "Z0325Name", "z0325", "Z0325"),
		"category":      recruitmentFirstText(row, "z0383", "Z0383"),
		"plan":          recruitmentNumber(recruitmentFirst(row, "z0315", "Z0315")),
		"application":   recruitmentFirstText(row, "ypljl", "YPLJL"),
		"state":         recruitmentFirstText(row, "state", "STATE"),
		"is_new":        recruitmentBool(row["isNewPos"]),
		"applied":       recruitmentBool(row["isApplyedPos"]),
		"collected":     recruitmentBool(row["isCollectPos"]),
		"raw":           row,
	}
}

func recruitmentPositionDetail(row map[string]any, id string, data map[string]any) map[string]any {
	position := recruitmentPosition(row)
	position["id"] = id
	position["channel"] = recruitmentText(data["hireChannel"])
	position["batch"] = recruitmentFirstText(row, "Z0101", "z0101")
	position["total"] = recruitmentNumber(recruitmentFirst(row, "total", "TOTAL"))
	return position
}

func recruitmentMaps(value any) []map[string]any {
	if typed, ok := value.([]map[string]any); ok {
		return typed
	}
	items, ok := value.([]any)
	if !ok {
		return nil
	}
	result := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if row, ok := item.(map[string]any); ok {
			result = append(result, row)
		}
	}
	return result
}

func recruitmentFirst(row map[string]any, keys ...string) any {
	for _, key := range keys {
		if value, ok := row[key]; ok && recruitmentText(value) != "" {
			return value
		}
	}
	return nil
}

func recruitmentFirstText(row map[string]any, keys ...string) string {
	return recruitmentText(recruitmentFirst(row, keys...))
}

func recruitmentText(value any) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func recruitmentNumber(value any) any {
	text := recruitmentText(value)
	if text == "" {
		return value
	}
	var number int
	if _, err := fmt.Sscan(text, &number); err == nil {
		return number
	}
	return value
}

func recruitmentBool(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	default:
		value := strings.ToLower(recruitmentText(typed))
		return value == "true" || value == "1" || value == "yes"
	}
}

func recruitmentResult(operation, evidence string) map[string]any {
	return map[string]any{
		"ok": true, "submitted": false, "confirmed": true, "evidence": evidence,
		"service": recruitmentServiceName, "operation": operation,
		"api": map[string]any{
			"endpoint": recruitmentEndpoint, "method": "POST",
			"transport": "application/x-www-form-urlencoded with SM2 C1C3C2 payload",
			"response":  "JSON",
		},
	}
}
