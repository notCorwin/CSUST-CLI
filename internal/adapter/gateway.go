package adapter

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strings"
)

const (
	teachingServiceName = "网络教学平台"
	qualityServiceName  = "教学一体化"
)

type gatewayRequest struct {
	path, method, referer, output string
	params, data                  []pair
	files                         []filePart
	json                          any
	hasJSON, yes, raw, readOnly   bool
	forceMutating                 bool
}

func (a NativeSite) runGatewayCommand(ctx context.Context, args []string, jsonMode bool) (bool, []byte, []byte, int, error) {
	if len(args) < 2 {
		return false, nil, nil, 0, nil
	}
	service, ok := gatewayCommandService(args[0])
	if !ok {
		return false, nil, nil, 0, nil
	}
	result, runErr := a.executeGatewayCommand(ctx, service, args[1:])
	if runErr != nil && runErr.Code == "login_required" && gatewayCanRecoverVPN(args[1:]) {
		if _, loginErr := a.runVPNLogin(ctx, []string{"--auth", "cas"}); loginErr != nil {
			runErr = loginErr
		} else {
			result, runErr = a.executeGatewayCommand(ctx, service, args[1:])
		}
	}
	if runErr != nil {
		if jsonMode {
			return true, errorJSON(runErr), nil, 2, nil
		}
		return true, nil, []byte("错误: " + runErr.Error() + "\n"), 2, nil
	}
	if jsonMode {
		encoded, err := json.Marshal(stripSiteInternal(result))
		if err != nil {
			return true, nil, nil, 2, err
		}
		return true, encoded, nil, 0, nil
	}
	return true, []byte(renderGatewayResult(stripSiteInternal(result).(map[string]any))), nil, 0, nil
}

func gatewayCanRecoverVPN(args []string) bool {
	if len(args) == 0 {
		return false
	}
	switch args[0] {
	case "catalog", "status", "logout", "login":
		return false
	default:
		return true
	}
}

func gatewayCommandService(command string) (string, bool) {
	switch command {
	case "teaching", "theol":
		return teachingServiceName, true
	case "quality", "quality-assurance", "assurance":
		return qualityServiceName, true
	default:
		return "", false
	}
}

func (a NativeSite) executeGatewayCommand(ctx context.Context, service string, args []string) (map[string]any, *siteError) {
	child := args[0]
	if child == "catalog" {
		if err := onlyJSONArgs(args[1:]); err != nil {
			return nil, err
		}
		return gatewayCatalog(service), nil
	}
	if service == teachingServiceName {
		switch child {
		case "courses":
			return a.teachingCourses(ctx, args[1:])
		case "course":
			return a.teachingCourse(ctx, args[1:])
		case "course-order":
			return a.teachingCourseOrder(ctx, args[1:])
		case "public-courses":
			return a.teachingPublicCourses(ctx, args[1:])
		case "public-teachers":
			return a.teachingPublicTeachers(ctx, args[1:])
		case "public-teacher":
			return a.teachingPublicTeacher(ctx, args[1:])
		case "public-departments":
			return a.teachingPublicDepartments(ctx, args[1:])
		}
	}
	if service == qualityServiceName && child == "status" {
		return a.qualityStatus(ctx, args[1:])
	}
	if service == qualityServiceName && child == "graduation-design" {
		return a.runQualityGraduation(ctx, args[1:])
	}
	if service == qualityServiceName && child == "login" {
		return a.runQualityLogin(ctx, args[1:])
	}
	if service == qualityServiceName && child == "logout" {
		return a.runQualityLogout(ctx, args[1:])
	}
	if service == qualityServiceName && (child == "evaluation" || child == "evaluate") {
		return a.runQualityEvaluation(ctx, args)
	}
	return nil, &siteError{Code: "invalid_argument", Message: "未知网关子命令: " + child}
}

func gatewayCatalog(service string) map[string]any {
	operations := []map[string]any{}
	system := ""
	if service == teachingServiceName {
		system = "网络教学平台"
		operations = []map[string]any{
			{"name": "courses", "description": "课程列表"},
			{"name": "course", "description": "课程详情"},
			{"name": "course-order", "description": "调整课程顺序", "mutating": true},
			{"name": "public-courses", "description": "公开课程检索"},
			{"name": "public-teachers", "description": "公开教师检索"},
			{"name": "public-teacher", "description": "公开教师主讲课程"},
			{"name": "public-departments", "description": "公开院系目录"},
		}
	} else {
		system = "教学质量保障系统"
		operations = []map[string]any{
			{"name": "status", "description": "认证状态"},
			{"name": "login", "description": "登录"},
			{"name": "logout", "description": "退出登录"},
			{"name": "evaluation", "description": "教学评价", "operations": []string{"batches", "courses", "form", "save", "submit"}},
			{"name": "graduation-design", "description": "毕业设计入口"},
		}
	}
	return map[string]any{
		"ok": true, "submitted": false, "confirmed": true, "evidence": "confirmed",
		"service": service, "system": system, "operations": operations,
	}
}

func (a NativeSite) runGatewayRequest(ctx context.Context, service string, request gatewayRequest) (map[string]any, *siteError) {
	mutating := !readOnlyMethod(request.method) || sideEffectSitePattern.MatchString(request.path) || gatewayParamsMutate(request.params)
	if request.readOnly {
		mutating = false
	}
	if request.forceMutating {
		mutating = true
	}
	if mutating && !request.yes {
		return nil, &siteError{Code: "confirmation_required", Message: "该网关请求可能改变远端状态，请加 --yes"}
	}
	result, err := a.executeGateway(ctx, service, request, mutating)
	if err != nil {
		return nil, err
	}
	result["service"] = service
	return result, nil
}

func gatewayParamsMutate(params []pair) bool {
	for _, item := range params {
		switch strings.ToLower(item.name) {
		case "action", "op", "operation":
			return true
		}
	}
	return false
}

func (a NativeSite) executeGateway(ctx context.Context, service string, request gatewayRequest, mutating bool) (map[string]any, *siteError) {
	base, cookie, prefix, _, err := a.discoverGateway(ctx, service)
	if err != nil {
		return nil, err
	}
	target, targetErr := gatewayTarget(base, prefix, request.path, request.params)
	if targetErr != nil {
		return nil, targetErr
	}
	headers := []pair{{"Accept", "text/html,application/xhtml+xml,application/json,text/plain,*/*"}, {"Accept-Encoding", "identity"}}
	if request.referer != "" {
		referer, parseErr := url.Parse(request.referer)
		if parseErr == nil && strings.EqualFold(referer.Host, target.Host) {
			headers = append(headers, pair{"Referer", referer.String()})
		}
	}
	submit := siteRequest{Target: target, CookieFile: cookie, Method: request.method, Params: nil, Data: request.data, Files: request.files, JSON: request.json, HasJSON: request.hasJSON, Headers: headers, Yes: request.yes, Output: request.output, RequireLogin: true, ReadOnly: !mutating}
	result, runErr := a.execute(ctx, submit)
	if runErr != nil {
		return nil, runErr
	}
	if request.raw {
		if response, ok := result["response"].(map[string]any); ok {
			if body, ok := response["body_internal"].(string); ok {
				result["raw_body"] = body
				result["raw_url"] = response["raw_url"]
			}
		}
	}
	return sitePageResult(result), nil
}

func gatewayTarget(base *url.URL, prefix, path string, params []pair) (*url.URL, *siteError) {
	if strings.TrimSpace(path) == "" {
		path = "/"
	}
	parsed, err := url.Parse(path)
	if err != nil || parsed.User != nil || strings.Contains(parsed.Path, "\\") {
		return nil, &siteError{Code: "invalid_path", Message: "网关路径格式无效"}
	}
	decoded, _ := url.PathUnescape(parsed.Path)
	for _, part := range strings.Split(decoded, "/") {
		if part == "." || part == ".." {
			return nil, &siteError{Code: "invalid_path", Message: "网关路径不能包含目录跳转"}
		}
	}
	if parsed.IsAbs() || parsed.Host != "" {
		return nil, &siteError{Code: "invalid_path", Message: "网关请求必须使用同一 VPN 服务路径"}
	}
	if !strings.HasPrefix(parsed.Path, "/http/") && !strings.HasPrefix(parsed.Path, "/https/") {
		parsed.Path = strings.TrimRight(prefix, "/") + "/" + strings.TrimLeft(parsed.Path, "/")
	} else if !strings.HasPrefix(parsed.Path, strings.TrimRight(prefix, "/")+"/") && parsed.Path != strings.TrimRight(prefix, "/") {
		return nil, &siteError{Code: "invalid_path", Message: "网关地址不属于当前服务"}
	}
	baseTarget := *base
	baseTarget.Path, baseTarget.RawQuery, baseTarget.Fragment = parsed.Path, parsed.RawQuery, ""
	query := baseTarget.Query()
	for _, item := range params {
		query.Add(item.name, item.value)
	}
	baseTarget.RawQuery = query.Encode()
	return &baseTarget, nil
}

func (a NativeSite) discoverGateway(ctx context.Context, service string) (*url.URL, string, string, map[string]any, *siteError) {
	base, cookie, session, connErr := vpnConnection(false)
	if connErr != nil {
		return nil, "", "", nil, connErr
	}
	if configured := os.Getenv(gatewayPrefixEnv(service)); configured != "" {
		prefix, prefixErr := validateGatewayPrefix(configured)
		if prefixErr != nil {
			return nil, "", "", nil, prefixErr
		}
		return base, cookie, prefix, map[string]any{"name": service, "urlPlus": prefix, "configured": true}, nil
	}
	target, pathErr := vpnAPIPath(base, "/api/client/users/service/group?endlessType=", false)
	if pathErr != nil {
		return nil, "", "", nil, pathErr
	}
	result, runErr := a.execute(ctx, siteRequest{Target: target, CookieFile: cookie, Method: "GET", Headers: vpnHeaders(base, target, session, false), RequireLogin: true, ReadOnly: true})
	if runErr != nil {
		return nil, "", "", nil, runErr
	}
	if vpnResponseCode(result) == "3010" {
		return nil, "", "", nil, &siteError{Code: "login_required", Message: "VPN 会话已失效"}
	}
	payload := resultJSON(result)
	serviceInfo := findGatewayService(payload, service)
	if serviceInfo == nil {
		return nil, "", "", nil, &siteError{Code: "service_unavailable", Message: "VPN 当前没有可用服务: " + service}
	}
	prefix, prefixErr := validateGatewayPrefix(fmt.Sprint(serviceInfo["urlPlus"]))
	if prefixErr != nil {
		return nil, "", "", nil, prefixErr
	}
	return base, cookie, prefix, serviceInfo, nil
}

func gatewayPrefixEnv(service string) string {
	if service == teachingServiceName {
		return "CSUST_TEACHING_PREFIX"
	}
	return "CSUST_QUALITY_PREFIX"
}

var gatewayPrefixPattern = regexp.MustCompile(`^/(?:http|https)/[^/]+(?:/.*)?$`)

func validateGatewayPrefix(value string) (string, *siteError) {
	prefix := strings.TrimRight(strings.TrimSpace(value), "/")
	decoded, _ := url.PathUnescape(prefix)
	if !gatewayPrefixPattern.MatchString(prefix) || strings.Contains(prefix, "\\") {
		return "", &siteError{Code: "invalid_path", Message: "网关前缀格式无效"}
	}
	for _, part := range strings.Split(decoded, "/") {
		if part == "." || part == ".." {
			return "", &siteError{Code: "invalid_path", Message: "网关前缀不能包含目录跳转"}
		}
	}
	return prefix, nil
}

func resultJSON(result map[string]any) map[string]any {
	response, _ := result["response"].(map[string]any)
	payload, _ := response["json_internal"].(map[string]any)
	if payload == nil {
		payload, _ = response["json"].(map[string]any)
	}
	return payload
}

func findGatewayService(payload map[string]any, service string) map[string]any {
	data, _ := payload["data"].(map[string]any)
	children, _ := data["children"].([]any)
	for _, group := range children {
		groupMap, _ := group.(map[string]any)
		services, _ := groupMap["serviceList"].([]any)
		for _, item := range services {
			candidate, _ := item.(map[string]any)
			if fmt.Sprint(candidate["name"]) == service {
				return candidate
			}
		}
	}
	return nil
}

func (a NativeSite) qualityStatus(ctx context.Context, args []string) (map[string]any, *siteError) {
	if err := onlyJSONArgs(args); err != nil {
		return nil, err
	}
	_, cookie, session, connectionErr := vpnConnection(false)
	if connectionErr != nil {
		return nil, connectionErr
	}
	if session["token"] == nil {
		if _, statErr := os.Stat(cookie); os.IsNotExist(statErr) {
			return map[string]any{"ok": true, "logged_in": false, "service": qualityServiceName, "system": "教学质量保障系统"}, nil
		}
	}
	request := gatewayRequest{path: "/jsxsd/framework/xsMain.jsp", method: "GET"}
	result, err := a.runGatewayRequest(ctx, qualityServiceName, request)
	if err != nil {
		if err.Code == "login_required" || err.Code == "service_unavailable" {
			return map[string]any{"ok": true, "logged_in": false, "service": qualityServiceName, "system": "教学质量保障系统"}, nil
		}
		return nil, err
	}
	return map[string]any{"ok": true, "logged_in": true, "service": qualityServiceName, "system": "教学质量保障系统", "page": result["response"]}, nil
}

func (a NativeSite) teachingCourses(ctx context.Context, args []string) (map[string]any, *siteError) {
	name, tutor := "", ""
	for index := 0; index < len(args); index++ {
		arg, value, inline := splitInline(args[index])
		if arg == "--json" {
			continue
		}
		if !inline {
			if index+1 >= len(args) || strings.HasPrefix(args[index+1], "--") {
				return nil, &siteError{Code: "invalid_argument", Message: arg + " 缺少参数值"}
			}
			index++
			value = args[index]
		}
		switch arg {
		case "--name":
			name = value
		case "--tutor":
			tutor = value
		default:
			return nil, &siteError{Code: "invalid_argument", Message: "teaching courses 参数无效: " + arg}
		}
	}
	_, _, _, _, discoverErr := a.discoverGateway(ctx, teachingServiceName)
	if discoverErr != nil {
		return nil, discoverErr
	}
	list := gatewayRequest{path: "/meol/lesson/blen.student.lesson.list.jsp", method: "GET"}
	list.readOnly = true
	if name != "" {
		list.method = "POST"
		list.data = append(list.data, pair{"name", name})
	}
	if tutor != "" {
		list.method = "POST"
		list.data = append(list.data, pair{"tutorName", tutor})
	}
	result, err := a.runGatewayRequest(ctx, teachingServiceName, list)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (a NativeSite) teachingCourse(ctx context.Context, args []string) (map[string]any, *siteError) {
	courseID, columnID := "", ""
	for index := 0; index < len(args); index++ {
		arg, value, inline := splitInline(args[index])
		if arg == "--json" {
			continue
		}
		if !inline {
			if index+1 >= len(args) || strings.HasPrefix(args[index+1], "--") {
				return nil, &siteError{Code: "invalid_argument", Message: arg + " 缺少参数值"}
			}
			index++
			value = args[index]
		}
		switch arg {
		case "--course-id":
			courseID = value
		case "--column-id":
			columnID = value
		default:
			return nil, &siteError{Code: "invalid_argument", Message: "teaching course 参数无效: " + arg}
		}
	}
	if courseID == "" {
		return nil, &siteError{Code: "invalid_argument", Message: "--course-id 不能为空"}
	}
	request := gatewayRequest{method: "GET"}
	if columnID != "" {
		request.path = "/meol/jpk/course/course_column_preview_transfer.jsp"
		request.params = []pair{{"columnId", columnID}}
	} else {
		request.path = "/meol/jpk/course/layout/newpage/index.jsp"
		request.params = []pair{{"courseId", courseID}}
	}
	result, err := a.runGatewayRequest(ctx, teachingServiceName, request)
	if err != nil {
		return nil, err
	}
	result["course_id"] = courseID
	result["column_id"] = columnID
	return result, nil
}

func (a NativeSite) teachingCourseOrder(ctx context.Context, args []string) (map[string]any, *siteError) {
	courseID, direction, yes := "", "", false
	for index := 0; index < len(args); index++ {
		arg, value, inline := splitInline(args[index])
		if arg == "--json" {
			continue
		}
		if arg == "--yes" {
			yes = true
			continue
		}
		if !inline {
			if index+1 >= len(args) || strings.HasPrefix(args[index+1], "--") {
				return nil, &siteError{Code: "invalid_argument", Message: arg + " 缺少参数值"}
			}
			index++
			value = args[index]
		}
		switch arg {
		case "--course-id":
			courseID = value
		case "--direction":
			direction = value
		default:
			return nil, &siteError{Code: "invalid_argument", Message: "teaching course-order 参数无效: " + arg}
		}
	}
	if courseID == "" || (direction != "up" && direction != "down") {
		return nil, &siteError{Code: "invalid_argument", Message: "必须提供 --course-id 和 --direction up|down"}
	}
	if !yes {
		return nil, &siteError{Code: "confirmation_required", Message: "调整课程顺序会改变远端状态，请加 --yes"}
	}
	request := gatewayRequest{path: "/meol/lesson/blen.student.lesson.list.jsp", method: "GET", params: []pair{{"ACTION", map[bool]string{true: "LESSUP", false: "LESSDOWN"}[direction == "up"]}, {"lid", courseID}}, yes: true, forceMutating: true}
	return a.runGatewayRequest(ctx, teachingServiceName, request)
}

func renderGatewayResult(result map[string]any) string {
	if response, ok := result["response"].(map[string]any); ok {
		return fmt.Sprintf("%v\t%v\n", response["title"], response["url"])
	}
	encoded, _ := json.MarshalIndent(result, "", "  ")
	return string(encoded) + "\n"
}
