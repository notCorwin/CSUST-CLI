package adapter

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type webRoute struct {
	command string
	group   string
	name    string
	path    string
}

var webRoutes = []webRoute{
	{"student-evaluation", "教学评价", "学生评价", "/jsxsd/xspj/xspj_find.do"},
	{"deferred-exam-application", "我的申请", "缓考申请", "/jsxsd/kscj/hksq_query"},
	{"exempt-exam-application", "我的申请", "免考申请", "/jsxsd/kscj/mksq_query"},
	{"enrollment-proof-application", "我的申请", "学籍在读证明申请", "/jsxsd/kscj/xjzdzmsq_query"},
	{"exam-schedule", "我的考试", "考试安排查询", "/jsxsd/xsks/xsksap_query"},
	{"graduate-exam-registration", "我的考试", "毕业生插考报名", "/jsxsd/xsks/bysckbm_query"},
	{"in-class-exam", "我的考试", "随堂考试查询", "/jsxsd/xsks/xsstk_query"},
	{"deferred-exam-registration", "我的考试", "缓考考试报名", "/jsxsd/kscj/hkbm_query"},
	{"social-exam-registration", "我的考试", "社会考试报名", "/jsxsd/xsdjks/xsdjks_list"},
	{"make-up-exam-registration", "成绩管理", "补考报名", "/jsxsd/kscj/bkbm_query"},
	{"summer-remedial-registration", "成绩管理", "暑期补修报名", "/jsxsd/kscj/qkbm_query"},
	{"retake-registration-selection", "成绩管理", "重修报名选课", "/jsxsd/kscj/cxbmxk_query"},
	{"teaching-process", "培养方案", "教学进程查询", "/jsxsd/pyfa/pyfajc_query"},
	{"execution-plan", "培养方案", "执行计划", "/jsxsd/pyfa/pyfa_query"},
	{"training-plan-progress", "培养方案", "培养方案及完成情况", "/jsxsd/pyfa/topyfamx"},
	{"minor-execution-plan", "培养方案", "辅修执行计划", "/jsxsd/pyfa/fxpyfa_query"},
	{"minor-training-plan", "培养方案", "辅修培养方案明细", "/jsxsd/pyfa/tofxpyfamx"},
	{"university-timetable", "我的课表", "全校性总课表查询", "/jsxsd/jskb/qxxzkb_find.do"},
	{"semester-timetable", "我的课表", "学期理论课表", "/jsxsd/xskb/xskb_list.do"},
	{"lab-timetable", "我的课表", "实验课表查询", "/jsxsd/syjx/toXskb.do"},
	{"class-timetable", "我的课表", "班级课表查询", "/jsxsd/kbcx/kbxx_xzb"},
	{"teacher-timetable", "我的课表", "教师课表查询", "/jsxsd/kbcx/kbxx_teacher"},
	{"classroom-timetable", "我的课表", "教室课表查询", "/jsxsd/kbcx/kbxx_classroom"},
	{"course-timetable", "我的课表", "课程课表查询", "/jsxsd/kbcx/kbxx_kc"},
	{"class-change", "我的课表", "调停课查询", "/jsxsd/xskb/xskb_ttkmx.do"},
	{"course-selection-center", "选课管理", "学生选课中心", "/jsxsd/xsxk/xklc_list"},
	{"special-course-application", "选课管理", "特殊选课申请", "/jsxsd/tsxk/tsxk_sqlist"},
	{"special-course-query", "选课管理", "特殊选课查询", "/jsxsd/tsxk/tsxk_cxlist"},
	{"preselection-management", "选课管理", "学生预选管理", "/jsxsd/xkgl/xsyxgl"},
	{"preselection-query", "选课管理", "学生预选查询", "/jsxsd/xkgl/xsyxcx"},
	{"classroom-loan-record", "选课管理", "教室借用记录", "/jsxsd/kbxx/jsjyjl_query"},
	{"teaching-progress", "选课管理", "教学进度查询", "/jsxsd/xkgl/skjhQuery.do"},
	{"drop-course-application", "选课管理", "学生退课申请", "/jsxsd/xkgl/xstk_list"},
	{"course-selection-result", "选课管理", "选课结果查询", "/jsxsd/xkgl/xsxkjgcx"},
	{"textbook-account-info", "教材管理", "教材账目信息", "/jsxsd/nxsjc/jczmxx"},
	{"textbook-confirmation", "教材管理", "学生教材确认", "/jsxsd/nxsjc/jccxcslg"},
	{"minor-application", "辅修管理", "辅修报名", "/jsxsd/fxgl/fxbmxx_query"},
	{"lab-booking", "实验教学", "实验预约管理", "/jsxsd/view/syjx/syyy_find.jsp"},
	{"open-lab-booking", "实验教学", "开放实验预约", "/jsxsd/view/syjx/kfsy_find.jsp"},
	{"second-class-credit-application", "第二课堂学分", "第二课堂学分申报", "/jsxsd/pyfa/cxxfsb_query"},
	{"second-class-credit-query", "第二课堂学分", "第二课堂学分查询", "/jsxsd/pyfa/cxxf_query"},
	{"discipline-competition-registration", "学科竞赛", "学科竞赛报名", "/jsxsd/xsxkjs/xkjsbm_query"},
	{"teacher-project-topics", "创新创业", "教师发布课题", "/jsxsd/view/cxcyxm/ktgl_xs_query.jsp"},
	{"project-change", "创新创业", "项目变更管理", "/jsxsd/cxcyxm/queryXmbg.do"},
	{"member-change", "创新创业", "成员变更管理", "/jsxsd/cxcyxm/queryCybg.do"},
	{"project-application", "创新创业", "项目申报管理", "/jsxsd/cxcyxm/querySq.do"},
	{"project-funding", "创新创业", "项目资金发放查看", "/jsxsd/cxcyxm/queryXmzjff.do"},
	{"received-notices", "公告留言", "已收公告", "/jsxsd/ggly/ysgg_query"},
	{"received-messages", "公告留言", "已收留言", "/jsxsd/ggly/ysly_query"},
	{"message-notifications", "公告留言", "消息通知", "/jsxsd/ggly/xxtz_query"},
	{"personal-info", "个人信息", "修改个人信息", "/jsxsd/grsz/grsz_xggrxx.do"},
	{"change-password", "个人信息", "修改密码", "/jsxsd/grsz/grsz_xgmm"},
	{"online-qa", "在线问答", "在线问答", "/jsxsd/zxwd/zxwd_opt"},
	{"teaching-calendar", "教学周历", "教学周历查看", "/jsxsd/jxzl/jxzl_query"},
	{"student-record-card", "学籍管理", "学籍卡片", "/jsxsd/grxx/xsxx"},
	{"graduation-status", "学籍管理", "毕业情况查询", "/jsxsd/xxwcqk/byqkcx.do"},
	{"student-status-management", "学籍管理", "学籍信息管理", "/jsxsd/xsxj/xjxxgl.do"},
	{"status-warning", "学籍管理", "学籍预警查询", "/jsxsd/xsxj/xsyjxx.do"},
	{"status-change", "学籍管理", "学籍异动信息", "/jsxsd/xsxj/xsydxx.do"},
	{"major-streaming", "学籍管理", "专业分流", "/jsxsd/xsxj/toQueryZyfl.do"},
	{"minor-status-change", "学籍管理", "辅修学生异动申请", "/jsxsd/fxxsxj/xsydxx.do"},
	{"direction-streaming", "学籍管理", "方向分流", "/jsxsd/xsxj/toQueryfxfl.do"},
	{"course-grades", "我的成绩", "课程成绩查询", "/jsxsd/kscj/cjcx_frm"},
	{"grade-recognition", "我的成绩", "成绩认定", "/jsxsd/kscj/cjrd_list"},
	{"grade-review-application", "我的成绩", "成绩查卷申请", "/jsxsd/kscj/cjfh_list"},
	{"grade-confirmation", "我的成绩", "成绩确认申请", "/jsxsd/kscj/cjqr_list"},
	{"graduate-info-check", "毕业管理", "毕业生信息核对", "/jsxsd/bygl/bysxx"},
	{"graduation-conclusion", "毕业管理", "毕业结论查看", "/jsxsd/bygl/bygl_ckxsList"},
	{"graduation-course-recognition", "毕业管理", "毕业课程认定查询", "/jsxsd/bygl/bykcrd_query"},
}

var webPublicRoutes = []map[string]string{
	{"command": "login", "name": "登录", "path": "/"},
	{"command": "forgot-password", "name": "忘记密码", "path": "/findmm.jsp"},
	{"command": "account-recovery-step", "name": "找回密码步骤页", "path": "/Logon.do"},
	{"command": "captcha", "name": "登录验证码", "path": "/verifycode.servlet"},
	{"command": "captcha-legacy", "name": "兼容登录验证码", "path": "/verifycode.servlet1"},
	{"command": "app-qr", "name": "APP 下载/返回登录", "path": "/css/images/codeFrame.png"},
}

var webMainMenus = []map[string]string{
	{"command": "desktop", "name": "我的桌面", "code": "NEW_XSD_WDZM"},
	{"command": "student-records", "name": "学籍成绩", "code": "NEW_XSD_XJCJ"},
	{"command": "training", "name": "培养管理", "code": "NEW_XSD_PYGL"},
	{"command": "exams", "name": "考试报名", "code": "NEW_XSD_KSBM"},
	{"command": "practice", "name": "实践环节", "code": "NEW_XSD_SJHJ"},
	{"command": "evaluation", "name": "教学评价", "code": "NEW_XSD_JXPJ"},
}

var webSecondLevelMenus = []map[string]string{
	{"name": "教学评价", "menu": "教学评价", "code": "NEW_XSD_JXPJ_JXPJ"},
	{"name": "我的申请", "menu": "考试报名", "code": "NEW_XSD_KSBM_WDSQ"},
	{"name": "我的考试", "menu": "考试报名", "code": "NEW_XSD_KSBM_WDKS"},
	{"name": "成绩管理", "menu": "考试报名", "code": "NEW_XSD_KSBM_CJGL"},
	{"name": "培养方案", "menu": "培养管理", "code": "NEW_XSD_PYGL_PYFA"},
	{"name": "我的课表", "menu": "培养管理", "code": "NEW_XSD_PYGL_WDKB"},
	{"name": "选课管理", "menu": "培养管理", "code": "NEW_XSD_PYGL_XKGL"},
	{"name": "教材管理", "menu": "培养管理", "code": "NEW_XSD_PYGL_JCGL"},
	{"name": "辅修管理", "menu": "培养管理", "code": "NEW_XSD_PYGL_FXGL"},
	{"name": "实验教学", "menu": "实践环节", "code": "NEW_XSD_SJHJ_SYJX"},
	{"name": "第二课堂学分", "menu": "实践环节", "code": "NEW_XSD_SJHJ_CXXF"},
	{"name": "毕业设计", "menu": "实践环节", "code": "NEW_XSD_KSBM_BYSJ"},
	{"name": "学科竞赛", "menu": "实践环节", "code": "NEW_XSD_SJHJ_XKJS"},
	{"name": "创新创业", "menu": "实践环节", "code": "NEW_XSD_SJHJ_CXCY"},
	{"name": "公告留言", "menu": "我的桌面", "code": "NEW_XSD_WDZM_GGLY"},
	{"name": "个人信息", "menu": "我的桌面", "code": "NEW_XSD_WDZM_GRXX"},
	{"name": "在线问答", "menu": "我的桌面", "code": "NEW_XSD_WDZM_ZXWD"},
	{"name": "教学周历", "menu": "我的桌面", "code": "NEW_XSD_WDZM_JXZL"},
	{"name": "学籍管理", "menu": "学籍成绩", "code": "NEW_XSD_XJCJ_XJGL"},
	{"name": "我的成绩", "menu": "学籍成绩", "code": "NEW_XSD_XJCJ_WDCJ"},
	{"name": "毕业管理", "menu": "学籍成绩", "code": "NEW_XSD_BYGL_BYGL"},
}

func (a NativeSite) runWebCommand(ctx context.Context, args []string, jsonMode bool) (bool, []byte, []byte, int, error) {
	if len(args) == 0 || (args[0] != "web" && args[0] != "routes" && args[0] != "menu" && args[0] != "discover") {
		return false, nil, nil, 0, nil
	}
	if containsHelp(args[1:]) {
		return false, nil, nil, 0, nil
	}
	command, parseErr := parseWebCommand(args)
	if parseErr != nil {
		if jsonMode {
			return true, errorJSON(parseErr), nil, 2, nil
		}
		return true, nil, []byte("错误: " + parseErr.Error() + "\n"), 2, nil
	}
	if command.kind == "routes" {
		result, routeErr := a.runWebRoutes(ctx, command.path)
		if routeErr != nil {
			if jsonMode {
				return true, errorJSON(routeErr), nil, 2, nil
			}
			return true, nil, []byte("错误: " + routeErr.Error() + "\n"), 2, nil
		}
		if jsonMode {
			encoded, encodeErr := json.Marshal(result)
			if encodeErr != nil {
				return true, nil, nil, 2, encodeErr
			}
			return true, encoded, nil, 0, nil
		}
		return true, []byte(renderWebResult(result)), nil, 0, nil
	}
	if command.kind == "graduation-design" {
		result, graduationErr := a.runWebGraduation(ctx, command.siteArgs)
		if graduationErr != nil {
			if jsonMode {
				return true, errorJSON(graduationErr), nil, 2, nil
			}
			return true, nil, []byte("错误: " + graduationErr.Error() + "\n"), 2, nil
		}
		if jsonMode {
			encoded, encodeErr := json.Marshal(result)
			if encodeErr != nil {
				return true, nil, nil, 2, encodeErr
			}
			return true, encoded, nil, 0, nil
		}
		return true, []byte(renderWebResult(result)), nil, 0, nil
	}

	if command.name != "" {
		resolved, resolveErr := a.resolveWebRoute(ctx, command.name)
		if resolveErr != nil {
			if jsonMode {
				return true, errorJSON(resolveErr), nil, 2, nil
			}
			return true, nil, []byte("错误: " + resolveErr.Error() + "\n"), 2, nil
		}
		command.siteArgs = append([]string{"get", "--service", "academic", "--cookie-file", academicCookiePath(), "--require-login", "--path", resolved}, command.siteArgs...)
	}

	var result map[string]any
	var err *siteError
	switch command.kind {
	case "catalog":
		result = webCatalogResult()
	case "request":
		request, parseRequestErr := parseSiteRequest(command.siteArgs[1:])
		if parseRequestErr != nil {
			err = parseRequestErr
			break
		}
		result, err = a.executeAcademicRequestWithRecovery(ctx, request)
		if err == nil {
			result = webPageResult(result)
		}
	default:
		parsed, siteErr := parseSiteCommand(command.siteArgs)
		if siteErr != nil {
			err = siteErr
			break
		}
		result, err = a.executeAcademicRunWithRecovery(ctx, func() (map[string]any, *siteError) {
			return a.executeSiteCommand(ctx, parsed)
		})
	}
	if err != nil {
		if jsonMode {
			return true, errorJSON(err), nil, 2, nil
		}
		return true, nil, []byte("错误: " + err.Error() + "\n"), 2, nil
	}
	if jsonMode {
		encoded, encodeErr := json.Marshal(result)
		if encodeErr != nil {
			return true, nil, nil, 2, encodeErr
		}
		return true, encoded, nil, 0, nil
	}
	return true, []byte(renderWebResult(result)), nil, 0, nil
}

type parsedWebCommand struct {
	kind     string
	siteArgs []string
	path     string
	name     string
}

func parseWebCommand(args []string) (parsedWebCommand, *siteError) {
	if args[0] != "web" {
		path, err := parseWebRoutesPath(args[1:])
		if err != nil {
			return parsedWebCommand{}, err
		}
		return parsedWebCommand{kind: "routes", path: path}, nil
	}
	if len(args) < 2 {
		return parsedWebCommand{}, &siteError{Code: "invalid_argument", Message: "缺少 web 子命令"}
	}
	child := args[1]
	if child == "catalog" || child == "routes" {
		if child == "catalog" {
			if err := onlyJSONArgs(args[2:]); err != nil {
				return parsedWebCommand{}, err
			}
			return parsedWebCommand{kind: child}, nil
		}
		parsedPath, parseErr := parseWebRoutesPath(args[2:])
		if parseErr != nil {
			return parsedWebCommand{}, parseErr
		}
		return parsedWebCommand{kind: child, path: parsedPath}, nil
	}
	if child == "graduation-design" {
		return parsedWebCommand{kind: child, siteArgs: append([]string(nil), args[2:]...)}, nil
	}
	if child == "run" {
		child = "action"
	}
	public := false
	start := 2
	if child == "public" {
		if len(args) < 3 {
			return parsedWebCommand{}, &siteError{Code: "invalid_argument", Message: "缺少 web public 子命令"}
		}
		public, child, start = true, args[2], 3
	}
	rest := append([]string(nil), args[start:]...)
	if child == "get" && public {
		if err := validatePublicPath(rest); err != nil {
			return parsedWebCommand{}, err
		}
	}
	route, routeFound := findWebRoute(child)
	if routeFound {
		operation := "get"
		if flagPresent(rest, "--form") {
			operation = "form"
		}
		if flagPresent(rest, "--action") {
			operation = "action"
			rest = replaceFlag(rest, "--action", "--index")
		}
		if (flagPresent(rest, "--data") || flagPresent(rest, "--file") || flagPresent(rest, "--button")) && operation == "get" {
			return parsedWebCommand{}, &siteError{Code: "invalid_argument", Message: "--data/--file/--button 需要同时指定 --form 或 --action"}
		}
		child = operation
		rest = append([]string{"--path", route.path}, rest...)
	}
	if child == "get" && !public && flagPresent(rest, "--name") {
		name, remaining, err := takeNamedFlag(rest, "--name")
		if err != nil {
			return parsedWebCommand{}, err
		}
		if flagPresent(remaining, "--path") {
			return parsedWebCommand{}, &siteError{Code: "invalid_argument", Message: "--name 与 --path 互斥"}
		}
		if route, found := findWebRoute(name); found {
			rest = append([]string{"--path", route.path}, remaining...)
		} else {
			return parsedWebCommand{kind: "get", name: name, siteArgs: remaining}, nil
		}
	}
	if (child == "get" || child == "action") && public {
		if err := validatePublicPath(rest); err != nil {
			return parsedWebCommand{}, err
		}
	}
	if child == "post" {
		child = "request"
		rest = append([]string{"--method", "POST"}, rest...)
		if public {
			if err := validatePublicPath(rest); err != nil {
				return parsedWebCommand{}, err
			}
		}
	}
	if child == "public" {
		return parsedWebCommand{}, &siteError{Code: "invalid_argument", Message: "web public 子命令无效"}
	}
	if child != "get" && child != "form" && child != "action" && child != "request" {
		return parsedWebCommand{}, &siteError{Code: "invalid_argument", Message: "未知 web 子命令: " + child}
	}
	service := "academic"
	cookie := academicCookiePath()
	if public {
		// Public pages intentionally use the same host but never require a login.
	}
	siteArgs := []string{child, "--service", service, "--cookie-file", cookie}
	if !public {
		siteArgs = append(siteArgs, "--require-login")
	}
	siteArgs = append(siteArgs, rest...)
	return parsedWebCommand{kind: child, siteArgs: siteArgs}, nil
}

func parseWebRoutesPath(args []string) (string, *siteError) {
	path := "/jsxsd/framework/xsMain.jsp"
	for index := 0; index < len(args); index++ {
		arg, value, inline := splitInline(args[index])
		if arg == "--json" {
			continue
		}
		if arg != "--path" {
			return "", &siteError{Code: "invalid_argument", Message: "routes 参数无效: " + arg}
		}
		if !inline {
			if index+1 >= len(args) || strings.HasPrefix(args[index+1], "--") {
				return "", &siteError{Code: "invalid_argument", Message: "--path 缺少参数值"}
			}
			index++
			value = args[index]
		}
		path = value
	}
	return path, nil
}

func webPageResult(result map[string]any) map[string]any {
	if _, ok := result["response"]; ok {
		return sitePageResult(result)
	}
	return result
}

func findWebRoute(value string) (webRoute, bool) {
	needle := strings.ToLower(strings.TrimSpace(value))
	for _, route := range webRoutes {
		if strings.ToLower(route.command) == needle || strings.ToLower(route.name) == needle {
			return route, true
		}
	}
	return webRoute{}, false
}

func webCatalogResult() map[string]any {
	catalog := make([]map[string]any, 0, len(webRoutes))
	groupCodes := make(map[string]map[string]string, len(webSecondLevelMenus))
	for _, group := range webSecondLevelMenus {
		groupCodes[group["name"]] = group
	}
	known := make([]map[string]any, 0, len(webRoutes))
	for _, route := range webRoutes {
		group := groupCodes[route.group]
		catalog = append(catalog, map[string]any{"command": route.command, "group": route.group, "group_code": group["code"], "menu": group["menu"], "name": route.name, "path": route.path})
		known = append(known, map[string]any{"name": route.command, "title": route.name, "path": route.path})
	}
	conditional := []map[string]string{{"command": "graduation-design", "name": "毕业设计", "kind": "external-sso", "path": "https://oauth.fanyu.com/sso/cas/10536/1004"}}
	return map[string]any{
		"ok": true, "submitted": false, "confirmed": true, "evidence": "confirmed",
		"menus": webMainMenus, "groups": webSecondLevelMenus, "catalog": catalog, "known": known,
		"route_count": len(catalog), "public": webPublicRoutes, "conditional": conditional,
	}
}

func (a NativeSite) runWebRoutes(ctx context.Context, path string) (map[string]any, *siteError) {
	if path == "" {
		path = "/jsxsd/framework/xsMain.jsp"
	}
	result, err := a.executeAcademicRequestWithRecovery(ctx, siteRequest{
		Service: "academic", Path: path, Method: "GET", CookieFile: academicCookiePath(), RequireLogin: true, ReadOnly: true, Yes: true,
	})
	if err != nil {
		return nil, err
	}
	pageResult := sitePageResult(result)
	page, _ := pageResult["response"].(map[string]any)
	discovered := discoveredWebRoutes(page)
	catalog := webCatalogResult()
	catalog["url"] = page["url"]
	catalog["discovered"] = discovered
	catalog["page"] = page
	return catalog, nil
}

func discoveredWebRoutes(page map[string]any) []map[string]any {
	result := make([]map[string]any, 0)
	seen := map[string]bool{}
	pageURL := fmt.Sprint(page["url"])
	if links, ok := page["links"].([]map[string]any); ok {
		for _, link := range links {
			path := fmt.Sprint(link["path"])
			if !strings.HasPrefix(path, "/jsxsd/") {
				continue
			}
			key := fmt.Sprint(link["text"]) + "\x1f" + path
			if !seen[key] {
				seen[key] = true
				result = append(result, map[string]any{"name": link["text"], "path": path, "method": "GET", "source": "link"})
			}
		}
	}
	if actions, ok := page["actions"].([]map[string]any); ok {
		for _, action := range actions {
			if strings.ToUpper(fmt.Sprint(action["method"])) != "GET" {
				continue
			}
			path := fmt.Sprint(action["target"])
			if !strings.HasPrefix(path, "/jsxsd/") || pageURL == "" {
				continue
			}
			key := fmt.Sprint(action["text"]) + "\x1f" + path
			if !seen[key] {
				seen[key] = true
				result = append(result, map[string]any{"name": action["text"], "path": path, "method": "GET", "ref": action["ref"], "source": "action"})
			}
		}
	}
	return result
}

func (a NativeSite) resolveWebRoute(ctx context.Context, name string) (string, *siteError) {
	requested := strings.TrimSpace(name)
	if requested == "" {
		return "", &siteError{Code: "invalid_argument", Message: "页面名称不能为空"}
	}
	result, err := a.runWebRoutes(ctx, "/jsxsd/framework/xsMain.jsp")
	if err != nil {
		return "", err
	}
	discovered, _ := result["discovered"].([]map[string]any)
	needle := strings.ToLower(requested)
	if route, found := findWebRoute(requested); found {
		needle = strings.ToLower(route.name)
	}
	candidates := make([]map[string]any, 0)
	for _, item := range discovered {
		if strings.ToLower(strings.TrimSpace(fmt.Sprint(item["name"]))) == needle {
			candidates = append(candidates, item)
		}
	}
	if len(candidates) == 0 && needle != strings.ToLower(requested) {
		for _, item := range discovered {
			if strings.Contains(strings.ToLower(strings.TrimSpace(fmt.Sprint(item["name"]))), needle) {
				candidates = append(candidates, item)
			}
		}
	}
	if len(candidates) == 1 {
		return fmt.Sprint(candidates[0]["path"]), nil
	}
	if len(candidates) > 1 {
		return "", &siteError{Code: "ambiguous_target", Message: "页面名称对应多个入口，请改用 --path", Details: map[string]any{"name": name, "candidates": candidates}}
	}
	return "", &siteError{Code: "route_not_found", Message: "当前页面未发现该入口", Details: map[string]any{"name": name}}
}

func (a NativeSite) runWebGraduation(ctx context.Context, args []string) (map[string]any, *siteError) {
	fetch, output := false, ""
	for index := 0; index < len(args); index++ {
		arg, value, inline := splitInline(args[index])
		switch arg {
		case "--json":
			continue
		case "--fetch":
			if inline {
				return nil, &siteError{Code: "invalid_argument", Message: "布尔参数不接受 =VALUE"}
			}
			fetch = true
		case "--output":
			if !inline {
				if index+1 >= len(args) || strings.HasPrefix(args[index+1], "--") {
					return nil, &siteError{Code: "invalid_argument", Message: "--output 缺少参数值"}
				}
				index++
				value = args[index]
			}
			output = expandUserPath(value)
		default:
			return nil, &siteError{Code: "invalid_argument", Message: "web graduation-design 参数无效: " + arg}
		}
	}
	if output != "" && !fetch {
		return nil, &siteError{Code: "invalid_argument", Message: "--output 需要同时指定 --fetch"}
	}
	landing, err := a.executeAcademicRequestWithRecovery(ctx, siteRequest{Service: "academic", Path: "/jsxsd/framework/xsMain.jsp", Method: "GET", CookieFile: academicCookiePath(), RequireLogin: true, ReadOnly: true, Yes: true})
	if err != nil {
		return nil, err
	}
	response, _ := landing["response"].(map[string]any)
	source, _ := response["body_internal"].(string)
	if source == "" {
		source, _ = response["body"].(string)
	}
	pageURL, _ := response["raw_url"].(string)
	if pageURL == "" {
		pageURL, _ = response["url"].(string)
	}
	document, parseErr := parsePage(source)
	if parseErr != nil {
		return nil, &siteError{Code: "parse_error", Message: parseErr.Error()}
	}
	target := ""
	found := false
	for _, node := range document.findAll("a") {
		if strings.Contains(strings.ToLower(node.attr("onclick")), "towptjbs") {
			found = true
			_, target = pageActionTarget(node, nil, document, pageURL)
			break
		}
	}
	if !found {
		return nil, &siteError{Code: "feature_unavailable", Message: "当前账号没有毕业设计入口"}
	}
	parsed, targetErr := validateGraduationTarget(target)
	if targetErr != nil {
		return nil, targetErr
	}
	result := map[string]any{"ok": true, "submitted": false, "confirmed": true, "evidence": "confirmed", "external": true, "url": safeSiteURL(parsed), "method": "GET"}
	if !fetch {
		return result, nil
	}
	fetched, fetchErr := a.execute(ctx, siteRequest{Target: parsed, CookieFile: academicCookiePath(), Method: "GET", Output: output, ReadOnly: true, Yes: true})
	if fetchErr != nil {
		return nil, fetchErr
	}
	if output != "" {
		result["download"] = fetched
		return result, nil
	}
	result["response"] = sitePageResult(fetched)["response"]
	return result, nil
}

func academicCookiePath() string {
	if value := os.Getenv("CSUST_COOKIE_FILE"); value != "" {
		return expandUserPath(value)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".", ".csust-cookies", "cookies.txt")
	}
	return filepath.Join(home, ".config", "csust-cli", "cookies.txt")
}

func flagPresent(values []string, flag string) bool {
	for _, value := range values {
		if value == flag || strings.HasPrefix(value, flag+"=") {
			return true
		}
	}
	return false
}

func replaceFlag(values []string, from, to string) []string {
	result := append([]string(nil), values...)
	for index, value := range result {
		if value == from {
			result[index] = to
		} else if strings.HasPrefix(value, from+"=") {
			result[index] = to + strings.TrimPrefix(value, from)
		}
	}
	return result
}

func takeNamedFlag(values []string, flag string) (string, []string, *siteError) {
	result := make([]string, 0, len(values))
	for index := 0; index < len(values); index++ {
		if values[index] == flag {
			if index+1 >= len(values) {
				return "", nil, &siteError{Code: "invalid_argument", Message: flag + " 缺少参数值"}
			}
			return values[index+1], append(result, values[index+2:]...), nil
		}
		if strings.HasPrefix(values[index], flag+"=") {
			return strings.TrimPrefix(values[index], flag+"="), append(result, values[index+1:]...), nil
		}
		result = append(result, values[index])
	}
	return "", values, &siteError{Code: "invalid_argument", Message: flag + " 缺少参数值"}
}

func validatePublicPath(values []string) *siteError {
	path, remaining, err := takeNamedFlag(values, "--path")
	if err != nil {
		return err
	}
	_ = remaining
	for _, item := range webPublicRoutes {
		if item["path"] == path {
			return nil
		}
	}
	return &siteError{Code: "invalid_path", Message: "公开页面路径不在允许目录中: " + path}
}

func renderWebResult(result map[string]any) string {
	if catalog, ok := result["catalog"].([]map[string]any); ok {
		var builder strings.Builder
		for _, item := range catalog {
			builder.WriteString(fmt.Sprintf("%v\t%v\t%v\t%v\n", item["command"], item["group"], item["name"], item["path"]))
		}
		return builder.String()
	}
	if response, ok := result["response"].(map[string]any); ok {
		return fmt.Sprintf("%v\t%v\n", response["title"], response["url"])
	}
	encoded, _ := json.MarshalIndent(result, "", "  ")
	return string(encoded) + "\n"
}
