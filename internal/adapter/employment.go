package adapter

import (
	"context"
	"crypto/aes"
	"encoding/base64"
	"fmt"
	"regexp"
	"strings"
)

var employmentRealIPPattern = regexp.MustCompile(`(?m)\bvar\s+real_ip\s*=\s*["']([^"']+)["']`)
var employmentEmailPattern = regexp.MustCompile(`^[a-zA-Z0-9]+([-_.][A-Za-z\d]+)*@([a-zA-Z0-9]+[-.])+[A-Za-z\d]{2,5}$`)

func employmentLoginPage(body string) bool {
	return strings.Contains(body, `id="user_name"`) && strings.Contains(body, `id="password"`)
}

func employmentRealIP(body string) string {
	match := employmentRealIPPattern.FindStringSubmatch(body)
	if len(match) == 2 {
		return match[1]
	}
	return ""
}

func (a NativeSite) employmentStatus(ctx context.Context, cookie string) (map[string]any, *siteError) {
	result, requestErr := a.businessGet(ctx, "employment", "/student", nil, businessRequestOptions{cookieFile: cookie})
	if requestErr != nil {
		return nil, requestErr
	}
	loggedIn := !employmentLoginPage(businessBody(result))
	result = sitePageResult(result)
	result["ok"], result["submitted"], result["confirmed"] = true, false, true
	result["evidence"] = "student-page-login-probe"
	result["service"], result["operation"], result["logged_in"] = "employment", "status", loggedIn
	return result, nil
}

func employmentPasswordLevel(password string) string {
	if len(password) == 8 {
		return "1"
	}
	if len(password) > 8 && len(password) <= 16 {
		if strings.ContainsAny(password, "!#@*&.") {
			return "3"
		}
		return "2"
	}
	return "1"
}

func employmentEncrypt(message, key string) (string, *siteError) {
	block, err := aes.NewCipher([]byte(key))
	if err != nil {
		return "", &siteError{Code: "protocol_error", Message: "就业平台加密密钥长度无效"}
	}
	plain := []byte(message)
	padding := aes.BlockSize - len(plain)%aes.BlockSize
	plain = append(plain, strings.Repeat(string(rune(padding)), padding)...)
	encrypted := make([]byte, len(plain))
	for offset := 0; offset < len(plain); offset += aes.BlockSize {
		block.Encrypt(encrypted[offset:offset+aes.BlockSize], plain[offset:offset+aes.BlockSize])
	}
	return base64.StdEncoding.EncodeToString(encrypted), nil
}

func (a NativeSite) employmentViCode(ctx context.Context, cookie string) (string, *siteError) {
	result, requestErr := a.businessGet(ctx, "employment", "/login/get_vi_code", nil, businessRequestOptions{cookieFile: cookie, allowBusinessFailure: true})
	if requestErr != nil {
		return "", requestErr
	}
	payload, parseErr := businessJSONMap(result)
	if parseErr != nil {
		return "", parseErr
	}
	if fmt.Sprint(payload["code"]) != "1" {
		return "", &siteError{Code: "protocol_error", Message: firstNonEmpty(fmt.Sprint(payload["msg"]), "就业平台未返回加密密钥")}
	}
	data, ok := payload["data"].(map[string]any)
	if !ok || strings.TrimSpace(fmt.Sprint(data["vi_code"])) == "" {
		return "", &siteError{Code: "protocol_error", Message: "就业平台加密密钥响应无效"}
	}
	return fmt.Sprint(data["vi_code"]), nil
}

func (a NativeSite) employmentLogin(ctx context.Context, args []string, cookie string) (map[string]any, *siteError) {
	account, password, credentialErr := businessCredentials(args, "CSUST_EMPLOYMENT_PASSWORD")
	if credentialErr != nil {
		return nil, credentialErr
	}
	captcha := strings.TrimSpace(flagValue(args, "--captcha"))
	captchaToken := strings.TrimSpace(flagValue(args, "--captcha-token"))
	if captcha == "" || captchaToken == "" {
		return nil, &siteError{Code: "captcha_required", Message: "就业平台登录需要行为验证码，请提供 --captcha 和 --captcha-token", Details: map[string]any{"submitted": false, "confirmed": false, "evidence": "behavioral-captcha-input"}}
	}
	page, requestErr := a.businessGet(ctx, "employment", "/login", nil, businessRequestOptions{cookieFile: cookie})
	if requestErr != nil {
		return nil, requestErr
	}
	realIP := employmentRealIP(businessBody(page))
	if realIP == "" {
		return nil, &siteError{Code: "protocol_error", Message: "就业平台登录页缺少 real_ip 参数"}
	}
	viCode, viCodeErr := a.employmentViCode(ctx, cookie)
	if viCodeErr != nil {
		return nil, viCodeErr
	}
	encodedPassword, encryptErr := employmentEncrypt(password, viCode)
	if encryptErr != nil {
		return nil, encryptErr
	}
	tokenResult, requestErr := businessRequest(ctx, "employment", "POST", "/login/get_encode_token", nil, []pair{{"code", encodedPassword}, {"encrypt", "1"}}, []pair{{"Referer", safeResponseURL(page)}}, businessRequestOptions{cookieFile: cookie, allowBusinessFailure: true}, true, true)
	if requestErr != nil {
		return nil, requestErr
	}
	tokenPayload, parseErr := businessJSONMap(tokenResult)
	if parseErr != nil {
		return nil, parseErr
	}
	if fmt.Sprint(tokenPayload["code"]) != "1" {
		return nil, &siteError{Code: "authentication_failed", Message: firstNonEmpty(fmt.Sprint(tokenPayload["msg"]), "就业平台登录加密失败"), Details: map[string]any{"submitted": false, "confirmed": false, "evidence": "get_encode_token-response"}}
	}
	encodeToken := strings.TrimSpace(fmt.Sprint(tokenPayload["data"]))
	if encodeToken == "" {
		return nil, &siteError{Code: "protocol_error", Message: "就业平台登录缺少一次性令牌"}
	}
	encodedAccount, encryptErr := employmentEncrypt(account, viCode)
	if encryptErr != nil {
		return nil, encryptErr
	}
	emptyCode, encryptErr := employmentEncrypt("", viCode)
	if encryptErr != nil {
		return nil, encryptErr
	}
	address, encryptErr := employmentEncrypt(realIP, viCode)
	if encryptErr != nil {
		return nil, encryptErr
	}
	loginResult, requestErr := businessRequest(ctx, "employment", "POST", "/login/submit", nil, []pair{
		{"user_name", encodedAccount}, {"mm", encodedPassword}, {"valid_code", emptyCode},
		{"real_address", address}, {"encrypt", "1"}, {"captcha_token", captchaToken}, {"captcha", captcha},
		{"password_level", employmentPasswordLevel(password)}, {"open_id", ""},
	}, []pair{{"Referer", safeResponseURL(page)}, {"t", encodeToken}}, businessRequestOptions{cookieFile: cookie, allowBusinessFailure: true}, true, true)
	if requestErr != nil {
		return nil, requestErr
	}
	loginPayload, parseErr := businessJSONMap(loginResult)
	if parseErr != nil {
		return nil, parseErr
	}
	if fmt.Sprint(loginPayload["code"]) != "1" {
		message := firstNonEmpty(fmt.Sprint(loginPayload["msg"]), "就业平台登录失败")
		code := "authentication_failed"
		if strings.Contains(message, "验证码") {
			code = "captcha_failed"
		}
		return nil, &siteError{Code: code, Message: message, Details: map[string]any{"submitted": true, "confirmed": false, "evidence": "login-submit-response"}}
	}
	if data, ok := loginPayload["data"].(map[string]any); ok {
		if _, requiresEmail := data["student_login_email"]; requiresEmail {
			details := map[string]any{"submitted": true, "confirmed": false, "evidence": "login-submit-email-verification", "next_operations": []string{"send-email-code", "verify-email"}}
			if email := strings.TrimSpace(fmt.Sprint(data["student_login_email"])); email != "" && email != "<nil>" {
				details["email_hint"] = employmentEmailHint(email)
			}
			return nil, &siteError{Code: "email_verification_required", Message: "就业平台要求邮箱二次验证，请先发送邮箱验证码，再执行 verify-email", Details: details}
		}
	}
	evidence, probeErr := a.confirmBusinessLogin(ctx, "employment", "/student", cookie, employmentLoginPage)
	if probeErr != nil {
		return nil, probeErr
	}
	return map[string]any{"ok": true, "submitted": true, "confirmed": true, "evidence": "get_encode_token-login-submit-and-" + evidence, "service": "employment", "operation": "login", "username": account}, nil
}

func employmentEmailValue(args []string) (string, *siteError) {
	email, requiredErr := businessRequired(args, "--email", "必须提供 --email")
	if requiredErr != nil {
		return "", requiredErr
	}
	email = strings.TrimSpace(email)
	if !employmentEmailPattern.MatchString(email) {
		return "", &siteError{Code: "invalid_argument", Message: "--email 格式无效"}
	}
	return email, nil
}

func employmentEmailHint(email string) string {
	at := strings.LastIndex(email, "@")
	if at <= 0 || at == len(email)-1 {
		return "<redacted>"
	}
	local := email[:at]
	if len(local) == 1 {
		return "*@" + email[at+1:]
	}
	if len(local) == 2 {
		return local[:1] + "*@" + email[at+1:]
	}
	return local[:1] + "***" + local[len(local)-1:] + email[at:]
}

func (a NativeSite) employmentSendEmailCode(ctx context.Context, args []string, cookie string) (map[string]any, *siteError) {
	if !businessBool(args, "--yes") {
		return nil, &siteError{Code: "confirmation_required", Message: "发送就业平台邮箱验证码需要 --yes"}
	}
	email, emailErr := employmentEmailValue(args)
	if emailErr != nil {
		return nil, emailErr
	}
	viCode, viCodeErr := a.employmentViCode(ctx, cookie)
	if viCodeErr != nil {
		return nil, viCodeErr
	}
	encrypted, encryptErr := employmentEncrypt(email, viCode)
	if encryptErr != nil {
		return nil, encryptErr
	}
	result, requestErr := businessRequest(ctx, "employment", "POST", "/login/send_mail_code", nil, []pair{{"mail", encrypted}}, nil, businessRequestOptions{cookieFile: cookie, allowBusinessFailure: true}, true, true)
	if requestErr != nil {
		return nil, requestErr
	}
	payload, parseErr := businessJSONMap(result)
	if parseErr != nil {
		return nil, parseErr
	}
	if fmt.Sprint(payload["code"]) != "1" {
		return nil, &siteError{Code: "business_rejected", Message: firstNonEmpty(fmt.Sprint(payload["msg"]), "邮箱验证码发送失败"), Details: map[string]any{"submitted": true, "confirmed": false, "evidence": "send-mail-code-response"}}
	}
	return map[string]any{"ok": true, "submitted": true, "confirmed": true, "evidence": "send-mail-code-response", "service": "employment", "operation": "send-email-code", "email_hint": employmentEmailHint(email)}, nil
}

func (a NativeSite) employmentVerifyEmail(ctx context.Context, args []string, cookie string) (map[string]any, *siteError) {
	email, emailErr := employmentEmailValue(args)
	if emailErr != nil {
		return nil, emailErr
	}
	emailCode, codeErr := businessRequired(args, "--email-code", "verify-email 必须提供 --email-code")
	if codeErr != nil {
		return nil, codeErr
	}
	result, requestErr := businessRequest(ctx, "employment", "POST", "/login/check_mail_login", nil, []pair{{"mail", email}, {"mail_code", strings.TrimSpace(emailCode)}}, nil, businessRequestOptions{cookieFile: cookie, allowBusinessFailure: true}, true, true)
	if requestErr != nil {
		return nil, requestErr
	}
	payload, parseErr := businessJSONMap(result)
	if parseErr != nil {
		return nil, parseErr
	}
	if fmt.Sprint(payload["code"]) != "1" {
		return nil, &siteError{Code: "authentication_failed", Message: firstNonEmpty(fmt.Sprint(payload["msg"]), "就业平台邮箱验证失败"), Details: map[string]any{"submitted": true, "confirmed": false, "evidence": "check-mail-login-response"}}
	}
	evidence, probeErr := a.confirmBusinessLogin(ctx, "employment", "/student", cookie, employmentLoginPage)
	if probeErr != nil {
		return nil, probeErr
	}
	return map[string]any{"ok": true, "submitted": true, "confirmed": true, "evidence": "check-mail-login-and-" + evidence, "service": "employment", "operation": "verify-email", "email_hint": employmentEmailHint(email), "logged_in": true}, nil
}
