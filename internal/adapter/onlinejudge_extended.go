package adapter

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
)

func (a NativeSite) executeOnlineJudgeExtended(ctx context.Context, operation string, args []string, options businessRequestOptions) (map[string]any, *siteError) {
	switch operation {
	case "profile":
		return a.onlineJudgeProfile(ctx, args, options)
	case "profile-update":
		return a.onlineJudgeProfileUpdate(ctx, args, options)
	case "register":
		return a.onlineJudgeRegister(ctx, args, options)
	case "captcha":
		return a.onlineJudgeCaptcha(ctx, args, options)
	case "tfa-setup":
		return a.onlineJudgeTFASetup(ctx, args, options)
	case "password-reset-request":
		return a.onlineJudgePasswordResetRequest(ctx, args, options)
	case "password-reset":
		return a.onlineJudgePasswordReset(ctx, args, options)
	case "check-account":
		return a.onlineJudgeCheckAccount(ctx, args, options)
	case "refresh-display-id":
		return a.onlineJudgeRefreshDisplayID(ctx, args, options)
	case "avatar-upload":
		return a.onlineJudgeAvatarUpload(ctx, args, options)
	case "change-password":
		return a.onlineJudgeChangePassword(ctx, args, options)
	case "change-email":
		return a.onlineJudgeChangeEmail(ctx, args, options)
	case "tfa-enable", "tfa-disable":
		return a.onlineJudgeTFA(ctx, operation, args, options)
	case "sessions":
		options.require = true
		return a.onlineJudgeRead(ctx, "sessions", "/api/sessions", nil, options)
	case "revoke-session":
		return a.onlineJudgeRevokeSession(ctx, args, options)
	case "rank", "acm-rank", "oi-rank":
		return a.onlineJudgeRank(ctx, operation, args, options)
	case "contest-rank":
		return a.onlineJudgeContestRank(ctx, args, options)
	case "contest-access":
		id, err := businessRequired(args, "--contest-id", "contest-access 必须提供 --contest-id")
		if err != nil {
			return nil, err
		}
		return a.onlineJudgeRead(ctx, "contest-access", "/api/contest/access", []pair{{"contest_id", id}}, options)
	case "contest-password":
		return a.onlineJudgeContestPassword(ctx, args, options)
	case "contest-announcements":
		id, err := businessRequired(args, "--contest-id", "contest-announcements 必须提供 --contest-id")
		if err != nil {
			return nil, err
		}
		return a.onlineJudgeRead(ctx, "contest-announcements", "/api/contest/announcement", []pair{{"contest_id", id}}, options)
	case "contest-problems":
		id, err := businessRequired(args, "--contest-id", "contest-problems 必须提供 --contest-id")
		if err != nil {
			return nil, err
		}
		return a.onlineJudgeRead(ctx, "contest-problems", "/api/contest/problem", []pair{{"contest_id", id}}, options)
	case "contest-problem":
		return a.onlineJudgeContestProblem(ctx, args, options)
	case "contest-submissions":
		return a.onlineJudgeContestSubmissions(ctx, args, options)
	case "questions":
		return a.onlineJudgeQuestions(ctx, args, options)
	case "question":
		id, err := businessRequired(args, "--id", "question 必须提供 --id")
		if err != nil {
			return nil, err
		}
		return a.onlineJudgeRead(ctx, "question", "/api/question", []pair{{"id", id}}, options)
	case "ask-question":
		return a.onlineJudgeAskQuestion(ctx, args, options)
	case "answer-question":
		return a.onlineJudgeAnswerQuestion(ctx, args, options)
	case "pick-one":
		return a.onlineJudgeRead(ctx, "pick-one", "/api/pickone", nil, options)
	case "languages":
		return a.onlineJudgeRead(ctx, "languages", "/api/languages", nil, options)
	case "announcements":
		page, limit, err := onlineJudgePage(args)
		if err != nil {
			return nil, err
		}
		return a.onlineJudgeRead(ctx, "announcements", "/api/announcement", []pair{
			{"offset", strconv.Itoa((page - 1) * limit)}, {"limit", strconv.Itoa(limit)},
		}, options)
	default:
		return nil, &siteError{Code: "invalid_argument", Message: "未知 OnlineJudge 业务操作: " + operation}
	}
}

func (a NativeSite) onlineJudgeProfile(ctx context.Context, args []string, options businessRequestOptions) (map[string]any, *siteError) {
	username := flagValue(args, "--username")
	if username == "" {
		options.require = true
		return a.onlineJudgeRead(ctx, "profile", "/api/profile", nil, options)
	}
	return a.onlineJudgeRead(ctx, "profile", "/api/profile", []pair{{"username", username}}, options)
}

func (a NativeSite) onlineJudgeProfileUpdate(ctx context.Context, args []string, options businessRequestOptions) (map[string]any, *siteError) {
	if !businessBool(args, "--yes") {
		return nil, &siteError{Code: "confirmation_required", Message: "OnlineJudge 资料更新需要 --yes"}
	}
	options.require = true
	fields := map[string]string{
		"--real-name": "real_name", "--mood": "mood", "--major": "major", "--blog": "blog",
		"--school": "school", "--github": "github", "--language": "language",
	}
	body := map[string]string{}
	for flag, name := range fields {
		value, found, err := businessValue(args, flag)
		if err != nil {
			return nil, err
		}
		if found {
			if strings.TrimSpace(value) == "" {
				return nil, &siteError{Code: "invalid_argument", Message: flag + " 不能为空"}
			}
			body[name] = value
		}
	}
	if len(body) == 0 {
		return nil, &siteError{Code: "invalid_argument", Message: "profile-update 至少需要一个资料字段"}
	}
	return a.onlineJudgeJSONMutation(ctx, "profile-update", http.MethodPut, "/api/profile", nil, body, options)
}

func (a NativeSite) onlineJudgeRegister(ctx context.Context, args []string, options businessRequestOptions) (map[string]any, *siteError) {
	if !businessBool(args, "--yes") {
		return nil, &siteError{Code: "confirmation_required", Message: "OnlineJudge 注册账号需要 --yes"}
	}
	username, password, err := businessCredentials(args, "CSUST_ONLINEJUDGE_PASSWORD")
	if err != nil {
		return nil, err
	}
	email, err := businessRequired(args, "--email", "register 必须提供 --email")
	if err != nil {
		return nil, err
	}
	captcha, err := businessRequired(args, "--captcha", "register 必须提供 --captcha")
	if err != nil {
		return nil, err
	}
	return a.onlineJudgeJSONMutation(ctx, "register", http.MethodPost, "/api/register", nil, map[string]string{
		"username": username, "password": password, "email": email, "captcha": captcha,
	}, options)
}

func (a NativeSite) onlineJudgeCaptcha(ctx context.Context, args []string, options businessRequestOptions) (map[string]any, *siteError) {
	output, err := businessRequired(args, "--output", "captcha 必须提供 --output")
	if err != nil {
		return nil, err
	}
	result, requestErr := a.onlineJudgeJSONRequest(ctx, http.MethodGet, "/api/captcha", nil, nil, options, true, true)
	if requestErr != nil {
		return nil, requestErr
	}
	content, decodeErr := onlineJudgeImageData(result, "验证码")
	if decodeErr != nil {
		return nil, decodeErr
	}
	output = expandUserPath(output)
	if writeErr := atomicWrite(output, content); writeErr != nil {
		return nil, writeErr
	}
	return map[string]any{"ok": true, "submitted": false, "confirmed": true, "evidence": "captcha-data-uri-decoded", "service": "onlinejudge", "operation": "captcha", "output": output, "bytes": len(content)}, nil
}

func (a NativeSite) onlineJudgeTFASetup(ctx context.Context, args []string, options businessRequestOptions) (map[string]any, *siteError) {
	output, err := businessRequired(args, "--output", "tfa-setup 必须提供 --output")
	if err != nil {
		return nil, err
	}
	options.require = true
	result, requestErr := a.businessGet(ctx, "onlinejudge", "/api/two_factor_auth", nil, options)
	if requestErr != nil {
		return nil, requestErr
	}
	content, decodeErr := onlineJudgeImageData(result, "TFA 二维码")
	if decodeErr != nil {
		return nil, decodeErr
	}
	output = expandUserPath(output)
	if writeErr := atomicWrite(output, content); writeErr != nil {
		return nil, writeErr
	}
	return map[string]any{"ok": true, "submitted": false, "confirmed": true, "evidence": "tfa-setup-data-uri-decoded", "service": "onlinejudge", "operation": "tfa-setup", "output": output, "bytes": len(content)}, nil
}

func onlineJudgeImageData(result map[string]any, label string) ([]byte, *siteError) {
	payload, _ := businessData(result)
	encoded := findString(payload, "data")
	if encoded == "" {
		return nil, &siteError{Code: "parse_error", Message: "OnlineJudge " + label + "响应缺少图片数据"}
	}
	if comma := strings.IndexByte(encoded, ','); comma >= 0 {
		encoded = encoded[comma+1:]
	}
	content, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, &siteError{Code: "parse_error", Message: "OnlineJudge " + label + "图片解码失败: " + err.Error()}
	}
	return content, nil
}

func (a NativeSite) onlineJudgePasswordResetRequest(ctx context.Context, args []string, options businessRequestOptions) (map[string]any, *siteError) {
	if !businessBool(args, "--yes") {
		return nil, &siteError{Code: "confirmation_required", Message: "OnlineJudge 发送找回密码邮件需要 --yes"}
	}
	email, err := businessRequired(args, "--email", "password-reset-request 必须提供 --email")
	if err != nil {
		return nil, err
	}
	captcha, err := businessRequired(args, "--captcha", "password-reset-request 必须提供 --captcha")
	if err != nil {
		return nil, err
	}
	return a.onlineJudgeJSONMutation(ctx, "password-reset-request", http.MethodPost, "/api/apply_reset_password", nil, map[string]string{"email": email, "captcha": captcha}, options)
}

func (a NativeSite) onlineJudgePasswordReset(ctx context.Context, args []string, options businessRequestOptions) (map[string]any, *siteError) {
	if !businessBool(args, "--yes") {
		return nil, &siteError{Code: "confirmation_required", Message: "OnlineJudge 重置密码需要 --yes"}
	}
	token, err := businessRequired(args, "--token", "password-reset 必须提供 --token")
	if err != nil {
		return nil, err
	}
	captcha, err := businessRequired(args, "--captcha", "password-reset 必须提供 --captcha")
	if err != nil {
		return nil, err
	}
	password, confirmation, err := onlineJudgePasswordPair(args)
	if err != nil {
		return nil, err
	}
	if password != confirmation {
		return nil, &siteError{Code: "invalid_argument", Message: "--new-password 与 --password-confirm 不一致"}
	}
	return a.onlineJudgeJSONMutation(ctx, "password-reset", http.MethodPost, "/api/reset_password", nil, map[string]string{"token": token, "captcha": captcha, "password": password}, options)
}

func onlineJudgePasswordPair(args []string) (string, string, *siteError) {
	if businessBool(args, "--new-password-stdin") && businessBool(args, "--password-confirm-stdin") {
		content, err := readBoundedSiteInput(os.Stdin)
		if err != nil {
			if errors.Is(err, errSiteRequestTooLarge) {
				return "", "", siteRequestTooLarge("标准输入密码")
			}
			return "", "", &siteError{Code: "credentials_required", Message: "无法读取标准输入密码: " + err.Error()}
		}
		lines := strings.Split(strings.TrimRight(string(content), "\r\n"), "\n")
		if len(lines) < 2 || strings.TrimSpace(lines[0]) == "" || strings.TrimSpace(lines[1]) == "" {
			return "", "", &siteError{Code: "credentials_required", Message: "两个密码均使用标准输入时需要两行密码"}
		}
		return lines[0], lines[1], nil
	}
	password, err := businessSecret(args, "--new-password", "CSUST_ONLINEJUDGE_NEW_PASSWORD")
	if err != nil {
		return "", "", err
	}
	confirmation, err := businessSecret(args, "--password-confirm", "CSUST_ONLINEJUDGE_PASSWORD_CONFIRM")
	if err != nil {
		return "", "", err
	}
	if strings.TrimSpace(password) == "" || strings.TrimSpace(confirmation) == "" {
		return "", "", &siteError{Code: "credentials_required", Message: "新密码和确认密码不能为空"}
	}
	return password, confirmation, nil
}

func (a NativeSite) onlineJudgeRefreshDisplayID(ctx context.Context, args []string, options businessRequestOptions) (map[string]any, *siteError) {
	if !businessBool(args, "--yes") {
		return nil, &siteError{Code: "confirmation_required", Message: "OnlineJudge 刷新展示 ID 需要 --yes"}
	}
	options.require = true
	result, err := a.execute(ctx, siteRequest{Service: "onlinejudge", Method: http.MethodGet, Path: "/api/profile/fresh_display_id", Headers: options.headers, CookieFile: options.cookieFile, RequireLogin: true, ReadOnly: false, Yes: true, InsecureTLS: options.insecure, RawJSON: true, AllowBusinessFailure: true, mutating: true})
	if err != nil {
		return nil, err
	}
	if failure := onlineJudgeMutationFailure(result); failure != nil {
		return nil, failure
	}
	return businessResult(result, "onlinejudge", "refresh-display-id"), nil
}

func (a NativeSite) onlineJudgeAvatarUpload(ctx context.Context, args []string, options businessRequestOptions) (map[string]any, *siteError) {
	if !businessBool(args, "--yes") {
		return nil, &siteError{Code: "confirmation_required", Message: "OnlineJudge 上传头像需要 --yes"}
	}
	filePath, err := businessRequired(args, "--file", "avatar-upload 必须提供 --file")
	if err != nil {
		return nil, err
	}
	file, fileErr := siteFilePart("image", filePath, "头像文件")
	if fileErr != nil {
		return nil, fileErr
	}
	if file.size == 0 {
		return nil, &siteError{Code: "invalid_argument", Message: "头像文件不能为空"}
	}
	options.require = true
	result, requestErr := a.execute(ctx, siteRequest{Service: "onlinejudge", Method: http.MethodPost, Path: "/api/upload_avatar", Files: []filePart{file}, Headers: options.headers, CookieFile: options.cookieFile, RequireLogin: true, ReadOnly: false, Yes: true, InsecureTLS: options.insecure, RawJSON: true, AllowBusinessFailure: true})
	if requestErr != nil {
		return nil, requestErr
	}
	if failure := onlineJudgeMutationFailure(result); failure != nil {
		return nil, failure
	}
	profile, profileErr := a.businessGet(ctx, "onlinejudge", "/api/profile", nil, options)
	if profileErr != nil {
		return nil, &siteError{Code: "mutation_unverified", Message: "头像上传请求已发送，但资料回读失败", Details: map[string]any{"submitted": true, "confirmed": false, "evidence": "avatar-profile-readback-error", "cause": profileErr.Code}}
	}
	result = businessResult(result, "onlinejudge", "avatar-upload")
	result["verification"] = businessResult(profile, "onlinejudge", "profile-readback")
	result["evidence"] = "avatar-upload-response-and-profile-readback"
	return result, nil
}

func (a NativeSite) onlineJudgeCheckAccount(ctx context.Context, args []string, options businessRequestOptions) (map[string]any, *siteError) {
	body := map[string]string{}
	for _, item := range []struct{ flag, field string }{{"--username", "username"}, {"--email", "email"}} {
		value, found, err := businessValue(args, item.flag)
		if err != nil {
			return nil, err
		}
		if found {
			if strings.TrimSpace(value) == "" {
				return nil, &siteError{Code: "invalid_argument", Message: item.flag + " 不能为空"}
			}
			body[item.field] = value
		}
	}
	if len(body) == 0 {
		return nil, &siteError{Code: "invalid_argument", Message: "check-account 至少需要 --username 或 --email"}
	}
	return a.onlineJudgeJSONRead(ctx, "check-account", http.MethodPost, "/api/check_username_or_email", nil, body, options)
}

func (a NativeSite) onlineJudgeChangePassword(ctx context.Context, args []string, options businessRequestOptions) (map[string]any, *siteError) {
	if !businessBool(args, "--yes") {
		return nil, &siteError{Code: "confirmation_required", Message: "OnlineJudge 修改密码需要 --yes"}
	}
	options.require = true
	oldPassword, err := businessSecret(args, "--old-password", "CSUST_ONLINEJUDGE_OLD_PASSWORD")
	if err != nil {
		return nil, err
	}
	newPassword, err := businessSecret(args, "--new-password", "CSUST_ONLINEJUDGE_NEW_PASSWORD")
	if err != nil {
		return nil, err
	}
	if oldPassword == newPassword {
		return nil, &siteError{Code: "invalid_argument", Message: "新密码不能与旧密码相同"}
	}
	body := map[string]string{"old_password": oldPassword, "new_password": newPassword}
	if tfa, secretErr := onlineJudgeOptionalSecret(args, "--tfa-code", "CSUST_ONLINEJUDGE_TFA_CODE"); secretErr != nil {
		return nil, secretErr
	} else if tfa != "" {
		body["tfa_code"] = tfa
	}
	return a.onlineJudgeJSONMutation(ctx, "change-password", http.MethodPost, "/api/change_password", nil, body, options)
}

func (a NativeSite) onlineJudgeChangeEmail(ctx context.Context, args []string, options businessRequestOptions) (map[string]any, *siteError) {
	if !businessBool(args, "--yes") {
		return nil, &siteError{Code: "confirmation_required", Message: "OnlineJudge 修改邮箱需要 --yes"}
	}
	options.require = true
	password, err := businessSecret(args, "--password", "CSUST_ONLINEJUDGE_PASSWORD")
	if err != nil {
		return nil, err
	}
	newEmail, err := businessRequired(args, "--new-email", "change-email 必须提供 --new-email")
	if err != nil {
		return nil, err
	}
	oldEmail := flagValue(args, "--old-email")
	if oldEmail == "" {
		profile, profileErr := a.businessGet(ctx, "onlinejudge", "/api/profile", nil, businessRequestOptions{cookieFile: options.cookieFile, insecure: options.insecure, require: true})
		if profileErr != nil {
			return nil, profileErr
		}
		profileData, _ := businessData(profile)
		oldEmail = findString(profileData, "email")
		if oldEmail == "" {
			return nil, &siteError{Code: "invalid_response", Message: "OnlineJudge 当前资料没有可用的旧邮箱"}
		}
	}
	body := map[string]string{"password": password, "old_email": oldEmail, "new_email": newEmail}
	if tfa, secretErr := onlineJudgeOptionalSecret(args, "--tfa-code", "CSUST_ONLINEJUDGE_TFA_CODE"); secretErr != nil {
		return nil, secretErr
	} else if tfa != "" {
		body["tfa_code"] = tfa
	}
	return a.onlineJudgeJSONMutation(ctx, "change-email", http.MethodPost, "/api/change_email", nil, body, options)
}

func (a NativeSite) onlineJudgeTFA(ctx context.Context, operation string, args []string, options businessRequestOptions) (map[string]any, *siteError) {
	if !businessBool(args, "--yes") {
		return nil, &siteError{Code: "confirmation_required", Message: "OnlineJudge TFA 设置需要 --yes"}
	}
	options.require = true
	code, err := businessSecret(args, "--code", "CSUST_ONLINEJUDGE_TFA_CODE")
	if err != nil {
		return nil, err
	}
	method := http.MethodPost
	if operation == "tfa-disable" {
		method = http.MethodPut
	}
	return a.onlineJudgeJSONMutation(ctx, operation, method, "/api/two_factor_auth", nil, map[string]string{"code": code}, options)
}

func (a NativeSite) onlineJudgeRevokeSession(ctx context.Context, args []string, options businessRequestOptions) (map[string]any, *siteError) {
	if !businessBool(args, "--yes") {
		return nil, &siteError{Code: "confirmation_required", Message: "OnlineJudge 撤销会话需要 --yes"}
	}
	options.require = true
	key, err := businessSecret(args, "--session-key", "CSUST_ONLINEJUDGE_SESSION_KEY")
	if err != nil {
		return nil, err
	}
	result, requestErr := businessRequest(ctx, "onlinejudge", http.MethodDelete, "/api/sessions", []pair{{"session_key", key}}, nil, nil, options, false, true)
	if requestErr != nil {
		return nil, requestErr
	}
	if failure := onlineJudgeMutationFailure(result); failure != nil {
		return nil, failure
	}
	sessions, readErr := a.businessGet(ctx, "onlinejudge", "/api/sessions", nil, businessRequestOptions{cookieFile: options.cookieFile, insecure: options.insecure, require: true})
	if readErr != nil {
		return nil, &siteError{Code: "mutation_unverified", Message: "会话撤销请求已发送，但回读会话失败", Details: map[string]any{"submitted": true, "confirmed": false, "evidence": "session-readback-error", "cause": readErr.Code}}
	}
	sessionData, _ := businessData(sessions)
	if onlineJudgeContainsSession(sessionData, key) {
		return nil, &siteError{Code: "mutation_unverified", Message: "会话撤销请求已发送，但目标会话仍在回读列表中", Details: map[string]any{"submitted": true, "confirmed": false, "evidence": "session-still-present"}}
	}
	result = businessResult(result, "onlinejudge", "revoke-session")
	result["verification"] = businessResult(sessions, "onlinejudge", "sessions-readback")
	result["evidence"] = "session-revoked-and-read-back"
	return result, nil
}

func (a NativeSite) onlineJudgeRank(ctx context.Context, operation string, args []string, options businessRequestOptions) (map[string]any, *siteError) {
	page, limit, err := onlineJudgePage(args)
	if err != nil {
		return nil, err
	}
	rule := strings.ToUpper(strings.TrimSpace(flagValue(args, "--rule")))
	if operation == "acm-rank" {
		rule = "ACM"
	} else if operation == "oi-rank" {
		rule = "OI"
	} else if rule == "" {
		rule = "ACM"
	}
	if rule != "ACM" && rule != "OI" {
		return nil, &siteError{Code: "invalid_argument", Message: "--rule 只能是 acm 或 oi"}
	}
	return a.onlineJudgeRead(ctx, "rank", "/api/user_rank", []pair{
		{"offset", strconv.Itoa((page - 1) * limit)}, {"limit", strconv.Itoa(limit)}, {"rule", rule},
	}, options)
}

func (a NativeSite) onlineJudgeContestRank(ctx context.Context, args []string, options businessRequestOptions) (map[string]any, *siteError) {
	id, err := businessRequired(args, "--contest-id", "contest-rank 必须提供 --contest-id")
	if err != nil {
		return nil, err
	}
	page, limit, err := onlineJudgePage(args)
	if err != nil {
		return nil, err
	}
	forceRefresh := "0"
	if businessBool(args, "--force-refresh") {
		forceRefresh = "1"
	}
	return a.onlineJudgeRead(ctx, "contest-rank", "/api/contest_rank", []pair{
		{"offset", strconv.Itoa((page - 1) * limit)}, {"limit", strconv.Itoa(limit)}, {"contest_id", id}, {"force_refresh", forceRefresh},
	}, options)
}

func (a NativeSite) onlineJudgeContestPassword(ctx context.Context, args []string, options businessRequestOptions) (map[string]any, *siteError) {
	id, err := businessRequired(args, "--contest-id", "contest-password 必须提供 --contest-id")
	if err != nil {
		return nil, err
	}
	password, err := businessSecret(args, "--password", "CSUST_ONLINEJUDGE_CONTEST_PASSWORD")
	if err != nil {
		return nil, err
	}
	return a.onlineJudgeJSONRead(ctx, "contest-password", http.MethodPost, "/api/contest/password", nil, map[string]string{
		"contest_id": id, "password": password,
	}, options)
}

func (a NativeSite) onlineJudgeContestProblem(ctx context.Context, args []string, options businessRequestOptions) (map[string]any, *siteError) {
	contestID, err := businessRequired(args, "--contest-id", "contest-problem 必须提供 --contest-id")
	if err != nil {
		return nil, err
	}
	problemID, err := businessRequired(args, "--problem-id", "contest-problem 必须提供 --problem-id")
	if err != nil {
		return nil, err
	}
	return a.onlineJudgeRead(ctx, "contest-problem", "/api/contest/problem", []pair{{"contest_id", contestID}, {"problem_id", problemID}}, options)
}

func (a NativeSite) onlineJudgeContestSubmissions(ctx context.Context, args []string, options businessRequestOptions) (map[string]any, *siteError) {
	contestID, err := businessRequired(args, "--contest-id", "contest-submissions 必须提供 --contest-id")
	if err != nil {
		return nil, err
	}
	page, limit, err := onlineJudgePage(args)
	if err != nil {
		return nil, err
	}
	params := []pair{{"contest_id", contestID}, {"page", strconv.Itoa(page)}, {"limit", strconv.Itoa(limit)}, {"offset", strconv.Itoa((page - 1) * limit)}}
	for _, item := range []struct{ flag, name string }{{"--username", "username"}, {"--problem-id", "problem_id"}, {"--language", "language"}, {"--result", "result"}} {
		if value := flagValue(args, item.flag); value != "" {
			params = append(params, pair{item.name, value})
		}
	}
	if businessBool(args, "--myself") {
		params = append(params, pair{"myself", "1"})
	}
	return a.onlineJudgeRead(ctx, "contest-submissions", "/api/contest_submissions", params, options)
}

func (a NativeSite) onlineJudgeQuestions(ctx context.Context, args []string, options businessRequestOptions) (map[string]any, *siteError) {
	page, limit, err := onlineJudgePage(args)
	if err != nil {
		return nil, err
	}
	params := []pair{{"page", strconv.Itoa(page)}, {"limit", strconv.Itoa(limit)}, {"offset", strconv.Itoa((page - 1) * limit)}}
	if businessBool(args, "--myself") {
		params = append(params, pair{"myself", "1"})
	} else {
		params = append(params, pair{"myself", "0"})
	}
	for _, item := range []struct{ flag, name string }{{"--solved", "solved"}, {"--username", "username"}, {"--problem-id", "problem_id"}} {
		if value := flagValue(args, item.flag); value != "" {
			params = append(params, pair{item.name, value})
		}
	}
	path := "/api/questions"
	if contestID := flagValue(args, "--contest-id"); contestID != "" {
		params = append(params, pair{"contest_id", contestID})
		path = "/api/contest_questions"
	}
	return a.onlineJudgeRead(ctx, "questions", path, params, options)
}

func (a NativeSite) onlineJudgeAskQuestion(ctx context.Context, args []string, options businessRequestOptions) (map[string]any, *siteError) {
	if !businessBool(args, "--yes") {
		return nil, &siteError{Code: "confirmation_required", Message: "OnlineJudge 提问需要 --yes"}
	}
	options.require = true
	problemID, err := businessRequired(args, "--problem-id", "ask-question 必须提供 --problem-id")
	if err != nil {
		return nil, err
	}
	content, err := onlineJudgeTextInput(args, "--content", "提问内容")
	if err != nil {
		return nil, err
	}
	body := map[string]string{"problem_id": problemID, "text": content}
	if contestID := flagValue(args, "--contest-id"); contestID != "" {
		body["contest_id"] = contestID
	}
	result, mutationErr := a.onlineJudgeJSONMutation(ctx, "ask-question", http.MethodPost, "/api/question", nil, body, options)
	if mutationErr != nil {
		return nil, mutationErr
	}
	questionID := onlineJudgeQuestionID(result["data"])
	if questionID == "" {
		return nil, &siteError{Code: "mutation_unverified", Message: "提问已发送但未返回可回读的问题编号", Details: map[string]any{"submitted": true, "confirmed": false, "evidence": "question-id-missing"}}
	}
	verification, verifyErr := a.onlineJudgeQuestionReadback(ctx, questionID, options)
	if verifyErr != nil {
		return nil, verifyErr
	}
	result["question_id"] = questionID
	result["verification"] = verification
	result["evidence"] = "question-created-and-read-back"
	return result, nil
}

func (a NativeSite) onlineJudgeAnswerQuestion(ctx context.Context, args []string, options businessRequestOptions) (map[string]any, *siteError) {
	if !businessBool(args, "--yes") {
		return nil, &siteError{Code: "confirmation_required", Message: "OnlineJudge 回答问题需要 --yes"}
	}
	options.require = true
	id, err := businessRequired(args, "--id", "answer-question 必须提供 --id")
	if err != nil {
		return nil, err
	}
	answer, err := onlineJudgeTextInput(args, "--answer", "回答内容")
	if err != nil {
		return nil, err
	}
	result, mutationErr := a.onlineJudgeJSONMutation(ctx, "answer-question", http.MethodPut, "/api/question_answer", nil, map[string]string{
		"id": id, "answer": answer, "solved": "True",
	}, options)
	if mutationErr != nil {
		return nil, mutationErr
	}
	verification, verifyErr := a.onlineJudgeQuestionReadback(ctx, id, options)
	if verifyErr != nil {
		return nil, verifyErr
	}
	result["question_id"] = id
	result["verification"] = verification
	result["evidence"] = "question-answer-submitted-and-read-back"
	return result, nil
}

func (a NativeSite) onlineJudgeQuestionReadback(ctx context.Context, id string, options businessRequestOptions) (map[string]any, *siteError) {
	verification, err := a.businessGet(ctx, "onlinejudge", "/api/question", []pair{{"id", id}}, options)
	if err != nil {
		return nil, &siteError{Code: "mutation_unverified", Message: "问题操作已发送但回读问题失败", Details: map[string]any{"submitted": true, "confirmed": false, "evidence": "question-readback-error", "cause": err.Code}}
	}
	verificationData, _ := businessData(verification)
	if got := onlineJudgeQuestionID(verificationData); got != "" && got != id {
		return nil, &siteError{Code: "mutation_unverified", Message: "问题操作已发送但回读编号不一致", Details: map[string]any{"submitted": true, "confirmed": false, "evidence": "question-readback-identity-mismatch"}}
	}
	return businessResult(verification, "onlinejudge", "question-readback"), nil
}

func (a NativeSite) onlineJudgeJSONRead(ctx context.Context, operation, method, path string, params []pair, body any, options businessRequestOptions) (map[string]any, *siteError) {
	result, err := a.onlineJudgeJSONRequest(ctx, method, path, params, body, options, true, true)
	if err != nil {
		return nil, err
	}
	return businessResult(result, "onlinejudge", operation), nil
}

func (a NativeSite) onlineJudgeJSONMutation(ctx context.Context, operation, method, path string, params []pair, body any, options businessRequestOptions) (map[string]any, *siteError) {
	options.allowBusinessFailure = true
	result, err := a.onlineJudgeJSONRequest(ctx, method, path, params, body, options, false, true)
	if err != nil {
		return nil, err
	}
	if failure := onlineJudgeMutationFailure(result); failure != nil {
		return nil, failure
	}
	return businessResult(result, "onlinejudge", operation), nil
}

func (a NativeSite) onlineJudgeJSONRequest(ctx context.Context, method, path string, params []pair, body any, options businessRequestOptions, readOnly, yes bool) (map[string]any, *siteError) {
	return a.execute(ctx, siteRequest{
		Service: "onlinejudge", Method: method, Path: path, Params: params, JSON: body, HasJSON: body != nil,
		Headers: options.headers, CookieFile: options.cookieFile, AllowBusinessFailure: options.allowBusinessFailure,
		RequireLogin: options.require, ReadOnly: readOnly, Yes: yes, InsecureTLS: options.insecure, RawJSON: true,
	})
}

func onlineJudgeMutationFailure(result map[string]any) *siteError {
	value, ok := businessData(result)
	if !ok {
		return nil
	}
	text := strings.ToLower(fmt.Sprint(value))
	if strings.Contains(text, "tfa_required") || strings.Contains(text, "two factor") {
		return &siteError{Code: "tfa_required", Message: "OnlineJudge 操作需要 TFA 验证码", Details: map[string]any{"submitted": true, "confirmed": false, "evidence": "api-error"}}
	}
	if state, known := jsonBusinessState(value); known && !state {
		return &siteError{Code: "business_rejected", Message: "OnlineJudge 业务操作被远端拒绝", Details: map[string]any{"submitted": true, "confirmed": false, "evidence": "api-error"}}
	}
	return nil
}

func onlineJudgePage(args []string) (int, int, *siteError) {
	page, err := businessInt(args, "--page", 1)
	if err != nil {
		return 0, 0, err
	}
	limit, err := businessInt(args, "--limit", 20)
	if err != nil {
		return 0, 0, err
	}
	return page, limit, nil
}

func onlineJudgeOptionalSecret(args []string, flag, envName string) (string, *siteError) {
	if value, found, err := businessValue(args, flag); err != nil {
		return "", err
	} else if found {
		return value, nil
	}
	if businessBool(args, flag+"-stdin") {
		value, err := readBoundedSiteInput(os.Stdin)
		if err != nil {
			if errors.Is(err, errSiteRequestTooLarge) {
				return "", siteRequestTooLarge("标准输入秘密")
			}
			return "", &siteError{Code: "credentials_required", Message: "无法读取标准输入秘密: " + err.Error()}
		}
		return strings.TrimRight(string(value), "\r\n"), nil
	}
	return os.Getenv(envName), nil
}

func onlineJudgeTextInput(args []string, flag, label string) (string, *siteError) {
	value, found, err := businessValue(args, flag)
	if err != nil {
		return "", err
	}
	if !found && businessBool(args, flag+"-stdin") {
		content, readErr := readBoundedSiteInput(os.Stdin)
		if readErr != nil {
			if errors.Is(readErr, errSiteRequestTooLarge) {
				return "", siteRequestTooLarge(label)
			}
			return "", &siteError{Code: "invalid_argument", Message: "无法读取" + label + ": " + readErr.Error()}
		}
		value = string(content)
		found = true
	}
	if !found {
		return "", &siteError{Code: "invalid_argument", Message: flag + " 必须提供"}
	}
	if value == "-" {
		content, readErr := readBoundedSiteInput(os.Stdin)
		if readErr != nil {
			if errors.Is(readErr, errSiteRequestTooLarge) {
				return "", siteRequestTooLarge(label)
			}
			return "", &siteError{Code: "invalid_argument", Message: "无法读取" + label + ": " + readErr.Error()}
		}
		value = string(content)
	} else if strings.HasPrefix(value, "@") {
		file, openErr := openSiteInput(expandUserPath(strings.TrimPrefix(value, "@")))
		if openErr != nil {
			return "", &siteError{Code: "invalid_argument", Message: "无法读取" + label + ": " + openErr.Error()}
		}
		content, readErr := readBoundedSiteInput(file)
		_ = file.Close()
		if readErr != nil {
			if errors.Is(readErr, errSiteRequestTooLarge) {
				return "", siteRequestTooLarge(label)
			}
			return "", &siteError{Code: "invalid_argument", Message: "无法读取" + label + ": " + readErr.Error()}
		}
		value = string(content)
	}
	if strings.TrimSpace(value) == "" {
		return "", &siteError{Code: "invalid_argument", Message: label + "不能为空"}
	}
	return value, nil
}

func onlineJudgeQuestionID(value any) string {
	var found string
	var visit func(any)
	visit = func(item any) {
		if found != "" {
			return
		}
		switch typed := item.(type) {
		case map[string]any:
			for _, key := range []string{"question_id", "id"} {
				if value, ok := typed[key]; ok {
					switch value := value.(type) {
					case string:
						found = value
					case float64:
						found = strconv.FormatInt(int64(value), 10)
					}
				}
				if found != "" {
					return
				}
			}
			for _, child := range typed {
				visit(child)
			}
		case []any:
			for _, child := range typed {
				visit(child)
			}
		}
	}
	visit(value)
	return found
}

func onlineJudgeContainsSession(value any, key string) bool {
	switch typed := value.(type) {
	case map[string]any:
		if session, ok := typed["session_key"].(string); ok && session == key {
			return true
		}
		for _, child := range typed {
			if onlineJudgeContainsSession(child, key) {
				return true
			}
		}
	case []any:
		for _, child := range typed {
			if onlineJudgeContainsSession(child, key) {
				return true
			}
		}
	}
	return false
}
