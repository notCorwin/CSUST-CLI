package adapter

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"
)

type teachingPasswordResetOptions struct {
	method, username, email, code, captcha, captchaImage, cookie string
	questionOne, questionTwo, questionThree                      string
	answerOne, answerTwo, answerThree                            string
	newPassword, passwordConfirm                                 string
	passwordStdin, yes                                           bool
}

func (a NativeSite) teachingPasswordReset(ctx context.Context, args []string) (map[string]any, *siteError) {
	opts, parseErr := parseTeachingPasswordResetOptions(args)
	if parseErr != nil {
		return nil, parseErr
	}
	if !opts.yes {
		return nil, &siteError{Code: "confirmation_required", Message: "找回或重置网络教学平台密码必须加 --yes"}
	}
	method := teachingPasswordResetMethod(opts.method)
	if method == "" {
		return nil, &siteError{Code: "invalid_argument", Message: "--method 只能是 email 或 question"}
	}
	if opts.code != "" {
		newPassword, confirm, passwordErr := teachingPasswordPair(opts)
		if passwordErr != nil {
			return nil, passwordErr
		}
		return a.teachingPasswordResetSubmit(ctx, method, opts.code, newPassword, confirm, opts.cookie)
	}
	if method == "email" {
		if opts.username == "" || opts.email == "" {
			return nil, &siteError{Code: "invalid_argument", Message: "邮箱找回必须提供 --username 和 --email"}
		}
		if opts.newPassword != "" || opts.passwordConfirm != "" || opts.passwordStdin {
			return nil, &siteError{Code: "invalid_argument", Message: "邮箱发信阶段不能提供新密码；收到邮件后用 --code 重试"}
		}
		captcha, captchaErr := a.teachingPasswordCaptcha(ctx, opts.captcha, opts.captchaImage, opts.cookie)
		if captchaErr != nil {
			return nil, captchaErr
		}
		result, requestErr := businessRequest(ctx, "theol", "POST", "/meol/findPasswdMailBoxAccount.do", nil, []pair{
			{"username", opts.username}, {"email", opts.email}, {"imgcode", captcha},
		}, nil, businessRequestOptions{cookieFile: opts.cookie, allowBusinessFailure: true}, true, true)
		if requestErr != nil {
			return nil, requestErr
		}
		status := teachingPasswordBodyCode(result)
		if status != "10000" {
			return nil, teachingPasswordResetFailure("findPasswdMailBoxAccount.do", status, false)
		}
		return map[string]any{
			"ok": true, "submitted": true, "confirmed": true,
			"evidence": "findPasswdMailBoxAccount.do 返回 10000",
			"service":  teachingServiceName, "operation": "password-reset-request", "method": "email",
			"pending": true, "next": "email-link",
		}, nil
	}

	newPassword, confirm, passwordErr := teachingPasswordPair(opts)
	if passwordErr != nil {
		return nil, passwordErr
	}
	if opts.username == "" {
		return nil, &siteError{Code: "invalid_argument", Message: "密保找回必须提供 --username"}
	}
	questions, questionsErr := a.teachingPasswordQuestionList(ctx, opts.cookie)
	if questionsErr != nil {
		return nil, questionsErr
	}
	questionOne, questionErr := teachingPasswordQuestionID(questions, opts.questionOne, "--question-one")
	if questionErr != nil {
		return nil, questionErr
	}
	questionTwo, questionErr := teachingPasswordQuestionID(questions, opts.questionTwo, "--question-two")
	if questionErr != nil {
		return nil, questionErr
	}
	questionThree, questionErr := teachingPasswordQuestionID(questions, opts.questionThree, "--question-three")
	if questionErr != nil {
		return nil, questionErr
	}
	if questionOne == questionTwo || questionOne == questionThree || questionTwo == questionThree {
		return nil, &siteError{Code: "invalid_argument", Message: "三个密保问题必须互不相同"}
	}
	captcha, captchaErr := a.teachingPasswordCaptcha(ctx, opts.captcha, opts.captchaImage, opts.cookie)
	if captchaErr != nil {
		return nil, captchaErr
	}
	verification, requestErr := businessRequest(ctx, "theol", "POST", "/meol/findPasswdQuestionAccount.do", nil, []pair{
		{"username", opts.username},
		{"questionId", questionOne}, {"questionVal", url.QueryEscape(opts.answerOne)},
		{"questionId2", questionTwo}, {"questionVal2", url.QueryEscape(opts.answerTwo)},
		{"questionId3", questionThree}, {"questionVal3", url.QueryEscape(opts.answerThree)},
		{"imgcode", captcha},
	}, nil, businessRequestOptions{cookieFile: opts.cookie, allowBusinessFailure: true}, true, true)
	if requestErr != nil {
		return nil, requestErr
	}
	payload, payloadErr := businessJSONMap(verification)
	if payloadErr != nil {
		return nil, payloadErr
	}
	if status := teachingPasswordValueCode(payload["status"]); status != "10000" {
		return nil, teachingPasswordResetFailure("findPasswdQuestionAccount.do", status, false)
	}
	resetCode := strings.TrimSpace(fmt.Sprint(payload["code"]))
	if resetCode == "" || resetCode == "<nil>" {
		return nil, &siteError{Code: "parse_error", Message: "密保验证成功响应缺少重置凭证"}
	}
	return a.teachingPasswordResetSubmit(ctx, "question", resetCode, newPassword, confirm, opts.cookie)
}

func parseTeachingPasswordResetOptions(args []string) (teachingPasswordResetOptions, *siteError) {
	opts := teachingPasswordResetOptions{method: "email"}
	for index := 0; index < len(args); index++ {
		arg, value, inline := splitInline(args[index])
		if arg == "--json" {
			continue
		}
		if arg == "--yes" {
			opts.yes = true
			continue
		}
		if arg == "--password-stdin" {
			opts.passwordStdin = true
			continue
		}
		if !inline {
			if index+1 >= len(args) || strings.HasPrefix(args[index+1], "--") {
				return teachingPasswordResetOptions{}, &siteError{Code: "invalid_argument", Message: arg + " 缺少参数值"}
			}
			index++
			value = args[index]
		}
		switch arg {
		case "--method":
			opts.method = value
		case "--username":
			opts.username = value
		case "--email":
			opts.email = value
		case "--code":
			opts.code = value
		case "--captcha":
			opts.captcha = value
		case "--captcha-image":
			opts.captchaImage = value
		case "--cookie-file":
			opts.cookie = value
		case "--question-one":
			opts.questionOne = value
		case "--question-two":
			opts.questionTwo = value
		case "--question-three":
			opts.questionThree = value
		case "--answer-one":
			opts.answerOne = value
		case "--answer-two":
			opts.answerTwo = value
		case "--answer-three":
			opts.answerThree = value
		case "--new-password":
			opts.newPassword = value
		case "--password-confirm":
			opts.passwordConfirm = value
		default:
			return teachingPasswordResetOptions{}, &siteError{Code: "invalid_argument", Message: "teaching password-reset 参数无效: " + arg}
		}
	}
	if opts.newPassword != "" && opts.passwordStdin || opts.passwordConfirm != "" && opts.passwordStdin {
		return teachingPasswordResetOptions{}, &siteError{Code: "invalid_argument", Message: "密码不能同时使用命令行参数和 --password-stdin"}
	}
	return opts, nil
}

func teachingPasswordResetMethod(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "email", "mail", "邮箱":
		return "email"
	case "question", "security-question", "密保", "密保问题":
		return "question"
	default:
		return ""
	}
}

func teachingPasswordPair(opts teachingPasswordResetOptions) (string, string, *siteError) {
	newPassword, confirm := opts.newPassword, opts.passwordConfirm
	if opts.passwordStdin {
		content, err := readBoundedSiteInput(os.Stdin)
		if err != nil {
			if err == errSiteRequestTooLarge {
				return "", "", siteRequestTooLarge("标准输入密码")
			}
			return "", "", &siteError{Code: "credentials_required", Message: "无法读取标准输入密码: " + err.Error()}
		}
		lines := strings.Split(strings.TrimRight(string(content), "\r\n"), "\n")
		if len(lines) < 2 || strings.TrimSpace(lines[0]) == "" || strings.TrimSpace(lines[1]) == "" {
			return "", "", &siteError{Code: "credentials_required", Message: "--password-stdin 需要两行新密码和确认密码"}
		}
		newPassword, confirm = lines[0], lines[1]
	}
	if strings.TrimSpace(newPassword) == "" || strings.TrimSpace(confirm) == "" {
		return "", "", &siteError{Code: "credentials_required", Message: "必须提供 --new-password 和 --password-confirm，或使用 --password-stdin"}
	}
	if newPassword != confirm {
		return "", "", &siteError{Code: "invalid_argument", Message: "新密码和确认密码不一致"}
	}
	if !teachingPasswordValid(newPassword) {
		return "", "", &siteError{Code: "invalid_argument", Message: "密码必须为 8-20 个字符，且包含字母、数字和特殊字符；不能含空白及 &<> 引号"}
	}
	return newPassword, confirm, nil
}

func teachingPasswordValid(value string) bool {
	runes := []rune(value)
	if len(runes) < 8 || len(runes) > 20 {
		return false
	}
	letter, digit, special := false, false, false
	for _, char := range runes {
		if unicode.IsSpace(char) || strings.ContainsRune("&<>\"'", char) {
			return false
		}
		switch {
		case char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z':
			letter = true
		case char >= '0' && char <= '9':
			digit = true
		default:
			special = true
		}
	}
	return letter && digit && special
}

func (a NativeSite) teachingPasswordCaptcha(ctx context.Context, value, image, cookie string) (string, *siteError) {
	if strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value), nil
	}
	_, cookiePath, resolveErr := resolveSite(siteRequest{Service: "theol", CookieFile: cookie})
	if resolveErr != nil {
		return "", resolveErr
	}
	if strings.TrimSpace(image) == "" {
		image = filepath.Join(filepath.Dir(cookiePath), "theol-password-reset-captcha.png")
	}
	image = expandUserPath(image)
	if _, requestErr := (NativeSite{}).execute(ctx, siteRequest{
		Service: "theol", Method: "GET", Path: "/meol/getCaptcha.do",
		Params: []pair{{"t", fmt.Sprint(time.Now().UnixNano())}}, CookieFile: cookie,
		Output: image, ReadOnly: true, Yes: true,
	}); requestErr != nil {
		return "", requestErr
	}
	return "", &siteError{Code: "captcha_required", Message: "网络教学平台找回密码需要验证码，请查看图片后提供 --captcha", Details: map[string]any{"captcha_image": image}}
}

func teachingPasswordBodyCode(result map[string]any) string {
	if body := strings.TrimSpace(businessBody(result)); body != "" {
		return strings.Trim(body, "\" \r\n")
	}
	if value, ok := businessData(result); ok {
		return teachingPasswordValueCode(value)
	}
	return ""
}

func teachingPasswordValueCode(value any) string {
	return strings.Trim(strings.TrimSpace(fmt.Sprint(value)), "\" \r\n")
}

func teachingPasswordResetFailure(stage, status string, mutation bool) *siteError {
	messages := map[string]string{
		"10001": "找回密码参数异常",
		"10002": "图形验证码错误",
		"10004": "用户名或邮箱不正确",
		"10005": "用户名或邮箱不正确",
		"10006": "用户名或邮箱不正确",
		"10007": "系统邮件发送失败",
		"10008": "账号验证异常",
		"10009": "密保问题答案不一致",
		"10010": "图形验证码错误",
		"10011": "密保验证异常",
		"30001": "重置凭证无效或已过期",
		"30002": "密码不能为空",
		"30003": "两次密码不一致",
		"30004": "密码不符合规则",
		"30005": "密码修改异常",
	}
	message := messages[status]
	if message == "" {
		message = "网络教学平台找回密码业务拒绝"
	}
	code := "business_rejected"
	if mutation {
		code = "mutation_rejected"
	}
	return &siteError{Code: code, Message: message, Details: map[string]any{
		"submitted": true, "confirmed": false, "evidence": stage + " 返回 " + status, "remote_status": status,
	}}
}

func (a NativeSite) teachingPasswordResetSubmit(ctx context.Context, method, code, newPassword, confirm, cookie string) (map[string]any, *siteError) {
	path := "/meol/findPasswdMailBoxReset.do"
	if method == "question" {
		path = "/meol/findPasswdQuestionReset.do"
	}
	result, requestErr := businessRequest(ctx, "theol", "POST", path, nil, []pair{
		{"code", code}, {"firstPasswd", newPassword}, {"secondPasswd", confirm},
	}, nil, businessRequestOptions{cookieFile: cookie, allowBusinessFailure: true}, true, true)
	if requestErr != nil {
		return nil, requestErr
	}
	status := teachingPasswordBodyCode(result)
	if status != "30000" {
		return nil, teachingPasswordResetFailure(strings.TrimPrefix(path, "/meol/"), status, true)
	}
	return map[string]any{
		"ok": true, "submitted": true, "confirmed": true, "reset": true,
		"evidence": path + " 返回 30000", "service": teachingServiceName,
		"operation": "password-reset", "method": method,
	}, nil
}

func (a NativeSite) teachingPasswordQuestionList(ctx context.Context, cookie string) ([]map[string]any, *siteError) {
	result, requestErr := a.businessGet(ctx, "theol", "/meol/findPasswdQuestionPreAccount.do", nil, businessRequestOptions{cookieFile: cookie})
	if requestErr != nil {
		return nil, requestErr
	}
	document, parseErr := parsePage(businessBody(result))
	if parseErr != nil {
		return nil, &siteError{Code: "parse_error", Message: "网络教学平台密保问题页面解析失败: " + parseErr.Error()}
	}
	items := make([]map[string]any, 0)
	seen := map[string]bool{}
	for _, list := range document.findAll("ul") {
		if !strings.Contains(list.attr("class"), "model-select-option") {
			continue
		}
		for _, option := range list.findAll("li") {
			id := strings.TrimSpace(option.attr("data-option"))
			name := strings.TrimSpace(pageDisplayText(option))
			if id == "" || name == "" || seen[id] {
				continue
			}
			seen[id] = true
			items = append(items, map[string]any{"id": id, "name": name})
		}
	}
	if len(items) == 0 {
		return nil, &siteError{Code: "parse_error", Message: "网络教学平台页面未返回密保问题列表"}
	}
	return items, nil
}

func (a NativeSite) teachingPasswordQuestions(ctx context.Context, args []string) (map[string]any, *siteError) {
	cookie, _, valueErr := businessValue(args, "--cookie-file")
	if valueErr != nil {
		return nil, valueErr
	}
	for _, arg := range args {
		if arg != "--json" && arg != "--cookie-file" && !strings.HasPrefix(arg, "--cookie-file=") {
			return nil, &siteError{Code: "invalid_argument", Message: "teaching password-questions 参数无效: " + arg}
		}
	}
	items, requestErr := a.teachingPasswordQuestionList(ctx, cookie)
	if requestErr != nil {
		return nil, requestErr
	}
	return map[string]any{
		"ok": true, "submitted": false, "confirmed": true,
		"evidence": "findPasswdQuestionPreAccount.do 页面返回密保问题选项",
		"service":  teachingServiceName, "operation": "password-questions", "data": items,
	}, nil
}

func teachingPasswordQuestionID(items []map[string]any, wanted, flag string) (string, *siteError) {
	wanted = strings.TrimSpace(wanted)
	if wanted == "" {
		return "", &siteError{Code: "invalid_argument", Message: flag + " 必须提供密保问题名称"}
	}
	for _, item := range items {
		if strings.EqualFold(strings.TrimSpace(fmt.Sprint(item["name"])), wanted) || strings.EqualFold(strings.TrimSpace(fmt.Sprint(item["id"])), wanted) {
			return fmt.Sprint(item["id"]), nil
		}
	}
	available := make([]string, 0, len(items))
	for _, item := range items {
		available = append(available, fmt.Sprint(item["name"]))
	}
	return "", &siteError{Code: "invalid_argument", Message: flag + " 未找到该密保问题", Details: map[string]any{"available": available}}
}
