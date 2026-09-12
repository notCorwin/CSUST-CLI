package adapter

import (
	"context"
	"net/url"
	"strings"
)

func (a NativeSite) executeContinuingPlatform(ctx context.Context, args []string) (map[string]any, *siteError) {
	if len(args) == 0 || args[0] == "status" {
		statusArgs := args
		if len(statusArgs) > 0 {
			statusArgs = statusArgs[1:]
		}
		cookie, _, valueErr := businessValue(statusArgs, "--cookie-file")
		if valueErr != nil {
			return nil, valueErr
		}
		return a.executeServiceStatusWithOptions(ctx, "continuing-platform", "status", "继续教育信息服务平台", businessRequestOptions{cookieFile: cookie})
	}
	if args[0] == "catalog" {
		result := businessCatalogFilter("continuing-platform")
		result["roles"] = []map[string]any{
			{"role": "authority", "label": "院内用户", "field": "textBoxAuthorityLoginName"},
			{"role": "student", "label": "学生用户", "field": "textBoxStudentLoginName", "student_types": []string{"成人教育", "湖南自考", "社会自考", "海南自考", "新疆自考", "青海自考", "天津自考", "福建自考"}},
			{"role": "station", "label": "站点用户", "field": "_textBoxStationLoginName"},
		}
		return result, nil
	}
	cookie, _, valueErr := businessValue(args[1:], "--cookie-file")
	if valueErr != nil {
		return nil, valueErr
	}
	switch args[0] {
	case "login":
		return a.continuingPlatformLogin(ctx, args[1:], cookie)
	case "logout":
		return a.continuingPlatformLogout(ctx, args[1:], cookie)
	default:
		return nil, &siteError{Code: "invalid_argument", Message: "continuing-platform 只支持 status、catalog、login、logout"}
	}
}

func continuingPlatformRole(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "authority", "院内用户", "staff":
		return "authority"
	case "student", "学生用户":
		return "student"
	case "station", "站点用户":
		return "station"
	default:
		return ""
	}
}

func continuingPlatformLoginPage(body string) bool {
	return strings.Contains(body, "formLoginMaster") && strings.Contains(body, "textBoxAuthorityLoginName")
}

func continuingPlatformLoginPath(result map[string]any) string {
	parsed, err := url.Parse(safeResponseURL(result))
	if err != nil || parsed.Path == "" {
		return "/Login/Login.aspx"
	}
	directory := strings.TrimSuffix(parsed.Path, "/default.aspx")
	return strings.TrimSuffix(directory, "/") + "/Login/Login.aspx"
}

func continuingPlatformLoginFields(body, role, studentType string, remember bool) ([]pair, *siteError) {
	fields, formErr := hiddenFormFields(body, "formLoginMaster")
	if formErr != nil {
		return nil, formErr
	}
	var account, password, button string
	switch role {
	case "authority":
		account = "ctl00$contentPlaceHolderLogin$textBoxAuthorityLoginName"
		password = "ctl00$contentPlaceHolderLogin$textBoxAuthorityPassword"
		button = "ctl00$contentPlaceHolderLogin$buttonAuthorityLogin"
	case "student":
		account = "ctl00$contentPlaceHolderLogin$textBoxStudentLoginName"
		password = "ctl00$contentPlaceHolderLogin$textBoxStudentPassword"
		button = "ctl00$contentPlaceHolderLogin$buttonStudentLogin"
		fields = append(fields, pair{"ctl00$contentPlaceHolderLogin$dropDownListStudentType", firstNonEmpty(studentType, "成人教育")})
	case "station":
		account = "ctl00$contentPlaceHolderLogin$_textBoxStationLoginName"
		password = "ctl00$contentPlaceHolderLogin$_textBoxStationPassword"
		button = "ctl00$contentPlaceHolderLogin$buttonStationLogin"
	}
	fields = append(fields, pair{account, ""}, pair{password, ""}, pair{button, "登    录"})
	if remember {
		fields = append(fields, pair{"ctl00$contentPlaceHolderLogin$checkBoxRememberMe", "on"})
	}
	return fields, nil
}

func (a NativeSite) continuingPlatformLogin(ctx context.Context, args []string, cookie string) (map[string]any, *siteError) {
	role := continuingPlatformRole(flagValue(args, "--role"))
	if role == "" {
		return nil, &siteError{Code: "invalid_argument", Message: "login 必须提供 --role authority、student 或 station"}
	}
	account, password, credentialErr := businessCredentials(args, "CSUST_CONTINUING_PLATFORM_PASSWORD")
	if credentialErr != nil {
		return nil, credentialErr
	}
	page, requestErr := a.businessGet(ctx, "continuing-platform", "/", nil, businessRequestOptions{cookieFile: cookie})
	if requestErr != nil {
		return nil, requestErr
	}
	studentType := flagValue(args, "--student-type")
	fields, fieldsErr := continuingPlatformLoginFields(businessBody(page), role, studentType, businessBool(args, "--remember"))
	if fieldsErr != nil {
		return nil, fieldsErr
	}
	for index := range fields {
		switch fields[index].name {
		case "ctl00$contentPlaceHolderLogin$textBoxAuthorityLoginName", "ctl00$contentPlaceHolderLogin$textBoxStudentLoginName", "ctl00$contentPlaceHolderLogin$_textBoxStationLoginName":
			fields[index].value = account
		case "ctl00$contentPlaceHolderLogin$textBoxAuthorityPassword", "ctl00$contentPlaceHolderLogin$textBoxStudentPassword", "ctl00$contentPlaceHolderLogin$_textBoxStationPassword":
			fields[index].value = password
		}
	}
	path := continuingPlatformLoginPath(page)
	result, requestErr := businessRequest(ctx, "continuing-platform", "POST", path, nil, fields, []pair{{"Referer", safeResponseURL(page)}}, businessRequestOptions{cookieFile: cookie, allowBusinessFailure: true}, true, true)
	if requestErr != nil {
		return nil, requestErr
	}
	if failure := businessLoginResponseFailure(result); failure != nil {
		return nil, failure
	}
	evidence, probeErr := a.confirmBusinessLogin(ctx, "continuing-platform", "/", cookie, continuingPlatformLoginPage)
	if probeErr != nil {
		return nil, probeErr
	}
	return map[string]any{"ok": true, "submitted": true, "confirmed": true, "evidence": "Login.aspx-and-" + evidence, "service": "continuing-platform", "operation": "login", "role": role, "username": account}, nil
}

func (a NativeSite) continuingPlatformLogout(ctx context.Context, args []string, cookie string) (map[string]any, *siteError) {
	if !businessBool(args, "--yes") {
		return nil, &siteError{Code: "confirmation_required", Message: "继续教育信息平台退出会话需要 --yes"}
	}
	_, cookiePath, resolveErr := resolveSite(siteRequest{Service: "continuing-platform", CookieFile: cookie})
	if resolveErr != nil {
		return nil, resolveErr
	}
	if removeErr := removeCookieFile(cookiePath); removeErr != nil {
		return nil, &siteError{Code: "cookie_write_failed", Message: removeErr.Error()}
	}
	return map[string]any{"ok": true, "submitted": false, "confirmed": true, "evidence": "local-cookie-removed", "service": "continuing-platform", "operation": "logout", "logged_out": true, "cookie_file": cookiePath}, nil
}
