package adapter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

const (
	authResetAESKey            = "rjBFAaHsNkKAhpoi"
	authResetDefaultCaptcha    = "authserver-reset-captcha.png"
	authResetPhoneCaptcha      = "authserver-reset-phone-captcha.png"
	authResetQuestionCaptcha   = "authserver-reset-question-captcha.png"
	authResetStateVersion      = 1
	authResetStateFileName     = "password-reset.state.json"
	authResetAccountCaptchaKey = "account"
)

type authPasswordResetOptions struct {
	username string
	method   string
	service  string

	phone       string
	countryCode string
	email       string
	birthday    string
	answerOne   string
	answerTwo   string
	code        string

	captchaID    string
	captcha      string
	captchaImage string

	phoneCaptchaID    string
	phoneCaptcha      string
	phoneCaptchaImage string

	questionCaptchaID    string
	questionCaptcha      string
	questionCaptchaImage string

	newPassword          string
	passwordConfirm      string
	newPasswordStdin     bool
	passwordConfirmStdin bool
	sendCode             bool
	yes                  bool
	cookieFile           string
	stateFile            string
}

type authPasswordResetState struct {
	Version           int            `json:"version"`
	Method            string         `json:"method"`
	Service           string         `json:"service,omitempty"`
	CookieFile        string         `json:"cookie_file"`
	Contact           string         `json:"contact,omitempty"`
	CountryCode       string         `json:"country_code,omitempty"`
	Birthday          string         `json:"birthday,omitempty"`
	PhoneCaptchaID    string         `json:"phone_captcha_id,omitempty"`
	QuestionCaptchaID string         `json:"question_captcha_id,omitempty"`
	QuestionOne       string         `json:"question_one,omitempty"`
	QuestionTwo       string         `json:"question_two,omitempty"`
	FormData          map[string]any `json:"form_data"`
}

func authPasswordResetOutput(jsonMode bool, result map[string]any, err *siteError) (bool, []byte, []byte, int, error) {
	if err != nil {
		if jsonMode {
			return true, errorJSON(err), nil, 2, nil
		}
		return true, nil, []byte("错误: " + err.Error() + "\n"), 2, nil
	}
	if jsonMode {
		return true, mustJSON(result), nil, 0, nil
	}
	if result["reset"] == true {
		return true, []byte("密码已重置\n"), nil, 0, nil
	}
	return true, []byte(fmt.Sprintf("找回密码流程已完成当前步骤，请继续使用 --state-file %v\n", result["resume_file"])), nil, 0, nil
}

func parseAuthPasswordResetOptions(args []string) (authPasswordResetOptions, *siteError) {
	opts := authPasswordResetOptions{method: "auto", countryCode: "86"}
	for index := 0; index < len(args); index++ {
		arg := args[index]
		value := ""
		if strings.HasPrefix(arg, "--") {
			if name, inline, found := strings.Cut(arg, "="); found {
				arg, value = name, inline
			} else if index+1 < len(args) && !strings.HasPrefix(args[index+1], "--") {
				index++
				value = args[index]
			}
		}
		switch arg {
		case "--username":
			opts.username = value
		case "--method":
			opts.method = value
		case "--service":
			opts.service = value
		case "--phone", "--mobile":
			opts.phone = value
		case "--country-code":
			opts.countryCode = value
		case "--email":
			opts.email = value
		case "--birthday":
			opts.birthday = value
		case "--answer-one":
			opts.answerOne = value
		case "--answer-two":
			opts.answerTwo = value
		case "--code":
			opts.code = value
		case "--captcha-id":
			opts.captchaID = value
		case "--captcha":
			opts.captcha = value
		case "--captcha-image":
			opts.captchaImage = value
		case "--phone-captcha-id":
			opts.phoneCaptchaID = value
		case "--phone-captcha":
			opts.phoneCaptcha = value
		case "--phone-captcha-image":
			opts.phoneCaptchaImage = value
		case "--question-captcha-id":
			opts.questionCaptchaID = value
		case "--question-captcha":
			opts.questionCaptcha = value
		case "--question-captcha-image":
			opts.questionCaptchaImage = value
		case "--new-password":
			opts.newPassword = value
		case "--password-confirm", "--confirm-password":
			opts.passwordConfirm = value
		case "--new-password-stdin":
			opts.newPasswordStdin = true
		case "--password-confirm-stdin", "--confirm-password-stdin":
			opts.passwordConfirmStdin = true
		case "--send-code":
			opts.sendCode = true
		case "--cookie-file":
			opts.cookieFile = value
		case "--state-file":
			opts.stateFile = value
		case "--yes":
			opts.yes = true
		case "--json":
		default:
			return authPasswordResetOptions{}, &siteError{Code: "invalid_argument", Message: "login reset-password 参数无效: " + arg}
		}
	}
	if opts.newPassword != "" && opts.newPasswordStdin || opts.passwordConfirm != "" && opts.passwordConfirmStdin {
		return authPasswordResetOptions{}, &siteError{Code: "invalid_argument", Message: "密码不能同时使用命令行参数和标准输入"}
	}
	if opts.newPasswordStdin || opts.passwordConfirmStdin {
		input, err := readBoundedSiteInput(os.Stdin)
		if err != nil {
			if errors.Is(err, errSiteRequestTooLarge) {
				return authPasswordResetOptions{}, siteRequestTooLarge("标准输入密码")
			}
			return authPasswordResetOptions{}, &siteError{Code: "credentials_required", Message: "无法读取标准输入密码: " + err.Error()}
		}
		lines := strings.Split(strings.TrimRight(string(input), "\r\n"), "\n")
		for index := range lines {
			lines[index] = strings.TrimSuffix(lines[index], "\r")
		}
		if opts.newPasswordStdin && opts.passwordConfirmStdin {
			if len(lines) < 2 {
				return authPasswordResetOptions{}, &siteError{Code: "credentials_required", Message: "标准输入需要两行密码"}
			}
			opts.newPassword, opts.passwordConfirm = lines[0], lines[1]
		} else if opts.newPasswordStdin {
			if len(lines) == 0 {
				return authPasswordResetOptions{}, &siteError{Code: "credentials_required", Message: "标准输入缺少新密码"}
			}
			opts.newPassword = lines[0]
		} else {
			if len(lines) == 0 {
				return authPasswordResetOptions{}, &siteError{Code: "credentials_required", Message: "标准输入缺少确认密码"}
			}
			opts.passwordConfirm = lines[0]
		}
	}
	return opts, nil
}

func (a NativeSite) executeAuthPasswordReset(ctx context.Context, args []string) (map[string]any, *siteError) {
	opts, parseErr := parseAuthPasswordResetOptions(args)
	if parseErr != nil {
		return nil, parseErr
	}
	if opts.newPassword != "" && opts.passwordConfirm == "" || opts.newPassword == "" && opts.passwordConfirm != "" {
		return nil, &siteError{Code: "invalid_argument", Message: "新密码和确认密码必须同时提供"}
	}
	statePath := authPasswordResetStatePath(opts)
	cookiePath := authPasswordResetCookiePath(opts)
	var state *authPasswordResetState
	if strings.TrimSpace(opts.username) == "" {
		if opts.stateFile == "" {
			return nil, &siteError{Code: "credentials_required", Message: "找回密码初始流程必须提供 --username；续办流程必须提供 --state-file"}
		}
		var readErr *siteError
		state, readErr = readAuthPasswordResetState(statePath)
		if readErr != nil {
			return nil, readErr
		}
		if opts.cookieFile == "" && state.CookieFile != "" {
			cookiePath = state.CookieFile
		}
		if opts.method == "auto" {
			opts.method = state.Method
		}
		if opts.service == "" {
			opts.service = state.Service
		}
		if opts.countryCode == "86" && state.CountryCode != "" {
			opts.countryCode = state.CountryCode
		}
	}
	if state == nil {
		return a.executeInitialAuthPasswordReset(ctx, opts, cookiePath, statePath)
	}
	return a.executeResumedAuthPasswordReset(ctx, opts, state, cookiePath, statePath)
}

func (a NativeSite) executeInitialAuthPasswordReset(ctx context.Context, opts authPasswordResetOptions, cookiePath, statePath string) (map[string]any, *siteError) {
	username := strings.TrimSpace(opts.username)
	if username == "" {
		return nil, &siteError{Code: "credentials_required", Message: "找回密码必须提供 --username"}
	}
	if opts.captchaID == "" || opts.captcha == "" {
		id := opts.captchaID
		if id == "" {
			var err error
			id, err = randomCASString(16)
			if err != nil {
				return nil, &siteError{Code: "captcha_error", Message: "无法生成账号验证码编号: " + err.Error()}
			}
		}
		image := authPasswordResetCaptchaImage(opts, authResetAccountCaptchaKey, cookiePath)
		if fetchErr := a.fetchAuthPasswordResetCaptcha(ctx, id, image, cookiePath); fetchErr != nil {
			return nil, fetchErr
		}
		return nil, authPasswordResetCaptchaError("account", id, image, "")
	}

	available, availableErr := a.authPasswordResetMethods(ctx, cookiePath)
	if availableErr != nil {
		return nil, availableErr
	}
	method, methodErr := authPasswordResetMethod(opts.method, available)
	if methodErr != nil {
		return nil, methodErr
	}
	form, checkErr := a.authPasswordResetCheckUser(ctx, username, opts.captchaID, opts.captcha, cookiePath)
	if checkErr != nil {
		return nil, checkErr
	}
	state := newAuthPasswordResetState(form, method, opts.service, cookiePath)
	if authPasswordResetValue(state.FormData["loginNo"]) == "" {
		state.FormData["loginNo"] = username
	}
	if err := authPasswordResetValidateContact(opts, state); err != nil {
		return nil, err
	}
	if err := authPasswordResetValidateQuestionAnswers(opts, method); err != nil {
		return nil, err
	}
	return a.continueAuthPasswordReset(ctx, opts, state, cookiePath, statePath)
}

func (a NativeSite) executeResumedAuthPasswordReset(ctx context.Context, opts authPasswordResetOptions, state *authPasswordResetState, cookiePath, statePath string) (map[string]any, *siteError) {
	if state.Method == "" || state.FormData == nil {
		return nil, &siteError{Code: "state_invalid", Message: "找回密码流程状态文件缺少必要字段"}
	}
	if opts.method == "auto" {
		opts.method = state.Method
	}
	if opts.method != "auto" {
		method, methodErr := authPasswordResetMethod(opts.method, []string{state.Method})
		if methodErr != nil {
			return nil, methodErr
		}
		opts.method = method
	}
	if opts.birthday == "" {
		opts.birthday = state.Birthday
	}
	if opts.phoneCaptchaID == "" {
		opts.phoneCaptchaID = state.PhoneCaptchaID
	}
	if opts.questionCaptchaID == "" {
		opts.questionCaptchaID = state.QuestionCaptchaID
	}
	if err := authPasswordResetValidateContact(opts, state); err != nil {
		return nil, err
	}
	if err := authPasswordResetValidateQuestionAnswers(opts, state.Method); err != nil {
		return nil, err
	}
	return a.continueAuthPasswordReset(ctx, opts, state, cookiePath, statePath)
}

func (a NativeSite) continueAuthPasswordReset(ctx context.Context, opts authPasswordResetOptions, state *authPasswordResetState, cookiePath, statePath string) (map[string]any, *siteError) {
	switch state.Method {
	case "cellphone":
		if opts.sendCode {
			if opts.phoneCaptcha == "" {
				id := opts.phoneCaptchaID
				if id == "" {
					var err error
					id, err = randomCASString(16)
					if err != nil {
						return nil, &siteError{Code: "captcha_error", Message: "无法生成手机验证码编号: " + err.Error()}
					}
				}
				state.PhoneCaptchaID = id
				if err := writeAuthPasswordResetState(statePath, state); err != nil {
					return nil, err
				}
				image := authPasswordResetCaptchaImage(opts, "phone", cookiePath)
				if fetchErr := a.fetchAuthPasswordResetCaptcha(ctx, id, image, cookiePath); fetchErr != nil {
					return nil, fetchErr
				}
				return nil, authPasswordResetCaptchaError("phone", id, image, statePath)
			}
			if opts.phoneCaptchaID == "" {
				return nil, &siteError{Code: "invalid_argument", Message: "发送手机验证码需要 --phone-captcha-id"}
			}
			state.PhoneCaptchaID = opts.phoneCaptchaID
			body, bodyErr := authPasswordResetContactBody(state, "")
			if bodyErr != nil {
				return nil, bodyErr
			}
			body["captchaId"], body["captcha"] = state.PhoneCaptchaID, opts.phoneCaptcha
			if _, sendErr := a.authPasswordResetPost(ctx, "/retrieve-password/sendCode", body, cookiePath, true, opts.yes); sendErr != nil {
				return nil, sendErr
			}
			if err := writeAuthPasswordResetState(statePath, state); err != nil {
				return nil, err
			}
			if opts.code == "" {
				return authPasswordResetPending(state, statePath, "code", true, "验证码发送成功"), nil
			}
		}
		if opts.code == "" {
			return nil, &siteError{Code: "invalid_argument", Message: "手机找回密码需要 --send-code 或已发送的 --code"}
		}
		return a.verifyAuthPasswordResetCode(ctx, opts, state, cookiePath, statePath)
	case "email":
		if opts.sendCode {
			body, bodyErr := authPasswordResetContactBody(state, "")
			if bodyErr != nil {
				return nil, bodyErr
			}
			if opts.service != "" {
				body["service"] = opts.service
			}
			if _, sendErr := a.authPasswordResetPost(ctx, "/retrieve-password/sendCode", body, cookiePath, true, opts.yes); sendErr != nil {
				return nil, sendErr
			}
			if err := writeAuthPasswordResetState(statePath, state); err != nil {
				return nil, err
			}
			if opts.code == "" {
				return authPasswordResetPending(state, statePath, "code", true, "验证码发送成功"), nil
			}
		}
		if opts.code == "" {
			return nil, &siteError{Code: "invalid_argument", Message: "邮箱找回密码需要 --send-code 或已发送的 --code"}
		}
		return a.verifyAuthPasswordResetCode(ctx, opts, state, cookiePath, statePath)
	case "question":
		return a.continueAuthPasswordResetQuestion(ctx, opts, state, cookiePath, statePath)
	default:
		return nil, &siteError{Code: "state_invalid", Message: "找回密码流程状态中的验证方式无效"}
	}
}

func (a NativeSite) continueAuthPasswordResetQuestion(ctx context.Context, opts authPasswordResetOptions, state *authPasswordResetState, cookiePath, statePath string) (map[string]any, *siteError) {
	if state.QuestionOne == "" || state.QuestionTwo == "" {
		if opts.birthday == "" {
			return nil, &siteError{Code: "invalid_argument", Message: "密保问题找回需要 --birthday"}
		}
		if opts.questionCaptcha == "" {
			id := opts.questionCaptchaID
			if id == "" {
				var err error
				id, err = randomCASString(16)
				if err != nil {
					return nil, &siteError{Code: "captcha_error", Message: "无法生成密保验证码编号: " + err.Error()}
				}
			}
			state.QuestionCaptchaID = id
			state.Birthday = opts.birthday
			if err := writeAuthPasswordResetState(statePath, state); err != nil {
				return nil, err
			}
			image := authPasswordResetCaptchaImage(opts, "question", cookiePath)
			if fetchErr := a.fetchAuthPasswordResetCaptcha(ctx, id, image, cookiePath); fetchErr != nil {
				return nil, fetchErr
			}
			return nil, authPasswordResetCaptchaError("question", id, image, statePath)
		}
		if opts.questionCaptchaID == "" {
			return nil, &siteError{Code: "invalid_argument", Message: "密保问题校验需要 --question-captcha-id"}
		}
		state.QuestionCaptchaID = opts.questionCaptchaID
		birthday, birthdayErr := a.authPasswordResetPost(ctx, "/retrieve-password/passwordRetrieve/birthdayCheck", map[string]any{
			"captchaId": state.QuestionCaptchaID,
			"captcha":   opts.questionCaptcha,
			"loginNo":   authPasswordResetValue(state.FormData["loginNo"]),
			"birthday":  opts.birthday,
			"sign":      authPasswordResetValue(state.FormData["sign"]),
		}, cookiePath, false, true)
		if birthdayErr != nil {
			return nil, birthdayErr
		}
		if apiErr := authPasswordResetCheckAPI(birthday, "birthdayCheck", false); apiErr != nil {
			return nil, apiErr
		}
		if data, ok := authPasswordResetData(birthday); ok {
			authPasswordResetMergeForm(state.FormData, data)
		}
		state.Birthday = opts.birthday
		questionPayload, questionErr := a.authPasswordResetPost(ctx, "/retrieve-password/passwordRetrieve/queryQuestion", state.FormData, cookiePath, false, true)
		if questionErr != nil {
			return nil, questionErr
		}
		if apiErr := authPasswordResetCheckAPI(questionPayload, "queryQuestion", false); apiErr != nil {
			return nil, apiErr
		}
		data, ok := authPasswordResetData(questionPayload)
		if !ok || authPasswordResetValue(data["questionOne"]) == "" || authPasswordResetValue(data["questionTwo"]) == "" {
			return nil, &siteError{Code: "parse_error", Message: "认证服务未返回完整密保问题"}
		}
		state.QuestionOne = authPasswordResetValue(data["questionOne"])
		state.QuestionTwo = authPasswordResetValue(data["questionTwo"])
		authPasswordResetMergeForm(state.FormData, data)
	}
	if opts.answerOne == "" || opts.answerTwo == "" {
		if err := writeAuthPasswordResetState(statePath, state); err != nil {
			return nil, err
		}
		return authPasswordResetPending(state, statePath, "answers", false, "密保问题已取得，请继续提供答案"), nil
	}
	answers, answerErr := a.authPasswordResetPost(ctx, "/retrieve-password/passwordRetrieve/checkQuestionAnswer", map[string]any{
		"questionOne": state.QuestionOne,
		"questionTwo": state.QuestionTwo,
		"answerOne":   opts.answerOne,
		"answerTwo":   opts.answerTwo,
		"sign":        authPasswordResetValue(state.FormData["sign"]),
	}, cookiePath, false, true)
	if answerErr != nil {
		return nil, answerErr
	}
	if apiErr := authPasswordResetCheckAPI(answers, "checkQuestionAnswer", false); apiErr != nil {
		return nil, apiErr
	}
	if data, ok := authPasswordResetData(answers); ok {
		authPasswordResetMergeForm(state.FormData, data)
	}
	return a.finishAuthPasswordReset(ctx, opts, state, cookiePath, statePath)
}

func (a NativeSite) verifyAuthPasswordResetCode(ctx context.Context, opts authPasswordResetOptions, state *authPasswordResetState, cookiePath, statePath string) (map[string]any, *siteError) {
	body, bodyErr := authPasswordResetContactBody(state, opts.code)
	if bodyErr != nil {
		return nil, bodyErr
	}
	checked, checkErr := a.authPasswordResetPost(ctx, "/retrieve-password/passwordRetrieve/checkCode", body, cookiePath, false, true)
	if checkErr != nil {
		return nil, checkErr
	}
	if apiErr := authPasswordResetCheckAPI(checked, "checkCode", false); apiErr != nil {
		return nil, apiErr
	}
	if data, ok := authPasswordResetData(checked); ok {
		authPasswordResetMergeForm(state.FormData, data)
	}
	return a.finishAuthPasswordReset(ctx, opts, state, cookiePath, statePath)
}

func (a NativeSite) finishAuthPasswordReset(ctx context.Context, opts authPasswordResetOptions, state *authPasswordResetState, cookiePath, statePath string) (map[string]any, *siteError) {
	if opts.newPassword == "" && opts.passwordConfirm == "" {
		if err := writeAuthPasswordResetState(statePath, state); err != nil {
			return nil, err
		}
		return authPasswordResetPending(state, statePath, "password", false, "身份校验成功，请继续提供新密码"), nil
	}
	if opts.newPassword != opts.passwordConfirm {
		return nil, &siteError{Code: "invalid_argument", Message: "新密码和确认密码不一致"}
	}
	if !opts.yes {
		if err := writeAuthPasswordResetState(statePath, state); err != nil {
			return nil, err
		}
		return nil, &siteError{Code: "confirmation_required", Message: "密码重置会修改远端账号，请加 --yes", Details: map[string]any{"submitted": false, "confirmed": false, "evidence": "verification_confirmed", "resume_file": statePath}}
	}
	body, bodyErr := authPasswordResetContactBody(state, "")
	if bodyErr != nil {
		return nil, bodyErr
	}
	password, passwordErr := encryptCASPassword(opts.newPassword, authResetAESKey)
	if passwordErr != nil {
		return nil, passwordErr
	}
	confirm, confirmErr := encryptCASPassword(opts.passwordConfirm, authResetAESKey)
	if confirmErr != nil {
		return nil, confirmErr
	}
	body["password"], body["confirmPassword"] = password, confirm
	reset, resetErr := a.authPasswordResetPost(ctx, "/retrieve-password/passwordRetrieve/resetPassword", body, cookiePath, true, true)
	if resetErr != nil {
		return nil, resetErr
	}
	if apiErr := authPasswordResetCheckAPI(reset, "resetPassword", true); apiErr != nil {
		return nil, apiErr
	}
	return map[string]any{
		"ok":          true,
		"submitted":   true,
		"confirmed":   true,
		"evidence":    "authserver_code_0",
		"operation":   "reset-password",
		"reset":       true,
		"method":      state.Method,
		"account":     authPasswordResetValue(state.FormData["loginNo"]),
		"verified_by": "passwordRetrieve/resetPassword code=0",
	}, nil
}

func (a NativeSite) authPasswordResetMethods(ctx context.Context, cookiePath string) ([]string, *siteError) {
	result, requestErr := a.businessGet(ctx, "auth", "/tenant/info", []pair{{"type", "1"}}, businessRequestOptions{cookieFile: cookiePath, allowBusinessFailure: true})
	if requestErr != nil {
		return nil, requestErr
	}
	payload, parseErr := businessJSONMap(result)
	if parseErr != nil {
		return nil, parseErr
	}
	value := payload["retrieveMethod"]
	if value == nil {
		if nested, ok := payload["datas"].(map[string]any); ok {
			value = nested["retrieveMethod"]
		}
	}
	var methods []string
	for _, item := range strings.Split(authPasswordResetValue(value), ",") {
		switch strings.TrimSpace(item) {
		case "1":
			methods = append(methods, "cellphone")
		case "2":
			methods = append(methods, "email")
		case "3":
			methods = append(methods, "question")
		}
	}
	if len(methods) == 0 {
		return nil, &siteError{Code: "business_rejected", Message: "认证服务未提供可用的找回密码方式"}
	}
	return methods, nil
}

func (a NativeSite) authPasswordResetCheckUser(ctx context.Context, username, captchaID, captcha, cookiePath string) (map[string]any, *siteError) {
	form := map[string]any{
		"accountId": "", "loginNo": username, "cellphone": "", "email": "",
		"hideCellphone": "", "hideEmail": "", "captchaId": captchaID, "captcha": captcha,
		"code": "", "type": "cellphone", "password": "", "confirmPassword": "", "sign": "",
	}
	payload, requestErr := a.authPasswordResetPost(ctx, "/retrieve-password/passwordRetrieve/checkUserInfo", form, cookiePath, false, true)
	if requestErr != nil {
		return nil, requestErr
	}
	if apiErr := authPasswordResetCheckAPI(payload, "checkUserInfo", false); apiErr != nil {
		return nil, apiErr
	}
	data, ok := authPasswordResetData(payload)
	if !ok {
		return nil, &siteError{Code: "parse_error", Message: "认证服务未返回账号找回状态"}
	}
	return data, nil
}

func (a NativeSite) authPasswordResetPost(ctx context.Context, path string, body map[string]any, cookiePath string, mutating, yes bool) (map[string]any, *siteError) {
	result, requestErr := a.execute(ctx, siteRequest{
		Service: "auth", Method: http.MethodPost, Path: path, JSON: body, HasJSON: true,
		CookieFile: cookiePath, ReadOnly: !mutating, Yes: yes, AllowBusinessFailure: true, RawJSON: true,
	})
	if requestErr != nil {
		return nil, requestErr
	}
	payload, parseErr := businessJSONMap(result)
	if parseErr != nil {
		return nil, parseErr
	}
	return payload, nil
}

func (a NativeSite) fetchAuthPasswordResetCaptcha(ctx context.Context, id, image, cookiePath string) *siteError {
	_, requestErr := a.execute(ctx, siteRequest{
		Service: "auth", Method: http.MethodGet, Path: "/retrieve-password/generateCaptcha",
		Params: []pair{{"ltId", id}, {"codeType", "2"}}, CookieFile: cookiePath,
		Output: image, ReadOnly: true, Yes: true,
	})
	return requestErr
}

func authPasswordResetMethod(value string, available []string) (string, *siteError) {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" || value == "auto" {
		return available[0], nil
	}
	switch value {
	case "phone", "mobile", "sms", "cellphone":
		value = "cellphone"
	case "mail", "email":
		value = "email"
	case "question", "security-question":
		value = "question"
	default:
		return "", &siteError{Code: "invalid_argument", Message: "--method 必须是 auto、phone、email 或 question"}
	}
	for _, method := range available {
		if method == value {
			return value, nil
		}
	}
	return "", &siteError{Code: "invalid_argument", Message: "认证服务当前不支持所选找回密码方式", Details: map[string]any{"method": value, "available_methods": available}}
}

func authPasswordResetValidateContact(opts authPasswordResetOptions, state *authPasswordResetState) *siteError {
	switch state.Method {
	case "cellphone":
		if opts.phone == "" && state.Contact != "" {
			if state.CountryCode == "" {
				state.CountryCode = strings.SplitN(state.Contact, "-", 2)[0]
			}
			return nil
		}
		value := opts.phone
		phone, err := authPasswordResetPhone(opts.countryCode, value)
		if err != nil {
			return err
		}
		state.Contact, state.CountryCode = phone, strings.TrimPrefix(strings.TrimSpace(opts.countryCode), "+")
	case "email":
		value := strings.TrimSpace(opts.email)
		if value == "" {
			value = state.Contact
		}
		if !strings.Contains(value, "@") || strings.HasPrefix(value, "@") || strings.HasSuffix(value, "@") {
			return &siteError{Code: "invalid_argument", Message: "邮箱地址无效"}
		}
		state.Contact = value
	}
	return nil
}

func authPasswordResetValidateQuestionAnswers(opts authPasswordResetOptions, method string) *siteError {
	if method != "question" {
		return nil
	}
	if opts.answerOne != "" && opts.answerTwo == "" || opts.answerOne == "" && opts.answerTwo != "" {
		return &siteError{Code: "invalid_argument", Message: "密保答案必须同时提供 --answer-one 和 --answer-two"}
	}
	return nil
}

func authPasswordResetPhone(country, phone string) (string, *siteError) {
	country = strings.TrimPrefix(strings.TrimSpace(country), "+")
	phone = strings.NewReplacer(" ", "", "-", "").Replace(strings.TrimSpace(phone))
	if country == "" || phone == "" || !authPasswordResetDigits(country) || !authPasswordResetDigits(phone) {
		return "", &siteError{Code: "invalid_argument", Message: "手机号和国家代码必须只包含数字"}
	}
	return country + "-" + phone, nil
}

func authPasswordResetDigits(value string) bool {
	for _, character := range value {
		if character < '0' || character > '9' {
			return false
		}
	}
	return true
}

func authPasswordResetContactBody(state *authPasswordResetState, code string) (map[string]any, *siteError) {
	body := make(map[string]any, len(state.FormData)+4)
	for key, value := range state.FormData {
		if key == "password" || key == "confirmPassword" || key == "captcha" || key == "captchaId" || key == "code" || key == "cellphone" || key == "email" {
			continue
		}
		body[key] = value
	}
	body["loginNo"] = authPasswordResetValue(state.FormData["loginNo"])
	body["type"] = state.Method
	if state.Contact != "" {
		field := "email"
		if state.Method == "cellphone" {
			field = "cellphone"
		}
		encrypted, err := encryptCASPassword(state.Contact, authResetAESKey)
		if err != nil {
			return nil, err
		}
		body[field] = encrypted
	}
	if code != "" {
		body["code"] = code
	}
	return body, nil
}

func authPasswordResetCheckAPI(payload map[string]any, operation string, mutating bool) *siteError {
	code := authPasswordResetValue(payload["code"])
	if code == "0" {
		return nil
	}
	message := authPasswordResetValue(payload["message"])
	if message == "" {
		message = "认证服务拒绝了" + operation
	}
	errorCode := "business_rejected"
	if mutating {
		errorCode = "mutation_rejected"
	}
	return &siteError{Code: errorCode, Message: message, Details: map[string]any{"operation": operation, "remote_code": code, "submitted": mutating, "confirmed": false, "evidence": "rejected"}}
}

func authPasswordResetData(payload map[string]any) (map[string]any, bool) {
	value, exists := payload["datas"]
	if !exists || value == nil {
		return nil, false
	}
	switch typed := value.(type) {
	case map[string]any:
		return typed, true
	case string:
		var data map[string]any
		if json.Unmarshal([]byte(typed), &data) == nil && data != nil {
			return data, true
		}
	}
	return nil, false
}

func authPasswordResetMergeForm(target, source map[string]any) {
	for key, value := range source {
		switch key {
		case "password", "confirmPassword", "cellphone", "email", "captcha", "captchaId", "code":
			continue
		default:
			target[key] = value
		}
	}
}

func newAuthPasswordResetState(form map[string]any, method, service, cookiePath string) *authPasswordResetState {
	clean := make(map[string]any, len(form)+1)
	for key, value := range form {
		switch key {
		case "password", "confirmPassword", "cellphone", "email", "captcha", "captchaId", "code":
			continue
		default:
			clean[key] = value
		}
	}
	clean["type"] = method
	return &authPasswordResetState{Version: authResetStateVersion, Method: method, Service: service, CookieFile: cookiePath, FormData: clean}
}

func authPasswordResetPending(state *authPasswordResetState, statePath, next string, submitted bool, message string) map[string]any {
	return map[string]any{
		"ok": true, "submitted": submitted, "confirmed": true, "evidence": "authserver_code_0",
		"operation": "reset-password", "pending": true, "next": next, "next_step": next,
		"method": state.Method, "account": authPasswordResetValue(state.FormData["loginNo"]),
		"resume_file": statePath, "message": message,
	}
}

func authPasswordResetCaptchaError(stage, id, image, statePath string) *siteError {
	details := map[string]any{"stage": stage, "captcha_id": id, "captcha_image": image}
	if statePath != "" {
		details["resume_file"] = statePath
	}
	return &siteError{Code: "captcha_required", Message: "请识别验证码图片后重试", Details: details}
}

func authPasswordResetCaptchaImage(opts authPasswordResetOptions, stage, cookiePath string) string {
	var value string
	switch stage {
	case authResetAccountCaptchaKey:
		value = opts.captchaImage
		if value == "" {
			value = authResetDefaultCaptcha
		}
	case "phone":
		value = opts.phoneCaptchaImage
		if value == "" {
			value = authResetPhoneCaptcha
		}
	case "question":
		value = opts.questionCaptchaImage
		if value == "" {
			value = authResetQuestionCaptcha
		}
	}
	if value == authResetDefaultCaptcha || value == authResetPhoneCaptcha || value == authResetQuestionCaptcha {
		value = filepath.Join(filepath.Dir(cookiePath), value)
	}
	return expandUserPath(value)
}

func authPasswordResetStatePath(opts authPasswordResetOptions) string {
	if opts.stateFile != "" {
		return expandUserPath(opts.stateFile)
	}
	return filepath.Join(filepath.Dir(authPasswordResetCookiePath(opts)), authResetStateFileName)
}

func authPasswordResetCookiePath(opts authPasswordResetOptions) string {
	if opts.cookieFile != "" {
		return expandUserPath(opts.cookieFile)
	}
	return academicCookiePath()
}

func writeAuthPasswordResetState(path string, state *authPasswordResetState) *siteError {
	encoded, err := json.Marshal(state)
	if err != nil {
		return &siteError{Code: "state_write_failed", Message: "无法序列化找回密码流程状态: " + err.Error()}
	}
	return atomicWrite(path, append(encoded, '\n'))
}

func readAuthPasswordResetState(path string) (*authPasswordResetState, *siteError) {
	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, &siteError{Code: "state_required", Message: "找不到找回密码流程状态文件，请先开始找回密码流程"}
		}
		return nil, &siteError{Code: "state_read_failed", Message: "无法读取找回密码流程状态: " + err.Error()}
	}
	var state authPasswordResetState
	if json.Unmarshal(content, &state) != nil || state.Version != authResetStateVersion || state.FormData == nil {
		return nil, &siteError{Code: "state_invalid", Message: "找回密码流程状态文件格式无效"}
	}
	return &state, nil
}

func authPasswordResetValue(value any) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}
