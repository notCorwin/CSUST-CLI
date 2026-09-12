package adapter

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

const electronicDocumentsService = "electronic-documents"

const electronicDocumentsCASPath = "/api/engine-system/cas/mangeLogin"

type electronicDocumentsSession struct {
	token     string
	userID    string
	cookie    string
	tokenPath string
	userPath  string
}

func (a NativeSite) executeElectronicDocuments(ctx context.Context, args []string) (map[string]any, *siteError) {
	if len(args) == 0 || args[0] == "catalog" {
		return businessCatalogFilter(electronicDocumentsService), nil
	}
	cookie, _, valueErr := businessValue(args, "--cookie-file")
	if valueErr != nil {
		return nil, valueErr
	}
	switch args[0] {
	case "login":
		return a.electronicDocumentsLogin(ctx, args[1:], cookie)
	case "logout":
		return a.electronicDocumentsLogout(ctx, args[1:], cookie)
	case "status":
		return a.electronicDocumentsStatus(ctx, args[1:], cookie)
	case "types", "file-types":
		session, sessionErr := loadElectronicDocumentsSession(args[1:], cookie)
		if sessionErr != nil {
			return nil, sessionErr
		}
		return a.electronicDocumentsTypes(ctx, session)
	case "applications", "records":
		session, sessionErr := loadElectronicDocumentsSession(args[1:], cookie)
		if sessionErr != nil {
			return nil, sessionErr
		}
		return a.electronicDocumentsApplications(ctx, args[1:], session)
	case "application", "record":
		session, sessionErr := loadElectronicDocumentsSession(args[1:], cookie)
		if sessionErr != nil {
			return nil, sessionErr
		}
		return a.electronicDocumentsApplication(ctx, args[1:], session)
	case "apply", "request":
		session, sessionErr := loadElectronicDocumentsSession(args[1:], cookie)
		if sessionErr != nil {
			return nil, sessionErr
		}
		return a.electronicDocumentsApply(ctx, args[1:], session)
	default:
		return nil, &siteError{Code: "invalid_argument", Message: "electronic-documents 只支持 login、logout、status、types、applications、application、apply、catalog"}
	}
}

func electronicDocumentsTokenPath(cookie string) string {
	if strings.HasSuffix(cookie, ".cookies.txt") {
		return strings.TrimSuffix(cookie, ".cookies.txt") + ".token"
	}
	return cookie + ".token"
}

func electronicDocumentsUserPath(cookie string) string {
	return electronicDocumentsTokenPath(cookie) + ".user-id"
}

func electronicDocumentsReadSessionFile(path, label string) (string, *siteError) {
	info, statErr := os.Lstat(path)
	if os.IsNotExist(statErr) {
		return "", nil
	}
	if statErr != nil {
		return "", &siteError{Code: "session_error", Message: "无法读取" + label + ": " + statErr.Error()}
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return "", &siteError{Code: "session_error", Message: label + "必须是普通文件且不能是符号链接"}
	}
	content, readErr := os.ReadFile(path)
	if readErr != nil {
		return "", &siteError{Code: "session_error", Message: "无法读取" + label + ": " + readErr.Error()}
	}
	return strings.TrimSpace(string(content)), nil
}

func loadElectronicDocumentsSession(args []string, cookie string) (electronicDocumentsSession, *siteError) {
	token, found, valueErr := businessValue(args, "--access-token")
	if valueErr != nil {
		return electronicDocumentsSession{}, valueErr
	}
	if !found || strings.TrimSpace(token) == "" {
		token = os.Getenv("CSUST_ELECTRONIC_DOCUMENTS_TOKEN")
	}
	userID, found, valueErr := businessValue(args, "--user-id")
	if valueErr != nil {
		return electronicDocumentsSession{}, valueErr
	}
	if !found || strings.TrimSpace(userID) == "" {
		userID = os.Getenv("CSUST_ELECTRONIC_DOCUMENTS_USER_ID")
	}
	_, cookiePath, resolveErr := resolveSite(siteRequest{Service: electronicDocumentsService, CookieFile: cookie})
	if resolveErr != nil {
		return electronicDocumentsSession{}, resolveErr
	}
	tokenPath := electronicDocumentsTokenPath(cookiePath)
	userPath := electronicDocumentsUserPath(cookiePath)
	if strings.TrimSpace(token) == "" {
		fileToken, readErr := electronicDocumentsReadSessionFile(tokenPath, "电子证明平台令牌文件")
		if readErr != nil {
			return electronicDocumentsSession{}, readErr
		}
		token = fileToken
	}
	if strings.TrimSpace(userID) == "" {
		fileUserID, readErr := electronicDocumentsReadSessionFile(userPath, "电子证明平台用户标识文件")
		if readErr != nil {
			return electronicDocumentsSession{}, readErr
		}
		userID = fileUserID
	}
	return electronicDocumentsSession{
		token: strings.TrimSpace(token), userID: strings.TrimSpace(userID), cookie: cookiePath,
		tokenPath: tokenPath, userPath: userPath,
	}, nil
}

func electronicDocumentsHeaders(token string) []pair {
	headers := []pair{{"language", "zh-CN"}}
	if strings.TrimSpace(token) != "" {
		headers = append(headers, pair{"X-Authorization", token})
	}
	return headers
}

func (a NativeSite) electronicDocumentsCall(ctx context.Context, session electronicDocumentsSession, method, path string, body any, readOnly, yes bool) (map[string]any, *siteError) {
	return a.execute(ctx, siteRequest{
		Service: electronicDocumentsService, Method: method, Path: path, JSON: body, HasJSON: body != nil,
		Headers: electronicDocumentsHeaders(session.token), CookieFile: session.cookie, RequireLogin: session.token != "",
		AllowBusinessFailure: true, ReadOnly: readOnly, Yes: yes, RawJSON: true,
	})
}

func electronicDocumentsPayload(result map[string]any) (map[string]any, *siteError) {
	payload, parseErr := businessJSONMap(result)
	if parseErr != nil {
		return nil, parseErr
	}
	if fmt.Sprint(payload["errcode"]) != "0" {
		message := strings.TrimSpace(fmt.Sprint(payload["errmsg"]))
		if message == "" {
			message = "电子证明平台接口拒绝请求"
		}
		code := "business_rejected"
		if fmt.Sprint(payload["errcode"]) == "401" {
			code = "login_required"
		}
		return nil, &siteError{Code: code, Message: message, Details: map[string]any{"remote_code": payload["errcode"]}}
	}
	return payload, nil
}

func electronicDocumentsResult(payload map[string]any) map[string]any {
	result, _ := payload["result"].(map[string]any)
	return result
}

func electronicDocumentsRows(value any) []map[string]any {
	rows, _ := value.([]any)
	result := make([]map[string]any, 0, len(rows))
	for _, item := range rows {
		if row, ok := item.(map[string]any); ok {
			result = append(result, row)
		}
	}
	return result
}

func redactElectronicDocuments(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		result := make(map[string]any, len(typed))
		for key, item := range typed {
			lower := strings.ToLower(key)
			if lower == "userid" || lower == "studentid" || lower == "toemail" || lower == "email" || lower == "phone" || lower == "cardnumber" || lower == "openid" || lower == "token" || lower == "signature" {
				result[key] = "<redacted>"
				continue
			}
			result[key] = redactElectronicDocuments(item)
		}
		return result
	case []any:
		result := make([]any, len(typed))
		for index, item := range typed {
			result[index] = redactElectronicDocuments(item)
		}
		return result
	default:
		return redactSiteJSON(value)
	}
}

func (a NativeSite) electronicDocumentsTypesData(ctx context.Context, session electronicDocumentsSession) ([]map[string]any, map[string]any, *siteError) {
	if session.token == "" || session.userID == "" {
		return nil, nil, &siteError{Code: "login_required", Message: "请先运行 electronic-documents login，或同时提供 --access-token 和 --user-id"}
	}
	result, requestErr := a.electronicDocumentsCall(ctx, session, "POST", "/api/engine-dzpz/ElectronicFile/getFilePrintType", map[string]any{"userId": session.userID}, true, true)
	if requestErr != nil {
		return nil, nil, requestErr
	}
	payload, payloadErr := electronicDocumentsPayload(result)
	if payloadErr != nil {
		return nil, nil, payloadErr
	}
	envelope := electronicDocumentsResult(payload)
	rows := electronicDocumentsRows(envelope["data"])
	return rows, payload, nil
}

func (a NativeSite) electronicDocumentsTypes(ctx context.Context, session electronicDocumentsSession) (map[string]any, *siteError) {
	rows, payload, requestErr := a.electronicDocumentsTypesData(ctx, session)
	if requestErr != nil {
		return nil, requestErr
	}
	data := make([]any, 0, len(rows))
	for _, row := range rows {
		data = append(data, redactElectronicDocuments(row))
	}
	return map[string]any{
		"ok": true, "submitted": false, "confirmed": true, "evidence": "电子证明平台 getFilePrintType 返回 errcode=0",
		"service": electronicDocumentsService, "operation": "types", "data": data, "count": len(data),
		"raw": redactElectronicDocuments(payload), "token_file": session.tokenPath, "user_id_file": session.userPath,
	}, nil
}

func electronicDocumentsKind(args []string) (any, string, *siteError) {
	kind, found, valueErr := businessValue(args, "--kind")
	if valueErr != nil {
		return nil, "", valueErr
	}
	if !found || strings.TrimSpace(kind) == "" {
		return nil, "", nil
	}
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "transcript", "成绩单":
		return 1, "transcript", nil
	case "certificate", "proof", "证明":
		return 2, "certificate", nil
	default:
		return nil, "", &siteError{Code: "invalid_argument", Message: "--kind 只能是 transcript 或 certificate"}
	}
}

func (a NativeSite) electronicDocumentsApplications(ctx context.Context, args []string, session electronicDocumentsSession) (map[string]any, *siteError) {
	page, pageErr := businessInt(args, "--page", 1)
	if pageErr != nil {
		return nil, pageErr
	}
	size, sizeErr := businessInt(args, "--page-size", 20)
	if sizeErr != nil {
		return nil, sizeErr
	}
	kind, kindName, kindErr := electronicDocumentsKind(args)
	if kindErr != nil {
		return nil, kindErr
	}
	body := map[string]any{"size": size, "page": page}
	if kind != nil {
		body["moduleType"] = kind
	}
	result, requestErr := a.electronicDocumentsCall(ctx, session, "POST", "/api/engine-dzpz/ElectronicFile/userOrderList", body, true, true)
	if requestErr != nil {
		return nil, requestErr
	}
	payload, payloadErr := electronicDocumentsPayload(result)
	if payloadErr != nil {
		return nil, payloadErr
	}
	envelope := electronicDocumentsResult(payload)
	rows := electronicDocumentsRows(envelope["data"])
	data := make([]any, 0, len(rows))
	for _, row := range rows {
		data = append(data, redactElectronicDocuments(row))
	}
	response := map[string]any{
		"ok": true, "submitted": false, "confirmed": true, "evidence": "电子证明平台 userOrderList 返回 errcode=0",
		"service": electronicDocumentsService, "operation": "applications", "page": page, "page_size": size,
		"data": data, "total": envelope["total"], "raw": redactElectronicDocuments(payload),
		"token_file": session.tokenPath, "user_id_file": session.userPath,
	}
	if kindName != "" {
		response["kind"] = kindName
	}
	return response, nil
}

func (a NativeSite) electronicDocumentsApplication(ctx context.Context, args []string, session electronicDocumentsSession) (map[string]any, *siteError) {
	orderID, requiredErr := businessRequired(args, "--id", "application 必须提供 --id（订单号）")
	if requiredErr != nil {
		return nil, requiredErr
	}
	result, requestErr := a.electronicDocumentsCall(ctx, session, "POST", "/api/engine-dzpz/ElectronicFile/userOrderDetails", map[string]any{"orderId": strings.TrimSpace(orderID)}, true, true)
	if requestErr != nil {
		return nil, requestErr
	}
	payload, payloadErr := electronicDocumentsPayload(result)
	if payloadErr != nil {
		return nil, payloadErr
	}
	envelope := electronicDocumentsResult(payload)
	rows := electronicDocumentsRows(envelope["data"])
	data := make([]any, 0, len(rows))
	for _, row := range rows {
		data = append(data, redactElectronicDocuments(row))
	}
	return map[string]any{
		"ok": true, "submitted": false, "confirmed": true, "evidence": "电子证明平台 userOrderDetails 返回 errcode=0",
		"service": electronicDocumentsService, "operation": "application", "order_id": orderID,
		"data": data, "raw": redactElectronicDocuments(payload), "token_file": session.tokenPath, "user_id_file": session.userPath,
	}, nil
}

func electronicDocumentsTypeAlias(value string) []string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "chinese-transcript":
		return []string{"中文成绩单"}
	case "english-transcript":
		return []string{"英文成绩单"}
	case "chinese-minor-transcript":
		return []string{"中文辅修成绩单"}
	case "english-minor-transcript":
		return []string{"英文辅修成绩单"}
	case "enrollment-proof", "student-proof":
		return []string{"在校生学籍证明"}
	case "graduation-estimate", "expected-graduation-proof":
		return []string{"预计毕业生情况证明"}
	default:
		return []string{strings.TrimSpace(value)}
	}
}

func electronicDocumentsFindType(rows []map[string]any, wanted string) (map[string]any, *siteError) {
	wantedNames := electronicDocumentsTypeAlias(wanted)
	for _, row := range rows {
		name := strings.TrimSpace(fmt.Sprint(row["printType"]))
		for _, wantedName := range wantedNames {
			if name == wantedName {
				return row, nil
			}
		}
	}
	return nil, &siteError{Code: "not_found", Message: "电子证明平台没有找到文件类型: " + wanted}
}

func (a NativeSite) electronicDocumentsPreview(ctx context.Context, session electronicDocumentsSession, row map[string]any) (map[string]any, *siteError) {
	fileID := strings.TrimSpace(fmt.Sprint(row["vcPrintTypeId"]))
	if fileID == "" {
		return nil, &siteError{Code: "parse_error", Message: "电子证明文件类型缺少标识"}
	}
	body := map[string]any{
		"userId": session.userID,
		"list":   []any{map[string]any{"fileProperty": fileID, "userId": session.userID}},
	}
	result, requestErr := a.electronicDocumentsCall(ctx, session, "POST", "/api/engine-dzpz/ElectronicFile/getPrintFilePictures", body, true, true)
	if requestErr != nil {
		return nil, requestErr
	}
	payload, payloadErr := electronicDocumentsPayload(result)
	if payloadErr != nil {
		return nil, payloadErr
	}
	rows := electronicDocumentsRows(electronicDocumentsResult(payload)["data"])
	if len(rows) == 0 {
		return nil, &siteError{Code: "business_rejected", Message: "电子证明平台没有生成文件预览"}
	}
	return rows[0], nil
}

func electronicDocumentsProductRow(row map[string]any) map[string]any {
	product := make(map[string]any, len(row)+4)
	for key, value := range row {
		product[key] = value
	}
	delete(product, "xh")
	delete(product, "userId")
	product["productName"] = row["printType"]
	product["vcid"] = row["vcPrintTypeId"]
	product["productNum"] = 1
	return product
}

func (a NativeSite) electronicDocumentsProducts(ctx context.Context, session electronicDocumentsSession, row, preview map[string]any) (string, []any, *siteError) {
	body := map[string]any{
		"userId":   session.userID,
		"products": []any{electronicDocumentsProductRow(row)},
	}
	result, requestErr := a.electronicDocumentsCall(ctx, session, "POST", "/api/engine-dzpz/ElectronicFile/getPayProductInfoEx", body, true, true)
	if requestErr != nil {
		return "", nil, requestErr
	}
	payload, payloadErr := electronicDocumentsPayload(result)
	if payloadErr != nil {
		return "", nil, payloadErr
	}
	data := electronicDocumentsRows(electronicDocumentsResult(payload)["data"])
	if len(data) == 0 {
		return "", nil, &siteError{Code: "parse_error", Message: "电子证明平台生成商品信息响应缺少 data"}
	}
	fileName := strings.TrimSpace(fmt.Sprint(data[0]["fileName"]))
	products, ok := data[0]["products"].([]any)
	if !ok || len(products) == 0 {
		return "", nil, &siteError{Code: "parse_error", Message: "电子证明平台生成商品信息响应缺少 products"}
	}
	for index, value := range products {
		product, ok := value.(map[string]any)
		if !ok {
			return "", nil, &siteError{Code: "parse_error", Message: "电子证明平台商品信息结构无效"}
		}
		if printerID := product["printerId"]; printerID != nil && fmt.Sprint(printerID) != "" {
			product["productId"] = printerID
		}
		if index == 0 {
			product["pdfSerialId"] = preview["pdfSerialId"]
			product["fileUrl"] = preview["fileUrl"]
		}
	}
	if fileName == "" {
		return "", nil, &siteError{Code: "parse_error", Message: "电子证明平台生成商品信息响应缺少文件名"}
	}
	return fileName, products, nil
}

func (a NativeSite) electronicDocumentsApply(ctx context.Context, args []string, session electronicDocumentsSession) (map[string]any, *siteError) {
	if !businessBool(args, "--yes") {
		return nil, &siteError{Code: "confirmation_required", Message: "申请电子证明可能生成文件、发送邮件或产生费用，请加 --yes"}
	}
	wanted, requiredErr := businessRequired(args, "--type", "apply 必须提供 --type")
	if requiredErr != nil {
		return nil, requiredErr
	}
	delivery, found, valueErr := businessValue(args, "--delivery")
	if valueErr != nil {
		return nil, valueErr
	}
	if !found || strings.TrimSpace(delivery) == "" {
		delivery = "download"
	}
	delivery = strings.ToLower(strings.TrimSpace(delivery))
	if delivery != "download" && delivery != "email" {
		return nil, &siteError{Code: "invalid_argument", Message: "--delivery 只能是 download 或 email"}
	}
	rows, _, typesErr := a.electronicDocumentsTypesData(ctx, session)
	if typesErr != nil {
		return nil, typesErr
	}
	row, typeErr := electronicDocumentsFindType(rows, wanted)
	if typeErr != nil {
		return nil, typeErr
	}
	preview, previewErr := a.electronicDocumentsPreview(ctx, session, row)
	if previewErr != nil {
		return nil, previewErr
	}
	fileName, products, productErr := a.electronicDocumentsProducts(ctx, session, row, preview)
	if productErr != nil {
		return nil, productErr
	}
	if delivery == "download" {
		output, outputErr := businessRequired(args, "--output", "下载申请结果必须提供 --output PDF 路径")
		if outputErr != nil {
			return nil, outputErr
		}
		output = expandUserPath(output)
		if strings.ToLower(filepath.Ext(output)) != ".pdf" {
			return nil, &siteError{Code: "invalid_argument", Message: "--output 必须使用 .pdf 扩展名"}
		}
		result, requestErr := a.execute(ctx, siteRequest{
			Service: electronicDocumentsService, Method: "POST", Path: "/api/engine-dzpz/Pay/webDownloadDoc",
			JSON: map[string]any{"userId": session.userID, "fileName": fileName, "products": products}, HasJSON: true,
			Headers: electronicDocumentsHeaders(session.token), CookieFile: session.cookie, RequireLogin: true,
			AllowBusinessFailure: true, Output: output, ReadOnly: true, Yes: true, RawJSON: true,
		})
		if requestErr != nil {
			return nil, requestErr
		}
		result["service"], result["operation"] = electronicDocumentsService, "apply"
		result["type"], result["delivery"], result["file_name"] = row["printType"], delivery, fileName
		result["verified_by"] = "webDownloadDoc 返回有效 PDF 并已原子保存"
		return result, nil
	}
	email, emailErr := businessRequired(args, "--email", "email 申请必须提供 --email")
	if emailErr != nil {
		return nil, emailErr
	}
	result, requestErr := a.electronicDocumentsCall(ctx, session, "POST", "/api/engine-dzpz/Pay/webWechatPay", map[string]any{
		"userId": session.userID, "fileName": fileName, "products": products, "toEmail": strings.TrimSpace(email),
	}, false, true)
	if requestErr != nil {
		return nil, requestErr
	}
	payload, payloadErr := electronicDocumentsPayload(result)
	if payloadErr != nil {
		return nil, payloadErr
	}
	return map[string]any{
		"ok": true, "submitted": true, "confirmed": true,
		"evidence":    "电子证明平台 webWechatPay 返回 errcode=0",
		"verified_by": "远端成功信封 errcode=0", "service": electronicDocumentsService, "operation": "apply",
		"type": row["printType"], "delivery": delivery, "file_name": fileName,
		"api_data": redactElectronicDocuments(payload), "token_file": session.tokenPath, "user_id_file": session.userPath,
	}, nil
}

func (a NativeSite) electronicDocumentsLogin(ctx context.Context, args []string, cookie string) (map[string]any, *siteError) {
	options, parseErr := parseLoginOptions(args)
	if parseErr != nil {
		return nil, parseErr
	}
	if options.auth != "auto" && options.auth != "sso" {
		return nil, &siteError{Code: "invalid_argument", Message: "electronic-documents 只支持 --auth auto 或 sso"}
	}
	account, password, credentialErr := credentialsGo(options.username, options.password)
	if credentialErr != nil {
		return nil, credentialErr
	}
	target, cookiePath, resolveErr := resolveSite(siteRequest{Service: electronicDocumentsService, CookieFile: cookie})
	if resolveErr != nil {
		return nil, resolveErr
	}
	target.Path, target.RawQuery, target.Fragment = electronicDocumentsCASPath, "", ""
	sessionTarget := *target
	sessionTarget.Path, sessionTarget.RawQuery, sessionTarget.Fragment = "/", "", ""
	loginResult, loginErr := a.loginSSOPassword(ctx, &sessionTarget, target.String(), account, password, options, cookiePath)
	if loginErr != nil {
		return nil, loginErr
	}
	_, responseURL, bodyErr := loginBody(loginResult)
	if bodyErr != nil {
		return nil, bodyErr
	}
	callback, parseCallbackErr := url.Parse(responseURL)
	if parseCallbackErr != nil || callback == nil || !strings.EqualFold(callback.Host, target.Host) || callback.Path != "/Integrated_platform/login" {
		return nil, &siteError{Code: "authentication_failed", Message: "电子证明平台 CAS 回跳地址无效"}
	}
	signature := strings.TrimSpace(callback.Query().Get("signature"))
	if signature == "" {
		return nil, &siteError{Code: "authentication_failed", Message: "电子证明平台 CAS 回跳缺少签名参数"}
	}
	apiResult, requestErr := a.execute(ctx, siteRequest{
		Service: electronicDocumentsService, Method: "POST", Path: "/api/User/login",
		JSON: map[string]any{"roleId": 3, "signature": signature, "loginType": 4}, HasJSON: true,
		Headers: electronicDocumentsHeaders(""), CookieFile: cookiePath, AllowBusinessFailure: true, ReadOnly: true, Yes: true, RawJSON: true,
	})
	if requestErr != nil {
		return nil, requestErr
	}
	payload, payloadErr := electronicDocumentsPayload(apiResult)
	if payloadErr != nil {
		return nil, &siteError{Code: "authentication_failed", Message: payloadErr.Message, Details: payloadErr.Details}
	}
	loginData := electronicDocumentsResult(payload)
	users := electronicDocumentsRows(loginData["data"])
	if len(users) == 0 {
		return nil, &siteError{Code: "authentication_failed", Message: "电子证明平台登录响应缺少用户数据"}
	}
	token := strings.TrimSpace(fmt.Sprint(users[0]["token"]))
	identityRows := electronicDocumentsRows(users[0]["data"])
	if token == "" || len(identityRows) == 0 {
		return nil, &siteError{Code: "authentication_failed", Message: "电子证明平台登录响应缺少令牌或用户标识"}
	}
	userID := strings.TrimSpace(fmt.Sprint(identityRows[0]["userId"]))
	if userID == "" {
		return nil, &siteError{Code: "authentication_failed", Message: "电子证明平台登录响应缺少用户标识"}
	}
	session := electronicDocumentsSession{token: token, userID: userID, cookie: cookiePath, tokenPath: electronicDocumentsTokenPath(cookiePath), userPath: electronicDocumentsUserPath(cookiePath)}
	if writeErr := atomicWrite(session.tokenPath, []byte(token+"\n")); writeErr != nil {
		return nil, &siteError{Code: "session_error", Message: "登录成功但令牌保存失败: " + writeErr.Error(), Details: map[string]any{"token_file": session.tokenPath}}
	}
	if writeErr := atomicWrite(session.userPath, []byte(userID+"\n")); writeErr != nil {
		_ = removeCookieFile(session.tokenPath)
		return nil, &siteError{Code: "session_error", Message: "登录成功但用户标识保存失败: " + writeErr.Error(), Details: map[string]any{"user_id_file": session.userPath}}
	}
	rows, _, verifyErr := a.electronicDocumentsTypesData(ctx, session)
	if verifyErr != nil {
		return nil, &siteError{Code: "authentication_unverified", Message: "登录已返回令牌，但文件类型回读验证失败: " + verifyErr.Error(), Details: map[string]any{"token_file": session.tokenPath, "user_id_file": session.userPath}}
	}
	return map[string]any{
		"ok": true, "submitted": false, "confirmed": true, "evidence": "CAS 回跳、User/login 令牌和文件类型回读均成功",
		"service": electronicDocumentsService, "operation": "login", "auth": "sso", "username": account,
		"token_file": session.tokenPath, "user_id_file": session.userPath, "file_type_count": len(rows),
		"user": redactElectronicDocuments(identityRows[0]),
	}, nil
}

func (a NativeSite) electronicDocumentsStatus(ctx context.Context, args []string, cookie string) (map[string]any, *siteError) {
	session, sessionErr := loadElectronicDocumentsSession(args, cookie)
	if sessionErr != nil {
		return nil, sessionErr
	}
	if session.token == "" || session.userID == "" {
		return map[string]any{
			"ok": true, "submitted": false, "confirmed": true, "evidence": "本地没有完整电子证明平台会话",
			"service": electronicDocumentsService, "operation": "status", "logged_in": false,
			"token_file": session.tokenPath, "user_id_file": session.userPath,
		}, nil
	}
	rows, _, verifyErr := a.electronicDocumentsTypesData(ctx, session)
	if verifyErr != nil {
		if verifyErr.Code == "login_required" {
			return map[string]any{
				"ok": true, "submitted": false, "confirmed": true, "evidence": "文件类型接口报告会话失效",
				"service": electronicDocumentsService, "operation": "status", "logged_in": false,
				"token_file": session.tokenPath, "user_id_file": session.userPath,
			}, nil
		}
		return nil, verifyErr
	}
	return map[string]any{
		"ok": true, "submitted": false, "confirmed": true, "evidence": "文件类型接口回读验证成功",
		"service": electronicDocumentsService, "operation": "status", "logged_in": true,
		"file_type_count": len(rows), "token_file": session.tokenPath, "user_id_file": session.userPath,
	}, nil
}

func (a NativeSite) electronicDocumentsLogout(ctx context.Context, args []string, cookie string) (map[string]any, *siteError) {
	if !businessBool(args, "--yes") {
		return nil, &siteError{Code: "confirmation_required", Message: "退出电子证明平台会话需要 --yes"}
	}
	session, sessionErr := loadElectronicDocumentsSession(args, cookie)
	if sessionErr != nil {
		return nil, sessionErr
	}
	if session.token != "" {
		if _, requestErr := a.electronicDocumentsCall(ctx, session, "POST", "/api/User/logout", map[string]any{}, false, true); requestErr != nil && requestErr.Code != "login_required" {
			return nil, requestErr
		}
	}
	for _, path := range []string{session.tokenPath, session.userPath, session.cookie} {
		if removeErr := removeCookieFile(path); removeErr != nil {
			return nil, &siteError{Code: "session_error", Message: "电子证明平台会话删除失败: " + removeErr.Error()}
		}
	}
	return map[string]any{
		"ok": true, "submitted": false, "confirmed": true, "evidence": "远端退出（如有会话）并删除本地令牌、用户标识和 Cookie",
		"service": electronicDocumentsService, "operation": "logout", "logged_out": true,
	}, nil
}
