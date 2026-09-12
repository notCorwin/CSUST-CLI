package adapter

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	siteDomain = "csust.edu.cn"
	// ponytail: keep printable responses bounded; binary downloads stream to disk.
	maxSiteBody        = 64 << 20
	maxSiteRequestBody = 64 << 20
)

var errSiteRequestTooLarge = errors.New("site request body exceeds limit")

// NativeSite handles the direct-protocol path for the generic site API.
type NativeSite struct{}

type siteRequest struct {
	Service              string
	Path                 string
	Scheme               string
	Method               string
	Params               []pair
	Data                 []pair
	JSON                 any
	HasJSON              bool
	Files                []filePart
	Headers              []pair
	Yes                  bool
	Output               string
	CookieFile           string
	RequireLogin         bool
	Target               *url.URL
	ReadOnly             bool
	SessionTarget        *url.URL
	AllowSSO             bool
	AllowBusinessFailure bool
	InsecureTLS          bool
	RawJSON              bool
	mutating             bool
}

type pair struct{ name, value string }

type filePart struct {
	name, filename string
	path           string
	size           int64
	content        []byte
}

type serviceInfo struct {
	host   string
	scheme string
	path   string
}

var knownSites = map[string]serviceInfo{
	"official":                 {host: "www.csust.edu.cn", scheme: "https", path: "/"},
	"ehall":                    {host: "ehall.csust.edu.cn", scheme: "https", path: "/"},
	"auth":                     {host: "authserver.csust.edu.cn", scheme: "https", path: "/authserver/login"},
	"academic":                 {host: "xk.csust.edu.cn", scheme: "http", path: "/"},
	"vpn":                      {host: "vpn.csust.edu.cn", scheme: "https", path: "/enclient/start.html"},
	"sunshine":                 {host: "fuwu.csust.edu.cn", scheme: "https", path: "/"},
	"map":                      {host: "gis.csust.edu.cn", scheme: "https", path: "/"},
	"mail":                     {host: "mail.csust.edu.cn", scheme: "https", path: "/"},
	"library":                  {host: "lib.csust.edu.cn", scheme: "https", path: "/"},
	"library-catalog":          {host: "opac.csust.edu.cn", scheme: "https", path: "/index"},
	"theol":                    {host: "pt.csust.edu.cn", scheme: "http", path: "/meol/homepage/common/"},
	"mooc":                     {host: "mooc.csust.edu.cn", scheme: "http", path: "/portal"},
	"quality":                  {host: "zbxt.csust.edu.cn", scheme: "https", path: "/login"},
	"quality-system":           {host: "zbxt.csust.edu.cn", scheme: "https", path: "/login"},
	"recruitment":              {host: "rczpw.csust.edu.cn", scheme: "https", path: "/zp.html"},
	"jxjy":                     {host: "jxjy.csust.edu.cn", scheme: "https", path: "/"},
	"zyjx":                     {host: "zyjx.csust.edu.cn", scheme: "https", path: "/"},
	"research":                 {host: "ky.csust.edu.cn", scheme: "https", path: "/"},
	"legacy-portal":            {host: "my.csust.edu.cn", scheme: "http", path: "/"},
	"journal":                  {host: "cslgqk.csust.edu.cn", scheme: "https", path: "/"},
	"journal-qk":               {host: "cslgqk.csust.edu.cn", scheme: "https", path: "/cslgdxxbqks/home"},
	"journal-social":           {host: "cslgxbsk.csust.edu.cn", scheme: "https", path: "/"},
	"journal-science":          {host: "cslgxbzk.csust.edu.cn", scheme: "https", path: "/"},
	"journal-experiment":       {host: "syjx.csust.edu.cn", scheme: "https", path: "/"},
	"admissions":               {host: "zslq.csust.edu.cn", scheme: "https", path: "/"},
	"finance-query":            {host: "cwcx.csust.edu.cn", scheme: "http", path: "/AC/sso/index"},
	"union":                    {host: "gonghui.csust.edu.cn", scheme: "https", path: "/front/page.do?dispatch=proindex"},
	"transport-lab":            {host: "jtsysyy.csust.edu.cn", scheme: "http", path: "/Login/Index"},
	"continuing-platform":      {host: "xwwy.csust.edu.cn", scheme: "https", path: "/"},
	"transport-info":           {host: "jtxxgl.csust.edu.cn", scheme: "https", path: "/"},
	"transport-mobile":         {host: "jtyxxh.csust.edu.cn", scheme: "https", path: "/"},
	"electronic-documents":     {host: "kxpz.csust.edu.cn", scheme: "https", path: "/Integrated_platform/modules/student/OnlineAppL"},
	"academic-affairs":         {host: "jwc.csust.edu.cn", scheme: "http", path: "/"},
	"continuing-education":     {host: "xwwy.csust.edu.cn", scheme: "https", path: "/"},
	"graduate-management":      {host: "yjsgl.csust.edu.cn", scheme: "https", path: "/"},
	"admissions-system":        {host: "zs.csust.edu.cn", scheme: "https", path: "/"},
	"training-platform":        {host: "peixun.csust.edu.cn", scheme: "http", path: "/"},
	"alumni":                   {host: "xy.csust.edu.cn", scheme: "https", path: "/"},
	"app":                      {host: "app.csust.edu.cn:8087", scheme: "http", path: "/magus/appapi/downloadpage"},
	"library-remote":           {host: "tsgvpn2.csust.edu.cn", scheme: "https", path: "/"},
	"campus-map":               {host: "gis.csust.edu.cn", scheme: "https", path: "/"},
	"campus-network":           {host: "bw.csust.edu.cn", scheme: "http", path: "/Self/idstarlogin.action"},
	"student-digital-archive":  {host: "pdp.csust.edu.cn:8900", scheme: "https", path: "/stu/home/"},
	"equipment":                {host: "cslgdygx.csust.edu.cn", scheme: "https", path: "/"},
	"highway":                  {host: "highwayexperiment.csust.edu.cn", scheme: "https", path: "/"},
	"training":                 {host: "gcxljxgl.csust.edu.cn", scheme: "http", path: "/"},
	"journal-highway":          {host: "zwgl.csust.edu.cn", scheme: "https", path: "/zwgl/home"},
	"journal-highway-legacy":   {host: "zwgl1980.csust.edu.cn", scheme: "https", path: "/journal"},
	"fcmg":                     {host: "fcmg.csust.edu.cn", scheme: "https", path: "/"},
	"sqyrjd":                   {host: "sqyrjd.csust.edu.cn", scheme: "https", path: "/"},
	"srv":                      {host: "srv.csust.edu.cn", scheme: "http", path: "/"},
	"icsai2003":                {host: "icsai2003.csust.edu.cn", scheme: "http", path: "/"},
	"trx":                      {host: "trx.csust.edu.cn", scheme: "http", path: "/"},
	"v":                        {host: "v.csust.edu.cn", scheme: "http", path: "/"},
	"live":                     {host: "live.csust.edu.cn", scheme: "http", path: "/"},
	"graduate-notice":          {host: "yjsyzs.csust.edu.cn", scheme: "https", path: "/"},
	"graduate-admissions":      {host: "yjszs.csust.edu.cn", scheme: "https", path: "/ksxt/login.aspx"},
	"undergraduate-admissions": {host: "zslq.csust.edu.cn", scheme: "https", path: "/zsw/zsjh.html"},
	"journal-transport":        {host: "jtkxygc.csust.edu.cn", scheme: "https", path: "/jtkxygc/home"},
	"journal-highways":         {host: "glyqy.csust.edu.cn", scheme: "https", path: "/glyqy/home"},
	"onlinejudge":              {host: "acm.csust.edu.cn", scheme: "https", path: "/"},
	"library-personal":         {host: "book.csust.edu.cn", scheme: "https", path: "/ClientWeb/default.aspx"},
	"employment":               {host: "csust.bysjy.com.cn", scheme: "https", path: "/"},
	"student-record-query":     {host: "cs.luyinqingzhuhu.com", scheme: "http", path: "/?a=add&c=form&fid=2"},
	"continuing-info":          {host: "10.255.196.10:8080", scheme: "http", path: "/"},
	"party-school-exam":        {host: "10.255.195.65", scheme: "http", path: "/"},
	"student-archive":          {host: "10.255.196.138:8060", scheme: "http", path: "/"},
	"archive-management":       {host: "10.255.196.138:8080", scheme: "http", path: "/DAS/login.jsp"},
	"virtual-lab":              {host: "10.21.20.244:8088", scheme: "http", path: "/"},
	"legacy-mail":              {host: "txyj.csust.edu.cn", scheme: "http", path: "/mail/changepass"},
	"security-admin":           {host: "baolei.csust.edu.cn", scheme: "https", path: "/"},
	"cms-admin":                {host: "10.255.196.62:8080", scheme: "http", path: "/system/login.jsp"},
	"cms-admin-legacy":         {host: "10.255.196.2:8080", scheme: "http", path: "/system/login.jsp"},
}

// Run dispatches every supported command through the native Go adapters.
func (a NativeSite) Run(ctx context.Context, args []string, jsonMode bool) (handled bool, stdout, stderr []byte, code int, err error) {
	args = withoutGlobalJSON(args)
	if len(args) == 0 {
		if jsonMode {
			missing := &siteError{Code: "invalid_argument", Message: "缺少命令"}
			return true, errorJSON(missing), nil, 2, nil
		}
		return true, nativeUsage(args), nil, 0, nil
	}
	if containsHelp(args) {
		return true, nativeUsage(args), nil, 0, nil
	}
	if handled, stdout, stderr, code, err := a.runLoginCommand(ctx, args, jsonMode); handled {
		return handled, stdout, stderr, code, err
	}
	if handled, stdout, stderr, code, err := a.runTextbookCommand(ctx, args, jsonMode); handled {
		return handled, stdout, stderr, code, err
	}
	if handled, stdout, stderr, code, err := a.runAcademicCommand(ctx, args, jsonMode); handled {
		return handled, stdout, stderr, code, err
	}
	if handled, stdout, stderr, code, err := a.runWebCommand(ctx, args, jsonMode); handled {
		return handled, stdout, stderr, code, err
	}
	if handled, stdout, stderr, code, err := a.runVPNCommand(ctx, args, jsonMode); handled {
		return handled, stdout, stderr, code, err
	}
	if handled, stdout, stderr, code, err := a.runGatewayCommand(ctx, args, jsonMode); handled {
		return handled, stdout, stderr, code, err
	}
	if handled, stdout, stderr, code, err := a.runBusinessCommand(ctx, args, jsonMode); handled {
		return handled, stdout, stderr, code, err
	}
	if handled, stdout, stderr, code, err := a.runSiteCommand(ctx, args, jsonMode); handled {
		return handled, stdout, stderr, code, err
	}
	start := siteRequestStart(args)
	if start < 0 {
		unknown := &siteError{Code: "unknown_command", Message: "未知命令: " + strings.Join(args, " ")}
		if jsonMode {
			return true, errorJSON(unknown), nil, 2, nil
		}
		return true, nil, []byte("错误: " + unknown.Message + "\n"), 2, nil
	}
	req, parseErr := parseSiteRequest(args[start+2:])
	if parseErr != nil {
		if jsonMode {
			return true, errorJSON(parseErr), nil, 2, nil
		}
		return true, nil, []byte("错误: " + parseErr.Error() + "\n"), 2, nil
	}
	result, runErr := a.execute(ctx, req)
	if runErr != nil {
		if jsonMode {
			return true, errorJSON(runErr), nil, 2, nil
		}
		return true, nil, []byte("错误: " + runErr.Error() + "\n"), 2, nil
	}
	if jsonMode {
		encoded, encodeErr := json.Marshal(stripSiteInternal(result))
		if encodeErr != nil {
			return true, nil, nil, 2, encodeErr
		}
		return true, encoded, nil, 0, nil
	}
	return true, []byte(renderSiteResult(result)), nil, 0, nil
}

func withoutGlobalJSON(args []string) []string {
	result := make([]string, 0, len(args))
	for _, arg := range args {
		if arg != "--json" {
			result = append(result, arg)
		}
	}
	return result
}

func siteRequestStart(args []string) int {
	if len(args) < 2 || args[1] != "request" {
		return -1
	}
	switch args[0] {
	case "site", "domain", "portal":
		return 0
	default:
		return -1
	}
}

func containsHelp(args []string) bool {
	for _, arg := range args {
		if arg == "--help" || arg == "-h" {
			return true
		}
	}
	return false
}

func parseSiteRequest(args []string) (siteRequest, *siteError) {
	req := siteRequest{Method: "GET"}
	for index := 0; index < len(args); index++ {
		arg := args[index]
		inlineValue := ""
		inline := false
		if strings.HasPrefix(arg, "--") {
			if name, value, found := strings.Cut(arg, "="); found {
				arg, inlineValue, inline = name, value, true
			}
		}
		if arg == "--json" {
			if inline {
				return siteRequest{}, &siteError{Code: "invalid_argument", Message: "布尔参数不接受 =VALUE"}
			}
			continue
		}
		if arg == "--yes" {
			if inline {
				return siteRequest{}, &siteError{Code: "invalid_argument", Message: "布尔参数不接受 =VALUE"}
			}
			req.Yes = true
			continue
		}
		value, next, hasValue := inlineValue, index, inline
		if !inline {
			value, next, hasValue = nextValue(args, index)
		}
		if hasValue && !inline {
			index = next
		}
		switch arg {
		case "--service":
			req.Service = value
		case "--path":
			req.Path = value
		case "--scheme":
			req.Scheme = value
		case "--method":
			req.Method = strings.ToUpper(value)
		case "--param":
			pairValue, parseErr := splitPair(value, "--param")
			if parseErr != nil {
				return siteRequest{}, parseErr
			}
			req.Params = append(req.Params, pairValue)
		case "--data":
			pairValue, parseErr := splitPair(value, "--data")
			if parseErr != nil {
				return siteRequest{}, parseErr
			}
			req.Data = append(req.Data, pairValue)
		case "--data-json":
			body, parseErr := readJSONArgument(value)
			if parseErr != nil {
				return siteRequest{}, parseErr
			}
			req.JSON, req.HasJSON = body, true
		case "--file":
			part, parseErr := readFilePart(value)
			if parseErr != nil {
				return siteRequest{}, parseErr
			}
			req.Files = append(req.Files, part)
		case "--header":
			pairValue, parseErr := splitPair(value, "--header")
			if parseErr != nil {
				return siteRequest{}, parseErr
			}
			req.Headers = append(req.Headers, pairValue)
		case "--output":
			req.Output = expandUserPath(value)
		case "--cookie-file":
			req.CookieFile = expandUserPath(value)
		case "--require-login":
			if inline {
				return siteRequest{}, &siteError{Code: "invalid_argument", Message: "布尔参数不接受 =VALUE"}
			}
			req.RequireLogin = true
		case "--insecure":
			if inline {
				return siteRequest{}, &siteError{Code: "invalid_argument", Message: "布尔参数不接受 =VALUE"}
			}
			req.InsecureTLS = true
		case "--allow-external":
			if inline {
				return siteRequest{}, &siteError{Code: "invalid_argument", Message: "布尔参数不接受 =VALUE"}
			}
			// Generic requests stay same-origin; retain the shared page CLI flag for compatibility.
		default:
			if strings.HasPrefix(arg, "-") {
				return siteRequest{}, &siteError{Code: "invalid_argument", Message: "site request 参数无效: " + arg}
			}
			return siteRequest{}, &siteError{Code: "invalid_argument", Message: "site request 不接受位置参数"}
		}
		if !hasValue && arg != "--yes" && arg != "--require-login" && arg != "--allow-external" && arg != "--insecure" && arg != "--json" {
			return siteRequest{}, &siteError{Code: "invalid_argument", Message: arg + " 缺少参数值"}
		}
	}
	if req.Service == "" {
		return siteRequest{}, &siteError{Code: "invalid_argument", Message: "必须提供 --service"}
	}
	if req.Method == "" {
		req.Method = "GET"
	}
	if !supportedSiteMethod(req.Method) {
		return siteRequest{}, &siteError{Code: "invalid_argument", Message: "不支持的 HTTP 方法: " + req.Method}
	}
	if req.Scheme != "" && req.Scheme != "http" && req.Scheme != "https" {
		return siteRequest{}, &siteError{Code: "invalid_argument", Message: "服务传输方案只能是 http 或 https"}
	}
	if req.HasJSON && (len(req.Data) > 0 || len(req.Files) > 0) {
		return siteRequest{}, &siteError{Code: "invalid_argument", Message: "--data-json 不能与 --data/--file 同时使用"}
	}
	if readOnlyMethod(req.Method) && (req.HasJSON || len(req.Data) > 0 || len(req.Files) > 0) {
		return siteRequest{}, &siteError{Code: "invalid_argument", Message: "GET/HEAD/OPTIONS 请使用 --param"}
	}
	if mutatingMethod(req.Method) && !req.Yes {
		return siteRequest{}, &siteError{Code: "confirmation_required", Message: "site 请求可能修改远端数据，请加 --yes"}
	}
	return req, nil
}

func nextValue(args []string, index int) (string, int, bool) {
	if index+1 < len(args) && !strings.HasPrefix(args[index+1], "--") {
		return args[index+1], index + 1, true
	}
	return "", index, false
}

func splitPair(value, flag string) (pair, *siteError) {
	name, item, found := strings.Cut(value, "=")
	if !found || name == "" || strings.ContainsAny(name, "\r\n") {
		return pair{}, &siteError{Code: "invalid_argument", Message: flag + " 必须是 NAME=VALUE"}
	}
	return pair{name: name, value: item}, nil
}

func readJSONArgument(value string) (any, *siteError) {
	if value == "" {
		return nil, &siteError{Code: "invalid_argument", Message: "--data-json 不能为空"}
	}
	var content []byte
	var err error
	switch {
	case value == "-":
		content, err = readBoundedSiteInput(os.Stdin)
	case strings.HasPrefix(value, "@"):
		file, openErr := openSiteInput(expandUserPath(value[1:]))
		if openErr != nil {
			return nil, &siteError{Code: "invalid_argument", Message: "无法读取 JSON 请求体: " + openErr.Error()}
		}
		content, err = readBoundedSiteInput(file)
		_ = file.Close()
	default:
		content = []byte(value)
		if len(content) > maxSiteRequestBody {
			return nil, siteRequestTooLarge("JSON 请求体")
		}
	}
	if err != nil {
		if errors.Is(err, errSiteRequestTooLarge) {
			return nil, siteRequestTooLarge("JSON 请求体")
		}
		return nil, &siteError{Code: "invalid_argument", Message: "无法读取 JSON 请求体: " + err.Error()}
	}
	var body any
	if err := json.Unmarshal(content, &body); err != nil {
		return nil, &siteError{Code: "invalid_argument", Message: "JSON 请求体无效: " + err.Error()}
	}
	return body, nil
}

func readFilePart(value string) (filePart, *siteError) {
	pairValue, parseErr := splitPair(value, "--file")
	if parseErr != nil {
		return filePart{}, parseErr
	}
	return siteFilePart(pairValue.name, pairValue.value, "上传文件")
}

func siteFilePart(name, filename, label string) (filePart, *siteError) {
	filename = expandUserPath(filename)
	info, err := os.Lstat(filename)
	if err != nil {
		return filePart{}, &siteError{Code: "invalid_argument", Message: "无法读取" + label + ": " + err.Error()}
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return filePart{}, &siteError{Code: "invalid_argument", Message: label + "必须是普通文件且不能是符号链接"}
	}
	if info.Size() > maxSiteRequestBody {
		return filePart{}, siteRequestTooLarge(label)
	}
	return filePart{name: name, filename: filepath.Base(filename), path: filename, size: info.Size()}, nil
}

func openSiteInput(filename string) (*os.File, error) {
	info, err := os.Lstat(filename)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("输入文件必须是普通文件且不能是符号链接")
	}
	return os.Open(filename)
}

func readBoundedSiteInput(source io.Reader) ([]byte, error) {
	content, err := io.ReadAll(io.LimitReader(source, maxSiteRequestBody+1))
	if err != nil {
		return nil, err
	}
	if len(content) > maxSiteRequestBody {
		return nil, errSiteRequestTooLarge
	}
	return content, nil
}

func siteRequestTooLarge(label string) *siteError {
	return &siteError{Code: "request_too_large", Message: label + "超过 64 MiB 限制"}
}

func supportedSiteMethod(method string) bool {
	switch method {
	case "GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS":
		return true
	default:
		return false
	}
}

func readOnlyMethod(method string) bool {
	return method == "GET" || method == "HEAD" || method == "OPTIONS"
}

func mutatingMethod(method string) bool { return !readOnlyMethod(method) }

type siteError struct {
	Code    string
	Message string
	Details map[string]any
}

func (e *siteError) Error() string {
	if e == nil {
		return ""
	}
	return safeSiteErrorText(e.Message)
}

func errorJSON(err *siteError) []byte {
	payload := map[string]any{"error": safeSiteErrorText(err.Message), "code": err.Code}
	if len(err.Details) > 0 {
		payload["details"] = redactSiteJSON(stripSiteInternal(err.Details))
	}
	encoded, _ := json.Marshal(payload)
	return encoded
}

func stripSiteInternal(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		result := make(map[string]any, len(typed))
		for key, item := range typed {
			switch key {
			case "raw_url", "raw_path", "body_internal", "json_internal", "serialno_internal":
				continue
			}
			result[key] = stripSiteInternal(item)
		}
		return result
	case []any:
		result := make([]any, len(typed))
		for index, item := range typed {
			result[index] = stripSiteInternal(item)
		}
		return result
	case []map[string]any:
		result := make([]map[string]any, len(typed))
		for index, item := range typed {
			result[index] = stripSiteInternal(item).(map[string]any)
		}
		return result
	default:
		return value
	}
}

func (a NativeSite) execute(ctx context.Context, req siteRequest) (map[string]any, *siteError) {
	target := req.Target
	cookiePath := req.CookieFile
	if target == nil {
		var resolveErr *siteError
		target, cookiePath, resolveErr = resolveSite(req)
		if resolveErr != nil {
			return nil, resolveErr
		}
	} else {
		copy := *target
		target = &copy
		for _, item := range req.Params {
			query := target.Query()
			query.Add(item.name, item.value)
			target.RawQuery = query.Encode()
		}
		if cookiePath == "" {
			cookiePath = cookieFile("", target)
		}
	}
	if req.Output != "" {
		if outputErr := validateOutput(req.Output); outputErr != nil {
			return nil, outputErr
		}
	}
	mutating := !req.ReadOnly && (mutatingMethod(req.Method) || sideEffectSiteURL(target))
	req.mutating = mutating
	if mutating && !req.Yes {
		return nil, &siteError{Code: "confirmation_required", Message: "site 请求可能修改远端数据，请加 --yes"}
	}
	body, contentType, contentLength, cleanupBody, bodyErr := requestBody(req)
	if bodyErr != nil {
		return nil, bodyErr
	}
	if cleanupBody != nil {
		defer cleanupBody()
	}
	requestInfo := map[string]any{
		"method": req.Method,
		"url":    safeSiteURL(target),
		"fields": fieldNames(req.Data),
		"json":   req.HasJSON,
	}
	jar, jarErr := cookiejar.New(nil)
	if jarErr != nil {
		return nil, &siteError{Code: "session_error", Message: "无法创建 Cookie 会话: " + jarErr.Error()}
	}
	if loadErr := loadCookies(jar, cookiePath, target); loadErr != nil {
		return nil, &siteError{Code: "session_error", Message: "无法读取 Cookie 会话: " + loadErr.Error()}
	}
	if req.SessionTarget != nil && req.SessionTarget.Host != target.Host {
		if loadErr := loadCookies(jar, cookiePath, req.SessionTarget); loadErr != nil {
			return nil, &siteError{Code: "session_error", Message: "无法读取服务会话: " + loadErr.Error()}
		}
	}
	httpRequest, requestErr := http.NewRequestWithContext(ctx, req.Method, target.String(), body)
	if requestErr != nil {
		return nil, &siteError{Code: "invalid_argument", Message: "请求地址无效: " + requestErr.Error()}
	}
	if contentType != "" {
		httpRequest.Header.Set("Content-Type", contentType)
	}
	if body != nil {
		httpRequest.ContentLength = contentLength
	}
	httpRequest.Header.Set("User-Agent", "csust-cli-go/0.4.0")
	httpRequest.Header.Set("Accept", "text/html,application/xhtml+xml,application/json;q=0.9,*/*;q=0.8")
	for _, header := range req.Headers {
		if strings.ContainsAny(header.name, "\r\n") || strings.ContainsAny(header.value, "\r\n") || header.name == "" {
			return nil, &siteError{Code: "invalid_argument", Message: "请求头格式无效"}
		}
		httpRequest.Header.Set(header.name, header.value)
	}
	transport, _ := http.DefaultTransport.(*http.Transport)
	if transport != nil {
		transport = transport.Clone()
		if req.InsecureTLS {
			// Explicitly requested for legacy services with an expired certificate.
			transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec -- command flag is explicit
		}
	}
	httpClient := &http.Client{
		Transport: transport,
		Jar:       jar,
		Timeout:   60 * time.Second,
		CheckRedirect: func(clientRequest *http.Request, via []*http.Request) error {
			previous := httpRequest.URL
			if len(via) > 0 {
				previous = via[len(via)-1].URL
			}
			redirectBase := target
			if req.SessionTarget != nil {
				redirectBase = req.SessionTarget
			}
			if req.AllowSSO {
				normalizeSSOHTTPRedirect(previous, clientRequest.URL, redirectBase)
			}
			if !safeSiteRedirect(previous, clientRequest.URL) && (!req.AllowSSO || !safeSiteSSORedirect(redirectBase, previous, clientRequest.URL)) {
				return fmt.Errorf("已拒绝跨站或 HTTPS 降级重定向")
			}
			if !strings.EqualFold(previous.Host, clientRequest.URL.Host) && mutatingMethod(clientRequest.Method) {
				return fmt.Errorf("已拒绝把写请求重定向到其他站点")
			}
			return nil
		},
	}
	warnHTTPTransport(target)
	response, doErr := httpClient.Do(httpRequest)
	if doErr != nil {
		return nil, requestFailure(req, requestInfo, "network_error", "请求失败: "+doErr.Error())
	}
	defer response.Body.Close()
	mutating = req.mutating || (!req.ReadOnly && mutatingMethod(req.Method))
	if req.Output != "" && !mutating && !req.RequireLogin && response.StatusCode >= 200 && response.StatusCode < 300 && isBinarySiteContent(response) && !outputNeedsContentValidation(req.Output) {
		written, writeErr := atomicStreamWrite(req.Output, response.Body)
		if writeErr != nil {
			return nil, writeErr
		}
		saveTarget := target
		if req.SessionTarget != nil {
			saveTarget = req.SessionTarget
		}
		if saveErr := saveCookiesWithResponse(jar, cookiePath, saveTarget, response); saveErr != nil {
			return nil, &siteError{Code: "session_error", Message: "无法保存 Cookie 会话: " + saveErr.Error()}
		}
		result := map[string]any{
			"ok":           true,
			"submitted":    false,
			"confirmed":    true,
			"evidence":     "confirmed",
			"request":      requestInfo,
			"site":         origin(target),
			"downloaded":   true,
			"output":       req.Output,
			"bytes":        written,
			"content_type": response.Header.Get("Content-Type"),
		}
		if serialNo := response.Header.Get("Serialno"); serialNo != "" {
			result["serialno_internal"] = serialNo
		}
		return result, nil
	}
	content, readErr := io.ReadAll(io.LimitReader(response.Body, maxSiteBody+1))
	if readErr != nil {
		return nil, requestFailure(req, requestInfo, "network_error", "读取响应失败: "+readErr.Error())
	}
	if len(content) > maxSiteBody {
		return nil, requestFailure(req, requestInfo, "response_too_large", "响应超过 64 MiB 限制")
	}
	if req.Output == "" && isBinarySiteResponse(response) {
		return nil, requestFailure(req, requestInfo, "binary_output_required", "响应是二进制内容，请使用 --output 保存文件")
	}
	saveTarget := target
	if req.SessionTarget != nil {
		saveTarget = req.SessionTarget
	}
	if saveErr := saveCookiesWithResponse(jar, cookiePath, saveTarget, response); saveErr != nil {
		if mutating {
			return nil, requestFailure(req, requestInfo, "mutation_unverified", "请求已发送但会话保存失败: "+saveErr.Error())
		}
		return nil, &siteError{Code: "session_error", Message: "无法保存 Cookie 会话: " + saveErr.Error()}
	}
	if req.RequireLogin && looksLikeLogin(response, content) {
		if mutating {
			return nil, requestFailure(req, requestInfo, "mutation_unverified", "请求已发送但返回登录页，结果未知")
		}
		return nil, &siteError{Code: "login_required", Message: "响应是登录页，会话可能已失效", Details: map[string]any{"request": requestInfo}}
	}
	state, knownState, decoded := businessStateForRequest(content, response.Header.Get("Content-Type"), mutating)
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		code := "http_error"
		if mutating {
			code = "mutation_unverified"
		}
		return nil, requestFailure(req, requestInfo, code, fmt.Sprintf("HTTP %d %s", response.StatusCode, response.Status))
	}
	if knownState && !state && !req.AllowBusinessFailure {
		if !mutating && jsonRequiresLogin(decoded) {
			return nil, requestFailure(req, requestInfo, "login_required", "远端要求先登录")
		}
		code := "business_rejected"
		if mutating {
			code = "mutation_rejected"
		}
		return nil, requestFailure(req, requestInfo, code, "远端明确报告操作失败")
	}
	if mutating && !knownState {
		return nil, requestFailure(req, requestInfo, "mutation_unverified", "请求已提交但未取得成功证据")
	}
	if req.Output != "" {
		if outputErr := validateOutputContent(req.Output, content); outputErr != nil {
			return nil, outputErr
		}
		if writeErr := atomicWrite(req.Output, content); writeErr != nil {
			return nil, writeErr
		}
		return map[string]any{
			"ok":           true,
			"submitted":    mutating,
			"confirmed":    true,
			"evidence":     "confirmed",
			"request":      requestInfo,
			"site":         origin(target),
			"downloaded":   true,
			"output":       req.Output,
			"bytes":        len(content),
			"content_type": response.Header.Get("Content-Type"),
		}, nil
	}
	payload := responsePayload(response, content, decoded)
	if req.RawJSON && decoded != nil {
		payload["json_internal"] = decoded
	}
	return map[string]any{
		"ok":        true,
		"submitted": mutating,
		"confirmed": true,
		"evidence":  "confirmed",
		"request":   requestInfo,
		"site":      origin(target),
		"response":  payload,
	}, nil
}

func normalizeSSOHTTPRedirect(previous, next, serviceTarget *url.URL) {
	if previous == nil || next == nil || serviceTarget == nil || !strings.EqualFold(serviceTarget.Scheme, "https") {
		return
	}
	if strings.EqualFold(previous.Host, serviceTarget.Host) && strings.EqualFold(next.Host, "authserver.csust.edu.cn") && strings.EqualFold(next.Scheme, "http") {
		next.Scheme = "https"
		return
	}
	if strings.EqualFold(previous.Host, "authserver.csust.edu.cn") && strings.EqualFold(next.Host, serviceTarget.Host) && strings.EqualFold(next.Scheme, "http") {
		next.Scheme = "https"
	}
}

func responsePayload(response *http.Response, content []byte, decoded any) map[string]any {
	responseURL := ""
	rawURL := ""
	if response.Request != nil && response.Request.URL != nil {
		responseURL = safeSiteURL(response.Request.URL)
		rawURL = response.Request.URL.String()
	}
	payload := map[string]any{
		"status":       response.StatusCode,
		"url":          responseURL,
		"raw_url":      rawURL,
		"content_type": response.Header.Get("Content-Type"),
		"adapter":      "http-contract",
		"contract":     "http-response-v1",
	}
	if decoded != nil {
		payload["format"] = "json"
		payload["json"] = redactSiteJSON(decoded)
	} else {
		payload["format"] = "text"
		if isHTMLSiteResponse(response, content) {
			payload["format"] = "html"
			payload["confidence"] = "low"
			payload["confidence_evidence"] = map[string]any{
				"reason":       "通用 HTTP 适配器保留原始 HTML；页面结构请使用 site get",
				"content_type": response.Header.Get("Content-Type"),
				"bytes":        len(content),
			}
		}
		payload["body"] = redactSiteText(string(content), payload["format"] == "html")
		payload["body_internal"] = string(content)
	}
	if serialNo := response.Header.Get("Serialno"); serialNo != "" {
		payload["serialno_internal"] = serialNo
	}
	return payload
}

func redactSiteJSON(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		result := make(map[string]any, len(typed))
		for key, item := range typed {
			if sensitiveSiteParam.MatchString(key) {
				result[key] = "<redacted>"
				continue
			}
			result[key] = redactSiteJSON(item)
		}
		return result
	case []any:
		result := make([]any, len(typed))
		for index, item := range typed {
			result[index] = redactSiteJSON(item)
		}
		return result
	case string:
		return redactSiteText(typed, false)
	default:
		return value
	}
}

func isHTMLSiteResponse(response *http.Response, content []byte) bool {
	return strings.Contains(strings.ToLower(response.Header.Get("Content-Type")), "html") || bytes.HasPrefix(bytes.TrimSpace(content), []byte("<"))
}

func origin(target *url.URL) string {
	return (&url.URL{Scheme: target.Scheme, Host: target.Host}).String()
}

var httpTransportWarning sync.Once

func warnHTTPTransport(target *url.URL) {
	if target == nil || !strings.EqualFold(target.Scheme, "http") {
		return
	}
	httpTransportWarning.Do(func() {
		_, _ = fmt.Fprintf(os.Stderr, "警告: 目标 %s 使用 HTTP，不保证传输机密性\n", target.Host)
	})
}

func requestFailure(req siteRequest, request map[string]any, code, message string) *siteError {
	details := map[string]any{"request": request, "submitted": req.mutating || mutatingMethod(req.Method), "confirmed": false, "evidence": "unknown"}
	return &siteError{Code: code, Message: safeSiteErrorText(message), Details: details}
}

func businessState(content []byte, contentType string) (bool, bool, any) {
	trimmed := bytes.TrimSpace(content)
	var decoded any
	looksJSON := strings.Contains(strings.ToLower(contentType), "json") || bytes.HasPrefix(trimmed, []byte("{")) || bytes.HasPrefix(trimmed, []byte("["))
	if looksJSON && json.Unmarshal(trimmed, &decoded) == nil {
		state, known := jsonBusinessState(decoded)
		return state, known, decoded
	}
	if strings.Contains(strings.ToLower(contentType), "html") || bytes.HasPrefix(trimmed, []byte("<")) {
		state, known := pageFeedback(string(trimmed), contentType)
		return state, known, nil
	}
	text := strings.TrimSpace(string(content))
	if failureMessage(text) {
		return false, true, nil
	}
	if successMessage(text) {
		return true, true, nil
	}
	return false, false, nil
}

func businessStateForRequest(content []byte, contentType string, mutating bool) (bool, bool, any) {
	trimmed := bytes.TrimSpace(content)
	if !mutating && (strings.Contains(strings.ToLower(contentType), "html") || bytes.HasPrefix(trimmed, []byte("<"))) {
		return false, false, nil
	}
	return businessState(content, contentType)
}

func jsonBusinessState(value any) (bool, bool) {
	states := []bool{}
	var visit func(any)
	visit = func(item any) {
		switch typed := item.(type) {
		case map[string]any:
			if value, ok := typed["success"].(bool); ok {
				states = append(states, value)
			}
			if value, ok := typed["ok"].(bool); ok {
				states = append(states, value)
			}
			if value, exists := typed["error"]; exists {
				switch value := value.(type) {
				case nil:
					// Several JSON APIs use {error:null,data:...} as their success envelope.
					states = append(states, true)
				case string:
					if strings.TrimSpace(value) == "" {
						states = append(states, true)
					} else {
						states = append(states, false)
					}
				}
			}
			if code, ok := typed["code"]; ok {
				switch numeric := code.(type) {
				case float64:
					if numeric == 0 || numeric == 200 || numeric == 2000 {
						states = append(states, true)
					} else if numeric >= 400 {
						states = append(states, false)
					}
				case string:
					if numeric == "0" || numeric == "200" || numeric == "2000" {
						states = append(states, true)
					} else if parsed, err := strconv.Atoi(numeric); err == nil && parsed >= 400 && parsed <= 599 {
						states = append(states, false)
					}
				}
			}
			if code, ok := typed["errcode"]; ok {
				switch numeric := code.(type) {
				case float64:
					states = append(states, numeric == 0)
				case string:
					states = append(states, numeric == "0")
				}
			}
			if ret, ok := typed["ret"]; ok {
				switch numeric := ret.(type) {
				case float64:
					if numeric == 1 {
						states = append(states, true)
					} else if numeric == 0 {
						states = append(states, false)
					}
				case string:
					if numeric == "1" {
						states = append(states, true)
					} else if numeric == "0" {
						states = append(states, false)
					}
				}
			}
			if message, ok := typed["outmessage"]; ok {
				switch value := message.(type) {
				case bool:
					states = append(states, value)
				case string:
					states = append(states, strings.EqualFold(strings.TrimSpace(value), "true"))
				}
			}
			if state, ok := typed["state"].(string); ok {
				switch strings.ToLower(strings.TrimSpace(state)) {
				case "success", "ok", "1":
					states = append(states, true)
				case "error", "fail", "failed", "failure", "0":
					states = append(states, false)
				}
			}
			for _, key := range []string{"message", "messages", "msg", "error", "response"} {
				if nested, exists := typed[key]; exists {
					visit(nested)
				}
			}
		case []any:
			for _, nested := range typed {
				visit(nested)
			}
		case string:
			if failureMessage(typed) {
				states = append(states, false)
			} else if successMessage(typed) {
				states = append(states, true)
			}
		}
	}
	visit(value)
	for _, state := range states {
		if !state {
			return false, true
		}
	}
	if len(states) > 0 {
		return true, true
	}
	return false, false
}

func jsonRequiresLogin(value any) bool {
	need := false
	var visit func(any)
	visit = func(item any) {
		if need {
			return
		}
		switch typed := item.(type) {
		case map[string]any:
			for _, key := range []string{"error", "message", "msg", "state", "code"} {
				if nested, exists := typed[key]; exists {
					visit(nested)
				}
			}
		case []any:
			for _, nested := range typed {
				visit(nested)
			}
		case string:
			text := strings.ToLower(strings.TrimSpace(typed))
			need = strings.Contains(text, "login") || strings.Contains(text, "unauthorized") || strings.Contains(text, "未登录") || strings.Contains(text, "请先登录")
		}
	}
	visit(value)
	return need
}

var successPattern = regexp.MustCompile(`^(?:邮件发送|操作|提交|保存|更新|删除|发布|评价|报名|选课|缴费|撤销|订购|退订|选订|处理|发送|回复|修改|设置|上传|排序|预约|退出|注销|登出)?(?:成功|完成|已保存|已提交)[！!。.]?$`)
var sideEffectSitePattern = regexp.MustCompile(`(?i)(?:/(?:logout|delete|remove|add|join|bind|ignore|favorite|collectService|collectServiceItem|recommend|subscribe|unsubscribe|cancel|submit|save|update|sort)(?:[/?._]|$)|[?&](?:action|op|ACTION|operation|act)=)`)
var sensitiveSiteParam = regexp.MustCompile(`(?i)pass|password|pwd|encrypted|token|secret|sign|randomcode|ticket|cookie|session|csrf|nonce|execution|flowexecutionkey|(?:^|[_-])(?:state|lt)(?:$|[_-])`)
var siteURLPattern = regexp.MustCompile(`https?://[^\s"']+`)

func successMessage(value string) bool {
	text := strings.TrimSpace(value)
	return successPattern.MatchString(text)
}

func failureMessage(value string) bool {
	text := strings.TrimSpace(value)
	// ponytail: inspect short plain-text acknowledgements only; structural HTML uses site get.
	if text == "" || len(text) > 4096 || (strings.Contains(text, "<") && strings.Contains(text, ">")) {
		return false
	}
	for _, signal := range []string{"失败", "错误", "拒绝", "无效", "异常", "未授权", "禁止", "failed", "failure", "error", "denied", "invalid", "unauthorized", "forbidden"} {
		if strings.Contains(strings.ToLower(text), strings.ToLower(signal)) {
			return true
		}
	}
	return false
}

func sideEffectSiteURL(target *url.URL) bool {
	value := target.Path
	if target.RawQuery != "" {
		value += "?" + target.RawQuery
	}
	return sideEffectSitePattern.MatchString(value)
}

func safeSiteURL(target *url.URL) string {
	if target == nil {
		return ""
	}
	copy := *target
	return redactedURL(&copy).String()
}

func redactedURL(target *url.URL) *url.URL {
	copy := *target
	copy.User = nil
	query := copy.Query()
	for name, values := range query {
		for index, value := range values {
			if sensitiveSiteParam.MatchString(name) || len(value) >= 18 {
				values[index] = "<redacted>"
			}
		}
		query[name] = values
	}
	copy.RawQuery = query.Encode()
	copy.Fragment = ""
	return &copy
}

func safeSiteReference(value string) string {
	if value == "" || strings.HasPrefix(strings.ToLower(strings.TrimSpace(value)), "javascript:") {
		return value
	}
	target, err := url.Parse(value)
	if err != nil {
		return "<redacted>"
	}
	return redactedURL(target).String()
}

func redactSiteText(value string, htmlResponse bool) string {
	if !htmlResponse {
		return safeSiteErrorText(value)
	}
	value = redactSiteHTML(value)
	return safeSiteErrorText(value)
}

var (
	siteHTMLInputTag   = regexp.MustCompile(`(?is)<input\b[^>]*>`)
	siteHTMLTextArea   = regexp.MustCompile(`(?is)<textarea\b[^>]*>.*?</textarea>`)
	siteHTMLTag        = regexp.MustCompile(`(?is)<[a-z][^>]*>`)
	siteHTMLURLAttr    = regexp.MustCompile(`(?is)(\b(?:href|action|src|data-url|data-href)\s*=\s*)(?:"([^"]*)"|'([^']*)'|([^\s>]+))`)
	siteHTMLValueAttr  = regexp.MustCompile(`(?is)(\bvalue\s*=\s*)(?:"[^"]*"|'[^']*'|[^\s>]+)`)
	siteHTMLSecretAttr = regexp.MustCompile(`(?is)(\b(?:value|content|data-value|data-token|data-secret|data-csrf(?:-token)?|data-nonce)\s*=\s*)(?:"[^"]*"|'[^']*'|[^\s>]+)`)
	siteHTMLAttr       = regexp.MustCompile(`(?is)\b(?:name|id|type)\s*=\s*(?:"([^"]*)"|'([^']*)'|([^\s>]+))`)
	siteSecretText     = regexp.MustCompile(`(?i)(\b(?:password|passwd|pwd|token|secret|csrf|nonce|execution|ticket|session)[\w-]*\b\s*[:=]\s*)["']?[^\s<"']+`)
	siteInlineSecret   = regexp.MustCompile(`(?i)((?:[?&]|\b)(?:password|passwd|pwd|token|secret|csrf|nonce|execution|ticket|session)[\w-]*\s*=\s*)["']?[^&"'\s<>)]+`)
)

func redactSiteHTML(source string) string {
	source = siteHTMLInputTag.ReplaceAllStringFunc(source, func(tag string) string {
		if !siteHTMLSensitiveTag(tag) {
			return tag
		}
		return siteHTMLValueAttr.ReplaceAllString(tag, `${1}"<redacted>"`)
	})
	source = siteHTMLTextArea.ReplaceAllStringFunc(source, func(tag string) string {
		if !siteHTMLSensitiveTag(tag[:strings.Index(tag, ">")+1]) {
			return tag
		}
		end := strings.Index(tag, ">")
		close := strings.LastIndex(strings.ToLower(tag), "</textarea>")
		if end < 0 || close < end {
			return tag
		}
		return tag[:end+1] + "<redacted>" + tag[close:]
	})
	source = siteHTMLTag.ReplaceAllStringFunc(source, func(tag string) string {
		if !siteHTMLSensitiveTag(tag) {
			return tag
		}
		return siteHTMLSecretAttr.ReplaceAllString(tag, `${1}"<redacted>"`)
	})
	source = siteHTMLURLAttr.ReplaceAllStringFunc(source, func(attribute string) string {
		matches := siteHTMLURLAttr.FindStringSubmatch(attribute)
		if len(matches) < 5 {
			return attribute
		}
		value := firstNonEmpty(matches[2], matches[3], matches[4])
		return matches[1] + `"` + safeSiteReference(value) + `"`
	})
	source = siteSecretText.ReplaceAllString(source, `${1}<redacted>`)
	return siteInlineSecret.ReplaceAllString(source, `${1}<redacted>`)
}

func siteHTMLSensitiveTag(tag string) bool {
	for _, match := range siteHTMLAttr.FindAllStringSubmatch(tag, -1) {
		value := strings.ToLower(firstNonEmpty(match[1], match[2], match[3]))
		if sensitiveSiteParam.MatchString(value) || value == "password" {
			return true
		}
	}
	return false
}

func safeSiteErrorText(value string) string {
	value = siteURLPattern.ReplaceAllStringFunc(value, func(candidate string) string {
		target, err := url.Parse(candidate)
		if err != nil {
			return "<redacted-url>"
		}
		return safeSiteURL(target)
	})
	value = siteSecretText.ReplaceAllString(value, `${1}<redacted>`)
	return siteInlineSecret.ReplaceAllString(value, `${1}<redacted>`)
}

func isBinarySiteResponse(response *http.Response) bool {
	if strings.Contains(strings.ToLower(response.Header.Get("Content-Disposition")), "attachment") {
		return true
	}
	return isBinarySiteContent(response)
}

func isBinarySiteContent(response *http.Response) bool {
	contentType := strings.ToLower(strings.SplitN(response.Header.Get("Content-Type"), ";", 2)[0])
	if strings.HasPrefix(contentType, "image/") || strings.HasPrefix(contentType, "audio/") || strings.HasPrefix(contentType, "video/") {
		return contentType != "image/svg+xml"
	}
	switch contentType {
	case "application/octet-stream", "application/pdf", "application/zip", "application/gzip", "application/x-7z-compressed", "application/x-rar-compressed", "application/vnd.rar":
		return true
	default:
		return false
	}
}

func looksLikeLogin(response *http.Response, content []byte) bool {
	if !strings.Contains(strings.ToLower(response.Header.Get("Content-Type")), "html") {
		return false
	}
	text := strings.ToLower(string(content))
	if !strings.Contains(text, "<form") {
		return false
	}
	hasPassword := strings.Contains(text, `type="password"`) || strings.Contains(text, `type='password'`)
	for _, marker := range []string{
		`id="loginform"`,
		`id='loginform'`,
		`id="pwdfromid"`,
		`id='pwdfromid'`,
		`action="/login`,
		`action='/login`,
		`action="/logon`,
		`action='/logon`,
	} {
		if strings.Contains(text, marker) {
			return hasPassword
		}
	}
	if !hasPassword {
		return false
	}
	for _, marker := range []string{
		`name="username"`,
		`name='username'`,
		`name="useraccount"`,
		`name='useraccount'`,
		`name="account"`,
		`name='account'`,
		"请输入账号",
		"用户名或密码",
	} {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}

func safeSiteRedirect(previous, next *url.URL) bool {
	if previous == nil || next == nil || !strings.EqualFold(previous.Host, next.Host) {
		return false
	}
	if strings.EqualFold(previous.Scheme, next.Scheme) {
		return strings.EqualFold(next.Scheme, "http") || strings.EqualFold(next.Scheme, "https")
	}
	return strings.EqualFold(previous.Scheme, "http") && strings.EqualFold(next.Scheme, "https")
}

func safeSiteSSORedirect(base, previous, next *url.URL) bool {
	if base == nil || previous == nil || next == nil || strings.EqualFold(base.Host, "authserver.csust.edu.cn") {
		return false
	}
	if strings.EqualFold(previous.Host, base.Host) && strings.EqualFold(next.Host, "authserver.csust.edu.cn") {
		return strings.EqualFold(next.Scheme, "https")
	}
	return strings.EqualFold(previous.Host, "authserver.csust.edu.cn") && strings.EqualFold(next.Host, base.Host) && strings.EqualFold(next.Scheme, base.Scheme)
}

func validateOutput(filename string) *siteError {
	if strings.TrimSpace(filename) == "" {
		return &siteError{Code: "invalid_argument", Message: "下载路径不能为空"}
	}
	info, err := os.Lstat(filename)
	if err == nil && (info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular()) {
		return &siteError{Code: "invalid_argument", Message: "输出路径必须是普通文件"}
	}
	if err != nil && !os.IsNotExist(err) {
		return &siteError{Code: "invalid_argument", Message: "输出路径无效: " + err.Error()}
	}
	return nil
}

func outputNeedsContentValidation(filename string) bool {
	switch strings.ToLower(filepath.Ext(filename)) {
	case ".pdf":
		return true
	default:
		return false
	}
}

func validateOutputContent(filename string, content []byte) *siteError {
	valid := true
	switch strings.ToLower(filepath.Ext(filename)) {
	case ".pdf":
		valid = bytes.HasPrefix(content, []byte("%PDF-"))
	default:
		return nil
	}
	if valid {
		return nil
	}
	return &siteError{Code: "download_invalid", Message: "响应内容与输出文件类型不匹配", Details: map[string]any{"output": filepath.Base(filename)}}
}

func atomicWrite(filename string, content []byte) *siteError {
	_, writeErr := atomicStreamWrite(filename, bytes.NewReader(content))
	return writeErr
}

func atomicStreamWrite(filename string, source io.Reader) (int64, *siteError) {
	if outputErr := validateOutput(filename); outputErr != nil {
		return 0, outputErr
	}
	parent := filepath.Dir(filename)
	if err := os.MkdirAll(parent, 0700); err != nil {
		return 0, &siteError{Code: "output_write_failed", Message: "无法创建输出目录: " + err.Error()}
	}
	temporary, err := os.CreateTemp(parent, ".csust-output-*")
	if err != nil {
		return 0, &siteError{Code: "output_write_failed", Message: "无法创建临时文件: " + err.Error()}
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if err := temporary.Chmod(0600); err != nil {
		_ = temporary.Close()
		return 0, &siteError{Code: "output_write_failed", Message: "无法设置输出权限: " + err.Error()}
	}
	written, err := io.Copy(temporary, source)
	if err != nil {
		_ = temporary.Close()
		return 0, &siteError{Code: "output_write_failed", Message: "无法写入输出文件: " + err.Error()}
	}
	if err := temporary.Close(); err != nil {
		return 0, &siteError{Code: "output_write_failed", Message: "无法关闭输出文件: " + err.Error()}
	}
	if err := os.Rename(temporaryName, filename); err != nil {
		return 0, &siteError{Code: "output_write_failed", Message: "无法提交输出文件: " + err.Error()}
	}
	return written, nil
}

func fieldNames(items []pair) []string {
	result := make([]string, 0, len(items))
	for _, item := range items {
		result = append(result, item.name)
	}
	return result
}

func cookieFile(explicit string, target *url.URL) string {
	if explicit != "" {
		return explicit
	}
	if configured := os.Getenv("CSUST_COOKIE_FILE"); configured != "" {
		return expandUserPath(configured)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".", ".csust-cookies", cookieHost(target)+".cookies.txt")
	}
	return filepath.Join(home, ".config", "csust-cli", "sites", cookieHost(target)+".cookies.txt")
}

func expandUserPath(value string) string {
	if value != "~" && !strings.HasPrefix(value, "~/") {
		return value
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return value
	}
	if value == "~" {
		return home
	}
	return filepath.Join(home, value[2:])
}

func cookieHost(target *url.URL) string {
	host := strings.ReplaceAll(target.Hostname(), ".", "_")
	if port := target.Port(); port != "" {
		host += "_" + port
	}
	return host
}

func loadCookies(jar *cookiejar.Jar, filename string, target *url.URL) error {
	records, err := readCookieRecords(filename)
	if err != nil {
		return err
	}
	for _, cookie := range records {
		cookie := cookie
		jar.SetCookies(target, []*http.Cookie{&cookie})
	}
	return nil
}

func saveCookies(jar *cookiejar.Jar, filename string, target *url.URL) error {
	return saveCookiesWithResponse(jar, filename, target, nil)
}

func saveCookiesWithResponse(jar *cookiejar.Jar, filename string, target *url.URL, response *http.Response) error {
	if err := validateSessionFile(filename); err != nil {
		return err
	}
	return withSiteFileLock(filename, func() error {
		records, err := readCookieRecords(filename)
		if err != nil {
			return err
		}
		before := len(records)
		mergeJarCookies(records, jar, target)
		mergeResponseCookies(records, response)
		if len(records) == 0 && before == 0 {
			return nil
		}
		return writeCookieFileUnlocked(filename, cookieRecordsContent(records))
	})
}

func readCookieRecords(filename string) (map[string]http.Cookie, error) {
	records := make(map[string]http.Cookie)
	info, err := os.Lstat(filename)
	if os.IsNotExist(err) {
		return records, nil
	}
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("会话文件必须是普通文件且不能是符号链接")
	}
	_ = os.Chmod(filename, 0600)
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "#HttpOnly_") {
			line = strings.TrimPrefix(line, "#HttpOnly_")
		}
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) < 7 {
			continue
		}
		expires, _ := strconv.ParseInt(fields[4], 10, 64)
		cookie := http.Cookie{
			Domain: fields[0],
			Path:   fields[2],
			Secure: fields[3] == "TRUE",
			Name:   fields[5],
			Value:  fields[6],
		}
		if expires > 0 {
			cookie.Expires = time.Unix(expires, 0)
		}
		records[cookieRecordKey(cookie)] = cookie
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return records, nil
}

func mergeJarCookies(records map[string]http.Cookie, jar *cookiejar.Jar, target *url.URL) {
	for _, cookie := range cookieSnapshot(jar, target) {
		if cookie.Domain == "" {
			cookie.Domain = target.Hostname()
		}
		if cookie.Path == "" {
			cookie.Path = defaultCookiePath(target.Path)
		}
		records[cookieRecordKey(*cookie)] = *cookie
	}
}

func mergeResponseCookies(records map[string]http.Cookie, response *http.Response) {
	if response == nil || response.Request == nil || response.Request.URL == nil {
		return
	}
	for _, item := range response.Cookies() {
		cookie := *item
		if cookie.Domain == "" {
			cookie.Domain = response.Request.URL.Hostname()
		}
		if cookie.Path == "" {
			cookie.Path = defaultCookiePath(response.Request.URL.Path)
		}
		key := cookieRecordKey(cookie)
		if cookie.MaxAge < 0 || (!cookie.Expires.IsZero() && cookie.Expires.Before(time.Now())) {
			delete(records, key)
			continue
		}
		records[key] = cookie
	}
}

func cookieRecordKey(cookie http.Cookie) string {
	return strings.ToLower(cookie.Domain) + "\x00" + cookie.Path + "\x00" + cookie.Name
}

func defaultCookiePath(path string) string {
	if path == "" || !strings.HasPrefix(path, "/") {
		return "/"
	}
	if index := strings.LastIndex(path, "/"); index > 0 {
		return path[:index]
	}
	return "/"
}

func cookieRecordsContent(records map[string]http.Cookie) string {
	if len(records) == 0 {
		return "# Netscape HTTP Cookie File\n"
	}
	keys := make([]string, 0, len(records))
	for key := range records {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	lines := []string{"# Netscape HTTP Cookie File"}
	for _, key := range keys {
		cookie := records[key]
		expires := int64(0)
		if !cookie.Expires.IsZero() {
			expires = cookie.Expires.Unix()
		}
		cookiePath := cookie.Path
		if cookiePath == "" {
			cookiePath = "/"
		}
		lines = append(lines, strings.Join([]string{cookie.Domain, "TRUE", cookiePath, strconv.FormatBool(cookie.Secure), strconv.FormatInt(expires, 10), cookie.Name, cookie.Value}, "\t"))
	}
	return strings.Join(lines, "\n") + "\n"
}

// cookiejar exposes only cookies applicable to one URL; the record map keeps
// same-name cookies on different paths and persists server-side deletions.

func validateSessionFile(filename string) error {
	if info, err := os.Lstat(filename); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return fmt.Errorf("会话文件必须是普通文件且不能是符号链接")
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	return nil
}

func cookieSnapshot(jar *cookiejar.Jar, target *url.URL) []*http.Cookie {
	root := *target
	root.Path = "/"
	cookies := append(jar.Cookies(&root), jar.Cookies(target)...)
	seen := make(map[string]bool, len(cookies))
	unique := cookies[:0]
	for _, cookie := range cookies {
		key := strings.Join([]string{cookie.Domain, cookie.Path, cookie.Name}, "\x00")
		if seen[key] {
			continue
		}
		seen[key] = true
		unique = append(unique, cookie)
	}
	return unique
}

func cookieFileContent(jar *cookiejar.Jar, target *url.URL) string {
	records := make(map[string]http.Cookie)
	mergeJarCookies(records, jar, target)
	if len(records) == 0 {
		return ""
	}
	return cookieRecordsContent(records)
}

func writeCookieFile(filename, content string) error {
	return withSiteFileLock(filename, func() error { return writeCookieFileUnlocked(filename, content) })
}

func writeCookieFileUnlocked(filename, content string) error {
	parent := filepath.Dir(filename)
	if err := os.MkdirAll(parent, 0700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(parent, ".csust-cookie-*")
	if err != nil {
		return err
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if err := temporary.Chmod(0600); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.WriteString(content); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryName, filename)
}

func withSiteFileLock(filename string, action func() error) error {
	parent := filepath.Dir(filename)
	if err := os.MkdirAll(parent, 0700); err != nil {
		return err
	}
	lock, err := os.OpenFile(filename+".lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		return err
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	return action()
}

func renderSiteResult(result map[string]any) string {
	if result["downloaded"] == true {
		return fmt.Sprintf("已保存：%v（%v bytes）\n", result["output"], result["bytes"])
	}
	if response, ok := result["response"].(map[string]any); ok {
		if body, ok := response["body"].(string); ok {
			return body
		}
		if value, ok := response["json"]; ok {
			encoded, _ := json.MarshalIndent(value, "", "  ")
			return string(encoded) + "\n"
		}
	}
	return "请求已确认\n"
}

func resolveSite(req siteRequest) (*url.URL, string, *siteError) {
	key := strings.ToLower(strings.TrimSpace(req.Service))
	if key == "" || strings.ContainsAny(key, "/?#") {
		return nil, "", &siteError{Code: "invalid_argument", Message: "服务必须是目录名或官方主机名，不是 URL"}
	}
	info, known := knownSites[key]
	if !known {
		parsed, parseErr := url.Parse("//" + key)
		if parseErr != nil || parsed.User != nil || parsed.Hostname() == "" {
			return nil, "", &siteError{Code: "invalid_argument", Message: "服务主机名格式无效"}
		}
		if parsed.Hostname() != siteDomain && !strings.HasSuffix(strings.ToLower(parsed.Hostname()), "."+siteDomain) {
			return nil, "", &siteError{Code: "invalid_argument", Message: "服务必须是 csust.edu.cn 及其子域名"}
		}
		info = serviceInfo{host: strings.ToLower(parsed.Host), scheme: "https", path: "/"}
	}
	if base := os.Getenv("CSUST_BASE_URL"); base != "" {
		parsedBase, parseErr := url.Parse(base)
		if parseErr != nil || parsedBase.Host == "" || parsedBase.Hostname() == "" || parsedBase.User != nil || parsedBase.RawQuery != "" || parsedBase.Fragment != "" || (parsedBase.Scheme != "http" && parsedBase.Scheme != "https") {
			return nil, "", &siteError{Code: "invalid_path", Message: "CSUST_BASE_URL 地址无效"}
		}
		info.host, info.scheme, info.path = parsedBase.Host, parsedBase.Scheme, "/"
	}
	if req.Scheme != "" {
		info.scheme = req.Scheme
	}
	requestedPath := req.Path
	if requestedPath == "" {
		requestedPath = info.path
	}
	parsedPath, parseErr := url.Parse(requestedPath)
	if parseErr != nil || parsedPath.IsAbs() || parsedPath.Host != "" || strings.Contains(parsedPath.Path, "\\") {
		return nil, "", &siteError{Code: "invalid_path", Message: "服务路径必须是当前服务内的相对路径"}
	}
	decodedPath := parsedPath.Path
	for i := 0; i < 2; i++ {
		decodedPath, _ = url.PathUnescape(decodedPath)
	}
	for _, segment := range strings.Split(decodedPath, "/") {
		if segment == "." || segment == ".." {
			return nil, "", &siteError{Code: "invalid_path", Message: "服务路径不能包含目录跳转"}
		}
	}
	if !strings.HasPrefix(parsedPath.Path, "/") {
		parsedPath.Path = "/" + parsedPath.Path
	}
	parsedPath.Fragment = ""
	base := &url.URL{Scheme: info.scheme, Host: info.host}
	target := base.ResolveReference(parsedPath)
	for _, item := range req.Params {
		query := target.Query()
		query.Add(item.name, item.value)
		target.RawQuery = query.Encode()
	}
	if target.Path == "" {
		target.Path = "/"
	}
	return target, cookieFile(req.CookieFile, target), nil
}

func requestBody(req siteRequest) (io.Reader, string, int64, func(), *siteError) {
	if req.HasJSON {
		var buffer bytes.Buffer
		encoder := json.NewEncoder(&buffer)
		encoder.SetEscapeHTML(false)
		if err := encoder.Encode(req.JSON); err != nil {
			return nil, "", 0, nil, &siteError{Code: "invalid_argument", Message: "JSON 请求体无效: " + err.Error()}
		}
		if buffer.Len() > maxSiteRequestBody {
			return nil, "", 0, nil, siteRequestTooLarge("JSON 请求体")
		}
		return bytes.NewReader(buffer.Bytes()), "application/json", int64(buffer.Len()), nil, nil
	}
	if len(req.Files) > 0 {
		temporary, err := os.CreateTemp("", ".csust-request-*")
		if err != nil {
			return nil, "", 0, nil, &siteError{Code: "request_body_failed", Message: "无法创建临时请求体: " + err.Error()}
		}
		temporaryName := temporary.Name()
		cleanup := func() {
			_ = temporary.Close()
			_ = os.Remove(temporaryName)
		}
		if err := temporary.Chmod(0600); err != nil {
			cleanup()
			return nil, "", 0, nil, &siteError{Code: "request_body_failed", Message: "无法设置临时请求体权限: " + err.Error()}
		}
		limited := &siteBodyWriter{writer: temporary, remaining: maxSiteRequestBody}
		writer := multipart.NewWriter(limited)
		for _, item := range req.Data {
			if err := writer.WriteField(item.name, item.value); err != nil {
				cleanup()
				if errors.Is(err, errSiteRequestTooLarge) {
					return nil, "", 0, nil, siteRequestTooLarge("multipart 请求体")
				}
				return nil, "", 0, nil, &siteError{Code: "invalid_argument", Message: "表单字段无效: " + err.Error()}
			}
		}
		for _, item := range req.Files {
			part, err := writer.CreateFormFile(item.name, item.filename)
			if err != nil {
				cleanup()
				if errors.Is(err, errSiteRequestTooLarge) {
					return nil, "", 0, nil, siteRequestTooLarge("multipart 请求体")
				}
				return nil, "", 0, nil, &siteError{Code: "invalid_argument", Message: "上传字段无效: " + err.Error()}
			}
			var source io.Reader = bytes.NewReader(item.content)
			var file *os.File
			if item.path != "" {
				file, err = openSiteInput(item.path)
				if err != nil {
					cleanup()
					return nil, "", 0, nil, &siteError{Code: "invalid_argument", Message: "无法读取上传文件: " + err.Error()}
				}
				source = file
			}
			_, copyErr := io.Copy(part, source)
			if file != nil {
				_ = file.Close()
			}
			if copyErr != nil {
				cleanup()
				if errors.Is(copyErr, errSiteRequestTooLarge) {
					return nil, "", 0, nil, siteRequestTooLarge("multipart 请求体")
				}
				return nil, "", 0, nil, &siteError{Code: "invalid_argument", Message: "上传文件读取失败: " + copyErr.Error()}
			}
		}
		if err := writer.Close(); err != nil {
			cleanup()
			if errors.Is(err, errSiteRequestTooLarge) {
				return nil, "", 0, nil, siteRequestTooLarge("multipart 请求体")
			}
			return nil, "", 0, nil, &siteError{Code: "invalid_argument", Message: "multipart 请求无效: " + err.Error()}
		}
		if err := temporary.Close(); err != nil {
			cleanup()
			return nil, "", 0, nil, &siteError{Code: "request_body_failed", Message: "无法关闭临时请求体: " + err.Error()}
		}
		reader, err := os.Open(temporaryName)
		if err != nil {
			cleanup()
			return nil, "", 0, nil, &siteError{Code: "request_body_failed", Message: "无法打开临时请求体: " + err.Error()}
		}
		info, err := reader.Stat()
		if err != nil {
			_ = reader.Close()
			cleanup()
			return nil, "", 0, nil, &siteError{Code: "request_body_failed", Message: "无法检查临时请求体: " + err.Error()}
		}
		cleanup = func() {
			_ = reader.Close()
			_ = os.Remove(temporaryName)
		}
		return reader, writer.FormDataContentType(), info.Size(), cleanup, nil
	}
	if len(req.Data) > 0 {
		values := url.Values{}
		for _, item := range req.Data {
			values.Add(item.name, item.value)
		}
		encoded := values.Encode()
		if len(encoded) > maxSiteRequestBody {
			return nil, "", 0, nil, siteRequestTooLarge("表单请求体")
		}
		return strings.NewReader(encoded), "application/x-www-form-urlencoded", int64(len(encoded)), nil, nil
	}
	return nil, "", 0, nil, nil
}

type siteBodyWriter struct {
	writer    io.Writer
	remaining int64
}

func (w *siteBodyWriter) Write(value []byte) (int, error) {
	if int64(len(value)) > w.remaining {
		if w.remaining > 0 {
			written, _ := w.writer.Write(value[:w.remaining])
			w.remaining -= int64(written)
			return written, errSiteRequestTooLarge
		}
		return 0, errSiteRequestTooLarge
	}
	written, err := w.writer.Write(value)
	w.remaining -= int64(written)
	return written, err
}
