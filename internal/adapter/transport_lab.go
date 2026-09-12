package adapter

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"regexp"
	"strings"
)

var transportLabPhonePattern = regexp.MustCompile(`^1[3-9]\d{9}$`)

func (a NativeSite) executeTransportLab(ctx context.Context, args []string) (map[string]any, *siteError) {
	if len(args) == 0 || args[0] == "status" {
		statusArgs := args
		if len(statusArgs) > 0 {
			statusArgs = statusArgs[1:]
		}
		cookie, _, valueErr := businessValue(statusArgs, "--cookie-file")
		if valueErr != nil {
			return nil, valueErr
		}
		return a.executeServiceStatusWithOptions(ctx, "transport-lab", "status", "实验室预约管理平台", businessRequestOptions{cookieFile: cookie})
	}
	if args[0] == "catalog" {
		result := businessCatalogFilter("transport-lab")
		result["roles"] = []map[string]any{
			{"role": "user", "label": "实验室预约用户", "value": "1", "scope": "交通学院研究生、非交通学院研究生及校外人员预约实验室/会议室"},
			{"role": "teacher", "label": "交通学院教职工", "value": "2", "scope": "交通学院教职工预约会议室"},
		}
		result["operations"] = []string{"status", "login", "logout", "getpwdquestion", "forgot", "register"}
		return result, nil
	}
	cookie, _, valueErr := businessValue(args[1:], "--cookie-file")
	if valueErr != nil {
		return nil, valueErr
	}
	switch args[0] {
	case "login":
		return a.transportLabLogin(ctx, args[1:], cookie)
	case "logout":
		return a.transportLabLogout(ctx, args[1:], cookie)
	case "getpwdquestion":
		return a.transportLabPasswordQuestion(ctx, args[1:], cookie)
	case "forgot":
		return a.transportLabForgotPassword(ctx, args[1:], cookie)
	case "register":
		return a.transportLabRegister(ctx, args[1:], cookie)
	default:
		return nil, &siteError{Code: "invalid_argument", Message: "transport-lab 只支持 status、catalog、login、logout、getpwdquestion、forgot、register"}
	}
}

func transportLabRole(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "user", "reservation-user", "实验室预约用户", "1":
		return "1"
	case "teacher", "staff", "交通学院教职工", "2":
		return "2"
	default:
		return ""
	}
}

func transportLabLoginPage(body string) bool {
	return strings.Contains(body, "请输入手机号码") && strings.Contains(body, "请输入验证码")
}

func (a NativeSite) transportLabLogin(ctx context.Context, args []string, cookie string) (map[string]any, *siteError) {
	role := transportLabRole(flagValue(args, "--role"))
	if role == "" {
		return nil, &siteError{Code: "invalid_argument", Message: "login 必须提供 --role user 或 teacher"}
	}
	phone, phoneErr := transportLabPhone(args)
	if phoneErr != nil {
		return nil, phoneErr
	}
	loginArgs := args
	if flagValue(args, "--username") == "" {
		loginArgs = append(append([]string(nil), args...), "--username", phone)
	}
	account, password, credentialErr := businessCredentials(loginArgs, "CSUST_TRANSPORT_LAB_PASSWORD")
	if credentialErr != nil {
		return nil, credentialErr
	}
	page, requestErr := a.businessGet(ctx, "transport-lab", "/Login/Index", nil, businessRequestOptions{cookieFile: cookie})
	if requestErr != nil {
		return nil, requestErr
	}
	captcha := flagValue(args, "--captcha")
	if captcha == "" {
		return nil, businessCaptcha(ctx, "transport-lab", "/login/GetVerifyCode", "transport-lab-captcha.png", args, cookie)
	}
	result, requestErr := businessRequest(ctx, "transport-lab", "POST", "/Login/CheckLogin", nil, []pair{{"role", role}, {"username", account}, {"password", password}, {"code", captcha}}, []pair{{"X-Requested-With", "XMLHttpRequest"}, {"Accept", "application/json, text/javascript, */*; q=0.01"}, {"Referer", safeResponseURL(page)}}, businessRequestOptions{cookieFile: cookie, allowBusinessFailure: true}, true, true)
	if requestErr != nil {
		return nil, requestErr
	}
	payload, parseErr := businessJSONMap(result)
	if parseErr != nil {
		return nil, parseErr
	}
	state, known := jsonBusinessState(payload)
	if !known || !state {
		return nil, &siteError{Code: "authentication_failed", Message: firstNonEmpty(fmt.Sprint(payload["message"]), "实验室预约平台登录失败"), Details: map[string]any{"submitted": true, "confirmed": false, "evidence": "CheckLogin-response", "reason": payload["data"]}}
	}
	evidence, probeErr := a.confirmBusinessLogin(ctx, "transport-lab", "/Home/Index", cookie, transportLabLoginPage)
	if probeErr != nil {
		return nil, probeErr
	}
	return map[string]any{"ok": true, "submitted": true, "confirmed": true, "evidence": "CheckLogin-and-" + evidence, "service": "transport-lab", "operation": "login", "role": role, "username": account}, nil
}

func (a NativeSite) transportLabLogout(ctx context.Context, args []string, cookie string) (map[string]any, *siteError) {
	if !businessBool(args, "--yes") {
		return nil, &siteError{Code: "confirmation_required", Message: "实验室预约平台退出会话需要 --yes"}
	}
	_, cookiePath, resolveErr := resolveSite(siteRequest{Service: "transport-lab", CookieFile: cookie})
	if resolveErr != nil {
		return nil, resolveErr
	}
	if removeErr := removeCookieFile(cookiePath); removeErr != nil {
		return nil, &siteError{Code: "cookie_write_failed", Message: removeErr.Error()}
	}
	return map[string]any{"ok": true, "submitted": false, "confirmed": true, "evidence": "local-cookie-removed", "service": "transport-lab", "operation": "logout", "logged_out": true, "cookie_file": cookiePath}, nil
}

func transportLabPhone(args []string) (string, *siteError) {
	phone := flagValue(args, "--phone")
	if phone == "" {
		phone = flagValue(args, "--username")
	}
	if phone == "" {
		return "", &siteError{Code: "invalid_argument", Message: "必须提供 --phone 手机号码"}
	}
	if !transportLabPhonePattern.MatchString(phone) {
		return "", &siteError{Code: "invalid_argument", Message: "--username 必须是有效手机号码"}
	}
	return phone, nil
}

func (a NativeSite) transportLabPasswordQuestion(ctx context.Context, args []string, cookie string) (map[string]any, *siteError) {
	phone, phoneErr := transportLabPhone(args)
	if phoneErr != nil {
		return nil, phoneErr
	}
	page, requestErr := a.businessGet(ctx, "transport-lab", "/Login/Forget", nil, businessRequestOptions{cookieFile: cookie})
	if requestErr != nil {
		return nil, requestErr
	}
	result, requestErr := businessRequest(ctx, "transport-lab", "POST", "/Login/ForgetPwdQuestion", nil, []pair{{"keyValue", phone}}, []pair{{"X-Requested-With", "XMLHttpRequest"}, {"Accept", "application/json, text/javascript, */*; q=0.01"}, {"Referer", safeResponseURL(page)}}, businessRequestOptions{cookieFile: cookie, allowBusinessFailure: true}, true, true)
	if requestErr != nil {
		return nil, requestErr
	}
	payload, parseErr := businessJSONMap(result)
	if parseErr != nil {
		return nil, parseErr
	}
	state, known := jsonBusinessState(payload)
	if !known || !state {
		return nil, &siteError{Code: "business_rejected", Message: firstNonEmpty(fmt.Sprint(payload["message"]), "密保问题获取失败"), Details: map[string]any{"submitted": true, "confirmed": false, "evidence": "ForgetPwdQuestion-response"}}
	}
	return map[string]any{"ok": true, "submitted": true, "confirmed": true, "evidence": "ForgetPwdQuestion-response", "service": "transport-lab", "operation": "getpwdquestion", "username": phone, "question": payload["message"]}, nil
}

func (a NativeSite) transportLabForgotPassword(ctx context.Context, args []string, cookie string) (map[string]any, *siteError) {
	if !businessBool(args, "--yes") {
		return nil, &siteError{Code: "confirmation_required", Message: "修改实验室预约平台密码需要 --yes"}
	}
	phone, phoneErr := transportLabPhone(args)
	if phoneErr != nil {
		return nil, phoneErr
	}
	answer, answerErr := businessRequired(args, "--answer", "forgot 必须提供 --answer")
	if answerErr != nil {
		return nil, answerErr
	}
	password, passwordErr := businessSecret(args, "--new-password", "CSUST_TRANSPORT_LAB_NEW_PASSWORD")
	if passwordErr != nil {
		return nil, passwordErr
	}
	confirm, confirmErr := businessRequired(args, "--password-confirm", "forgot 必须提供 --password-confirm")
	if confirmErr != nil {
		return nil, confirmErr
	}
	if password != confirm {
		return nil, &siteError{Code: "invalid_argument", Message: "--new-password 与 --password-confirm 不一致"}
	}
	page, requestErr := a.businessGet(ctx, "transport-lab", "/Login/Forget", nil, businessRequestOptions{cookieFile: cookie})
	if requestErr != nil {
		return nil, requestErr
	}
	result, requestErr := businessRequest(ctx, "transport-lab", "POST", "/Login/CheckForget", nil, []pair{{"membername", phone}, {"memberpwdanswer", answer}, {"memberpwd", password}}, []pair{{"X-Requested-With", "XMLHttpRequest"}, {"Accept", "application/json, text/javascript, */*; q=0.01"}, {"Referer", safeResponseURL(page)}}, businessRequestOptions{cookieFile: cookie, allowBusinessFailure: true}, false, true)
	if requestErr != nil {
		return nil, requestErr
	}
	payload, parseErr := businessJSONMap(result)
	if parseErr != nil {
		return nil, parseErr
	}
	state, known := jsonBusinessState(payload)
	if !known || !state {
		return nil, &siteError{Code: "business_rejected", Message: firstNonEmpty(fmt.Sprint(payload["message"]), "实验室预约平台密码修改失败"), Details: map[string]any{"submitted": true, "confirmed": false, "evidence": "CheckForget-response"}}
	}
	return map[string]any{"ok": true, "submitted": true, "confirmed": true, "evidence": "CheckForget-response", "service": "transport-lab", "operation": "forgot", "username": phone}, nil
}

func transportLabImage(path string) (string, *siteError) {
	file, err := openSiteInput(path)
	if err != nil {
		return "", &siteError{Code: "invalid_argument", Message: "无法读取图片: " + err.Error()}
	}
	defer file.Close()
	content, err := readBoundedSiteInput(file)
	if err != nil {
		if err == errSiteRequestTooLarge {
			return "", siteRequestTooLarge("图片")
		}
		return "", &siteError{Code: "invalid_argument", Message: "无法读取图片: " + err.Error()}
	}
	mimeType := http.DetectContentType(content)
	if !strings.HasPrefix(mimeType, "image/") {
		return "", &siteError{Code: "invalid_argument", Message: "图片文件类型无效"}
	}
	return "data:" + mimeType + ";base64," + base64.StdEncoding.EncodeToString(content), nil
}

func (a NativeSite) transportLabRegister(ctx context.Context, args []string, cookie string) (map[string]any, *siteError) {
	if !businessBool(args, "--yes") {
		return nil, &siteError{Code: "confirmation_required", Message: "注册实验室预约平台账号需要 --yes"}
	}
	typeValue, typeErr := businessRequired(args, "--type", "register 必须提供 --type 2、3、4 或 5")
	if typeErr != nil {
		return nil, typeErr
	}
	if typeValue != "2" && typeValue != "3" && typeValue != "4" && typeValue != "5" {
		return nil, &siteError{Code: "invalid_argument", Message: "--type 只能是 2、3、4 或 5"}
	}
	name, nameErr := businessRequired(args, "--name", "register 必须提供 --name")
	if nameErr != nil {
		return nil, nameErr
	}
	sex, sexErr := businessRequired(args, "--sex", "register 必须提供 --sex")
	if sexErr != nil {
		return nil, sexErr
	}
	phone, phoneErr := transportLabPhone(args)
	if phoneErr != nil {
		return nil, phoneErr
	}
	password, passwordErr := businessSecret(args, "--password", "CSUST_TRANSPORT_LAB_PASSWORD")
	if passwordErr != nil {
		return nil, passwordErr
	}
	confirm, confirmErr := businessRequired(args, "--password-confirm", "register 必须提供 --password-confirm")
	if confirmErr != nil {
		return nil, confirmErr
	}
	if password != confirm {
		return nil, &siteError{Code: "invalid_argument", Message: "--password 与 --password-confirm 不一致"}
	}
	question, questionErr := businessRequired(args, "--question", "register 必须提供 --question")
	if questionErr != nil {
		return nil, questionErr
	}
	answer, answerErr := businessRequired(args, "--answer", "register 必须提供 --answer")
	if answerErr != nil {
		return nil, answerErr
	}
	photoPath, photoErr := businessRequired(args, "--photo", "register 必须提供 --photo")
	if photoErr != nil {
		return nil, photoErr
	}
	photo, imageErr := transportLabImage(photoPath)
	if imageErr != nil {
		return nil, imageErr
	}
	cardPath, cardErr := businessRequired(args, "--card", "register 必须提供 --card")
	if cardErr != nil {
		return nil, cardErr
	}
	card, imageErr := transportLabImage(cardPath)
	if imageErr != nil {
		return nil, imageErr
	}
	page, requestErr := a.businessGet(ctx, "transport-lab", "/Login/Register", nil, businessRequestOptions{cookieFile: cookie})
	if requestErr != nil {
		return nil, requestErr
	}
	result, requestErr := businessRequest(ctx, "transport-lab", "POST", "/Login/CheckRegister", nil, []pair{{"usertype", typeValue}, {"fullname", name}, {"usersex", sex}, {"userimage", photo}, {"username", phone}, {"usercard", card}, {"userpwd", password}, {"memberpwdquestion", question}, {"memberpwdanswer", answer}}, []pair{{"X-Requested-With", "XMLHttpRequest"}, {"Accept", "application/json, text/javascript, */*; q=0.01"}, {"Referer", safeResponseURL(page)}}, businessRequestOptions{cookieFile: cookie, allowBusinessFailure: true}, false, true)
	if requestErr != nil {
		return nil, requestErr
	}
	payload, parseErr := businessJSONMap(result)
	if parseErr != nil {
		return nil, parseErr
	}
	state, known := jsonBusinessState(payload)
	if !known || !state {
		return nil, &siteError{Code: "business_rejected", Message: firstNonEmpty(fmt.Sprint(payload["message"]), "实验室预约平台注册失败"), Details: map[string]any{"submitted": true, "confirmed": false, "evidence": "CheckRegister-response"}}
	}
	return map[string]any{"ok": true, "submitted": true, "confirmed": true, "evidence": "CheckRegister-response", "service": "transport-lab", "operation": "register", "username": phone, "user_type": typeValue}, nil
}
