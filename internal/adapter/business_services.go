package adapter

import (
	"context"
	"crypto/md5"
	cryptorand "crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"math/big"
	"math/bits"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// These commands are semantic adapters for services discovered outside the
// original academic/VPN catalog.  Their inputs are business concepts; the
// service paths below stay inside the adapter boundary.

type businessRequestOptions struct {
	cookieFile           string
	insecure             bool
	allowSSO             bool
	allowBusinessFailure bool
	require              bool
	headers              []pair
}

type businessService struct {
	name       string
	label      string
	service    string
	kind       string
	confidence string
	evidence   string
}

var businessServices = []businessService{
	{"graduate-notice", "研究生录取通知书", "graduate-notice", "招生", "high", "Nuxt bundle exports /api/print/admissionnotice/query/idcard and /generate/pdf; live endpoint returned JSON"},
	{"journal-transport", "交通科学与工程期刊", "journal-transport", "期刊", "high", "homepage links author/reviewer/editor login and /ajax/search returned article JSON"},
	{"journal-highways", "公路与汽运期刊", "journal-highways", "期刊", "high", "homepage links author/reviewer/editor login and /ajax/search returned article JSON"},
	{"onlinejudge", "程序设计 OnlineJudge", "onlinejudge", "竞赛", "high", "frontend bundle defines /api/problem, /api/contest, /api/submissions and /api/submission"},
	{"library-personal", "图书馆个人中心", "library-personal", "图书馆", "medium", "book.csust.edu.cn redirects ClientWeb personal center into authserver CAS"},
	{"employment", "云就业平台", "employment", "就业", "high", "official homepage embeds career, job_fair and online data and exposes student/company modules"},
	{"student-record-query", "学生学籍档案查询预约", "student-record-query", "档案", "high", "linked external page returned title 统招生学籍查询_长沙理工大学档案馆 查询预约系统"},
	{"staff-record-appointment", "教工人事档案预约", "student-record-query", "档案", "high", "official archive page exposes personal/unit appointment forms fid=4/5 with live fields and token"},
	{"sunshine", "教育阳光服务", "sunshine", "诉求服务", "high", "official homepage links 阳光服务; live Angular API exposes public issues, detail, departments, statistics, system limits and phone verification"},
	{"equipment", "实验室仪器", "equipment", "实验", "high", "live equipmentlist.js exposes encrypted GetApparatusList_Nei, GetIndexDevBm, GetDevListCols and GetApparatusOne APIs"},
	{"recruitment", "人才招聘", "recruitment", "招聘", "high", "live rczpw public SM2 ajaxService exposes channels, notices, organizations, positions and position detail"},
	{"professional-learning", "专业技术人员继续教育", "jxjy", "继续教育", "high", "live jxjy public course, category, notice and course-detail APIs"},
	{"institutional-learning", "事业单位工作人员继续教育", "zyjx", "继续教育", "high", "live zyjx public course, category, notice and course-detail APIs"},
	{"transport-mobile", "交通运输工程综合信息", "transport-mobile", "学院管理", "high", "live WiJat SPA defines token authentication, user profile, pending count, public dictionaries and protected defense, finance, note, access, achievement, KPI, notice, workflow and vacation tables"},
	{"continuing-info", "继续教育学生信息管理", "continuing-info", "继续教育", "high", "10.255.196.10:8080 returned ASP.NET student information login"},
	{"party-school-exam", "党校评教和考试", "party-school-exam", "考试", "high", "mobile login returned documented status codes 0/1/2/3/4/-2 and page links exam/score"},
	{"student-archive", "学生档案管理", "student-archive", "档案", "high", "10.255.196.138:8060 returned Vue archive SPA and archive API modules"},
	{"archive-management", "综合档案管理", "archive-management", "档案", "high", "DAS login returned Vue archive collection/user/file API modules"},
	{"virtual-lab", "公路交通虚拟仿真实验中心", "virtual-lab", "实验", "high", "official page link target returned HTTP 200 and titled virtual simulation center"},
	{"graduate-admissions", "研究生招生旧系统", "graduate-admissions", "招生", "medium", "official graduate pages link zsgl/bswb and ksxt paths; root currently IIS default"},
	{"legacy-mail", "旧邮件改密入口", "legacy-mail", "邮件", "low", "mail/changepass redirects to CAS but root returns 404"},
	{"security-admin", "安全运维管理平台", "security-admin", "运维", "medium", "baolei host returned NSFOCUS OSMS page; administrative scope"},
	{"cms-admin", "内容后台", "cms-admin", "后台", "medium", "official pages expose 10.255.196.62:8080/system/login.jsp"},
	{"cms-admin-legacy", "旧内容后台", "cms-admin-legacy", "后台", "medium", "official pages expose 10.255.196.2:8080/system/login.jsp"},
}

var journalCSRF = regexp.MustCompile(`CsrfCheckCode=([A-Za-z0-9]+)`)
var recordFormToken = regexp.MustCompile(`name=["']token["'][^>]*value=["']([^"']+)["']`)

func businessCommand(value string) bool {
	switch value {
	case "services", "service", "ehall", "admission-notice", "admission", "journal", "employment", "onlinejudge", "judge", "party-exam", "archive", "student-record", "records", "staff-record", "sunshine", "equipment", "recruitment", "professional-learning", "institutional-learning", "transport-mobile", "continuing-education", "virtual-lab", "library-center", "graduate-admissions", "legacy-mail", "security-admin", "cms-admin", "cms-admin-legacy":
		return true
	default:
		return false
	}
}

func (a NativeSite) runBusinessCommand(ctx context.Context, args []string, jsonMode bool) (bool, []byte, []byte, int, error) {
	if len(args) == 0 || !businessCommand(args[0]) {
		return false, nil, nil, 0, nil
	}
	result, runErr := a.executeBusinessCommand(ctx, args)
	if runErr != nil {
		if jsonMode {
			return true, errorJSON(runErr), nil, 2, nil
		}
		return true, nil, []byte("错误: " + runErr.Error() + "\n"), 2, nil
	}
	if jsonMode {
		return true, mustJSON(result), nil, 0, nil
	}
	return true, []byte(renderBusinessResult(result)), nil, 0, nil
}

func (a NativeSite) executeBusinessCommand(ctx context.Context, args []string) (map[string]any, *siteError) {
	if err := validateBusinessArgs(args); err != nil {
		return nil, err
	}
	switch args[0] {
	case "services", "service":
		return businessCatalog(), nil
	case "ehall":
		return a.executeEhall(ctx, args[1:])
	case "admission-notice", "admission":
		return a.executeAdmissionNotice(ctx, args[1:])
	case "journal":
		return a.executeJournal(ctx, args[1:])
	case "employment":
		return a.executeEmployment(ctx, args[1:])
	case "onlinejudge", "judge":
		return a.executeOnlineJudge(ctx, args[1:])
	case "party-exam":
		return a.executePartyExam(ctx, args[1:])
	case "archive":
		return a.executeArchive(ctx, args[1:])
	case "student-record", "records":
		return a.executeStudentRecord(ctx, args[1:])
	case "staff-record":
		return a.executeStaffRecord(ctx, args[1:])
	case "sunshine":
		return a.executeSunshine(ctx, args[1:])
	case "equipment":
		return a.executeEquipment(ctx, args[1:])
	case "recruitment":
		return a.executeRecruitment(ctx, args[1:])
	case "professional-learning", "institutional-learning":
		return a.executeLearning(ctx, args[1:], args[0])
	case "transport-mobile":
		return a.executeTransportMobile(ctx, args[1:])
	case "continuing-education":
		return a.executeContinuingEducation(ctx, args[1:])
	case "virtual-lab":
		return a.executeVirtualLab(ctx, args[1:])
	case "library-center":
		return a.executeSSOServiceCommand(ctx, args[1:], "library-personal", "图书馆个人中心")
	case "graduate-admissions":
		return a.executeGraduateAdmissions(ctx, args[1:])
	case "legacy-mail":
		return a.executeSSOServiceCommand(ctx, args[1:], "legacy-mail", "旧邮件改密入口")
	case "security-admin":
		return a.executeSecurityAdmin(ctx, args[1:])
	case "cms-admin":
		return a.executeCMSAdmin(ctx, args[1:], "cms-admin", "内容后台")
	case "cms-admin-legacy":
		return a.executeCMSAdmin(ctx, args[1:], "cms-admin-legacy", "旧内容后台")
	default:
		return nil, &siteError{Code: "invalid_argument", Message: "未知业务命令: " + args[0]}
	}
}

func validateBusinessArgs(args []string) *siteError {
	if len(args) == 0 {
		return &siteError{Code: "invalid_argument", Message: "缺少业务命令"}
	}
	service := args[0]
	switch service {
	case "admission":
		service = "admission-notice"
	case "judge":
		service = "onlinejudge"
	case "records":
		service = "student-record"
	case "party-exam":
		service = "party-school-exam"
	}
	operation := "catalog"
	if len(args) > 1 && !strings.HasPrefix(args[1], "--") {
		operation = args[1]
	}
	allowed := businessAllowedFlags(service, operation)
	for _, arg := range args[1:] {
		if !strings.HasPrefix(arg, "--") {
			continue
		}
		name := strings.SplitN(arg, "=", 2)[0]
		if name == "--json" {
			continue
		}
		if !allowed[name] {
			return &siteError{Code: "invalid_argument", Message: service + " " + operation + " 不支持参数: " + name}
		}
	}
	return nil
}

func businessAllowedFlags(service, operation string) map[string]bool {
	allowed := make(map[string]bool)
	add := func(values ...string) {
		for _, value := range values {
			allowed[value] = true
		}
	}
	common := func() { add("--cookie-file") }
	switch service {
	case "services":
	case "admission-notice":
		switch operation {
		case "query":
			common()
			add("--code", "--id-card", "--password", "--password-stdin")
		case "print":
			common()
			add("--id-card", "--output", "--password", "--password-stdin")
		}
	case "journal":
		switch operation {
		case "search":
			common()
			add("--journal", "--query", "--author", "--year", "--keyword", "--field", "--page", "--page-size")
		case "article":
			common()
			add("--journal", "--id")
		case "login":
			common()
			add("--journal", "--role", "--username", "--password", "--password-stdin", "--captcha", "--captcha-image")
		case "logout":
			common()
			add("--journal")
		}
	case "employment":
		switch operation {
		case "home":
			common()
		case "list":
			common()
			add("--kind")
		case "detail":
			common()
			add("--kind", "--id")
		}
	case "onlinejudge":
		common()
		add("--insecure")
		switch operation {
		case "problems":
			add("--page", "--limit", "--keyword", "--difficulty", "--tag")
		case "problem", "contest", "submission":
			add("--id")
		case "contests":
			add("--page", "--limit")
		case "submissions":
			add("--page", "--limit", "--username", "--problem-id", "--contest-id", "--language", "--result", "--myself")
		case "user":
			add("--username")
		case "submit":
			add("--problem-id", "--language", "--code", "--contest-id", "--yes")
		}
	case "party-school-exam":
		common()
		if operation == "login" {
			add("--username", "--password", "--password-stdin", "--checkcode")
		}
	case "archive":
		common()
		add("--system", "--access-token")
		switch operation {
		case "login":
			add("--username", "--password", "--password-stdin", "--check-key", "--captcha", "--captcha-image", "--remember")
		case "report":
			add("--report-code", "--page", "--page-size", "--filter", "--sort")
		}
	case "student-record":
		common()
		switch operation {
		case "upload":
			add("--yes", "--field", "--file")
		case "request":
			add("--yes", "--type", "--name", "--id-card", "--phone", "--education", "--enroll", "--graduate", "--class", "--origin", "--college", "--major", "--recipient-phone", "--recipient-email", "--captcha", "--purpose", "--content", "--school", "--work", "--unit-letter-token", "--photo-token", "--recipient-address", "--recipient-name", "--notes")
		}
	case "staff-record":
		common()
		switch operation {
		case "form":
			add("--kind")
		case "upload":
			add("--kind", "--yes", "--file")
		case "request":
			add("--kind", "--yes", "--subject-name", "--birth-date", "--employee-id", "--subject-unit", "--applicant-name", "--phone", "--unit", "--introduction-token", "--introduction-file", "--usage", "--reason", "--content", "--appointment-date", "--captcha", "--captcha-image")
		}
	case "sunshine":
		common()
		switch operation {
		case "issues":
			add("--page", "--page-size", "--status", "--include-retracted")
		case "issue":
			add("--id")
		case "submit", "create", "suggestion", "complaint":
			add("--title", "--name", "--department", "--department-id", "--content", "--type", "--expected-date", "--date-expected", "--reporter", "--phone", "--email", "--role", "--code", "--attachment", "--public", "--private", "--yes")
		case "send-code":
			add("--phone", "--yes")
		}
	case "equipment":
		common()
		switch operation {
		case "list", "instruments":
			add("--keyword", "--department-id", "--lab-id", "--category-id", "--discipline", "--year", "--year-to", "--page", "--page-size", "--all")
		case "detail":
			add("--id")
		}
	case "recruitment":
		common()
		switch operation {
		case "notices":
			add("--channel", "--page", "--page-size")
		case "filters", "organizations":
			add("--channel")
		case "positions", "list":
			add("--channel", "--unit", "--keyword", "--page", "--page-size")
		case "position", "detail":
			add("--channel", "--id")
		}
	case "professional-learning", "institutional-learning":
		common()
		switch operation {
		case "courses", "list":
			add("--keyword", "--kind", "--category-id", "--year", "--level", "--plan-type", "--min-hours", "--max-hours", "--page", "--page-size")
		case "categories":
		case "course", "detail":
			add("--code", "--id", "--include-video-url")
		case "notices":
			add("--page", "--page-size")
		case "notice":
			add("--id")
		}
	case "transport-mobile":
		common()
		add("--access-token")
		switch operation {
		case "login":
			add("--username", "--password", "--password-stdin", "--phone", "--sms-code", "--remember")
		case "send-code":
			add("--phone", "--yes")
		case "profile", "pending":
		case "dictionaries", "dict":
			add("--code")
		case "defenses":
			add("--keyword", "--page", "--page-size")
		case "notes", "access-records", "achievements", "kpis", "notices", "workflows", "vacations":
			add("--keyword", "--page", "--page-size")
		case "defense", "finance":
			add("--id")
		case "finances":
			add("--keyword", "--page", "--page-size")
		case "logout":
		}
	case "ehall":
		common()
		if operation == "mail-status" || operation == "mail" || operation == "service-item-favorites" || operation == "item-favorites" {
			return allowed
		}
		if operation == "news" {
			add("--channel", "--page")
		}
		if operation == "rating" || operation == "service-rating" {
			add("--id", "--page", "--page-size")
		}
		if operation == "service-item-favorite" {
			add("--item-id", "--folder-id", "--yes")
		}
		if operation == "service" || operation == "detail" || operation == "health" {
			add("--id")
		}
		if operation == "favorite" {
			add("--service-id", "--folder-id", "--yes")
		}
	case "continuing-education":
		common()
		if operation == "login" {
			add("--username", "--password", "--password-stdin")
		}
	case "virtual-lab":
		common()
		switch operation {
		case "resources":
			add("--discipline")
		case "messages":
			add("--keyword")
		case "login":
			add("--username", "--password", "--password-stdin", "--captcha", "--captcha-image")
		case "register":
			add("--yes", "--username", "--real-name", "--phone", "--password", "--question", "--answer", "--photo-token", "--captcha", "--captcha-image")
		case "forgot":
			add("--yes", "--username", "--question", "--answer", "--new-password", "--captcha", "--captcha-image")
		case "upload-photo":
			add("--yes", "--file")
		}
	case "library-center", "legacy-mail":
		common()
		if operation == "login" {
			add("--username", "--password-stdin", "--auth", "--captcha", "--captcha-image")
		}
	case "graduate-admissions":
		common()
		if operation == "login" {
			add("--username", "--password", "--password-stdin", "--captcha", "--captcha-image")
		}
	case "security-admin":
		common()
		add("--insecure")
		if operation == "login" {
			add("--username", "--password", "--password-stdin")
		}
	case "cms-admin", "cms-admin-legacy":
		common()
		if operation == "login" {
			add("--username", "--password", "--password-stdin", "--captcha", "--captcha-image", "--scope")
		}
	}
	return allowed
}

func businessCatalog() map[string]any {
	items := make([]map[string]any, 0, len(businessServices))
	for _, item := range businessServices {
		info := knownSites[item.service]
		entry := map[string]any{
			"name":                item.name,
			"label":               item.label,
			"service":             item.service,
			"host":                info.host,
			"kind":                item.kind,
			"url":                 info.scheme + "://" + info.host + info.path,
			"confidence":          item.confidence,
			"confidence_evidence": item.evidence,
		}
		if item.name == "staff-record-appointment" {
			entry["url"] = info.scheme + "://" + info.host + staffRecordFormPath("personal")
			entry["entrypoints"] = map[string]string{"personal": staffRecordFormPath("personal"), "unit": staffRecordFormPath("unit")}
		}
		items = append(items, entry)
	}
	sort.Slice(items, func(i, j int) bool { return items[i]["name"].(string) < items[j]["name"].(string) })
	return map[string]any{
		"ok": true, "submitted": false, "confirmed": true, "evidence": "confirmed",
		"catalog": items, "source": "official links, live service responses and frontend bundles",
	}
}

func businessValue(args []string, flag string) (string, bool, *siteError) {
	for index, arg := range args {
		if arg == flag {
			if index+1 >= len(args) || strings.HasPrefix(args[index+1], "--") {
				return "", true, &siteError{Code: "invalid_argument", Message: flag + " 缺少参数值"}
			}
			return args[index+1], true, nil
		}
		if strings.HasPrefix(arg, flag+"=") {
			return strings.TrimPrefix(arg, flag+"="), true, nil
		}
	}
	return "", false, nil
}

func businessBool(args []string, flag string) bool {
	for _, arg := range args {
		if arg == flag {
			return true
		}
	}
	return false
}

func businessInt(args []string, flag string, fallback int) (int, *siteError) {
	value, found, err := businessValue(args, flag)
	if err != nil {
		return 0, err
	}
	if !found || value == "" {
		return fallback, nil
	}
	parsed, parseErr := strconv.Atoi(value)
	if parseErr != nil || parsed < 1 {
		return 0, &siteError{Code: "invalid_argument", Message: flag + " 必须是正整数"}
	}
	return parsed, nil
}

func businessRequired(args []string, flag, message string) (string, *siteError) {
	value, found, err := businessValue(args, flag)
	if err != nil {
		return "", err
	}
	if !found || strings.TrimSpace(value) == "" {
		return "", &siteError{Code: "invalid_argument", Message: message}
	}
	return value, nil
}

func businessSecret(args []string, flag, envName string) (string, *siteError) {
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
	if value := os.Getenv(envName); value != "" {
		return value, nil
	}
	return "", &siteError{Code: "credentials_required", Message: "请使用 " + flag + "-stdin 或环境变量 " + envName}
}

func businessCredentials(args []string, passwordEnv string) (string, string, *siteError) {
	username := flagValue(args, "--username")
	password, found, err := businessValue(args, "--password")
	if err != nil {
		return "", "", err
	}
	if !found && businessBool(args, "--password-stdin") {
		value, readErr := readBoundedSiteInput(os.Stdin)
		if readErr != nil {
			if errors.Is(readErr, errSiteRequestTooLarge) {
				return "", "", siteRequestTooLarge("标准输入密码")
			}
			return "", "", &siteError{Code: "credentials_required", Message: "无法读取标准输入秘密: " + readErr.Error()}
		}
		password = strings.TrimRight(string(value), "\r\n")
	} else if !found {
		password = os.Getenv(passwordEnv)
	}
	account, password, credentialErr := credentialsGo(username, password)
	if credentialErr != nil {
		return "", "", credentialErr
	}
	return account, password, nil
}

func businessRequest(ctx context.Context, service, method, path string, params, data, headers []pair, options businessRequestOptions, readOnly, yes bool) (map[string]any, *siteError) {
	request := siteRequest{
		Service: service, Method: method, Path: path, Params: params, Data: data, Headers: append(options.headers, headers...),
		CookieFile: options.cookieFile, AllowSSO: options.allowSSO, AllowBusinessFailure: options.allowBusinessFailure, RequireLogin: options.require,
		ReadOnly: readOnly, Yes: yes, InsecureTLS: options.insecure, RawJSON: true,
	}
	return (NativeSite{}).execute(ctx, request)
}

func (a NativeSite) businessGet(ctx context.Context, service, path string, params []pair, options businessRequestOptions) (map[string]any, *siteError) {
	request := siteRequest{Service: service, Method: "GET", Path: path, Params: params, Headers: options.headers, CookieFile: options.cookieFile, AllowSSO: options.allowSSO, AllowBusinessFailure: options.allowBusinessFailure, RequireLogin: options.require, ReadOnly: true, Yes: true, InsecureTLS: options.insecure, RawJSON: true}
	return a.execute(ctx, request)
}

func (a NativeSite) businessPostJSON(ctx context.Context, service, path string, body any, options businessRequestOptions) (map[string]any, *siteError) {
	return a.execute(ctx, siteRequest{
		Service: service, Method: "POST", Path: path, JSON: body, HasJSON: true, RawJSON: true,
		Headers: options.headers, CookieFile: options.cookieFile, AllowSSO: options.allowSSO,
		AllowBusinessFailure: options.allowBusinessFailure, RequireLogin: options.require, ReadOnly: true, Yes: true, InsecureTLS: options.insecure,
	})
}

func removeInternalResponseJSON(result map[string]any) {
	if response, ok := result["response"].(map[string]any); ok {
		delete(response, "json_internal")
	}
}

func businessData(result map[string]any) (any, bool) {
	response, ok := result["response"].(map[string]any)
	if !ok {
		return nil, false
	}
	if value, exists := response["json_internal"]; exists {
		return value, true
	}
	if value, exists := response["json"]; exists {
		return value, true
	}
	return nil, false
}

func businessResult(result map[string]any, service, operation string) map[string]any {
	result["service"] = service
	result["operation"] = operation
	if value, ok := businessData(result); ok {
		if envelope, isEnvelope := value.(map[string]any); isEnvelope {
			if code, exists := envelope["code"]; exists {
				result["api_code"] = code
			}
			if message, exists := envelope["msg"]; exists {
				result["api_message"] = message
			}
			if apiError, exists := envelope["error"]; exists && apiError != nil {
				result["api_error"] = apiError
			}
			if data, exists := envelope["data"]; exists {
				result["data"] = data
			} else {
				result["data"] = value
			}
		} else {
			result["data"] = value
		}
		delete(result, "response")
	}
	return result
}

func businessJSONMap(result map[string]any) (map[string]any, *siteError) {
	value, ok := businessData(result)
	if !ok {
		return nil, &siteError{Code: "parse_error", Message: "业务响应不是 JSON 对象"}
	}
	decoded, ok := value.(map[string]any)
	if !ok {
		return nil, &siteError{Code: "parse_error", Message: "业务响应 JSON 结构无效"}
	}
	return decoded, nil
}

func businessBody(result map[string]any) string {
	response, _ := result["response"].(map[string]any)
	if body, ok := response["body_internal"].(string); ok {
		return body
	}
	body, _ := response["body"].(string)
	return body
}

func renderBusinessResult(result map[string]any) string {
	result = stripSiteInternal(result).(map[string]any)
	if articles, ok := result["articles"].([]map[string]any); ok {
		var builder strings.Builder
		for _, article := range articles {
			builder.WriteString(fmt.Sprintf("%v\t%v\t%v\n", article["id"], article["title"], article["authors"]))
		}
		return builder.String()
	}
	encoded, _ := json.MarshalIndent(result, "", "  ")
	return string(encoded) + "\n"
}

func (a NativeSite) executeAdmissionNotice(ctx context.Context, args []string) (map[string]any, *siteError) {
	if len(args) == 0 || args[0] == "catalog" {
		return businessCatalogFilter("graduate-notice"), nil
	}
	operation := args[0]
	if operation != "query" && operation != "print" {
		return nil, &siteError{Code: "invalid_argument", Message: "admission-notice 只支持 query、print、catalog"}
	}
	cookie, _, err := businessValue(args[1:], "--cookie-file")
	if err != nil {
		return nil, err
	}
	if operation == "query" {
		if code, found, valueErr := businessValue(args[1:], "--code"); valueErr != nil {
			return nil, valueErr
		} else if found {
			result, requestErr := a.businessGet(ctx, "graduate-notice", "/api/print/admissionnotice/query/"+url.PathEscape(code), nil, businessRequestOptions{cookieFile: cookie})
			if requestErr != nil {
				return nil, requestErr
			}
			return businessResult(result, "graduate-notice", "query"), nil
		}
		idCard, requiredErr := businessRequired(args[1:], "--id-card", "query 必须提供 --id-card 或 --code")
		if requiredErr != nil {
			return nil, requiredErr
		}
		password, secretErr := businessSecret(args[1:], "--password", "CSUST_ADMISSION_PASSWORD")
		if secretErr != nil {
			return nil, secretErr
		}
		result, requestErr := a.businessPostJSON(ctx, "graduate-notice", "/api/print/admissionnotice/query/idcard", map[string]string{"idCard": idCard, "password": password}, businessRequestOptions{cookieFile: cookie})
		if requestErr != nil {
			return nil, requestErr
		}
		removeInternalResponseJSON(result)
		return businessResult(result, "graduate-notice", "query"), nil
	}
	idCard, requiredErr := businessRequired(args[1:], "--id-card", "print 必须提供 --id-card")
	if requiredErr != nil {
		return nil, requiredErr
	}
	output, requiredErr := businessRequired(args[1:], "--output", "print 必须提供 --output")
	if requiredErr != nil {
		return nil, requiredErr
	}
	password, secretErr := businessSecret(args[1:], "--password", "CSUST_ADMISSION_PASSWORD")
	if secretErr != nil {
		return nil, secretErr
	}
	query, requestErr := a.businessPostJSON(ctx, "graduate-notice", "/api/print/admissionnotice/query/idcard", map[string]string{"idCard": idCard, "password": password}, businessRequestOptions{cookieFile: cookie})
	if requestErr != nil {
		return nil, requestErr
	}
	payload, parseErr := businessJSONMap(query)
	if parseErr != nil {
		return nil, parseErr
	}
	token := nestedString(payload, "token")
	if token == "" {
		if data, ok := payload["data"].(map[string]any); ok {
			token = nestedString(data, "token")
		}
	}
	removeInternalResponseJSON(query)
	if token == "" {
		return nil, &siteError{Code: "business_rejected", Message: "查询成功响应缺少打印令牌", Details: map[string]any{"submitted": false, "confirmed": false, "evidence": "rejected"}}
	}
	result, downloadErr := a.execute(ctx, siteRequest{Service: "graduate-notice", Path: "/api/print/admissionnotice/generate/pdf/" + url.PathEscape(idCard), Method: "GET", Headers: []pair{{"token", token}}, CookieFile: cookie, Output: output, ReadOnly: true, Yes: true})
	if downloadErr != nil {
		return nil, downloadErr
	}
	result["service"], result["operation"] = "graduate-notice", "print"
	result["verified_by"] = "查询令牌有效且 PDF 已原子保存"
	return result, nil
}

func businessCatalogFilter(name string) map[string]any {
	return businessCatalogNames(name)
}

func businessCatalogNames(names ...string) map[string]any {
	all := businessCatalog()
	allowed := make(map[string]bool, len(names))
	for _, name := range names {
		allowed[name] = true
	}
	filtered := make([]map[string]any, 0, len(names))
	for _, item := range all["catalog"].([]map[string]any) {
		if allowed[item["name"].(string)] {
			filtered = append(filtered, item)
		}
	}
	all["catalog"] = filtered
	return all
}

func nestedString(value map[string]any, key string) string {
	if text, ok := value[key].(string); ok {
		return text
	}
	return ""
}

func (a NativeSite) executeJournal(ctx context.Context, args []string) (map[string]any, *siteError) {
	if len(args) == 0 || args[0] == "catalog" {
		return businessCatalogNames("journal-transport", "journal-highways"), nil
	}
	journal, requiredErr := businessRequired(args[1:], "--journal", "journal 必须提供 --journal transport 或 highways")
	if requiredErr != nil {
		return nil, requiredErr
	}
	prefix, service, ok := journalService(journal)
	if !ok {
		return nil, &siteError{Code: "invalid_argument", Message: "--journal 只能是 transport 或 highways"}
	}
	switch args[0] {
	case "search":
		return a.journalSearch(ctx, args[1:], prefix, service)
	case "article":
		id, err := businessRequired(args[1:], "--id", "article 必须提供 --id")
		if err != nil {
			return nil, err
		}
		cookie, _, valueErr := businessValue(args[1:], "--cookie-file")
		if valueErr != nil {
			return nil, valueErr
		}
		result, requestErr := a.businessGet(ctx, service, "/"+prefix+"/article/abstract/"+url.PathEscape(id), nil, businessRequestOptions{cookieFile: cookie})
		if requestErr != nil {
			return nil, requestErr
		}
		result = sitePageResult(result)
		result["service"], result["operation"], result["article_id"] = service, "article", id
		return result, nil
	case "login":
		return a.journalLogin(ctx, args[1:], prefix, service)
	case "logout":
		cookie, _, valueErr := businessValue(args[1:], "--cookie-file")
		if valueErr != nil {
			return nil, valueErr
		}
		_, cookiePath, resolveErr := resolveSite(siteRequest{Service: service, CookieFile: cookie})
		if resolveErr != nil {
			return nil, resolveErr
		}
		if removeErr := removeCookieFile(cookiePath); removeErr != nil {
			return nil, &siteError{Code: "cookie_write_failed", Message: removeErr.Error()}
		}
		return map[string]any{"ok": true, "submitted": false, "confirmed": true, "evidence": "local-cookie-removed", "service": service, "operation": "logout", "logged_out": true, "cookie_file": cookiePath}, nil
	default:
		return nil, &siteError{Code: "invalid_argument", Message: "journal 只支持 search、article、login、logout、catalog"}
	}
}

func journalService(value string) (prefix, service string, ok bool) {
	switch strings.ToLower(value) {
	case "transport", "jtkxygc", "交通科学与工程":
		return "jtkxygc", "journal-transport", true
	case "highways", "glyqy", "公路与汽运":
		return "glyqy", "journal-highways", true
	default:
		return "", "", false
	}
}

const journalLoginModulus = "90B105D7925701AEFC63535FB064A71A4479D44CD4283C65E6F6CD97A816A5270FF314F45D93FCA0A5FEE35692B5625F0CDFB14C02254F70F244211737AA896D89950D7ECD0CC64921C1B31F5C712F5C4E12EBFB162D83A528BD33BF48D394DA34D0553C9A7B6B1E643E34A0C72D69C31D1AAB3B1BAB3A3F51077EF19"

func journalRole(value string) string {
	switch strings.ToLower(value) {
	case "author", "作者":
		return "author"
	case "reviewer", "审稿", "审稿人":
		return "reviewer"
	case "editor", "编辑":
		return "editor"
	default:
		return ""
	}
}

func journalLoginPage(body string) bool {
	return strings.Contains(body, "EtUserName") && strings.Contains(body, "EtPwd")
}

func journalEncryptedPassword(password string) (string, *siteError) {
	return journalEncryptedPasswordWithRandom(password, cryptorand.Reader)
}

func journalEncryptedPasswordWithRandom(password string, random io.Reader) (string, *siteError) {
	number, ok := new(big.Int).SetString(journalLoginModulus, 16)
	if !ok {
		return "", &siteError{Code: "protocol_error", Message: "期刊登录 RSA 公钥无效"}
	}
	digest := md5.Sum([]byte(password))
	plain := password + "#" + hex.EncodeToString(digest[:])
	message := []byte(plain)
	digitSize := 2 * ((len(journalLoginModulus) + 3) / 4)
	if len(message) > digitSize-11 {
		return "", &siteError{Code: "invalid_argument", Message: "期刊登录密码过长"}
	}
	padding := make([]byte, digitSize-len(message)-3)
	if _, readErr := io.ReadFull(random, padding); readErr != nil {
		return "", &siteError{Code: "protocol_error", Message: "期刊登录密码加密失败: " + readErr.Error()}
	}
	for index := range padding {
		if padding[index] == 0 {
			padding[index] = 1
		}
	}
	em := make([]byte, 0, digitSize)
	em = append(em, 0, 2)
	em = append(em, padding...)
	em = append(em, 0)
	em = append(em, message...)
	ciphertext := new(big.Int).Exp(new(big.Int).SetBytes(em), big.NewInt(65537), number).Bytes()
	if len(ciphertext) < digitSize {
		ciphertext = append(make([]byte, digitSize-len(ciphertext)), ciphertext...)
	}
	return hex.EncodeToString(ciphertext), nil
}

func (a NativeSite) journalLogin(ctx context.Context, args []string, prefix, service string) (map[string]any, *siteError) {
	role := journalRole(flagValue(args, "--role"))
	if role == "" {
		return nil, &siteError{Code: "invalid_argument", Message: "login 必须提供 --role author、reviewer 或 editor"}
	}
	username, password, credentialErr := businessCredentials(args, "CSUST_JOURNAL_PASSWORD")
	if credentialErr != nil {
		return nil, credentialErr
	}
	captcha := flagValue(args, "--captcha")
	cookie, _, valueErr := businessValue(args, "--cookie-file")
	if valueErr != nil {
		return nil, valueErr
	}
	loginPath := "/" + prefix + "/" + role + "/login"
	page, requestErr := a.businessGet(ctx, service, loginPath, nil, businessRequestOptions{cookieFile: cookie})
	if requestErr != nil {
		return nil, requestErr
	}
	csrfMatch := journalCSRF.FindStringSubmatch(businessBody(page))
	if len(csrfMatch) < 2 {
		return nil, &siteError{Code: "parse_error", Message: "期刊登录页缺少 CsrfCheckCode"}
	}
	if captcha == "" {
		_, cookiePath, resolveErr := resolveSite(siteRequest{Service: service, CookieFile: cookie})
		if resolveErr != nil {
			return nil, resolveErr
		}
		imagePath := flagValue(args, "--captcha-image")
		if imagePath == "" {
			imagePath = filepath.Join(filepath.Dir(cookiePath), cookieHost(mustParseURL(knownSites[service].scheme+"://"+knownSites[service].host))+"-journal-captcha.png")
		}
		imagePath = expandUserPath(imagePath)
		if _, imageErr := a.execute(ctx, siteRequest{Service: service, Path: "/" + prefix + "/action/validate_image", Method: "GET", Params: []pair{{"time", strconv.FormatInt(time.Now().UnixMilli(), 10)}}, CookieFile: cookie, Output: imagePath, ReadOnly: true, Yes: true}); imageErr != nil {
			return nil, imageErr
		}
		return nil, &siteError{Code: "captcha_required", Message: "期刊登录需要验证码，请提供 --captcha", Details: map[string]any{"captcha_image": imagePath}}
	}
	encrypted, encryptErr := journalEncryptedPassword(password)
	if encryptErr != nil {
		return nil, encryptErr
	}
	params := []pair{{"EtUserName", username}, {"EtRsaEncryptedPwd", encrypted}, {"Code", captcha}, {"op_type", ""}, {"file_no", ""}, {"journal_id", prefix}, {"ReturnURL", ""}, {"CsrfCheckCode", csrfMatch[1]}}
	result, requestErr := a.businessGet(ctx, service, "/"+prefix+"/"+role+"/login_submit", params, businessRequestOptions{cookieFile: cookie, allowBusinessFailure: true})
	if requestErr != nil {
		return nil, requestErr
	}
	body := strings.TrimSpace(businessBody(result))
	if strings.HasPrefix(strings.ToLower(body), "error:") {
		return nil, &siteError{Code: "authentication_failed", Message: strings.TrimSpace(strings.TrimPrefix(body, "error:")), Details: map[string]any{"submitted": true, "confirmed": false, "evidence": "rejected"}}
	}
	if failure := businessLoginResponseFailure(result); failure != nil {
		return nil, failure
	}
	if evidence, probeErr := a.confirmBusinessLogin(ctx, service, loginPath, cookie, journalLoginPage); probeErr != nil {
		return nil, probeErr
	} else {
		result["submitted"], result["confirmed"] = true, true
		result["service"], result["operation"], result["role"], result["username"] = service, "login", role, username
		result["evidence"] = "login-response-and-" + evidence
		return result, nil
	}
}

func (a NativeSite) journalSearch(ctx context.Context, args []string, prefix, service string) (map[string]any, *siteError) {
	query := flagValue(args, "--query")
	author := flagValue(args, "--author")
	year := flagValue(args, "--year")
	keyword := flagValue(args, "--keyword")
	if query == "" && author == "" && year == "" && keyword == "" {
		return nil, &siteError{Code: "invalid_argument", Message: "search 至少需要 --query、--author、--year 或 --keyword"}
	}
	field := flagValue(args, "--field")
	if field == "" {
		field = "title"
	}
	page, pageErr := businessInt(args, "--page", 1)
	if pageErr != nil {
		return nil, pageErr
	}
	pageSize, sizeErr := businessInt(args, "--page-size", 10)
	if sizeErr != nil {
		return nil, sizeErr
	}
	cookie, _, valueErr := businessValue(args, "--cookie-file")
	if valueErr != nil {
		return nil, valueErr
	}
	pageResult, requestErr := a.businessGet(ctx, service, "/"+prefix+"/article/search", nil, businessRequestOptions{cookieFile: cookie})
	if requestErr != nil {
		return nil, requestErr
	}
	csrfMatch := journalCSRF.FindStringSubmatch(businessBody(pageResult))
	if len(csrfMatch) < 2 {
		return nil, &siteError{Code: "parse_error", Message: "期刊检索页缺少 CsrfCheckCode，无法调用检索接口"}
	}
	data := []pair{{"from_year", ""}, {"to_year", ""}, {"search_type", "search"}, {"source_type", "meta"}, {"field", field}, {"key", query}, {"additional_year", year}, {"additional_author", author}, {"additional_keyword", keyword}, {"page", strconv.Itoa(page)}, {"page_size", strconv.Itoa(pageSize)}, {"CsrfCheckCode", csrfMatch[1]}}
	result, requestErr := businessRequest(ctx, service, "POST", "/"+prefix+"/ajax/search", nil, data, []pair{{"Referer", safeResponseURL(pageResult)}}, businessRequestOptions{cookieFile: cookie}, true, true)
	if requestErr != nil {
		return nil, requestErr
	}
	payload, parseErr := businessJSONMap(result)
	if parseErr != nil {
		return nil, parseErr
	}
	rows, _ := payload["rows"].([]any)
	articles := make([]map[string]any, 0, len(rows))
	for _, item := range rows {
		row, ok := item.(map[string]any)
		if !ok {
			continue
		}
		articles = append(articles, journalArticle(row, service, prefix))
	}
	return map[string]any{
		"ok": true, "submitted": false, "confirmed": true, "evidence": "journal JSON search response",
		"service": service, "operation": "search", "journal": prefix, "query": query,
		"filters":     map[string]any{"field": field, "author": author, "year": year, "keyword": keyword, "page": page, "page_size": pageSize},
		"total_pages": payload["total"], "total_records": payload["records"], "articles": articles,
	}, nil
}

func journalArticle(row map[string]any, service, prefix string) map[string]any {
	id := nestedString(row, "file_no")
	return map[string]any{
		"id": id, "journal": service, "title": row["title"], "authors": row["author_name"],
		"keywords": row["key_word"], "abstract": row["abstract"], "year": row["year_id"],
		"volume": row["volume"], "issue": row["issue"], "pages": row["position"], "doi": row["doi"],
		"citation": row["citation"],
		"links": map[string]string{
			"abstract": "https://" + prefix + ".csust.edu.cn/" + prefix + "/article/abstract/" + url.PathEscape(id),
			"html":     "https://" + prefix + ".csust.edu.cn/" + prefix + "/article/html/" + url.PathEscape(id),
			"pdf":      "https://" + prefix + ".csust.edu.cn/" + prefix + "/article/pdf/" + url.PathEscape(id),
		},
		"raw": row,
	}
}

func safeResponseURL(result map[string]any) string {
	response, _ := result["response"].(map[string]any)
	if value, ok := response["raw_url"].(string); ok && value != "" {
		return value
	}
	value, _ := response["url"].(string)
	return value
}

func (a NativeSite) executeEmployment(ctx context.Context, args []string) (map[string]any, *siteError) {
	if len(args) == 0 || args[0] == "catalog" {
		return businessCatalogFilter("employment"), nil
	}
	switch args[0] {
	case "home", "list":
		cookie, _, err := businessValue(args[1:], "--cookie-file")
		if err != nil {
			return nil, err
		}
		result, requestErr := a.businessGet(ctx, "employment", "/", nil, businessRequestOptions{cookieFile: cookie})
		if requestErr != nil {
			return nil, requestErr
		}
		data, parseErr := decodeJSAssignment(businessBody(result), "const data =")
		if parseErr != nil {
			return nil, parseErr
		}
		if args[0] == "home" {
			return map[string]any{"ok": true, "submitted": false, "confirmed": true, "evidence": "homepage embedded JSON assignment", "service": "employment", "operation": "home", "data": data}, nil
		}
		kind, requiredErr := businessRequired(args[1:], "--kind", "list 必须提供 --kind career、job-fair、online 或 news")
		if requiredErr != nil {
			return nil, requiredErr
		}
		if !employmentKind(kind, false) {
			return nil, &siteError{Code: "invalid_argument", Message: "list --kind 只能是 career、job-fair、online 或 news"}
		}
		items := employmentItems(data, kind)
		return map[string]any{"ok": true, "submitted": false, "confirmed": true, "evidence": "homepage embedded JSON assignment", "service": "employment", "operation": "list", "kind": kind, "items": items}, nil
	case "detail":
		kind, requiredErr := businessRequired(args[1:], "--kind", "detail 必须提供 --kind career、job-fair、online 或 job")
		if requiredErr != nil {
			return nil, requiredErr
		}
		if !employmentKind(kind, true) {
			return nil, &siteError{Code: "invalid_argument", Message: "detail --kind 只能是 career、job-fair、online 或 job"}
		}
		id, requiredErr := businessRequired(args[1:], "--id", "detail 必须提供 --id")
		if requiredErr != nil {
			return nil, requiredErr
		}
		path := employmentDetailPath(kind, id)
		if path == "" {
			return nil, &siteError{Code: "invalid_argument", Message: "detail --kind 无效"}
		}
		cookie, _, valueErr := businessValue(args[1:], "--cookie-file")
		if valueErr != nil {
			return nil, valueErr
		}
		result, requestErr := a.businessGet(ctx, "employment", path, nil, businessRequestOptions{cookieFile: cookie})
		if requestErr != nil {
			return nil, requestErr
		}
		result = sitePageResult(result)
		result["service"], result["operation"], result["kind"], result["id"] = "employment", "detail", kind, id
		return result, nil
	default:
		return nil, &siteError{Code: "invalid_argument", Message: "employment 只支持 home、list、detail、catalog"}
	}
}

func employmentKind(value string, detail bool) bool {
	if detail {
		return value == "career" || value == "job-fair" || value == "fair" || value == "online" || value == "job"
	}
	return value == "career" || value == "job-fair" || value == "online" || value == "news"
}

func decodeJSAssignment(source, marker string) (map[string]any, *siteError) {
	index := strings.Index(source, marker)
	if index < 0 {
		return nil, &siteError{Code: "parse_error", Message: "响应缺少结构化数据赋值: " + marker}
	}
	var value map[string]any
	decoder := json.NewDecoder(strings.NewReader(source[index+len(marker):]))
	if err := decoder.Decode(&value); err != nil || value == nil {
		if err == nil {
			err = fmt.Errorf("对象为空")
		}
		return nil, &siteError{Code: "parse_error", Message: "就业首页 JSON 解析失败: " + err.Error()}
	}
	return value, nil
}

func employmentItems(data map[string]any, kind string) []map[string]any {
	keys := map[string][]string{"career": {"career"}, "job-fair": {"job_fair", "jobFair"}, "online": {"online", "online_recruitment"}, "news": {"news", "notice", "notices"}}
	var value any
	for _, key := range keys[kind] {
		if value = data[key]; value != nil {
			break
		}
	}
	items := []map[string]any{}
	if list, ok := value.([]any); ok {
		for _, item := range list {
			if row, ok := item.(map[string]any); ok {
				items = append(items, employmentItem(row, kind))
			}
		}
	}
	return items
}

func employmentItem(row map[string]any, kind string) map[string]any {
	item := map[string]any{"kind": kind, "raw": row}
	for _, key := range []string{"career_talk_id", "fair_id", "recruitment_id", "notice_id", "publish_id"} {
		if value := row[key]; value != nil {
			item["id"] = value
			break
		}
	}
	for _, key := range []string{"meet_name", "title", "name", "company_name", "notice_name"} {
		if value := row[key]; value != nil && value != "" {
			item["title"] = value
			break
		}
	}
	for _, key := range []string{"company_name", "school_name", "city_name", "address", "meet_time", "create_time"} {
		if value := row[key]; value != nil && value != "" {
			item[key] = value
		}
	}
	return item
}

func employmentDetailPath(kind, id string) string {
	switch kind {
	case "career":
		return "/detail/career?id=" + url.QueryEscape(id)
	case "job-fair", "fair":
		return "/detail/jobfair?id=" + url.QueryEscape(id)
	case "online":
		return "/detail/online?id=" + url.QueryEscape(id)
	case "job":
		return "/detail/job?id=" + url.QueryEscape(id)
	default:
		return ""
	}
}

const studentRecordFormPath = "/?a=add&c=form&fid=2"

func (a NativeSite) executeStudentRecord(ctx context.Context, args []string) (map[string]any, *siteError) {
	if len(args) == 0 {
		args = []string{"form"}
	}
	if args[0] == "form" || args[0] == "catalog" {
		if args[0] == "catalog" {
			return businessCatalogFilter("student-record-query"), nil
		}
		cookie, _, err := businessValue(args[1:], "--cookie-file")
		if err != nil {
			return nil, err
		}
		result, requestErr := a.businessGet(ctx, "student-record-query", studentRecordFormPath, nil, businessRequestOptions{cookieFile: cookie})
		if requestErr != nil {
			return nil, requestErr
		}
		result = sitePageResult(result)
		result["service"], result["operation"] = "student-record-query", "form"
		return result, nil
	}
	if args[0] == "upload" {
		return a.studentRecordUpload(ctx, args[1:])
	}
	if args[0] != "request" {
		return nil, &siteError{Code: "invalid_argument", Message: "student-record 只支持 form、upload、request、catalog"}
	}
	return a.studentRecordRequest(ctx, args[1:])
}

func (a NativeSite) studentRecordUpload(ctx context.Context, args []string) (map[string]any, *siteError) {
	if !businessBool(args, "--yes") {
		return nil, &siteError{Code: "confirmation_required", Message: "档案材料上传会写入远端，必须加 --yes"}
	}
	field, err := businessRequired(args, "--field", "upload 必须提供 --field photo 或 unit-letter")
	if err != nil {
		return nil, err
	}
	field = strings.ToLower(field)
	if field != "photo" && field != "unit-letter" {
		return nil, &siteError{Code: "invalid_argument", Message: "--field 只能是 photo 或 unit-letter"}
	}
	filePath, err := businessRequired(args, "--file", "upload 必须提供 --file")
	if err != nil {
		return nil, err
	}
	filePath = expandUserPath(filePath)
	file, fileErr := siteFilePart("file", filePath, "上传文件")
	if fileErr != nil {
		return nil, fileErr
	}
	if file.size == 0 {
		return nil, &siteError{Code: "invalid_argument", Message: "上传文件不能为空"}
	}
	cookie, _, valueErr := businessValue(args, "--cookie-file")
	if valueErr != nil {
		return nil, valueErr
	}
	form, requestErr := a.businessGet(ctx, "student-record-query", studentRecordFormPath, nil, businessRequestOptions{cookieFile: cookie})
	if requestErr != nil {
		return nil, requestErr
	}
	result, requestErr := a.execute(ctx, siteRequest{
		Service: "student-record-query", Method: "POST", Path: "/?c=upload&a=upfile&type=1", CookieFile: cookie,
		Headers: []pair{{"Referer", safeResponseURL(form)}}, Files: []filePart{file},
		ReadOnly: false, Yes: true, RawJSON: true,
	})
	if requestErr != nil {
		return nil, requestErr
	}
	payload, parseErr := businessJSONMap(result)
	if parseErr != nil {
		return nil, parseErr
	}
	state, _ := payload["state"].(string)
	if !strings.EqualFold(state, "success") {
		return nil, &siteError{Code: "mutation_unverified", Message: "上传响应未返回 state=success", Details: map[string]any{"submitted": true, "confirmed": false, "evidence": "unknown"}}
	}
	result = businessResult(result, "student-record-query", "upload")
	result["field"] = field
	result["filename"] = filepath.Base(filePath)
	result["verified_by"] = "upload-response-state-success"
	if token, ok := payload["msg"].(string); ok && strings.TrimSpace(token) != "" {
		result["upload_token"] = token
	}
	return result, nil
}

func (a NativeSite) studentRecordRequest(ctx context.Context, args []string) (map[string]any, *siteError) {
	if !businessBool(args, "--yes") {
		return nil, &siteError{Code: "confirmation_required", Message: "档案查询预约会提交个人资料，必须加 --yes"}
	}
	cookie, _, err := businessValue(args, "--cookie-file")
	if err != nil {
		return nil, err
	}
	formResult, requestErr := a.businessGet(ctx, "student-record-query", studentRecordFormPath, nil, businessRequestOptions{cookieFile: cookie})
	if requestErr != nil {
		return nil, requestErr
	}
	tokenMatch := recordFormToken.FindStringSubmatch(businessBody(formResult))
	if len(tokenMatch) < 2 {
		return nil, &siteError{Code: "parse_error", Message: "档案预约表单缺少动态 token"}
	}
	requestType, err := businessRequired(args, "--type", "request 必须提供 --type personal 或 unit")
	if err != nil {
		return nil, err
	}
	typeValue := map[string]string{"personal": "个人查档", "unit": "单位查档", "个人查档": "个人查档", "单位查档": "单位查档"}[strings.ToLower(requestType)]
	if typeValue == "" {
		return nil, &siteError{Code: "invalid_argument", Message: "--type 只能是 personal 或 unit"}
	}
	name, err := businessRequired(args, "--name", "request 必须提供 --name")
	if err != nil {
		return nil, err
	}
	idCard, err := businessRequired(args, "--id-card", "request 必须提供 --id-card")
	if err != nil {
		return nil, err
	}
	phone, err := businessRequired(args, "--phone", "request 必须提供 --phone")
	if err != nil {
		return nil, err
	}
	education, err := businessRequired(args, "--education", "request 必须提供 --education")
	if err != nil {
		return nil, err
	}
	enrol, err := businessRequired(args, "--enroll", "request 必须提供 --enroll")
	if err != nil {
		return nil, err
	}
	graduate, err := businessRequired(args, "--graduate", "request 必须提供 --graduate")
	if err != nil {
		return nil, err
	}
	className, err := businessRequired(args, "--class", "request 必须提供 --class")
	if err != nil {
		return nil, err
	}
	origin, err := businessRequired(args, "--origin", "request 必须提供 --origin")
	if err != nil {
		return nil, err
	}
	college, err := businessRequired(args, "--college", "request 必须提供 --college")
	if err != nil {
		return nil, err
	}
	major, err := businessRequired(args, "--major", "request 必须提供 --major")
	if err != nil {
		return nil, err
	}
	recipientPhone, err := businessRequired(args, "--recipient-phone", "request 必须提供 --recipient-phone")
	if err != nil {
		return nil, err
	}
	recipientEmail, err := businessRequired(args, "--recipient-email", "request 必须提供 --recipient-email")
	if err != nil {
		return nil, err
	}
	captcha, err := businessRequired(args, "--captcha", "request 必须提供 --captcha")
	if err != nil {
		return nil, err
	}
	purposes, err := businessValues(args, "--purpose")
	if err != nil {
		return nil, err
	}
	contents, err := businessValues(args, "--content")
	if err != nil {
		return nil, err
	}
	if len(purposes) == 0 || len(contents) == 0 {
		return nil, &siteError{Code: "invalid_argument", Message: "request 至少需要一个 --purpose 和一个 --content"}
	}
	school := recordSchoolValue(flagValue(args, "--school"))
	if school == "" {
		return nil, &siteError{Code: "invalid_argument", Message: "--school 必须是 csust、transport、electric、light-industry 或 water"}
	}
	originCode := recordOriginValue(origin)
	if originCode == "" {
		return nil, &siteError{Code: "invalid_argument", Message: "--origin 必须是省份编号 1-34，或当前支持的省份名称"}
	}
	data := []pair{
		{"mytype", typeValue}, {"myquery_name", name}, {"myquery_sfz", idCard}, {"myquery_work", flagValue(args, "--work")},
		{"myunit_id", flagValue(args, "--unit-letter-token")}, {"myquery_tel", phone}, {"myschool", school}, {"myeducational", education},
		{"myphoto", flagValue(args, "--photo-token")}, {"myenrol", enrol}, {"mygraduate", graduate}, {"myclass", className},
		{"mycollege", college}, {"mymajor", major}, {"mystudents", originCode}, {"myrecipient_add", flagValue(args, "--recipient-address")}, {"myrecipient_name", flagValue(args, "--recipient-name")},
		{"myrecipient_phone", recipientPhone}, {"myrecipient_email", recipientEmail}, {"mynotes", flagValue(args, "--notes")}, {"code", captcha}, {"token", tokenMatch[1]},
	}
	for _, purpose := range purposes {
		data = append(data, pair{"myobject[]", purpose})
	}
	for _, content := range contents {
		data = append(data, pair{"myshow[]", content})
	}
	result, requestErr := businessRequest(ctx, "student-record-query", "POST", studentRecordFormPath, nil, data, []pair{{"Referer", safeResponseURL(formResult)}}, businessRequestOptions{cookieFile: cookie}, false, true)
	if requestErr != nil {
		return nil, requestErr
	}
	result = sitePageResult(result)
	result["service"], result["operation"] = "student-record-query", "request"
	result["request_type"] = typeValue
	return result, nil
}

const staffRecordService = "student-record-query"

func (a NativeSite) executeStaffRecord(ctx context.Context, args []string) (map[string]any, *siteError) {
	if len(args) == 0 || args[0] == "catalog" {
		return businessCatalogFilter("staff-record-appointment"), nil
	}
	kind, kindErr := staffRecordKind(args[1:])
	if kindErr != nil {
		return nil, kindErr
	}
	switch args[0] {
	case "form":
		cookie, _, valueErr := businessValue(args[1:], "--cookie-file")
		if valueErr != nil {
			return nil, valueErr
		}
		result, requestErr := a.businessGet(ctx, staffRecordService, staffRecordFormPath(kind), nil, businessRequestOptions{cookieFile: cookie})
		if requestErr != nil {
			return nil, requestErr
		}
		result = sitePageResult(result)
		result["service"], result["operation"], result["kind"] = "staff-record-appointment", "form", kind
		return result, nil
	case "upload":
		return a.staffRecordUpload(ctx, args[1:], kind)
	case "request":
		return a.staffRecordRequest(ctx, args[1:], kind)
	default:
		return nil, &siteError{Code: "invalid_argument", Message: "staff-record 只支持 form、upload、request、catalog"}
	}
}

func staffRecordKind(args []string) (string, *siteError) {
	kind, err := businessRequired(args, "--kind", "staff-record 必须提供 --kind personal 或 unit")
	if err != nil {
		return "", err
	}
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "personal", "个人":
		return "personal", nil
	case "unit", "单位":
		return "unit", nil
	default:
		return "", &siteError{Code: "invalid_argument", Message: "--kind 只能是 personal 或 unit"}
	}
}

func staffRecordFormPath(kind string) string {
	return "/?c=form&a=add&fid=" + map[string]string{"personal": "4", "unit": "5"}[kind]
}

func (a NativeSite) staffRecordCaptcha(ctx context.Context, args []string, kind, cookie string) (string, *siteError) {
	if captcha := flagValue(args, "--captcha"); captcha != "" {
		return captcha, nil
	}
	_, cookiePath, resolveErr := resolveSite(siteRequest{Service: staffRecordService, CookieFile: cookie})
	if resolveErr != nil {
		return "", resolveErr
	}
	imagePath := flagValue(args, "--captcha-image")
	if imagePath == "" {
		imagePath = filepath.Join(filepath.Dir(cookiePath), "staff-record-"+kind+"-captcha.png")
	}
	imagePath = expandUserPath(imagePath)
	if _, imageErr := a.execute(ctx, siteRequest{Service: staffRecordService, Path: "/?c=form&a=code", CookieFile: cookie, Output: imagePath, ReadOnly: true, Yes: true}); imageErr != nil {
		return "", imageErr
	}
	return "", &siteError{Code: "captcha_required", Message: "教工人事档案预约需要验证码，请提供 --captcha", Details: map[string]any{"captcha_image": imagePath}}
}

func (a NativeSite) staffRecordUpload(ctx context.Context, args []string, kind string) (map[string]any, *siteError) {
	if kind != "unit" {
		return nil, &siteError{Code: "invalid_argument", Message: "只有 unit 预约需要上传单位介绍信"}
	}
	if !businessBool(args, "--yes") {
		return nil, &siteError{Code: "confirmation_required", Message: "上传单位介绍信会写入远端，必须加 --yes"}
	}
	filePath, fileErr := businessRequired(args, "--file", "upload 必须提供 --file")
	if fileErr != nil {
		return nil, fileErr
	}
	cookie, _, valueErr := businessValue(args, "--cookie-file")
	if valueErr != nil {
		return nil, valueErr
	}
	form, requestErr := a.businessGet(ctx, staffRecordService, staffRecordFormPath(kind), nil, businessRequestOptions{cookieFile: cookie})
	if requestErr != nil {
		return nil, requestErr
	}
	return a.uploadRecordMaterial(ctx, "staff-record-appointment", "upload", "introduction", filePath, cookie, form)
}

func (a NativeSite) uploadRecordMaterial(ctx context.Context, service, operation, field, filePath, cookie string, form map[string]any) (map[string]any, *siteError) {
	filePath = expandUserPath(filePath)
	file, fileErr := siteFilePart("file", filePath, "上传文件")
	if fileErr != nil {
		return nil, fileErr
	}
	if file.size == 0 {
		return nil, &siteError{Code: "invalid_argument", Message: "上传文件不能为空"}
	}
	result, requestErr := (NativeSite{}).execute(ctx, siteRequest{
		Service: staffRecordService, Method: "POST", Path: "/?c=upload&a=upfile&type=1", CookieFile: cookie,
		Headers: []pair{{"Referer", safeResponseURL(form)}}, Files: []filePart{file},
		ReadOnly: false, Yes: true, RawJSON: true,
	})
	if requestErr != nil {
		return nil, requestErr
	}
	payload, parseErr := businessJSONMap(result)
	if parseErr != nil {
		return nil, parseErr
	}
	if !strings.EqualFold(fmt.Sprint(payload["state"]), "success") {
		return nil, &siteError{Code: "mutation_unverified", Message: "上传响应未返回 state=success", Details: map[string]any{"submitted": true, "confirmed": false, "evidence": "unknown"}}
	}
	result = businessResult(result, service, operation)
	result["field"], result["filename"] = field, filepath.Base(filePath)
	result["verified_by"] = "upload-response-state-success"
	if token, ok := payload["msg"].(string); ok && strings.TrimSpace(token) != "" {
		result["upload_token"] = token
	} else {
		return nil, &siteError{Code: "mutation_unverified", Message: "上传响应成功但未返回可提交的文件令牌", Details: map[string]any{"submitted": true, "confirmed": false, "evidence": "upload-response-missing-token"}}
	}
	return result, nil
}

func (a NativeSite) staffRecordRequest(ctx context.Context, args []string, kind string) (map[string]any, *siteError) {
	if !businessBool(args, "--yes") {
		return nil, &siteError{Code: "confirmation_required", Message: "教工人事档案预约会提交个人资料，必须加 --yes"}
	}
	cookie, _, valueErr := businessValue(args, "--cookie-file")
	if valueErr != nil {
		return nil, valueErr
	}
	form, requestErr := a.businessGet(ctx, staffRecordService, staffRecordFormPath(kind), nil, businessRequestOptions{cookieFile: cookie})
	if requestErr != nil {
		return nil, requestErr
	}
	tokenMatch := recordFormToken.FindStringSubmatch(businessBody(form))
	if len(tokenMatch) < 2 {
		return nil, &siteError{Code: "parse_error", Message: "教工人事档案预约表单缺少动态 token"}
	}
	data, dataErr := staffRecordFields(args, kind)
	if dataErr != nil {
		return nil, dataErr
	}
	captcha, captchaErr := a.staffRecordCaptcha(ctx, args, kind, cookie)
	if captchaErr != nil {
		return nil, captchaErr
	}
	introFile, introFound, introErr := businessValue(args, "--introduction-file")
	if introErr != nil {
		return nil, introErr
	}
	introToken, tokenFound, tokenErr := businessValue(args, "--introduction-token")
	if tokenErr != nil {
		return nil, tokenErr
	}
	if kind == "unit" {
		if introFound && tokenFound {
			return nil, &siteError{Code: "invalid_argument", Message: "--introduction-file 与 --introduction-token 只能二选一"}
		}
		if introFound {
			upload, uploadErr := a.uploadRecordMaterial(ctx, "staff-record-appointment", "upload", "introduction", introFile, cookie, form)
			if uploadErr != nil {
				return nil, uploadErr
			}
			introToken, _ = upload["upload_token"].(string)
		}
		if strings.TrimSpace(introToken) == "" {
			return nil, &siteError{Code: "invalid_argument", Message: "unit 预约必须提供 --introduction-token 或 --introduction-file"}
		}
		data = append(data, pair{"myintroduce", introToken})
	}
	data = append(data, pair{"code", captcha}, pair{"token", tokenMatch[1]})
	result, requestErr := businessRequest(ctx, staffRecordService, "POST", staffRecordFormPath(kind), nil, data, []pair{{"Referer", safeResponseURL(form)}}, businessRequestOptions{cookieFile: cookie}, false, true)
	if requestErr != nil {
		return nil, requestErr
	}
	result = sitePageResult(result)
	result["service"], result["operation"], result["kind"] = "staff-record-appointment", "request", kind
	return result, nil
}

func staffRecordFields(args []string, kind string) ([]pair, *siteError) {
	date := func(flag, message string) (string, *siteError) {
		value, err := businessRequired(args, flag, message)
		if err != nil {
			return "", err
		}
		if _, parseErr := time.Parse("2006-01-02", value); parseErr != nil {
			return "", &siteError{Code: "invalid_argument", Message: flag + " 必须是 YYYY-MM-DD"}
		}
		return value, nil
	}
	text := func(flag, message string) (string, *siteError) {
		return businessRequired(args, flag, message)
	}
	multiline := func(flag, message string) (string, *siteError) {
		items, valueErr := businessValues(args, flag)
		if valueErr != nil {
			return "", valueErr
		}
		if len(items) == 0 {
			return "", &siteError{Code: "invalid_argument", Message: message}
		}
		return strings.Join(items, "\n"), nil
	}
	appointment, err := date("--appointment-date", "request 必须提供 --appointment-date")
	if err != nil {
		return nil, err
	}
	reason, err := text("--reason", "request 必须提供 --reason")
	if err != nil {
		return nil, err
	}
	usage, err := businessValues(args, "--usage")
	if err != nil {
		return nil, err
	}
	if len(usage) == 0 {
		return nil, &siteError{Code: "invalid_argument", Message: "request 至少需要一个 --usage"}
	}
	allowed := map[string]bool{}
	if kind == "personal" {
		allowed = map[string]bool{"查阅": true, "复制": true, "开具证明": true}
	} else {
		allowed = map[string]bool{"查阅": true, "复印材料": true, "借阅": true}
	}
	data := []pair{}
	for _, item := range usage {
		if !allowed[item] {
			return nil, &siteError{Code: "invalid_argument", Message: "--usage 不符合当前预约类型"}
		}
		data = append(data, pair{"mygoal[]", item})
	}
	data = append(data, pair{"mymatter", reason}, pair{"mytime", appointment})
	if kind == "personal" {
		fields := []struct{ flag, name, message string }{
			{"--subject-name", "myquery_name", "request 必须提供 --subject-name"},
			{"--birth-date", "mydate", "request 必须提供 --birth-date"},
			{"--employee-id", "myquery_sfz", "request 必须提供 --employee-id"},
			{"--subject-unit", "myquery_work", "request 必须提供 --subject-unit"},
			{"--applicant-name", "mytransactors", "request 必须提供 --applicant-name"},
			{"--phone", "mycontact", "request 必须提供 --phone"},
		}
		for _, field := range fields {
			value, valueErr := text(field.flag, field.message)
			if valueErr != nil {
				return nil, valueErr
			}
			if field.flag == "--birth-date" {
				if _, parseErr := time.Parse("2006-01-02", value); parseErr != nil {
					return nil, &siteError{Code: "invalid_argument", Message: field.flag + " 必须是 YYYY-MM-DD"}
				}
			}
			data = append(data, pair{field.name, value})
		}
	} else {
		fields := []struct{ flag, name, message string }{
			{"--unit", "mymyunit", "request 必须提供 --unit"},
			{"--applicant-name", "myquery_name", "request 必须提供 --applicant-name"},
			{"--phone", "mytel", "request 必须提供 --phone"},
			{"--subject-name", "myname", "request 必须提供 --subject-name"},
			{"--subject-unit", "mycompany", "request 必须提供 --subject-unit"},
		}
		for _, field := range fields {
			value, valueErr := text(field.flag, field.message)
			if field.name == "myname" || field.name == "mycompany" {
				value, valueErr = multiline(field.flag, field.message)
			}
			if valueErr != nil {
				return nil, valueErr
			}
			data = append(data, pair{field.name, value})
		}
		content, contentErr := text("--content", "unit request 必须提供 --content")
		if contentErr != nil {
			return nil, contentErr
		}
		data = append(data, pair{"mycontent", content})
	}
	return data, nil
}

func businessValues(args []string, flag string) ([]string, *siteError) {
	values := []string{}
	for index, arg := range args {
		if arg != flag && !strings.HasPrefix(arg, flag+"=") {
			continue
		}
		value := strings.TrimPrefix(arg, flag+"=")
		if arg == flag {
			if index+1 >= len(args) || strings.HasPrefix(args[index+1], "--") {
				return nil, &siteError{Code: "invalid_argument", Message: flag + " 缺少参数值"}
			}
			value = args[index+1]
		}
		if strings.TrimSpace(value) == "" {
			return nil, &siteError{Code: "invalid_argument", Message: flag + " 不能为空"}
		}
		values = append(values, value)
	}
	return values, nil
}

func recordSchoolValue(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "csust", "长沙理工大学":
		return "1"
	case "2", "transport", "长沙交通学院":
		return "2"
	case "3", "electric", "长沙电力学院":
		return "3"
	case "4", "light-industry", "湖南省轻工业高等专科学校":
		return "4"
	case "5", "water", "湖南省水利水电学校":
		return "5"
	default:
		return ""
	}
}

func recordOriginValue(value string) string {
	if parsed, err := strconv.Atoi(value); err == nil && parsed >= 1 && parsed <= 34 {
		return strconv.Itoa(parsed)
	}
	return recordProvinceCodes[strings.TrimSpace(value)]
}

var recordProvinceCodes = func() map[string]string {
	values := []string{"河北", "山西", "辽宁", "吉林", "黑龙江", "江苏", "浙江", "安徽", "福建", "江西", "山东", "河南", "湖北", "湖南", "广东", "广西", "海南", "四川", "贵州", "云南", "陕西", "甘肃", "青海", "台湾", "内蒙古", "新疆", "西藏", "宁夏", "北京", "天津", "上海", "重庆", "香港", "澳门"}
	codes := make(map[string]string, len(values)*2)
	for index, value := range values {
		code := strconv.Itoa(index + 1)
		codes[value] = code
		codes[value+"省"] = code
	}
	codes["内蒙古自治区"], codes["新疆维吾尔自治区"], codes["西藏自治区"], codes["宁夏回族自治区"] = "25", "26", "27", "28"
	codes["广西壮族自治区"], codes["香港特别行政区"], codes["澳门特别行政区"] = "16", "33", "34"
	return codes
}()

func (a NativeSite) executeOnlineJudge(ctx context.Context, args []string) (map[string]any, *siteError) {
	if len(args) == 0 || args[0] == "catalog" {
		return map[string]any{
			"ok": true, "submitted": false, "confirmed": true, "evidence": "frontend service module",
			"service": "onlinejudge", "operations": map[string]any{
				"problems": "/api/problem", "problem": "/api/problem?problem_id=ID", "contests": "/api/contests", "contest": "/api/contest?id=ID", "submissions": "/api/submissions", "submission": "/api/submission?id=ID", "user": "/api/profile?username=NAME", "submit": "POST /api/submission",
			},
		}, nil
	}
	insecure := businessBool(args[1:], "--insecure")
	cookie, _, valueErr := businessValue(args[1:], "--cookie-file")
	if valueErr != nil {
		return nil, valueErr
	}
	options := businessRequestOptions{cookieFile: cookie, insecure: insecure}
	switch args[0] {
	case "problems":
		page, err := businessInt(args[1:], "--page", 1)
		if err != nil {
			return nil, err
		}
		limit, err := businessInt(args[1:], "--limit", 20)
		if err != nil {
			return nil, err
		}
		params := []pair{{"paging", "true"}, {"offset", strconv.Itoa((page - 1) * limit)}, {"limit", strconv.Itoa(limit)}}
		for _, flag := range []string{"--keyword", "--difficulty", "--tag"} {
			if value := flagValue(args[1:], flag); value != "" {
				params = append(params, pair{strings.TrimPrefix(flag, "--"), value})
			}
		}
		return a.onlineJudgeRead(ctx, "problems", "/api/problem", params, options)
	case "problem":
		id, err := businessRequired(args[1:], "--id", "problem 必须提供 --id")
		if err != nil {
			return nil, err
		}
		return a.onlineJudgeRead(ctx, "problem", "/api/problem", []pair{{"problem_id", id}}, options)
	case "contests":
		page, err := businessInt(args[1:], "--page", 1)
		if err != nil {
			return nil, err
		}
		limit, err := businessInt(args[1:], "--limit", 20)
		if err != nil {
			return nil, err
		}
		return a.onlineJudgeRead(ctx, "contests", "/api/contests", []pair{{"offset", strconv.Itoa((page - 1) * limit)}, {"limit", strconv.Itoa(limit)}}, options)
	case "contest":
		id, err := businessRequired(args[1:], "--id", "contest 必须提供 --id")
		if err != nil {
			return nil, err
		}
		return a.onlineJudgeRead(ctx, "contest", "/api/contest", []pair{{"id", id}}, options)
	case "submissions":
		page, err := businessInt(args[1:], "--page", 1)
		if err != nil {
			return nil, err
		}
		limit, err := businessInt(args[1:], "--limit", 20)
		if err != nil {
			return nil, err
		}
		params := []pair{{"offset", strconv.Itoa((page - 1) * limit)}, {"limit", strconv.Itoa(limit)}}
		for _, item := range []struct{ flag, name string }{{"--username", "username"}, {"--problem-id", "problem_id"}, {"--contest-id", "contest_id"}, {"--language", "language"}, {"--result", "result"}} {
			if value := flagValue(args[1:], item.flag); value != "" {
				params = append(params, pair{item.name, value})
			}
		}
		if businessBool(args[1:], "--myself") {
			params = append(params, pair{"myself", "1"})
		}
		return a.onlineJudgeRead(ctx, "submissions", "/api/submissions", params, options)
	case "submission":
		id, err := businessRequired(args[1:], "--id", "submission 必须提供 --id")
		if err != nil {
			return nil, err
		}
		return a.onlineJudgeRead(ctx, "submission", "/api/submission", []pair{{"id", id}}, options)
	case "user":
		name, err := businessRequired(args[1:], "--username", "user 必须提供 --username")
		if err != nil {
			return nil, err
		}
		return a.onlineJudgeRead(ctx, "user", "/api/profile", []pair{{"username", name}}, options)
	case "tags":
		return a.onlineJudgeRead(ctx, "tags", "/api/problem/tags", nil, options)
	case "submit":
		return a.onlineJudgeSubmit(ctx, args[1:], options)
	default:
		return nil, &siteError{Code: "invalid_argument", Message: "onlinejudge 只支持 problems、problem、contests、contest、submissions、submission、user、tags、submit、catalog"}
	}
}

func (a NativeSite) onlineJudgeRead(ctx context.Context, operation, path string, params []pair, options businessRequestOptions) (map[string]any, *siteError) {
	result, err := a.businessGet(ctx, "onlinejudge", path, params, options)
	if err != nil {
		return nil, err
	}
	return businessResult(result, "onlinejudge", operation), nil
}

func (a NativeSite) onlineJudgeSubmit(ctx context.Context, args []string, options businessRequestOptions) (map[string]any, *siteError) {
	problem, err := businessRequired(args, "--problem-id", "submit 必须提供 --problem-id")
	if err != nil {
		return nil, err
	}
	language, err := businessRequired(args, "--language", "submit 必须提供 --language")
	if err != nil {
		return nil, err
	}
	code, err := businessCode(args)
	if err != nil {
		return nil, err
	}
	if !businessBool(args, "--yes") {
		return nil, &siteError{Code: "confirmation_required", Message: "OnlineJudge 提交需要 --yes"}
	}
	data := []pair{{"problem_id", problem}, {"language", language}, {"code", code}}
	if contest := flagValue(args, "--contest-id"); contest != "" {
		data = append(data, pair{"contest_id", contest})
	}
	options.require = true
	result, requestErr := businessRequest(ctx, "onlinejudge", "POST", "/api/submission", nil, data, nil, options, false, true)
	if requestErr != nil {
		return nil, requestErr
	}
	payload, parseErr := businessJSONMap(result)
	if parseErr != nil {
		return nil, parseErr
	}
	id := findID(payload)
	if id == "" {
		return nil, &siteError{Code: "mutation_unverified", Message: "提交接口未返回可回读的提交编号", Details: map[string]any{"submitted": true, "confirmed": false, "evidence": "unknown"}}
	}
	verification, verifyErr := a.businessGet(ctx, "onlinejudge", "/api/submission", []pair{{"id", id}}, options)
	if verifyErr != nil {
		return nil, &siteError{Code: "mutation_unverified", Message: "提交已发送但回读提交状态失败", Details: map[string]any{"submitted": true, "confirmed": false, "evidence": "unknown", "cause": verifyErr.Code, "submission_id": id}}
	}
	verificationPayload, verificationParseErr := businessJSONMap(verification)
	if verificationParseErr != nil || findID(verificationPayload) != id {
		return nil, &siteError{Code: "mutation_unverified", Message: "提交已发送但回读记录与提交编号不一致", Details: map[string]any{"submitted": true, "confirmed": false, "evidence": "readback-identity-mismatch", "submission_id": id}}
	}
	result = businessResult(result, "onlinejudge", "submit")
	result["submission_id"] = id
	result["verification"] = businessResult(verification, "onlinejudge", "submission-readback")
	result["evidence"] = "submission-created-and-read-back"
	return result, nil
}

func businessCode(args []string) (string, *siteError) {
	value, err := businessRequired(args, "--code", "submit 必须提供 --code")
	if err != nil {
		return "", err
	}
	if value == "-" {
		content, readErr := readBoundedSiteInput(os.Stdin)
		if readErr != nil {
			if errors.Is(readErr, errSiteRequestTooLarge) {
				return "", siteRequestTooLarge("代码输入")
			}
			return "", &siteError{Code: "invalid_argument", Message: "无法读取标准输入代码: " + readErr.Error()}
		}
		value = string(content)
	} else if strings.HasPrefix(value, "@") {
		file, openErr := openSiteInput(expandUserPath(strings.TrimPrefix(value, "@")))
		if openErr != nil {
			return "", &siteError{Code: "invalid_argument", Message: "无法读取代码文件: " + openErr.Error()}
		}
		content, readErr := readBoundedSiteInput(file)
		_ = file.Close()
		if readErr != nil {
			if errors.Is(readErr, errSiteRequestTooLarge) {
				return "", siteRequestTooLarge("代码文件")
			}
			return "", &siteError{Code: "invalid_argument", Message: "无法读取代码文件: " + readErr.Error()}
		}
		value = string(content)
	}
	if strings.TrimSpace(value) == "" {
		return "", &siteError{Code: "invalid_argument", Message: "--code 不能为空"}
	}
	return value, nil
}

func findID(value any) string {
	var found string
	var visit func(any)
	visit = func(item any) {
		if found != "" {
			return
		}
		switch typed := item.(type) {
		case map[string]any:
			for _, key := range []string{"id", "submission_id", "submissionId"} {
				if value, ok := typed[key]; ok {
					switch value := value.(type) {
					case string:
						found = value
					case float64:
						found = strconv.FormatInt(int64(value), 10)
					}
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

func (a NativeSite) executePartyExam(ctx context.Context, args []string) (map[string]any, *siteError) {
	if len(args) == 0 || args[0] == "catalog" {
		return map[string]any{"ok": true, "submitted": false, "confirmed": true, "evidence": "live page and index.js mapping", "service": "party-school-exam", "operations": []string{"login", "courses", "scores", "logout"}}, nil
	}
	cookie, _, valueErr := businessValue(args[1:], "--cookie-file")
	if valueErr != nil {
		return nil, valueErr
	}
	switch args[0] {
	case "login":
		username, err := businessRequired(args[1:], "--username", "login 必须提供 --username")
		if err != nil {
			return nil, err
		}
		password, err := businessSecret(args[1:], "--password", "CSUST_PASSWORD")
		if err != nil {
			return nil, err
		}
		checkcode := flagValue(args[1:], "--checkcode")
		result, requestErr := businessRequest(ctx, "party-school-exam", "POST", "/mobile/login", nil, []pair{{"username", username}, {"pwd", password}, {"checkcode", checkcode}}, nil, businessRequestOptions{cookieFile: cookie, allowBusinessFailure: true}, true, true)
		if requestErr != nil {
			return nil, requestErr
		}
		code := strings.TrimSpace(businessBody(result))
		if code != "1" && code != "3" {
			message := map[string]string{"0": "用户名或密码错误", "2": "验证码错误", "4": "提交参数错误", "-2": "登录失败，请稍后重试"}[code]
			if message == "" {
				message = "登录失败"
			}
			return nil, &siteError{Code: "authentication_failed", Message: message, Details: map[string]any{"submitted": false, "confirmed": false, "evidence": "rejected", "remote_code": code}}
		}
		result["service"], result["operation"], result["role"] = "party-school-exam", "login", map[string]string{"1": "student", "3": "admin"}[code]
		result["confirmed"], result["evidence"] = true, "remote-login-code"
		return result, nil
	case "courses", "scores":
		path := "/subsys/examcourse/student"
		if args[0] == "scores" {
			path = "/mobile/score"
		}
		result, requestErr := a.businessGet(ctx, "party-school-exam", path, nil, businessRequestOptions{cookieFile: cookie, require: true})
		if requestErr != nil {
			return nil, requestErr
		}
		result = sitePageResult(result)
		result["service"], result["operation"] = "party-school-exam", args[0]
		return result, nil
	case "logout":
		result, requestErr := a.businessGet(ctx, "party-school-exam", "/mobile/logout", nil, businessRequestOptions{cookieFile: cookie})
		if requestErr != nil {
			return nil, requestErr
		}
		_, cookiePath, resolveErr := resolveSite(siteRequest{Service: "party-school-exam", CookieFile: cookie})
		if resolveErr != nil {
			return nil, resolveErr
		}
		if removeErr := removeCookieFile(cookiePath); removeErr != nil {
			return nil, &siteError{Code: "cookie_write_failed", Message: removeErr.Error()}
		}
		result["service"], result["operation"], result["logged_out"] = "party-school-exam", "logout", true
		return result, nil
	default:
		return nil, &siteError{Code: "invalid_argument", Message: "party-exam 只支持 login、courses、scores、logout、catalog"}
	}
}

func (a NativeSite) executeArchive(ctx context.Context, args []string) (map[string]any, *siteError) {
	if len(args) == 0 || args[0] == "catalog" {
		return map[string]any{"ok": true, "submitted": false, "confirmed": true, "evidence": "frontend bundles and live login pages", "service": "archive", "systems": []map[string]any{
			{"name": "student", "service": "student-archive", "label": "学生档案管理", "capabilities": []string{"status", "login", "logout", "report"}},
			{"name": "management", "service": "archive-management", "label": "综合档案管理", "capabilities": []string{"status", "login", "logout", "report"}},
		}}, nil
	}
	system, err := businessRequired(args[1:], "--system", "archive 必须提供 --system student 或 management")
	if err != nil {
		return nil, err
	}
	service := map[string]string{"student": "student-archive", "management": "archive-management"}[system]
	if service == "" {
		return nil, &siteError{Code: "invalid_argument", Message: "--system 只能是 student 或 management"}
	}
	switch args[0] {
	case "status":
		options, optionsErr := archiveRequestOptions(args[1:], service, false)
		if optionsErr != nil {
			return nil, optionsErr
		}
		return a.executeServiceStatusWithOptions(ctx, service, "status", map[string]string{"student": "学生档案管理", "management": "综合档案管理"}[system], options)
	case "login":
		return a.archiveLogin(ctx, args[1:], service, system)
	case "logout":
		return a.archiveLogout(ctx, args[1:], service, system)
	case "report":
		return a.archiveReport(ctx, args[1:], service, system)
	default:
		return nil, &siteError{Code: "invalid_argument", Message: "archive 只支持 status、login、logout、report、catalog"}
	}
}

const archiveAPIBase = "/archive"

func archiveRequestOptions(args []string, service string, require bool) (businessRequestOptions, *siteError) {
	cookie, _, err := businessValue(args, "--cookie-file")
	if err != nil {
		return businessRequestOptions{}, err
	}
	token, _, tokenErr := archiveAccessToken(args, service, cookie)
	if tokenErr != nil {
		return businessRequestOptions{}, tokenErr
	}
	options := businessRequestOptions{cookieFile: cookie, require: require, headers: []pair{{"tenant-id", "0"}}}
	if token != "" {
		options.headers = append(options.headers, pair{"X-Access-Token", token})
	}
	return options, nil
}

func archiveAccessToken(args []string, service, cookie string) (string, string, *siteError) {
	token, found, err := businessValue(args, "--access-token")
	if err != nil {
		return "", "", err
	}
	if !found {
		token = os.Getenv("CSUST_ARCHIVE_TOKEN")
	}
	_, cookiePath, resolveErr := resolveSite(siteRequest{Service: service, CookieFile: cookie})
	if resolveErr != nil {
		return "", "", resolveErr
	}
	tokenPath := strings.TrimSuffix(cookiePath, ".cookies.txt") + ".token"
	if token == "" {
		if content, readErr := os.ReadFile(tokenPath); readErr == nil {
			token = strings.TrimSpace(string(content))
		} else if !os.IsNotExist(readErr) {
			return "", "", &siteError{Code: "session_error", Message: "无法读取档案系统令牌: " + readErr.Error()}
		}
	}
	return token, tokenPath, nil
}

func archiveServicePath(path string) string {
	return archiveAPIBase + "/" + strings.TrimPrefix(path, "/")
}

func (a NativeSite) archiveLogin(ctx context.Context, args []string, service, system string) (map[string]any, *siteError) {
	account, password, credentialErr := businessCredentials(args, "CSUST_ARCHIVE_PASSWORD")
	if credentialErr != nil {
		return nil, credentialErr
	}
	cookie, _, valueErr := businessValue(args, "--cookie-file")
	if valueErr != nil {
		return nil, valueErr
	}
	_, cookiePath, resolveErr := resolveSite(siteRequest{Service: service, CookieFile: cookie})
	if resolveErr != nil {
		return nil, resolveErr
	}
	checkKey := flagValue(args, "--check-key")
	if checkKey == "" {
		checkKey = strconv.FormatInt(time.Now().UnixMilli(), 10)
	}
	captcha := flagValue(args, "--captcha")
	if captcha == "" {
		imagePath := flagValue(args, "--captcha-image")
		if imagePath == "" {
			imagePath = filepath.Join(filepath.Dir(cookiePath), cookieHost(mustParseURL(knownSites[service].scheme+"://"+knownSites[service].host))+"-archive-captcha.png")
		}
		imagePath = expandUserPath(imagePath)
		if _, imageErr := a.execute(ctx, siteRequest{Service: service, Path: archiveServicePath("sys/randomImage/" + url.PathEscape(checkKey)), Method: "GET", CookieFile: cookie, Output: imagePath, ReadOnly: true, Yes: true}); imageErr != nil {
			return nil, imageErr
		}
		return nil, &siteError{Code: "captcha_required", Message: "档案系统登录需要验证码，请提供 --captcha", Details: map[string]any{"captcha_image": imagePath, "check_key": checkKey}}
	}
	request := siteRequest{
		Service: service, Path: archiveServicePath("sys/login"), Method: "POST", CookieFile: cookie,
		JSON:    map[string]any{"username": account, "password": password, "captcha": captcha, "checkKey": checkKey, "remember_me": businessBool(args, "--remember")},
		HasJSON: true, RawJSON: true, ReadOnly: true, Yes: true, AllowBusinessFailure: true,
	}
	result, requestErr := a.execute(ctx, request)
	if requestErr != nil {
		if requestErr.Code == "business_rejected" {
			requestErr.Code = "authentication_failed"
		}
		return nil, requestErr
	}
	payload, parseErr := businessJSONMap(result)
	if parseErr != nil {
		return nil, parseErr
	}
	if fmt.Sprint(payload["code"]) != "200" {
		return nil, &siteError{Code: "authentication_failed", Message: firstNonEmpty(fmt.Sprint(payload["message"]), fmt.Sprint(payload["msg"]), "档案系统登录失败"), Details: map[string]any{"submitted": false, "confirmed": false, "evidence": "rejected", "remote_code": payload["code"]}}
	}
	token := findString(payload, "token", "accessToken")
	if token == "" {
		return nil, &siteError{Code: "authentication_failed", Message: "登录成功响应缺少访问令牌", Details: map[string]any{"submitted": false, "confirmed": false, "evidence": "unknown"}}
	}
	tokenPath := strings.TrimSuffix(cookiePath, ".cookies.txt") + ".token"
	if writeErr := atomicWrite(tokenPath, []byte(token+"\n")); writeErr != nil {
		return nil, &siteError{Code: "session_error", Message: "登录成功但令牌保存失败: " + writeErr.Error(), Details: map[string]any{"submitted": false, "confirmed": true, "evidence": "remote-login-code"}}
	}
	return map[string]any{"ok": true, "submitted": false, "confirmed": true, "evidence": "remote-login-code-and-token-saved", "service": service, "operation": "login", "system": system, "username": account, "cookie_file": cookiePath, "token_file": tokenPath}, nil
}

func findString(value any, keys ...string) string {
	wanted := make(map[string]bool, len(keys))
	for _, key := range keys {
		wanted[key] = true
	}
	var found string
	var visit func(any)
	visit = func(item any) {
		if found != "" {
			return
		}
		switch typed := item.(type) {
		case map[string]any:
			for key, child := range typed {
				if wanted[key] {
					if text, ok := child.(string); ok && text != "" {
						found = text
						return
					}
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

func (a NativeSite) archiveLogout(ctx context.Context, args []string, service, system string) (map[string]any, *siteError) {
	options, optionsErr := archiveRequestOptions(args, service, false)
	if optionsErr != nil {
		return nil, optionsErr
	}
	result, requestErr := a.execute(ctx, siteRequest{Service: service, Path: archiveServicePath("sys/logout"), Method: "POST", CookieFile: options.cookieFile, Headers: options.headers, JSON: map[string]any{}, HasJSON: true, RawJSON: true, ReadOnly: true, Yes: true})
	if requestErr != nil && requestErr.Code != "login_required" {
		return nil, requestErr
	}
	if requestErr == nil {
		payload, parseErr := businessJSONMap(result)
		if parseErr != nil {
			return nil, &siteError{Code: "mutation_unverified", Message: "档案系统退出响应不是可验证的 JSON", Details: map[string]any{"submitted": true, "confirmed": false, "evidence": "logout-response"}}
		}
		code := fmt.Sprint(payload["code"])
		if code != "200" && code != "0" {
			return nil, &siteError{Code: "mutation_rejected", Message: "档案系统退出失败", Details: map[string]any{"submitted": true, "confirmed": false, "evidence": "logout-response", "remote_code": code}}
		}
	}
	_, cookiePath, resolveErr := resolveSite(siteRequest{Service: service, CookieFile: options.cookieFile})
	if resolveErr != nil {
		return nil, resolveErr
	}
	tokenPath := strings.TrimSuffix(cookiePath, ".cookies.txt") + ".token"
	if removeErr := removeCookieFile(tokenPath); removeErr != nil {
		return nil, &siteError{Code: "session_error", Message: "远端退出后令牌删除失败: " + removeErr.Error()}
	}
	if cookieErr := removeCookieFile(cookiePath); cookieErr != nil {
		return nil, &siteError{Code: "cookie_write_failed", Message: cookieErr.Error()}
	}
	if requestErr != nil {
		return map[string]any{"ok": true, "submitted": false, "confirmed": true, "evidence": "local-token-removed", "service": service, "operation": "logout", "system": system, "logged_out": true}, nil
	}
	result["ok"], result["submitted"], result["confirmed"] = true, true, true
	result["service"], result["operation"], result["system"], result["logged_out"] = service, "logout", system, true
	result["evidence"] = "remote-logout-and-token-removed"
	return result, nil
}

func (a NativeSite) archiveReport(ctx context.Context, args []string, service, system string) (map[string]any, *siteError) {
	options, optionsErr := archiveRequestOptions(args, service, true)
	if optionsErr != nil {
		return nil, optionsErr
	}
	code, requiredErr := businessRequired(args, "--report-code", "report 必须提供 --report-code")
	if requiredErr != nil {
		return nil, requiredErr
	}
	page, pageErr := businessInt(args, "--page", 1)
	if pageErr != nil {
		return nil, pageErr
	}
	pageSize, sizeErr := businessInt(args, "--page-size", 10)
	if sizeErr != nil {
		return nil, sizeErr
	}
	filters, filterErr := businessValues(args, "--filter")
	if filterErr != nil {
		return nil, filterErr
	}
	params := []pair{{"pageNo", strconv.Itoa(page)}, {"pageSize", strconv.Itoa(pageSize)}}
	for _, filter := range filters {
		name, value, found := strings.Cut(filter, "=")
		if !found || strings.TrimSpace(name) == "" {
			return nil, &siteError{Code: "invalid_argument", Message: "--filter 必须是字段=值"}
		}
		params = append(params, pair{"self_" + name, value})
	}
	if sortValue := flagValue(args, "--sort"); sortValue != "" {
		params = append(params, pair{"column", sortValue})
	}
	base := archiveServicePath("online/cgreport/api/")
	columns, requestErr := a.businessGet(ctx, service, base+"getRpColumns/"+url.PathEscape(code), nil, options)
	if requestErr != nil {
		return nil, requestErr
	}
	queryInfo, requestErr := a.businessGet(ctx, service, base+"getQueryInfo/"+url.PathEscape(code), nil, options)
	if requestErr != nil {
		return nil, requestErr
	}
	data, requestErr := a.businessGet(ctx, service, base+"getData/"+url.PathEscape(code), params, options)
	if requestErr != nil {
		return nil, requestErr
	}
	return map[string]any{
		"ok": true, "submitted": false, "confirmed": true, "evidence": "archive-report-api-responses",
		"service": service, "operation": "report", "system": system, "report_code": code,
		"page": page, "page_size": pageSize, "columns": businessPayload(columns), "query": businessPayload(queryInfo), "data": businessPayload(data),
	}, nil
}

func businessPayload(result map[string]any) any {
	if value, ok := businessData(result); ok {
		return value
	}
	return result
}

func (a NativeSite) executeServiceStatus(ctx context.Context, service, operation, label string) (map[string]any, *siteError) {
	return a.executeServiceStatusWithOptions(ctx, service, operation, label, businessRequestOptions{})
}

func (a NativeSite) executeServiceStatusWithOptions(ctx context.Context, service, operation, label string, options businessRequestOptions) (map[string]any, *siteError) {
	result, err := a.businessGet(ctx, service, "", nil, options)
	if err != nil {
		return nil, err
	}
	result = sitePageResult(result)
	result["service"], result["operation"], result["label"] = service, operation, label
	return result, nil
}

func (a NativeSite) executeServiceStatusCommand(ctx context.Context, args []string, service, label string) (map[string]any, *siteError) {
	if len(args) == 0 || args[0] == "status" {
		statusArgs := args
		if len(statusArgs) > 0 {
			statusArgs = statusArgs[1:]
		}
		cookie, _, valueErr := businessValue(statusArgs, "--cookie-file")
		if valueErr != nil {
			return nil, valueErr
		}
		return a.executeServiceStatusWithOptions(ctx, service, "status", label, businessRequestOptions{cookieFile: cookie, insecure: businessBool(statusArgs, "--insecure")})
	}
	if args[0] == "catalog" {
		return businessCatalogFilter(service), nil
	}
	return nil, &siteError{Code: "invalid_argument", Message: service + " 只支持 status、catalog"}
}

func (a NativeSite) executeGraduateAdmissions(ctx context.Context, args []string) (map[string]any, *siteError) {
	service, label := "graduate-admissions", "研究生招生旧系统"
	if len(args) == 0 || args[0] == "status" {
		statusArgs := args
		if len(statusArgs) > 0 {
			statusArgs = statusArgs[1:]
		}
		cookie, _, valueErr := businessValue(statusArgs, "--cookie-file")
		if valueErr != nil {
			return nil, valueErr
		}
		return a.executeServiceStatusWithOptions(ctx, service, "status", label, businessRequestOptions{cookieFile: cookie})
	}
	if args[0] == "catalog" {
		return businessCatalogFilter(service), nil
	}
	cookie, _, valueErr := businessValue(args[1:], "--cookie-file")
	if valueErr != nil {
		return nil, valueErr
	}
	switch args[0] {
	case "login":
		return a.graduateAdmissionsLogin(ctx, args[1:], cookie)
	case "logout":
		return a.graduateAdmissionsLogout(ctx, args[1:], cookie)
	default:
		return nil, &siteError{Code: "invalid_argument", Message: service + " 只支持 status、catalog、login、logout"}
	}
}

func graduateAdmissionsLoginPage(body string) bool {
	document, err := parsePage(body)
	return err == nil && document.first("form", "Form") != nil && document.first("input", "txtLoginName") != nil
}

func (a NativeSite) graduateAdmissionsLogin(ctx context.Context, args []string, cookie string) (map[string]any, *siteError) {
	account, password, credentialErr := businessCredentials(args, "CSUST_GRADUATE_ADMISSIONS_PASSWORD")
	if credentialErr != nil {
		return nil, credentialErr
	}
	page, requestErr := a.businessGet(ctx, "graduate-admissions", "/ksxt/login.aspx", nil, businessRequestOptions{cookieFile: cookie})
	if requestErr != nil {
		return nil, requestErr
	}
	captcha := flagValue(args, "--captcha")
	if captcha == "" {
		_, cookiePath, resolveErr := resolveSite(siteRequest{Service: "graduate-admissions", CookieFile: cookie})
		if resolveErr != nil {
			return nil, resolveErr
		}
		imagePath := flagValue(args, "--captcha-image")
		if imagePath == "" {
			imagePath = filepath.Join(filepath.Dir(cookiePath), "graduate-admissions-captcha.png")
		}
		if _, imageErr := a.execute(ctx, siteRequest{Service: "graduate-admissions", Path: "/ksxt/createyzm.aspx", Params: []pair{{"time", strconv.FormatInt(time.Now().UnixMilli(), 10)}}, Method: "GET", CookieFile: cookie, Output: expandUserPath(imagePath), ReadOnly: true, Yes: true}); imageErr != nil {
			return nil, imageErr
		}
		return nil, &siteError{Code: "captcha_required", Message: "研究生招生系统登录需要验证码，请提供 --captcha", Details: map[string]any{"captcha_image": expandUserPath(imagePath)}}
	}
	fields, formErr := hiddenFormFields(businessBody(page), "Form")
	if formErr != nil {
		return nil, formErr
	}
	fields = append(fields, pair{"txtLoginName", account}, pair{"txtPassWord", password}, pair{"txtyzm", captcha}, pair{"btnLogin.x", "1"}, pair{"btnLogin.y", "1"})
	result, requestErr := businessRequest(ctx, "graduate-admissions", "POST", "/ksxt/login.aspx", nil, fields, []pair{{"Referer", safeResponseURL(page)}}, businessRequestOptions{cookieFile: cookie, allowBusinessFailure: true}, true, true)
	if requestErr != nil {
		return nil, requestErr
	}
	if failure := businessLoginResponseFailure(result); failure != nil {
		return nil, failure
	}
	evidence, probeErr := a.confirmBusinessLogin(ctx, "graduate-admissions", "/ksxt/login.aspx", cookie, graduateAdmissionsLoginPage)
	if probeErr != nil {
		return nil, probeErr
	}
	result["service"], result["operation"], result["username"] = "graduate-admissions", "login", account
	result["submitted"], result["confirmed"] = true, true
	result["evidence"] = "login-response-and-" + evidence
	return result, nil
}

func (a NativeSite) graduateAdmissionsLogout(ctx context.Context, args []string, cookie string) (map[string]any, *siteError) {
	page, requestErr := a.businessGet(ctx, "graduate-admissions", "/ksxt/login.aspx", nil, businessRequestOptions{cookieFile: cookie})
	if requestErr != nil {
		return nil, requestErr
	}
	fields, formErr := hiddenFormFields(businessBody(page), "Form")
	if formErr != nil {
		return nil, formErr
	}
	fields = append(fields, pair{"btnExit.x", "1"}, pair{"btnExit.y", "1"})
	result, requestErr := businessRequest(ctx, "graduate-admissions", "POST", "/ksxt/login.aspx", nil, fields, []pair{{"Referer", safeResponseURL(page)}}, businessRequestOptions{cookieFile: cookie}, true, true)
	if requestErr != nil {
		return nil, requestErr
	}
	if !graduateAdmissionsLoginPage(businessBody(result)) {
		return nil, &siteError{Code: "mutation_unverified", Message: "退出请求已发送，但响应未返回登录表单", Details: map[string]any{"submitted": true, "confirmed": false, "evidence": "unknown"}}
	}
	_, cookiePath, resolveErr := resolveSite(siteRequest{Service: "graduate-admissions", CookieFile: cookie})
	if resolveErr != nil {
		return nil, resolveErr
	}
	if removeErr := removeCookieFile(cookiePath); removeErr != nil {
		return nil, &siteError{Code: "cookie_write_failed", Message: removeErr.Error()}
	}
	result["service"], result["operation"], result["logged_out"] = "graduate-admissions", "logout", true
	result["submitted"], result["confirmed"], result["evidence"] = true, true, "login-form-returned"
	return result, nil
}

func (a NativeSite) executeCMSAdmin(ctx context.Context, args []string, service, label string) (map[string]any, *siteError) {
	if len(args) == 0 || args[0] == "status" {
		statusArgs := args
		if len(statusArgs) > 0 {
			statusArgs = statusArgs[1:]
		}
		cookie, _, valueErr := businessValue(statusArgs, "--cookie-file")
		if valueErr != nil {
			return nil, valueErr
		}
		return a.executeServiceStatusWithOptions(ctx, service, "status", label, businessRequestOptions{cookieFile: cookie})
	}
	if args[0] == "catalog" {
		return businessCatalogFilter(service), nil
	}
	cookie, _, valueErr := businessValue(args[1:], "--cookie-file")
	if valueErr != nil {
		return nil, valueErr
	}
	switch args[0] {
	case "login":
		return a.cmsLogin(ctx, args[1:], service, cookie)
	case "logout":
		_, cookiePath, resolveErr := resolveSite(siteRequest{Service: service, CookieFile: cookie})
		if resolveErr != nil {
			return nil, resolveErr
		}
		if removeErr := removeCookieFile(cookiePath); removeErr != nil {
			return nil, &siteError{Code: "cookie_write_failed", Message: removeErr.Error()}
		}
		return map[string]any{"ok": true, "submitted": false, "confirmed": true, "evidence": "local-cookie-removed", "service": service, "operation": "logout", "logged_out": true, "cookie_file": cookiePath}, nil
	default:
		return nil, &siteError{Code: "invalid_argument", Message: service + " 只支持 status、catalog、login、logout"}
	}
}

func cmsLoginPage(body string) bool {
	document, err := parsePage(body)
	if err != nil {
		return false
	}
	form := document.first("form", "loginform")
	if form == nil {
		for _, candidate := range document.findAll("form") {
			if strings.EqualFold(candidate.attr("name"), "loginform") {
				form = candidate
				break
			}
		}
	}
	if form == nil {
		return false
	}
	for _, input := range form.findAll("input") {
		if input.attr("name") == "user" || input.attr("id") == "user" {
			return true
		}
	}
	return false
}

func setFormField(fields []pair, name, value string) []pair {
	for index := range fields {
		if fields[index].name == name {
			fields[index].value = value
			return fields
		}
	}
	return append(fields, pair{name, value})
}

func cmsPasswordState(password string) string {
	if len([]rune(password)) < 7 || len([]rune(password)) > 20 || strings.ContainsAny(password, "\r\n") {
		return "0"
	}
	classes := 0
	for _, pattern := range []*regexp.Regexp{regexp.MustCompile(`[A-Z]`), regexp.MustCompile(`[a-z]`), regexp.MustCompile(`[0-9]`), regexp.MustCompile(`[^a-zA-Z0-9_]`)} {
		if pattern.MatchString(password) {
			classes++
		}
	}
	if classes >= 2 && !regexp.MustCompile(`[\x{4e00}-\x{9fa5}]`).MatchString(password) {
		return "true"
	}
	return "0"
}

func (a NativeSite) cmsLogin(ctx context.Context, args []string, service, cookie string) (map[string]any, *siteError) {
	account, password, credentialErr := businessCredentials(args, "CSUST_CMS_PASSWORD")
	if credentialErr != nil {
		return nil, credentialErr
	}
	scope := strings.ToLower(flagValue(args, "--scope"))
	if scope == "" {
		scope = "website"
	}
	scopeValue := map[string]string{"website": "0", "site": "0", "system": "1"}[scope]
	if scopeValue == "" {
		return nil, &siteError{Code: "invalid_argument", Message: "--scope 只能是 website 或 system"}
	}
	page, requestErr := a.businessGet(ctx, service, "/system/login.jsp", nil, businessRequestOptions{cookieFile: cookie})
	if requestErr != nil {
		return nil, requestErr
	}
	captcha := flagValue(args, "--captcha")
	if captcha == "" {
		_, cookiePath, resolveErr := resolveSite(siteRequest{Service: service, CookieFile: cookie})
		if resolveErr != nil {
			return nil, resolveErr
		}
		imagePath := flagValue(args, "--captcha-image")
		if imagePath == "" {
			imagePath = filepath.Join(filepath.Dir(cookiePath), cookieHost(mustParseURL(knownSites[service].scheme+"://"+knownSites[service].host))+"-cms-captcha.png")
		}
		if _, imageErr := a.execute(ctx, siteRequest{Service: service, Path: "/system/login/new/codeimg.jsp", Params: []pair{{"randnum", strconv.FormatInt(time.Now().UnixMilli(), 10)}}, Method: "GET", CookieFile: cookie, Output: expandUserPath(imagePath), ReadOnly: true, Yes: true}); imageErr != nil {
			return nil, imageErr
		}
		return nil, &siteError{Code: "captcha_required", Message: "CMS 登录需要验证码，请提供 --captcha", Details: map[string]any{"captcha_image": expandUserPath(imagePath)}}
	}
	hidden, formErr := hiddenFormFields(businessBody(page), "loginform")
	if formErr != nil {
		return nil, formErr
	}
	digest := md5.Sum([]byte(password))
	passwordValue := strings.ToUpper(hex.EncodeToString(digest[:]))
	passwordValue += "_" + sm3Hex(password)
	hidden = setFormField(hidden, "action", map[bool]string{true: "syslogin", false: "login"}[scopeValue == "1"])
	hidden = setFormField(hidden, "password", passwordValue)
	hidden = setFormField(hidden, "pwd_state", cmsPasswordState(password))
	hidden = setFormField(hidden, "vsblogintype", scopeValue)
	hidden = setFormField(hidden, "user", account)
	hidden = setFormField(hidden, "logincode", captcha)
	result, requestErr := businessRequest(ctx, service, "POST", "/system/login.jsp", nil, hidden, []pair{{"Referer", safeResponseURL(page)}}, businessRequestOptions{cookieFile: cookie, allowBusinessFailure: true}, true, true)
	if requestErr != nil {
		return nil, requestErr
	}
	if failure := businessLoginResponseFailure(result); failure != nil {
		return nil, failure
	}
	evidence, probeErr := a.confirmBusinessLogin(ctx, service, "/system/login.jsp", cookie, cmsLoginPage)
	if probeErr != nil {
		return nil, probeErr
	}
	result["service"], result["operation"], result["username"], result["scope"] = service, "login", account, scope
	result["submitted"], result["confirmed"] = true, true
	result["evidence"] = "login-response-and-" + evidence
	return result, nil
}

func sm3Hex(value string) string {
	digest := sm3Digest([]byte(value))
	return strings.ToUpper(hex.EncodeToString(digest[:]))
}

func sm3Digest(message []byte) [32]byte {
	data := append([]byte(nil), message...)
	bitLength := uint64(len(data)) * 8
	data = append(data, 0x80)
	for len(data)%64 != 56 {
		data = append(data, 0)
	}
	var length [8]byte
	binary.BigEndian.PutUint64(length[:], bitLength)
	data = append(data, length[:]...)
	h := [8]uint32{0x7380166f, 0x4914b2b9, 0x172442d7, 0xda8a0600, 0xa96f30bc, 0x163138aa, 0xe38dee4d, 0xb0fb0e4e}
	for offset := 0; offset < len(data); offset += 64 {
		var w [68]uint32
		for index := 0; index < 16; index++ {
			w[index] = binary.BigEndian.Uint32(data[offset+index*4:])
		}
		for index := 16; index < 68; index++ {
			w[index] = sm3P1(w[index-16]^w[index-9]^bits.RotateLeft32(w[index-3], 15)) ^ bits.RotateLeft32(w[index-13], 7) ^ w[index-6]
		}
		var wPrime [64]uint32
		for index := range wPrime {
			wPrime[index] = w[index] ^ w[index+4]
		}
		a, b, c, d, e, f, g, hValue := h[0], h[1], h[2], h[3], h[4], h[5], h[6], h[7]
		for index := 0; index < 64; index++ {
			t := uint32(0x79cc4519)
			if index >= 16 {
				t = 0x7a879d8a
			}
			ss1 := bits.RotateLeft32(bits.RotateLeft32(a, 12)+e+bits.RotateLeft32(t, index%32), 7)
			ss2 := ss1 ^ bits.RotateLeft32(a, 12)
			ff, gg := a^b^c, e^f^g
			if index >= 16 {
				ff = (a & b) | (a & c) | (b & c)
				gg = (e & f) | (^e & g)
			}
			tt1 := ff + d + ss2 + wPrime[index]
			tt2 := gg + hValue + ss1 + w[index]
			d, c, b, a = c, bits.RotateLeft32(b, 9), a, tt1
			hValue, g, f, e = g, bits.RotateLeft32(f, 19), e, sm3P0(tt2)
		}
		h[0] ^= a
		h[1] ^= b
		h[2] ^= c
		h[3] ^= d
		h[4] ^= e
		h[5] ^= f
		h[6] ^= g
		h[7] ^= hValue
	}
	var result [32]byte
	for index, value := range h {
		binary.BigEndian.PutUint32(result[index*4:], value)
	}
	return result
}

func sm3P0(value uint32) uint32 {
	return value ^ bits.RotateLeft32(value, 9) ^ bits.RotateLeft32(value, 17)
}
func sm3P1(value uint32) uint32 {
	return value ^ bits.RotateLeft32(value, 15) ^ bits.RotateLeft32(value, 23)
}

func (a NativeSite) executeVirtualLab(ctx context.Context, args []string) (map[string]any, *siteError) {
	service, label := "virtual-lab", "公路交通虚拟仿真实验中心"
	if len(args) == 0 || args[0] == "status" {
		statusArgs := args
		if len(statusArgs) > 0 {
			statusArgs = statusArgs[1:]
		}
		cookie, _, valueErr := businessValue(statusArgs, "--cookie-file")
		if valueErr != nil {
			return nil, valueErr
		}
		return a.executeServiceStatusWithOptions(ctx, service, "status", label, businessRequestOptions{cookieFile: cookie})
	}
	if args[0] == "catalog" {
		return map[string]any{"ok": true, "submitted": false, "confirmed": true, "evidence": "live page and userdo.js protocol", "service": service, "operations": []string{"status", "resources", "appointment", "messages", "login", "register", "forgot", "upload-photo", "logout"}}, nil
	}
	cookie, _, valueErr := businessValue(args[1:], "--cookie-file")
	if valueErr != nil {
		return nil, valueErr
	}
	switch args[0] {
	case "resources":
		path := map[string]string{"road": "/Home/Sypt/dlqlydhgc", "surveying": "/Home/Sypt/chgc", "transport": "/Home/Sypt/jtys", "management": "/Home/Sypt/gcgl"}[strings.ToLower(flagValue(args[1:], "--discipline"))]
		if path == "" {
			return nil, &siteError{Code: "invalid_argument", Message: "resources 必须提供 --discipline road、surveying、transport 或 management"}
		}
		return a.virtualLabPage(ctx, path, "resources", cookie, false)
	case "appointment":
		return a.virtualLabPage(ctx, "/Home/Zxyy", "appointment", cookie, true)
	case "messages":
		params := []pair{}
		if keyword := flagValue(args[1:], "--keyword"); keyword != "" {
			params = append(params, pair{"keyword", keyword})
		}
		return a.virtualLabPageWithParams(ctx, "/Home/Hdjl/", "messages", cookie, false, params)
	case "login":
		return a.virtualLabLogin(ctx, args[1:], cookie)
	case "register":
		return a.virtualLabRegister(ctx, args[1:], cookie)
	case "forgot":
		return a.virtualLabForgot(ctx, args[1:], cookie)
	case "upload-photo":
		return a.virtualLabUploadPhoto(ctx, args[1:], cookie)
	case "logout":
		_, cookiePath, resolveErr := resolveSite(siteRequest{Service: service, CookieFile: cookie})
		if resolveErr != nil {
			return nil, resolveErr
		}
		if removeErr := removeCookieFile(cookiePath); removeErr != nil {
			return nil, &siteError{Code: "cookie_write_failed", Message: removeErr.Error()}
		}
		return map[string]any{"ok": true, "submitted": false, "confirmed": true, "evidence": "local-cookie-removed", "service": service, "operation": "logout", "logged_out": true, "cookie_file": cookiePath}, nil
	default:
		return nil, &siteError{Code: "invalid_argument", Message: service + " 只支持 status、catalog、resources、appointment、messages、login、register、forgot、upload-photo、logout"}
	}
}

func (a NativeSite) virtualLabPage(ctx context.Context, path, operation, cookie string, requireLogin bool) (map[string]any, *siteError) {
	return a.virtualLabPageWithParams(ctx, path, operation, cookie, requireLogin, nil)
}

func (a NativeSite) virtualLabPageWithParams(ctx context.Context, path, operation, cookie string, requireLogin bool, params []pair) (map[string]any, *siteError) {
	result, requestErr := a.businessGet(ctx, "virtual-lab", path, params, businessRequestOptions{cookieFile: cookie, require: requireLogin})
	if requestErr != nil {
		return nil, requestErr
	}
	if requireLogin && strings.Contains(businessBody(result), "您还未登录") {
		return nil, &siteError{Code: "login_required", Message: "虚拟实验中心需要登录", Details: map[string]any{"request": result["request"]}}
	}
	result = sitePageResult(result)
	result["service"], result["operation"] = "virtual-lab", operation
	return result, nil
}

func (a NativeSite) virtualLabCaptcha(ctx context.Context, args []string, cookie string) (string, *siteError) {
	if captcha := flagValue(args, "--captcha"); captcha != "" {
		return captcha, nil
	}
	_, cookiePath, resolveErr := resolveSite(siteRequest{Service: "virtual-lab", CookieFile: cookie})
	if resolveErr != nil {
		return "", resolveErr
	}
	imagePath := flagValue(args, "--captcha-image")
	if imagePath == "" {
		imagePath = filepath.Join(filepath.Dir(cookiePath), "virtual-lab-captcha.png")
	}
	imagePath = expandUserPath(imagePath)
	if _, imageErr := a.execute(ctx, siteRequest{Service: "virtual-lab", Path: "/Home/VerificationCode", Params: []pair{{"time", strconv.FormatInt(time.Now().UnixNano(), 10)}}, Method: "GET", CookieFile: cookie, Output: imagePath, ReadOnly: true, Yes: true}); imageErr != nil {
		return "", imageErr
	}
	return "", &siteError{Code: "captcha_required", Message: "虚拟实验中心需要验证码，请提供 --captcha", Details: map[string]any{"captcha_image": imagePath}}
}

func virtualLabPayload(result map[string]any) (map[string]any, *siteError) {
	payload, parseErr := businessJSONMap(result)
	if parseErr != nil {
		return nil, parseErr
	}
	return payload, nil
}

func virtualLabMutation(result map[string]any, operation string) (map[string]any, *siteError) {
	payload, parseErr := virtualLabPayload(result)
	if parseErr != nil {
		return nil, parseErr
	}
	state, _ := payload["state"].(string)
	if !strings.EqualFold(state, "success") {
		message := firstNonEmpty(fmt.Sprint(payload["message"]), "虚拟实验中心未确认操作成功")
		return nil, &siteError{Code: "mutation_rejected", Message: message, Details: map[string]any{"submitted": true, "confirmed": false, "evidence": "state-not-success", "remote_state": state}}
	}
	result = businessResult(result, "virtual-lab", operation)
	result["submitted"], result["confirmed"], result["evidence"] = true, true, "response-state-success"
	return result, nil
}

func (a NativeSite) virtualLabLogin(ctx context.Context, args []string, cookie string) (map[string]any, *siteError) {
	username, err := businessRequired(args, "--username", "login 必须提供 --username")
	if err != nil {
		return nil, err
	}
	password, err := businessSecret(args, "--password", "CSUST_VIRTUAL_LAB_PASSWORD")
	if err != nil {
		return nil, err
	}
	captcha, captchaErr := a.virtualLabCaptcha(ctx, args, cookie)
	if captchaErr != nil {
		return nil, captchaErr
	}
	result, requestErr := businessRequest(ctx, "virtual-lab", "POST", "/Home/Login", nil, []pair{{"username", username}, {"password", password}, {"code", captcha}}, nil, businessRequestOptions{cookieFile: cookie, allowBusinessFailure: true}, true, true)
	if requestErr != nil {
		return nil, requestErr
	}
	payload, parseErr := virtualLabPayload(result)
	if parseErr != nil {
		return nil, parseErr
	}
	if !strings.EqualFold(fmt.Sprint(payload["state"]), "success") {
		code := "authentication_failed"
		if strings.EqualFold(fmt.Sprint(payload["control"]), "msg_Code") {
			code = "captcha_failed"
		}
		return nil, &siteError{Code: code, Message: firstNonEmpty(fmt.Sprint(payload["message"]), "虚拟实验中心登录失败"), Details: map[string]any{"submitted": false, "confirmed": false, "evidence": "response-state", "control": payload["control"]}}
	}
	result = businessResult(result, "virtual-lab", "login")
	result["username"], result["confirmed"], result["evidence"] = username, true, "response-state-success"
	return result, nil
}

func (a NativeSite) virtualLabRegister(ctx context.Context, args []string, cookie string) (map[string]any, *siteError) {
	if !businessBool(args, "--yes") {
		return nil, &siteError{Code: "confirmation_required", Message: "虚拟实验中心注册会提交个人资料，必须加 --yes"}
	}
	fields := []struct{ flag, name, message string }{
		{"--username", "username", "register 必须提供 --username"}, {"--real-name", "realname", "register 必须提供 --real-name"},
		{"--phone", "Phone", "register 必须提供 --phone"}, {"--password", "password", "register 必须提供 --password"},
		{"--question", "question", "register 必须提供 --question"}, {"--answer", "answer", "register 必须提供 --answer"},
		{"--photo-token", "fj", "register 必须提供 --photo-token"},
	}
	data := []pair{}
	for _, field := range fields {
		value, requiredErr := businessRequired(args, field.flag, field.message)
		if requiredErr != nil {
			return nil, requiredErr
		}
		data = append(data, pair{field.name, value})
	}
	captcha, captchaErr := a.virtualLabCaptcha(ctx, args, cookie)
	if captchaErr != nil {
		return nil, captchaErr
	}
	data = append(data, pair{"code", captcha})
	result, requestErr := businessRequest(ctx, "virtual-lab", "POST", "/Home/Register", nil, data, nil, businessRequestOptions{cookieFile: cookie, allowBusinessFailure: true}, true, true)
	if requestErr != nil {
		return nil, requestErr
	}
	return virtualLabMutation(result, "register")
}

func (a NativeSite) virtualLabForgot(ctx context.Context, args []string, cookie string) (map[string]any, *siteError) {
	if !businessBool(args, "--yes") {
		return nil, &siteError{Code: "confirmation_required", Message: "虚拟实验中心密码重置会修改凭据，必须加 --yes"}
	}
	data := []pair{}
	for _, field := range []struct{ flag, name, message string }{{"--username", "username", "forgot 必须提供 --username"}, {"--question", "question", "forgot 必须提供 --question"}, {"--answer", "answer", "forgot 必须提供 --answer"}, {"--new-password", "password", "forgot 必须提供 --new-password"}} {
		value, requiredErr := businessRequired(args, field.flag, field.message)
		if requiredErr != nil {
			return nil, requiredErr
		}
		data = append(data, pair{field.name, value})
	}
	captcha, captchaErr := a.virtualLabCaptcha(ctx, args, cookie)
	if captchaErr != nil {
		return nil, captchaErr
	}
	data = append(data, pair{"code", captcha})
	result, requestErr := businessRequest(ctx, "virtual-lab", "POST", "/Home/Forgot", nil, data, nil, businessRequestOptions{cookieFile: cookie, allowBusinessFailure: true}, true, true)
	if requestErr != nil {
		return nil, requestErr
	}
	return virtualLabMutation(result, "forgot")
}

func (a NativeSite) virtualLabUploadPhoto(ctx context.Context, args []string, cookie string) (map[string]any, *siteError) {
	if !businessBool(args, "--yes") {
		return nil, &siteError{Code: "confirmation_required", Message: "上传注册证照会写入远端，必须加 --yes"}
	}
	filePath, err := businessRequired(args, "--file", "upload-photo 必须提供 --file")
	if err != nil {
		return nil, err
	}
	filePath = expandUserPath(filePath)
	file, fileErr := siteFilePart("file", filePath, "证照文件")
	if fileErr != nil {
		return nil, fileErr
	}
	if file.size == 0 {
		return nil, &siteError{Code: "invalid_argument", Message: "证照文件不能为空"}
	}
	result, requestErr := a.execute(ctx, siteRequest{Service: "virtual-lab", Method: "POST", Path: "/Home/UpdateImg", CookieFile: cookie, Files: []filePart{file}, ReadOnly: false, Yes: true, RawJSON: true, AllowBusinessFailure: true})
	if requestErr != nil {
		return nil, requestErr
	}
	payload, parseErr := virtualLabPayload(result)
	if parseErr != nil {
		return nil, parseErr
	}
	code := fmt.Sprint(payload["code"])
	imageURL, _ := payload["url"].(string)
	if code == "0" || strings.TrimSpace(imageURL) == "" {
		return nil, &siteError{Code: "mutation_rejected", Message: firstNonEmpty(fmt.Sprint(payload["message"]), "证照上传失败"), Details: map[string]any{"submitted": true, "confirmed": false, "evidence": "upload-response"}}
	}
	result = businessResult(result, "virtual-lab", "upload-photo")
	result["submitted"], result["confirmed"], result["evidence"], result["photo_token"] = true, true, "upload-response-code-and-url", imageURL
	return result, nil
}

func (a NativeSite) executeSecurityAdmin(ctx context.Context, args []string) (map[string]any, *siteError) {
	service, label := "security-admin", "安全运维管理平台"
	if len(args) == 0 || args[0] == "status" {
		statusArgs := args
		if len(statusArgs) > 0 {
			statusArgs = statusArgs[1:]
		}
		cookie, _, valueErr := businessValue(statusArgs, "--cookie-file")
		if valueErr != nil {
			return nil, valueErr
		}
		return a.executeServiceStatusWithOptions(ctx, service, "status", label, businessRequestOptions{cookieFile: cookie, insecure: businessBool(statusArgs, "--insecure")})
	}
	if args[0] == "catalog" {
		return businessCatalogFilter(service), nil
	}
	cookie, _, valueErr := businessValue(args[1:], "--cookie-file")
	if valueErr != nil {
		return nil, valueErr
	}
	switch args[0] {
	case "login":
		return a.securityLogin(ctx, args[1:], cookie)
	case "logout":
		_, cookiePath, resolveErr := resolveSite(siteRequest{Service: service, CookieFile: cookie})
		if resolveErr != nil {
			return nil, resolveErr
		}
		if removeErr := removeCookieFile(cookiePath); removeErr != nil {
			return nil, &siteError{Code: "cookie_write_failed", Message: removeErr.Error()}
		}
		return map[string]any{"ok": true, "submitted": false, "confirmed": true, "evidence": "local-cookie-removed", "service": service, "operation": "logout", "logged_out": true, "cookie_file": cookiePath}, nil
	default:
		return nil, &siteError{Code: "invalid_argument", Message: service + " 只支持 status、catalog、login、logout"}
	}
}

func securityLoginForm(body string) bool {
	document, err := parsePage(body)
	if err != nil {
		return false
	}
	form := document.first("form", "loginForm")
	if form == nil {
		return false
	}
	for _, input := range form.findAll("input") {
		if input.attr("name") == "user[account]" || input.attr("name") == "username" || input.attr("id") == "username" {
			return true
		}
	}
	return false
}

func securityPublicKey(body string) (string, *siteError) {
	document, err := parsePage(body)
	if err != nil {
		return "", &siteError{Code: "parse_error", Message: "安全运维登录页解析失败: " + err.Error()}
	}
	key := document.first("input", "public-key")
	if key == nil || strings.TrimSpace(key.attr("value")) == "" {
		return "", &siteError{Code: "parse_error", Message: "安全运维登录页缺少 RSA 公钥"}
	}
	return key.attr("value"), nil
}

func securityEncryptedPassword(publicKey, password string) (string, *siteError) {
	block, _ := pem.Decode([]byte(publicKey))
	if block == nil {
		return "", &siteError{Code: "protocol_error", Message: "安全运维 RSA 公钥格式无效"}
	}
	parsed, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return "", &siteError{Code: "protocol_error", Message: "安全运维 RSA 公钥解析失败: " + err.Error()}
	}
	key, ok := parsed.(*rsa.PublicKey)
	if !ok {
		return "", &siteError{Code: "protocol_error", Message: "安全运维登录公钥不是 RSA"}
	}
	ciphertext, err := rsa.EncryptPKCS1v15(cryptorand.Reader, key, []byte(password))
	if err != nil {
		return "", &siteError{Code: "protocol_error", Message: "安全运维密码加密失败: " + err.Error()}
	}
	return hex.EncodeToString(ciphertext), nil
}

func (a NativeSite) securityLogin(ctx context.Context, args []string, cookie string) (map[string]any, *siteError) {
	account, password, credentialErr := businessCredentials(args, "CSUST_SECURITY_PASSWORD")
	if credentialErr != nil {
		return nil, credentialErr
	}
	insecure := businessBool(args, "--insecure")
	options := businessRequestOptions{cookieFile: cookie, insecure: insecure, allowBusinessFailure: true}
	page, requestErr := a.businessGet(ctx, "security-admin", "/user/requireLogin", nil, options)
	if requestErr != nil {
		return nil, requestErr
	}
	publicKey, keyErr := securityPublicKey(businessBody(page))
	if keyErr != nil {
		return nil, keyErr
	}
	encrypted, encryptErr := securityEncryptedPassword(publicKey, password)
	if encryptErr != nil {
		return nil, encryptErr
	}
	preflight, requestErr := businessRequest(ctx, "security-admin", "POST", "/needUsbkey.php", nil, []pair{{"username", account}, {"password", encrypted}}, nil, options, true, true)
	if requestErr != nil {
		return nil, requestErr
	}
	authType := strings.TrimSpace(businessBody(preflight))
	if authType != "" && authType != "0" {
		method := map[string]string{"1": "certificate", "2": "sms", "3": "otp", "4": "guomi"}[authType]
		if method == "" {
			method = "unknown"
		}
		return nil, &siteError{Code: "authentication_required", Message: "安全运维平台要求额外认证: " + method, Details: map[string]any{"submitted": false, "confirmed": false, "evidence": "needUsbkey-response", "auth_type": authType}}
	}
	document, parseErr := parsePage(businessBody(page))
	if parseErr != nil {
		return nil, &siteError{Code: "parse_error", Message: "安全运维登录表单解析失败: " + parseErr.Error()}
	}
	csrf := document.first("input", "login-csrf")
	guid := document.first("input", "guid")
	if csrf == nil || guid == nil {
		return nil, &siteError{Code: "parse_error", Message: "安全运维登录表单缺少 CSRF 或 guid"}
	}
	data := []pair{{"login-csrf", csrf.attr("value")}, {"user[account]", account}, {"user[guid]", guid.attr("value")}, {"user[password]", encrypted}}
	result, requestErr := businessRequest(ctx, "security-admin", "POST", "/user/login", nil, data, []pair{{"Referer", safeResponseURL(page)}}, options, true, true)
	if requestErr != nil {
		return nil, requestErr
	}
	if failure := businessLoginResponseFailure(result); failure != nil {
		return nil, failure
	}
	evidence, probeErr := a.confirmBusinessLogin(ctx, "security-admin", "/user/requireLogin", cookie, securityLoginForm)
	if probeErr != nil {
		return nil, probeErr
	}
	result["service"], result["operation"], result["username"] = "security-admin", "login", account
	result["submitted"], result["confirmed"] = true, true
	result["evidence"] = "RSA-login-and-" + evidence
	return result, nil
}

func hiddenFormFields(body, formID string) ([]pair, *siteError) {
	document, parseErr := parsePage(body)
	if parseErr != nil {
		return nil, &siteError{Code: "parse_error", Message: "登录表单解析失败: " + parseErr.Error()}
	}
	form := document.first("form", formID)
	if form == nil {
		for _, candidate := range document.findAll("form") {
			if candidate.attr("name") == formID {
				form = candidate
				break
			}
		}
	}
	if form == nil {
		return nil, &siteError{Code: "parse_error", Message: "登录页面缺少预期表单: " + formID}
	}
	fields := make([]pair, 0)
	for _, input := range form.findAll("input") {
		if input.disabled() || input.attr("name") == "" || !strings.EqualFold(firstNonEmpty(input.attr("type"), "text"), "hidden") {
			continue
		}
		fields = append(fields, pair{input.attr("name"), input.attr("value")})
	}
	return fields, nil
}

func businessLoginFailure(body string) *siteError {
	if strings.TrimSpace(body) == "" {
		return &siteError{Code: "authentication_failed", Message: "登录响应为空，无法确认业务会话", Details: map[string]any{"submitted": true, "confirmed": false, "evidence": "empty-login-response"}}
	}
	if failure := loginFailure(body); failure != nil {
		failure.Details = map[string]any{"submitted": true, "confirmed": false, "evidence": "login-response"}
		return failure
	}
	if state, known, _ := businessState([]byte(body), "text/html"); known && !state {
		return &siteError{Code: "authentication_failed", Message: "登录响应明确报告失败", Details: map[string]any{"submitted": true, "confirmed": false, "evidence": "login-response-business-failure"}}
	}
	if document, err := parsePage(body); err == nil && pageFailureMessage.MatchString(strings.TrimSpace(pageDisplayText(document))) {
		return &siteError{Code: "authentication_failed", Message: "登录响应包含业务错误", Details: map[string]any{"submitted": true, "confirmed": false, "evidence": "login-response-error-text"}}
	}
	return nil
}

func businessLoginResponseFailure(result map[string]any) *siteError {
	if response, ok := result["response"].(map[string]any); ok {
		for _, key := range []string{"json_internal", "json"} {
			if value, exists := response[key]; exists {
				if state, known := jsonBusinessState(value); known && !state {
					return &siteError{Code: "authentication_failed", Message: "登录响应明确报告失败", Details: map[string]any{"submitted": true, "confirmed": false, "evidence": "login-response-business-failure"}}
				}
				return nil
			}
		}
	}
	return businessLoginFailure(businessBody(result))
}

func (a NativeSite) confirmBusinessLogin(ctx context.Context, service, path, cookie string, loginPage func(string) bool) (string, *siteError) {
	result, requestErr := a.businessGet(ctx, service, path, nil, businessRequestOptions{cookieFile: cookie})
	if requestErr != nil {
		return "", &siteError{Code: "authentication_failed", Message: "登录请求已发送，但业务会话探针失败", Details: map[string]any{"submitted": true, "confirmed": false, "evidence": "probe-error", "cause": requestErr.Code}}
	}
	if response, ok := result["response"].(map[string]any); ok {
		for _, key := range []string{"json_internal", "json"} {
			if value, exists := response[key]; exists {
				if state, known := jsonBusinessState(value); known {
					if !state {
						return "", &siteError{Code: "authentication_failed", Message: "业务会话探针明确返回失败", Details: map[string]any{"submitted": true, "confirmed": false, "evidence": "probe-business-failure"}}
					}
					return "probe-json-success", nil
				}
			}
		}
	}
	body := businessBody(result)
	if strings.TrimSpace(body) == "" {
		return "", &siteError{Code: "authentication_failed", Message: "登录请求已发送，但业务会话探针为空", Details: map[string]any{"submitted": true, "confirmed": false, "evidence": "probe-empty"}}
	}
	if loginPage(body) {
		return "", &siteError{Code: "authentication_failed", Message: "登录请求已发送，但业务会话仍返回登录表单", Details: map[string]any{"submitted": true, "confirmed": false, "evidence": "probe-login-form"}}
	}
	if failure := businessLoginFailure(body); failure != nil {
		failure.Details["evidence"] = "probe-business-failure"
		return "", failure
	}
	return "probe-page-without-login-form", nil
}

func continuingLoginPage(body string) bool {
	document, err := parsePage(body)
	return err == nil && document.first("form", "formLogin") != nil && document.first("input", "edtLoginAccount") != nil
}

func (a NativeSite) executeContinuingEducation(ctx context.Context, args []string) (map[string]any, *siteError) {
	if len(args) == 0 || args[0] == "status" {
		return a.executeServiceStatus(ctx, "continuing-info", "status", "继续教育学生信息管理")
	}
	if args[0] == "catalog" {
		return businessCatalogFilter("continuing-info"), nil
	}
	cookie, _, valueErr := businessValue(args[1:], "--cookie-file")
	if valueErr != nil {
		return nil, valueErr
	}
	switch args[0] {
	case "login":
		return a.continuingLogin(ctx, args[1:], cookie)
	case "logout":
		return a.continuingLogout(ctx, args[1:], cookie)
	default:
		return nil, &siteError{Code: "invalid_argument", Message: "continuing-education 只支持 status、catalog、login、logout"}
	}
}

func (a NativeSite) continuingLogin(ctx context.Context, args []string, cookie string) (map[string]any, *siteError) {
	account, password, credentialErr := businessCredentials(args, "CSUST_CONTINUING_PASSWORD")
	if credentialErr != nil {
		return nil, credentialErr
	}
	page, requestErr := a.businessGet(ctx, "continuing-info", "/", nil, businessRequestOptions{cookieFile: cookie})
	if requestErr != nil {
		return nil, requestErr
	}
	fields, formErr := hiddenFormFields(businessBody(page), "formLogin")
	if formErr != nil {
		return nil, formErr
	}
	fields = append(fields, pair{"edtLoginAccount", account}, pair{"edtLoginPassword", password}, pair{"btnLogin", "登录"})
	result, requestErr := businessRequest(ctx, "continuing-info", "POST", "/default.aspx", nil, fields, []pair{{"Referer", safeResponseURL(page)}}, businessRequestOptions{cookieFile: cookie, allowBusinessFailure: true}, true, true)
	if requestErr != nil {
		return nil, requestErr
	}
	if failure := businessLoginResponseFailure(result); failure != nil {
		return nil, failure
	}
	evidence, probeErr := a.confirmBusinessLogin(ctx, "continuing-info", "/", cookie, continuingLoginPage)
	if probeErr != nil {
		return nil, probeErr
	}
	result["service"], result["operation"], result["username"] = "continuing-info", "login", account
	result["submitted"], result["confirmed"] = true, true
	result["evidence"] = "login-response-and-" + evidence
	return result, nil
}

func (a NativeSite) continuingLogout(ctx context.Context, args []string, cookie string) (map[string]any, *siteError) {
	page, requestErr := a.businessGet(ctx, "continuing-info", "/", nil, businessRequestOptions{cookieFile: cookie})
	if requestErr != nil {
		return nil, requestErr
	}
	fields, formErr := hiddenFormFields(businessBody(page), "formLogin")
	if formErr != nil {
		return nil, formErr
	}
	fields = append(fields, pair{"btnQuit", "退出"})
	result, requestErr := businessRequest(ctx, "continuing-info", "POST", "/default.aspx", nil, fields, []pair{{"Referer", safeResponseURL(page)}}, businessRequestOptions{cookieFile: cookie}, true, true)
	if requestErr != nil {
		return nil, requestErr
	}
	if !continuingLoginPage(businessBody(result)) {
		return nil, &siteError{Code: "mutation_unverified", Message: "退出请求已发送，但响应未返回登录表单", Details: map[string]any{"submitted": true, "confirmed": false, "evidence": "unknown"}}
	}
	_, cookiePath, resolveErr := resolveSite(siteRequest{Service: "continuing-info", CookieFile: cookie})
	if resolveErr != nil {
		return nil, resolveErr
	}
	if removeErr := removeCookieFile(cookiePath); removeErr != nil {
		return nil, &siteError{Code: "cookie_write_failed", Message: removeErr.Error()}
	}
	result["service"], result["operation"], result["logged_out"] = "continuing-info", "logout", true
	result["submitted"], result["confirmed"], result["evidence"] = true, true, "login-form-returned"
	return result, nil
}

func (a NativeSite) executeSSOServiceCommand(ctx context.Context, args []string, service, label string) (map[string]any, *siteError) {
	if len(args) == 0 || args[0] == "status" {
		return a.executeServiceStatusWithOptions(ctx, service, "status", label, businessRequestOptions{allowSSO: true, require: true})
	}
	if args[0] == "catalog" {
		return businessCatalogFilter(service), nil
	}
	cookie, _, valueErr := businessValue(args[1:], "--cookie-file")
	if valueErr != nil {
		return nil, valueErr
	}
	switch args[0] {
	case "login":
		loginArgs := make([]string, 0, len(args)-1)
		for index := 1; index < len(args); index++ {
			if args[index] == "--cookie-file" {
				index++
				continue
			}
			loginArgs = append(loginArgs, args[index])
		}
		options, parseErr := parseLoginOptions(loginArgs)
		if parseErr != nil {
			return nil, parseErr
		}
		target, cookiePath, resolveErr := resolveSite(siteRequest{Service: service, CookieFile: cookie})
		if resolveErr != nil {
			return nil, resolveErr
		}
		return a.loginSSOService(ctx, target, cookiePath, options)
	case "logout":
		_, cookiePath, resolveErr := resolveSite(siteRequest{Service: service, CookieFile: cookie})
		if resolveErr != nil {
			return nil, resolveErr
		}
		if removeErr := removeCookieFile(cookiePath); removeErr != nil {
			return nil, &siteError{Code: "cookie_write_failed", Message: removeErr.Error()}
		}
		return map[string]any{"ok": true, "submitted": false, "confirmed": true, "evidence": "local-cookie-removed", "service": service, "operation": "logout", "logged_out": true, "cookie_file": cookiePath}, nil
	default:
		return nil, &siteError{Code: "invalid_argument", Message: service + " 只支持 status、catalog、login、logout"}
	}
}
