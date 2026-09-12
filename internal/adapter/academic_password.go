package adapter

import (
	"context"
	"strings"
)

const academicPasswordPath = "/jsxsd/grsz/grsz_xgmm"

func (a NativeSite) academicPasswordChange(ctx context.Context, args []string) (map[string]any, *siteError) {
	if !flagPresent(args, "--yes") {
		return nil, &siteError{Code: "confirmation_required", Message: "修改教务密码会改变账号设置，请加 --yes"}
	}
	current, currentErr := businessSecret(args, "--current-password", "CSUST_PASSWORD")
	if currentErr != nil {
		return nil, currentErr
	}
	newPassword, newErr := businessSecret(args, "--new-password", "CSUST_ACADEMIC_NEW_PASSWORD")
	if newErr != nil {
		return nil, newErr
	}
	confirmation, confirmationErr := businessRequired(args, "--password-confirm", "change-password 必须提供 --password-confirm")
	if confirmationErr != nil {
		return nil, confirmationErr
	}
	if strings.TrimSpace(current) == "" || strings.TrimSpace(newPassword) == "" {
		return nil, &siteError{Code: "invalid_argument", Message: "当前密码和新密码不能为空"}
	}
	if newPassword != confirmation {
		return nil, &siteError{Code: "invalid_argument", Message: "--new-password 与 --password-confirm 不一致"}
	}
	if current == newPassword {
		return nil, &siteError{Code: "invalid_argument", Message: "新密码不能与当前密码相同"}
	}
	if len([]rune(newPassword)) < 8 || len([]rune(newPassword)) > 50 || !academicPasswordComplex(newPassword) || strings.ContainsRune(newPassword, '&') {
		return nil, &siteError{Code: "invalid_argument", Message: "新密码必须为 8 到 50 个字符，并同时包含大写字母、小写字母、数字及特殊字符，且不能包含 &"}
	}

	body, pageURL, err := a.academicPage(ctx, "GET", academicPasswordPath, nil, nil)
	if err != nil {
		return nil, err
	}
	document, parseErr := parsePage(body)
	if parseErr != nil {
		return nil, &siteError{Code: "parse_error", Message: parseErr.Error()}
	}
	form := academicPasswordForm(document)
	if form == nil {
		return nil, &siteError{Code: "parse_error", Message: "未找到教务修改密码表单"}
	}
	data := pageFormFields(form, nil, document)
	data = academicPasswordSetField(data, "oldpassword", current)
	data = academicPasswordSetField(data, "password1", newPassword)
	data = academicPasswordSetField(data, "password2", confirmation)
	data = academicPasswordEnsureField(data, "button1", "保 存")
	data = academicPasswordEnsureField(data, "upt", "1")
	result, submitErr := a.executeAcademicRequestWithRecovery(ctx, siteRequest{
		Service: "academic", Path: academicPasswordPath, Method: "POST", CookieFile: academicCookiePath(), Data: data,
		Headers: []pair{{"Referer", pageURL}}, RequireLogin: true, Yes: true,
	})
	if submitErr != nil {
		return nil, submitErr
	}
	return academicWrap(map[string]any{
		"kind": "academic-password", "operation": "change-password", "path": academicPasswordPath,
		"request":  map[string]any{"method": "POST", "path": academicPasswordPath, "fields": fieldNames(data)},
		"response": result["response"], "submitted": true, "confirmed": true, "evidence": "response-success",
	}), nil
}

func academicPasswordComplex(value string) bool {
	upper, lower, digit, special := false, false, false, false
	for _, char := range value {
		switch {
		case char >= 'A' && char <= 'Z':
			upper = true
		case char >= 'a' && char <= 'z':
			lower = true
		case char >= '0' && char <= '9':
			digit = true
		case strings.ContainsRune("~!@#$%^*()+-/.。,，", char):
			special = true
		}
	}
	return upper && lower && digit && special
}

func academicPasswordForm(document *pageNode) *pageNode {
	for _, form := range document.findAll("form") {
		seen := map[string]bool{}
		for _, field := range form.findAll("input") {
			seen[field.attr("name")] = true
		}
		if seen["oldpassword"] && seen["password1"] && seen["password2"] {
			return form
		}
	}
	return nil
}

func academicPasswordSetField(data []pair, name, value string) []pair {
	for index := range data {
		if data[index].name == name {
			data[index].value = value
			return data
		}
	}
	return append(data, pair{name, value})
}

func academicPasswordEnsureField(data []pair, name, value string) []pair {
	for _, item := range data {
		if item.name == name {
			return data
		}
	}
	return append(data, pair{name, value})
}
