package adapter

import (
	"context"
	"crypto/cipher"
	"crypto/des"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

const (
	equipmentServiceName = "equipment"
	equipmentEndpoint    = "/WebService/wxPublicInterface.asmx/IoControl"
	equipmentDESKey      = "31113001"
)

func (a NativeSite) executeEquipment(ctx context.Context, args []string) (map[string]any, *siteError) {
	if len(args) == 0 || args[0] == "catalog" {
		return businessCatalogNames("equipment"), nil
	}
	if args[0] == "status" || args[0] == "login" || args[0] == "logout" {
		return a.executeSSOServiceCommand(ctx, args, equipmentServiceName, "实验室综合管理系统")
	}
	cookie, _, valueErr := businessValue(args[1:], "--cookie-file")
	if valueErr != nil {
		return nil, valueErr
	}
	switch args[0] {
	case "list", "instruments":
		page, pageErr := businessInt(args[1:], "--page", 1)
		if pageErr != nil {
			return nil, pageErr
		}
		pageSize, pageSizeErr := businessInt(args[1:], "--page-size", 10)
		if pageSizeErr != nil {
			return nil, pageSizeErr
		}
		plaintext, filters, payloadErr := equipmentListPayload(args[1:], page, pageSize)
		if payloadErr != nil {
			return nil, payloadErr
		}
		token, tokenErr := a.equipmentToken(ctx, cookie)
		if tokenErr != nil {
			return nil, tokenErr
		}
		value, requestErr := a.equipmentCall(ctx, cookie, "GetApparatusList_Nei", "YQKF", plaintext, token)
		if requestErr != nil {
			return nil, requestErr
		}
		payload, payloadErr := equipmentMap(value, "仪器列表")
		if payloadErr != nil {
			return nil, payloadErr
		}
		if code := equipmentText(payload["code"]); code != "0" {
			return nil, equipmentRejected("GetApparatusList_Nei", code, equipmentText(payload["msg"]))
		}
		rows := equipmentMaps(payload["data"])
		items := make([]map[string]any, 0, len(rows))
		for _, row := range rows {
			items = append(items, equipmentInstrument(row))
		}
		result := equipmentResult("list", "GetApparatusList_Nei returned code=0")
		result["data"], result["raw"] = items, payload
		result["total"] = equipmentNumber(payload["count"])
		result["page"], result["page_size"], result["filters"] = page, pageSize, filters
		return result, nil
	case "filters":
		token, tokenErr := a.equipmentToken(ctx, cookie)
		if tokenErr != nil {
			return nil, tokenErr
		}
		filters, filtersErr := a.equipmentCall(ctx, cookie, "GetIndexDevBm", "PublicInterface", "{}", token)
		if filtersErr != nil {
			return nil, filtersErr
		}
		columns, columnsErr := a.equipmentCall(ctx, cookie, "GetDevListCols", "YQKF", "{}", token)
		if columnsErr != nil {
			return nil, columnsErr
		}
		result := equipmentResult("filters", "GetIndexDevBm and GetDevListCols returned structured dictionaries")
		result["data"] = map[string]any{"filters": filters, "columns": columns}
		result["raw"] = result["data"]
		return result, nil
	case "detail":
		id, requiredErr := businessRequired(args[1:], "--id", "equipment detail 必须提供 --id")
		if requiredErr != nil {
			return nil, requiredErr
		}
		id = strings.TrimSpace(id)
		if id == "" {
			return nil, &siteError{Code: "invalid_argument", Message: "equipment detail 的 --id 不能为空"}
		}
		token, tokenErr := a.equipmentToken(ctx, cookie)
		if tokenErr != nil {
			return nil, tokenErr
		}
		plaintext := fmt.Sprintf("{ZCBH:'%s',URL:'', kzJson : '{ \"lang\":\"zh-CN\"}'}", equipmentJSString(id))
		value, requestErr := a.equipmentCall(ctx, cookie, "GetApparatusOne", "YQKF", plaintext, token)
		if requestErr != nil {
			return nil, requestErr
		}
		rows := equipmentMaps(value)
		if len(rows) == 0 {
			return nil, &siteError{Code: "not_found", Message: "未找到该仪器", Details: map[string]any{"id": id}}
		}
		result := equipmentResult("detail", "GetApparatusOne returned the instrument record")
		result["data"], result["instrument"], result["raw"] = value, equipmentInstrument(rows[0]), value
		return result, nil
	case "availability", "calendar":
		id, requiredErr := businessRequired(args[1:], "--id", "equipment availability 必须提供 --id")
		if requiredErr != nil {
			return nil, requiredErr
		}
		id = strings.TrimSpace(id)
		if id == "" {
			return nil, &siteError{Code: "invalid_argument", Message: "equipment availability 的 --id 不能为空"}
		}
		date, dateErr := equipmentDate(args[1:])
		if dateErr != nil {
			return nil, dateErr
		}
		token, tokenErr := a.equipmentToken(ctx, cookie)
		if tokenErr != nil {
			return nil, tokenErr
		}
		plaintext := fmt.Sprintf("{YQBH:'%s',URL:'',Date:'%s', kzJson : '{ \"lang\":\"zh-CN\",\"cma\":\"0\"}'}", equipmentJSString(id), equipmentJSString(date))
		value, requestErr := a.equipmentCall(ctx, cookie, "GetDeviceCalendar", "YQKF", plaintext, token)
		if requestErr != nil {
			return nil, requestErr
		}
		payload, payloadErr := equipmentMap(value, "预约日历")
		if payloadErr != nil {
			return nil, payloadErr
		}
		if !equipmentCalendarSucceeded(payload["message"]) {
			return nil, equipmentRejected("GetDeviceCalendar", equipmentText(payload["message"]), "预约日历读取失败")
		}
		result := equipmentResult("availability", "GetDeviceCalendar returned the instrument availability calendar")
		result["id"], result["date"], result["data"], result["raw"] = id, date, payload["data"], payload
		return result, nil
	case "profile":
		return a.equipmentProfile(ctx, cookie)
	case "reservations":
		return a.equipmentReservations(ctx, args[1:], cookie)
	case "favorites":
		return a.equipmentFavorites(ctx, args[1:], cookie)
	case "cancel":
		return a.equipmentCancel(ctx, args[1:], cookie)
	case "favorite":
		if !businessBool(args[1:], "--yes") {
			return nil, &siteError{Code: "confirmation_required", Message: "修改仪器收藏状态必须加 --yes"}
		}
		id, requiredErr := businessRequired(args[1:], "--id", "equipment favorite 必须提供 --id")
		if requiredErr != nil {
			return nil, requiredErr
		}
		id = strings.TrimSpace(id)
		if id == "" {
			return nil, &siteError{Code: "invalid_argument", Message: "equipment favorite 的 --id 不能为空"}
		}
		token, tokenErr := a.equipmentToken(ctx, cookie)
		if tokenErr != nil {
			return nil, tokenErr
		}
		plaintext := fmt.Sprintf("{YQBH:'%s',kzJson:'{\"lang\":\"zh-CN\"}'}", equipmentJSString(id))
		value, requestErr := a.equipmentCall(ctx, cookie, "AddDevsCollect", "YQKF", plaintext, token)
		if requestErr != nil {
			return nil, requestErr
		}
		payload, payloadErr := equipmentMap(value, "收藏")
		if payloadErr != nil {
			return nil, payloadErr
		}
		flag := equipmentText(payload["flag"])
		message := equipmentText(payload["msg"])
		switch flag {
		case "0":
			favorite := !strings.Contains(message, "取消")
			return map[string]any{
				"ok": true, "submitted": true, "confirmed": true,
				"evidence": "AddDevsCollect flag=0", "service": equipmentServiceName,
				"operation": "favorite", "id": id, "favorite": favorite,
				"message": message, "api": "AddDevsCollect", "raw": payload,
			}, nil
		case "2":
			return nil, &siteError{Code: "login_required", Message: "仪器收藏需要先登录", Details: map[string]any{
				"submitted": false, "confirmed": false, "evidence": "AddDevsCollect flag=2", "id": id,
			}}
		default:
			return nil, &siteError{Code: "mutation_rejected", Message: "仪器收藏被服务端拒绝: " + firstNonEmpty(message, "远端返回失败"), Details: map[string]any{
				"submitted": true, "confirmed": false, "evidence": "AddDevsCollect flag=" + flag, "id": id, "raw": payload,
			}}
		}
	default:
		return nil, &siteError{Code: "invalid_argument", Message: "equipment 只支持 status、login、logout、list、filters、detail、availability、calendar、profile、reservations、favorites、cancel、favorite、catalog"}
	}
}

func (a NativeSite) equipmentProfile(ctx context.Context, cookie string) (map[string]any, *siteError) {
	token, tokenErr := a.equipmentToken(ctx, cookie)
	if tokenErr != nil {
		return nil, tokenErr
	}
	value, requestErr := a.equipmentCall(ctx, cookie, "GetUserInfo", "PublicInterface", "{}", token)
	if requestErr != nil {
		return nil, requestErr
	}
	payload, payloadErr := equipmentMap(value, "个人资料")
	if payloadErr != nil {
		return nil, payloadErr
	}
	flag, message, ok := equipmentReturnStatus(payload["message"])
	if !ok {
		return nil, equipmentRejected("GetUserInfo", flag, message)
	}
	rows := equipmentMaps(payload["data"])
	if len(rows) == 0 {
		return nil, &siteError{Code: "not_found", Message: "实验室系统未返回个人资料"}
	}
	result := equipmentResult("profile", "GetUserInfo returned the authenticated user profile")
	result["profile"], result["data"], result["raw"] = equipmentUserProfile(rows[0]), rows, payload
	return result, nil
}

func (a NativeSite) equipmentReservations(ctx context.Context, args []string, cookie string) (map[string]any, *siteError) {
	page, pageErr := businessInt(args, "--page", 1)
	if pageErr != nil {
		return nil, pageErr
	}
	pageSize, pageSizeErr := businessInt(args, "--page-size", 6)
	if pageSizeErr != nil {
		return nil, pageSizeErr
	}
	statusValue, statusFound, statusErr := businessValue(args, "--status")
	if statusErr != nil {
		return nil, statusErr
	}
	status := "all"
	statusCode := ""
	if statusFound {
		statusValue = strings.ToLower(strings.TrimSpace(statusValue))
		codes := map[string]string{
			"pending": "0", "approved": "6", "awaiting-sample": "1", "testing": "2",
			"awaiting-confirmation": "5", "awaiting-payment": "3", "completed": "4",
		}
		if statusValue != "" && statusValue != "all" {
			var known bool
			statusCode, known = codes[statusValue]
			if !known {
				return nil, &siteError{Code: "invalid_argument", Message: "--status 只能是 all、pending、approved、awaiting-sample、testing、awaiting-confirmation、awaiting-payment 或 completed"}
			}
			status = statusValue
		}
	}
	inner := `{"data":[{"1":"1"`
	if statusCode != "" {
		inner += `,"crzt":"` + statusCode + `"`
	}
	inner += `}]}`
	plaintext := fmt.Sprintf("{StrJson:'%s',page:'%d',limit:'%d',URL:'',lang:'zh-CN'}", equipmentJSString(inner), page, pageSize)
	token, tokenErr := a.equipmentToken(ctx, cookie)
	if tokenErr != nil {
		return nil, tokenErr
	}
	value, requestErr := a.equipmentCall(ctx, cookie, "GetDevsPreList", "Center", plaintext, token)
	if requestErr != nil {
		return nil, requestErr
	}
	payload, payloadErr := equipmentMap(value, "预约列表")
	if payloadErr != nil {
		return nil, payloadErr
	}
	rows := equipmentMaps(payload["data"])
	items := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		items = append(items, equipmentReservation(row))
	}
	result := equipmentResult("reservations", "GetDevsPreList returned reservation records")
	result["data"], result["raw"] = items, payload
	result["total"], result["page"], result["page_size"], result["status"] = equipmentNumber(payload["count"]), page, pageSize, status
	return result, nil
}

func (a NativeSite) equipmentFavorites(ctx context.Context, args []string, cookie string) (map[string]any, *siteError) {
	page, pageErr := businessInt(args, "--page", 1)
	if pageErr != nil {
		return nil, pageErr
	}
	pageSize, pageSizeErr := businessInt(args, "--page-size", 20)
	if pageSizeErr != nil {
		return nil, pageSizeErr
	}
	plaintext := fmt.Sprintf("{PageIndex:'%d',PageSize:'%d',lang:'zh-CN'}", page, pageSize)
	token, tokenErr := a.equipmentToken(ctx, cookie)
	if tokenErr != nil {
		return nil, tokenErr
	}
	value, requestErr := a.equipmentCall(ctx, cookie, "GetDevsCollect", "Center", plaintext, token)
	if requestErr != nil {
		return nil, requestErr
	}
	payload, payloadErr := equipmentMap(value, "收藏列表")
	if payloadErr != nil {
		return nil, payloadErr
	}
	rows := equipmentMaps(payload["rows"])
	items := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		items = append(items, equipmentInstrument(row))
	}
	result := equipmentResult("favorites", "GetDevsCollect returned favorite instruments")
	result["data"], result["raw"] = items, payload
	result["total"], result["page"], result["page_size"] = equipmentNumber(payload["total"]), page, pageSize
	return result, nil
}

func (a NativeSite) equipmentCancel(ctx context.Context, args []string, cookie string) (map[string]any, *siteError) {
	if !businessBool(args, "--yes") {
		return nil, &siteError{Code: "confirmation_required", Message: "取消仪器预约必须加 --yes"}
	}
	id, requiredErr := businessRequired(args, "--id", "equipment cancel 必须提供预约 --id")
	if requiredErr != nil {
		return nil, requiredErr
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, &siteError{Code: "invalid_argument", Message: "equipment cancel 的 --id 不能为空"}
	}
	token, tokenErr := a.equipmentToken(ctx, cookie)
	if tokenErr != nil {
		return nil, tokenErr
	}
	plaintext := fmt.Sprintf("{yyid:'%s',URL:''}", equipmentJSString(id))
	value, requestErr := a.equipmentCall(ctx, cookie, "CancelPre", "Center", plaintext, token)
	if requestErr != nil {
		return nil, requestErr
	}
	payload, payloadErr := equipmentMap(value, "取消预约")
	if payloadErr != nil {
		return nil, payloadErr
	}
	flag, message, ok := equipmentReturnStatus(payload["message"])
	if !ok {
		return nil, &siteError{Code: "mutation_rejected", Message: "取消仪器预约被服务端拒绝: " + firstNonEmpty(message, "远端返回失败"), Details: map[string]any{
			"submitted": true, "confirmed": false, "evidence": "CancelPre flag=" + flag, "id": id, "raw": payload,
		}}
	}
	return map[string]any{
		"ok": true, "submitted": true, "confirmed": true, "evidence": "CancelPre ReturnFlag=1",
		"service": equipmentServiceName, "operation": "cancel", "id": id, "message": message,
		"api": "CancelPre", "raw": payload,
	}, nil
}

func equipmentDate(args []string) (string, *siteError) {
	value, found, valueErr := businessValue(args, "--date")
	if valueErr != nil {
		return "", valueErr
	}
	if !found || strings.EqualFold(strings.TrimSpace(value), "today") {
		return time.Now().Format("2006-01-02"), nil
	}
	value = strings.TrimSpace(value)
	if _, parseErr := time.Parse("2006-01-02", value); parseErr != nil {
		return "", &siteError{Code: "invalid_argument", Message: "--date 必须是 YYYY-MM-DD 或 today"}
	}
	return value, nil
}

func equipmentCalendarSucceeded(value any) bool {
	_, _, ok := equipmentReturnStatus(value)
	return ok
}

func equipmentReturnStatus(value any) (flag, message string, ok bool) {
	items := equipmentMaps(value)
	if len(items) == 0 {
		return "", "", false
	}
	flag = equipmentText(items[0]["ReturnFlag"])
	message = equipmentText(items[0]["ReturnMsg"])
	return flag, message, flag == "1"
}

func equipmentListPayload(args []string, page, pageSize int) (string, map[string]any, *siteError) {
	inner := `{"data":[{"1":"1"`
	filters := map[string]any{}
	for _, item := range []struct {
		flag, key, name string
	}{
		{"--keyword", "txtkey", "keyword"},
		{"--department-id", "SSBMID", "department_id"},
		{"--lab-id", "sssysid", "lab_id"},
		{"--category-id", "catagoryid", "category_id"},
		{"--discipline", "xkly", "discipline"},
		{"--year", "grrq", "year"},
		{"--year-to", "grrqa", "year_to"},
	} {
		value, found, valueErr := businessValue(args, item.flag)
		if valueErr != nil {
			return "", nil, valueErr
		}
		if !found {
			continue
		}
		value = strings.TrimSpace(value)
		if value == "" {
			return "", nil, &siteError{Code: "invalid_argument", Message: item.flag + " 不能为空"}
		}
		encoded, _ := json.Marshal(value)
		inner += `,"` + item.key + `":` + string(encoded)
		filters[item.name] = value
	}
	if businessBool(args, "--all") {
		inner += `,"isall":"1"`
		filters["all"] = true
	}
	inner += `}]}`
	kzJSON := `{"BM":[],"SYS":[],"GB":[],"DJ":[],"GZRQ":[],"FL":[],"XKLY":[],"CNAME":[]}`
	plaintext := fmt.Sprintf("{strJson:'%s',page:'%d',limit:'%d',strOrder:'',gjztype:'0', kzJson : '%s'}", equipmentJSString(inner), page, pageSize, equipmentJSString(kzJSON))
	return plaintext, filters, nil
}

func (a NativeSite) equipmentToken(ctx context.Context, cookie string) (string, *siteError) {
	value, requestErr := a.equipmentCall(ctx, cookie, "GetUIToken", "PublicInterface", "{}", "")
	if requestErr != nil {
		return "", requestErr
	}
	payload, payloadErr := equipmentMap(value, "UIToken")
	if payloadErr != nil {
		return "", payloadErr
	}
	token := strings.TrimSpace(equipmentText(payload["token"]))
	if token == "" {
		return "", &siteError{Code: "protocol_error", Message: "实验室仪器接口未返回 UIToken"}
	}
	return token, nil
}

func (a NativeSite) equipmentCall(ctx context.Context, cookie, action, component, plaintext, token string) (any, *siteError) {
	authPayload, marshalErr := json.Marshal(struct {
		Function  string `json:"f"`
		Component string `json:"c"`
	}{Function: action, Component: component})
	if marshalErr != nil {
		return nil, &siteError{Code: "protocol_error", Message: "无法构造实验室仪器接口授权: " + marshalErr.Error()}
	}
	authorization, encryptErr := equipmentEncrypt(string(authPayload))
	if encryptErr != nil {
		return nil, &siteError{Code: "protocol_error", Message: "无法加密实验室仪器接口授权: " + encryptErr.Error()}
	}
	desJSON, encryptErr := equipmentEncrypt(plaintext)
	if encryptErr != nil {
		return nil, &siteError{Code: "protocol_error", Message: "无法加密实验室仪器请求: " + encryptErr.Error()}
	}
	headers := []pair{{"wxAuthorization", authorization}}
	if token != "" {
		headers = append(headers, pair{"UIToken", token})
	}
	result, requestErr := a.execute(ctx, siteRequest{
		Service: equipmentServiceName, Method: "POST", Path: equipmentEndpoint,
		JSON: map[string]string{"DESJson": desJSON}, HasJSON: true, Headers: headers,
		CookieFile: cookie, RawJSON: true, ReadOnly: true, Yes: true,
	})
	if requestErr != nil {
		return nil, requestErr
	}
	return equipmentDecodeResponse(result)
}

func equipmentDecodeResponse(result map[string]any) (any, *siteError) {
	value, ok := businessData(result)
	if !ok {
		return nil, &siteError{Code: "parse_error", Message: "实验室仪器接口响应不是 JSON"}
	}
	envelope, ok := value.(map[string]any)
	if !ok {
		return value, nil
	}
	ciphertext, exists := envelope["d"]
	if !exists {
		return value, nil
	}
	encoded, ok := ciphertext.(string)
	if !ok || strings.TrimSpace(encoded) == "" {
		return nil, &siteError{Code: "parse_error", Message: "实验室仪器接口响应缺少加密数据"}
	}
	plaintext, decryptErr := equipmentDecrypt(encoded)
	if decryptErr != nil {
		return nil, &siteError{Code: "parse_error", Message: "无法解密实验室仪器接口响应: " + decryptErr.Error()}
	}
	var decoded any
	if unmarshalErr := json.Unmarshal([]byte(plaintext), &decoded); unmarshalErr != nil {
		return nil, &siteError{Code: "parse_error", Message: "实验室仪器接口明文不是 JSON: " + unmarshalErr.Error()}
	}
	return decoded, nil
}

func equipmentEncrypt(value string) (string, error) {
	block, err := des.NewCipher([]byte(equipmentDESKey))
	if err != nil {
		return "", err
	}
	padded := equipmentPKCS7Pad([]byte(value), block.BlockSize())
	ciphertext := make([]byte, len(padded))
	cipher.NewCBCEncrypter(block, []byte(equipmentDESKey)).CryptBlocks(ciphertext, padded)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

func equipmentDecrypt(encoded string) (string, error) {
	block, err := des.NewCipher([]byte(equipmentDESKey))
	if err != nil {
		return "", err
	}
	ciphertext, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", err
	}
	if len(ciphertext) == 0 || len(ciphertext)%block.BlockSize() != 0 {
		return "", fmt.Errorf("密文长度无效")
	}
	plaintext := make([]byte, len(ciphertext))
	cipher.NewCBCDecrypter(block, []byte(equipmentDESKey)).CryptBlocks(plaintext, ciphertext)
	plaintext, err = equipmentPKCS7Unpad(plaintext, block.BlockSize())
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}

func equipmentPKCS7Pad(value []byte, size int) []byte {
	padding := size - len(value)%size
	result := make([]byte, len(value)+padding)
	copy(result, value)
	for index := len(value); index < len(result); index++ {
		result[index] = byte(padding)
	}
	return result
}

func equipmentPKCS7Unpad(value []byte, size int) ([]byte, error) {
	if len(value) == 0 || len(value)%size != 0 {
		return nil, fmt.Errorf("明文长度无效")
	}
	padding := int(value[len(value)-1])
	if padding < 1 || padding > size || padding > len(value) {
		return nil, fmt.Errorf("PKCS#7 填充无效")
	}
	for _, item := range value[len(value)-padding:] {
		if int(item) != padding {
			return nil, fmt.Errorf("PKCS#7 填充无效")
		}
	}
	return value[:len(value)-padding], nil
}

func equipmentJSString(value string) string {
	return strings.NewReplacer("\\", "\\\\", "'", "\\'", "\r", "\\r", "\n", "\\n").Replace(value)
}

func equipmentMap(value any, label string) (map[string]any, *siteError) {
	result, ok := value.(map[string]any)
	if !ok {
		return nil, &siteError{Code: "parse_error", Message: "实验室仪器" + label + "响应结构无效"}
	}
	return result, nil
}

func equipmentMaps(value any) []map[string]any {
	items, ok := value.([]any)
	if !ok {
		if typed, ok := value.([]map[string]any); ok {
			return typed
		}
		if text, ok := value.(string); ok {
			var decoded any
			if json.Unmarshal([]byte(text), &decoded) == nil {
				return equipmentMaps(decoded)
			}
		}
		return nil
	}
	result := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if typed, ok := item.(map[string]any); ok {
			result = append(result, typed)
		}
	}
	return result
}

func equipmentText(value any) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func equipmentNumber(value any) any {
	if text := equipmentText(value); text != "" {
		if parsed, err := strconv.Atoi(text); err == nil {
			return parsed
		}
	}
	return value
}

func equipmentInstrument(record map[string]any) map[string]any {
	modes := []string{}
	for _, key := range []string{"ZZNAME", "zzname", "SYNAME", "syname"} {
		if value := equipmentText(record[key]); value != "" && !containsString(modes, value) {
			modes = append(modes, value)
		}
	}
	return map[string]any{
		"id":            firstEquipmentText(record, "ZCBH", "YQBH", "zcbh"),
		"name":          firstEquipmentText(record, "YQMC", "yqmc"),
		"model":         firstEquipmentText(record, "YQXH", "yqxh"),
		"specification": firstEquipmentText(record, "YQGG", "yqgg"),
		"manufacturer":  firstEquipmentText(record, "SCCJ", "sccj"),
		"country":       firstEquipmentText(record, "GBMC", "gbmc"),
		"department_id": firstEquipmentText(record, "SSBMID", "ssbmid"),
		"department":    firstEquipmentText(record, "SSBMMC", "ssbmmc"),
		"lab_id":        firstEquipmentText(record, "SSSYSID", "sssysid"),
		"lab":           firstEquipmentText(record, "SSSYSMC", "sssysmc"),
		"status":        firstEquipmentText(record, "XZMC1", "XZ"),
		"availability":  firstEquipmentText(record, "YXZT", "XZMC"),
		"open":          firstEquipmentText(record, "OpenState", "openstate"),
		"service_modes": modes,
		"raw":           record,
	}
}

func equipmentUserProfile(record map[string]any) map[string]any {
	return map[string]any{
		"id":            firstEquipmentText(record, "ID", "GUID"),
		"account":       firstEquipmentText(record, "ZH"),
		"name":          firstEquipmentText(record, "XM"),
		"type":          firstEquipmentText(record, "LX"),
		"role":          firstEquipmentText(record, "YHWP"),
		"department_id": firstEquipmentText(record, "BMID"),
		"department":    firstEquipmentText(record, "BMMC"),
		"phone":         firstEquipmentText(record, "LXDH"),
		"email":         firstEquipmentText(record, "LXYX"),
		"card_number":   firstEquipmentText(record, "KH1", "KH2"),
		"credit":        equipmentNumber(record["XYJF"]),
		"group_count":   equipmentNumber(record["GROUPNUM"]),
		"balance":       equipmentNumber(record["YHYE"]),
		"raw":           record,
	}
}

func equipmentReservation(record map[string]any) map[string]any {
	return map[string]any{
		"reservation_id":   firstEquipmentText(record, "YYBH", "ID"),
		"status":           firstEquipmentText(record, "YYDZT"),
		"state":            firstEquipmentText(record, "STATE"),
		"instrument_id":    firstEquipmentText(record, "YQBH"),
		"instrument_name":  firstEquipmentText(record, "YQMC"),
		"reservation_type": firstEquipmentText(record, "YYLX"),
		"start_at":         firstEquipmentText(record, "YYKSSJ"),
		"end_at":           firstEquipmentText(record, "YYJSSJ"),
		"requester":        firstEquipmentText(record, "YYRXM"),
		"manager":          firstEquipmentText(record, "GLYMC"),
		"manager_phone":    firstEquipmentText(record, "GLYDH"),
		"amount":           equipmentNumber(record["YYMONEY"]),
		"actual_amount":    equipmentNumber(record["HDJG"]),
		"raw":              record,
	}
}

func firstEquipmentText(record map[string]any, keys ...string) string {
	for _, key := range keys {
		if value := equipmentText(record[key]); value != "" {
			return value
		}
	}
	return ""
}

func equipmentResult(operation, evidence string) map[string]any {
	return map[string]any{
		"ok": true, "submitted": false, "confirmed": true,
		"evidence": evidence, "service": equipmentServiceName, "operation": operation,
		"api": map[string]any{"endpoint": equipmentEndpoint},
	}
}

func equipmentRejected(action, code, message string) *siteError {
	if message == "" {
		message = "远端返回失败"
	}
	return &siteError{Code: "business_rejected", Message: "实验室仪器接口 " + action + " 失败: " + message, Details: map[string]any{"api": action, "code": code}}
}
