package adapter

import (
	"context"
	"fmt"
	"math"
	"net/url"
	"os"
	"strconv"
	"strings"
)

const learningBasicAuthorization = "Basic YWRtaW46emhvbmdzaGVuMTIzNDU="

type learningPlatform struct {
	command        string
	siteService    string
	website        string
	customerNo     string
	courseBase     string
	courseListPath string
	categoryPath   string
	noticeBase     string
	detailBase     string
}

var learningPlatforms = map[string]learningPlatform{
	"professional-learning": {
		command: "professional-learning", siteService: "jxjy",
		website: "jxjy.csust.edu.cn", customerNo: "cslgz2024",
		courseBase: "https://api.share.zsjykj.net", courseListPath: "/GetBaseResource",
		categoryPath: "/GetCustomerCategory", noticeBase: "https://newapi.ylxue.net",
		detailBase: "https://api.share.zsjykj.net",
	},
	"institutional-learning": {
		command: "institutional-learning", siteService: "zyjx",
		website: "zyjx.csust.edu.cn", customerNo: "cslgsy123",
		courseBase: "https://hnapishare.ylxue.net", courseListPath: "/api/cslgsy/query-resource",
		categoryPath: "/GetCustomerCategory", noticeBase: "https://newapi.ylxue.net",
		detailBase: "https://api.share.zsjykj.net",
	},
}

func (a NativeSite) executeLearning(ctx context.Context, args []string, command string) (map[string]any, *siteError) {
	platform, ok := learningPlatforms[command]
	if !ok {
		return nil, &siteError{Code: "invalid_argument", Message: "未知继续教育平台: " + command}
	}
	if len(args) == 0 || args[0] == "catalog" {
		return businessCatalogNames(command), nil
	}
	cookie, _, valueErr := businessValue(args[1:], "--cookie-file")
	if valueErr != nil {
		return nil, valueErr
	}
	switch args[0] {
	case "courses", "list":
		return a.learningCourses(ctx, args[1:], cookie, platform)
	case "categories":
		return a.learningCategories(ctx, args[1:], cookie, platform)
	case "course", "detail":
		return a.learningCourseDetail(ctx, args[1:], cookie, platform)
	case "notices":
		return a.learningNotices(ctx, args[1:], cookie, platform)
	case "notice":
		return a.learningNoticeDetail(ctx, args[1:], cookie, platform)
	default:
		return nil, &siteError{Code: "invalid_argument", Message: command + " 只支持 courses、categories、course、notices、notice、catalog"}
	}
}

func (a NativeSite) learningCourses(ctx context.Context, args []string, cookie string, platform learningPlatform) (map[string]any, *siteError) {
	page, pageErr := businessInt(args, "--page", 1)
	if pageErr != nil {
		return nil, pageErr
	}
	pageSize, pageSizeErr := businessInt(args, "--page-size", 10)
	if pageSizeErr != nil {
		return nil, pageSizeErr
	}
	kind, kindErr := learningKind(args)
	if kindErr != nil {
		return nil, kindErr
	}
	body := map[string]any{
		"customerNo": platform.customerNo,
		"pageIndex":  page,
		"pageSize":   pageSize,
		"typeId":     kind,
		"isYlx":      1,
	}
	filters := map[string]any{"kind": kind}
	if keyword, found, err := businessValue(args, "--keyword"); err != nil {
		return nil, err
	} else if found {
		if strings.TrimSpace(keyword) == "" {
			return nil, &siteError{Code: "invalid_argument", Message: "--keyword 不能为空"}
		}
		body["courseName"], filters["keyword"] = keyword, keyword
	}
	if year, found, err := businessValue(args, "--year"); err != nil {
		return nil, err
	} else if found {
		if strings.TrimSpace(year) == "" {
			return nil, &siteError{Code: "invalid_argument", Message: "--year 不能为空"}
		}
		body["trainYears"], filters["year"] = year, year
	}
	if category, found, err := businessValue(args, "--category-id"); err != nil {
		return nil, err
	} else if found {
		categoryID, parseErr := learningPositiveInt(category, "--category-id")
		if parseErr != nil {
			return nil, parseErr
		}
		body["typeId"], filters["category_id"] = categoryID, categoryID
		delete(filters, "kind")
	}
	for _, item := range []struct {
		flag, key string
	}{
		{"--level", "professionalLevel"},
		{"--plan-type", "planType"},
		{"--min-hours", "minClassHoursLimit"},
		{"--max-hours", "maxClassHoursLimit"},
	} {
		value, found, err := businessValue(args, item.flag)
		if err != nil {
			return nil, err
		}
		if !found {
			continue
		}
		number, numberErr := learningNonNegativeNumber(value, item.flag)
		if numberErr != nil {
			return nil, numberErr
		}
		body[item.key], filters[strings.TrimPrefix(item.flag, "--")] = number, number
	}
	if platform.siteService == "jxjy" {
		body["platformId"] = 1
	} else {
		body["courseName"] = ""
		body["professionalLevel"] = 0
		body["planType"] = 0
		body["minClassHoursLimit"] = 0
		body["maxClassHoursLimit"] = 15
	}
	data, requestErr := a.learningRequest(ctx, platform.courseBase+platform.courseListPath, "POST", nil, body, cookie, platform.website, true)
	if requestErr != nil {
		return nil, requestErr
	}
	payload, payloadErr := learningMap(data, "课程列表")
	if payloadErr != nil {
		return nil, payloadErr
	}
	rows := learningList(learningField(payload, "Item1", "List", "Data", "Rows"))
	items := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		items = append(items, learningCourse(row, false))
	}
	total := learningNumber(learningField(payload, "Item2", "TotalCount", "Total", "Count"))
	if total == nil {
		total = len(items)
	}
	result := learningResult(platform, "courses", "公开课程列表接口返回结构化课程数据")
	result["data"], result["total"], result["total_learning_hours"] = items, total, learningNumber(learningField(payload, "Item3", "TotalLearningHours"))
	result["page"], result["page_size"], result["filters"], result["raw"] = page, pageSize, filters, learningSafeRaw(payload, false)
	return result, nil
}

func (a NativeSite) learningCategories(ctx context.Context, args []string, cookie string, platform learningPlatform) (map[string]any, *siteError) {
	params := []pair{{"customerNo", platform.customerNo}}
	if platform.siteService == "jxjy" {
		params = append(params, pair{"platformId", "1"})
	}
	data, requestErr := a.learningRequest(ctx, platform.courseBase+platform.categoryPath, "GET", params, nil, cookie, platform.website, true)
	if requestErr != nil {
		return nil, requestErr
	}
	rows := learningList(data)
	if len(rows) == 0 {
		if payload, ok := data.(map[string]any); ok {
			rows = learningList(learningField(payload, "Data", "List", "Item1", "Rows"))
		}
	}
	items := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		items = append(items, map[string]any{
			"id":      learningIdentifier(learningField(row, "CategoryId", "categoryId", "Id", "id")),
			"name":    learningText(learningField(row, "CategoryName", "categoryName", "Name", "name")),
			"type_id": learningNumber(learningField(row, "TypeId", "typeId")),
			"raw":     learningSafeRaw(row, false),
		})
	}
	result := learningResult(platform, "categories", "公开课程分类接口返回结构化分类数据")
	result["data"], result["raw"] = items, learningSafeRaw(data, false)
	return result, nil
}

func (a NativeSite) learningCourseDetail(ctx context.Context, args []string, cookie string, platform learningPlatform) (map[string]any, *siteError) {
	code, found, valueErr := businessValue(args, "--code")
	if valueErr != nil {
		return nil, valueErr
	}
	if !found {
		code, found, valueErr = businessValue(args, "--id")
		if valueErr != nil {
			return nil, valueErr
		}
	}
	if !found || strings.TrimSpace(code) == "" {
		return nil, &siteError{Code: "invalid_argument", Message: "course detail 必须提供 --code"}
	}
	includeVideo := businessBool(args, "--include-video-url")
	path := "/GetResourceByCode/" + url.PathEscape(strings.TrimSpace(code))
	params := []pair{{"userId", "0"}, {"platformId", "1"}}
	data, requestErr := a.learningRequest(ctx, platform.detailBase+path, "GET", params, nil, cookie, platform.website, true)
	if requestErr != nil {
		return nil, requestErr
	}
	payload, payloadErr := learningMap(data, "课程详情")
	if payloadErr != nil {
		return nil, payloadErr
	}
	course := learningCourse(payload, includeVideo)
	if extend, ok := learningField(payload, "CourseExtend", "courseExtend").(map[string]any); ok {
		encoded := learningText(learningField(extend, "Description", "description"))
		if encoded != "" {
			course["description_encoded"] = encoded
			if decoded, err := url.QueryUnescape(encoded); err == nil {
				course["description"] = decoded
			}
		}
	}
	classes := make([]map[string]any, 0)
	for _, row := range learningList(learningField(payload, "ClassesList", "classesList", "Classes")) {
		classes = append(classes, learningClass(row, includeVideo))
	}
	course["classes"] = classes
	result := learningResult(platform, "course", "公开课程详情接口返回课程与课时结构")
	result["code"], result["course"], result["raw"] = strings.TrimSpace(code), course, learningSafeRaw(payload, includeVideo)
	return result, nil
}

func (a NativeSite) learningNotices(ctx context.Context, args []string, cookie string, platform learningPlatform) (map[string]any, *siteError) {
	page, pageErr := businessInt(args, "--page", 1)
	if pageErr != nil {
		return nil, pageErr
	}
	pageSize, pageSizeErr := businessInt(args, "--page-size", 10)
	if pageSizeErr != nil {
		return nil, pageSizeErr
	}
	params := []pair{{"customerNo", platform.customerNo}, {"pageIndex", strconv.Itoa(page)}, {"pageSize", strconv.Itoa(pageSize)}, {"platformId", "1"}}
	data, requestErr := a.learningRequest(ctx, platform.noticeBase+"/api/Notice/GetNoticeListPage", "GET", params, nil, cookie, platform.website, false)
	if requestErr != nil {
		return nil, requestErr
	}
	payload, payloadErr := learningMap(data, "通知公告")
	if payloadErr != nil {
		return nil, payloadErr
	}
	rows := learningList(learningField(payload, "Data", "List", "Rows", "Item1"))
	items := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		items = append(items, learningNotice(row))
	}
	result := learningResult(platform, "notices", "公开通知接口返回结构化公告数据")
	result["data"], result["page"], result["page_size"], result["raw"] = items, page, pageSize, learningSafeRaw(payload, false)
	return result, nil
}

func (a NativeSite) learningNoticeDetail(ctx context.Context, args []string, cookie string, platform learningPlatform) (map[string]any, *siteError) {
	id, requiredErr := businessRequired(args, "--id", "notice detail 必须提供 --id")
	if requiredErr != nil {
		return nil, requiredErr
	}
	data, requestErr := a.learningRequest(ctx, platform.noticeBase+"/api/Notice/GetNoticeInfo", "GET", []pair{{"id", strings.TrimSpace(id)}, {"platformId", "1"}}, nil, cookie, platform.website, false)
	if requestErr != nil {
		return nil, requestErr
	}
	payload, payloadErr := learningMap(data, "通知详情")
	if payloadErr != nil {
		return nil, payloadErr
	}
	notice := payload
	if nested, ok := learningField(payload, "Data", "data").(map[string]any); ok {
		notice = nested
	}
	result := learningResult(platform, "notice", "公开通知详情接口返回公告正文")
	result["id"], result["notice"], result["raw"] = strings.TrimSpace(id), learningNotice(notice), learningSafeRaw(payload, false)
	return result, nil
}

func (a NativeSite) learningRequest(ctx context.Context, rawTarget, method string, params []pair, body any, cookie, website string, basic bool) (any, *siteError) {
	target, targetErr := learningTarget(rawTarget)
	if targetErr != nil {
		return nil, targetErr
	}
	headers := []pair{{"Referer", "https://" + website + "/"}}
	if basic {
		headers = append(headers, pair{"Authorization", learningBasicAuthorization})
	}
	request := siteRequest{
		Target: target, Method: method, Params: params, CookieFile: cookie, RawJSON: true,
		Headers:  headers,
		ReadOnly: true, Yes: true,
	}
	if strings.EqualFold(method, "POST") {
		request.JSON, request.HasJSON = body, true
	}
	result, requestErr := a.execute(ctx, request)
	if requestErr != nil {
		return nil, requestErr
	}
	data, ok := businessData(result)
	if !ok {
		return nil, &siteError{Code: "parse_error", Message: "继续教育接口响应不是 JSON"}
	}
	if payload, ok := data.(map[string]any); ok {
		if rejection := learningRejection(payload); rejection != nil {
			return nil, rejection
		}
	}
	return data, nil
}

func learningTarget(raw string) (*url.URL, *siteError) {
	target, err := url.Parse(raw)
	if err != nil || target.Scheme == "" || target.Host == "" || target.User != nil {
		return nil, &siteError{Code: "invalid_argument", Message: "继续教育接口地址无效"}
	}
	if base := os.Getenv("CSUST_BASE_URL"); base != "" {
		override, parseErr := url.Parse(base)
		if parseErr != nil || override.Scheme == "" || override.Host == "" || override.User != nil || override.RawQuery != "" || override.Fragment != "" || (override.Scheme != "http" && override.Scheme != "https") {
			return nil, &siteError{Code: "invalid_path", Message: "CSUST_BASE_URL 地址无效"}
		}
		target.Scheme, target.Host = override.Scheme, override.Host
	}
	return target, nil
}

func learningKind(args []string) (int, *siteError) {
	value, found, err := businessValue(args, "--kind")
	if err != nil {
		return 0, err
	}
	if !found {
		return 1, nil
	}
	if strings.TrimSpace(value) == "" {
		return 0, &siteError{Code: "invalid_argument", Message: "--kind 不能为空"}
	}
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "professional", "professional-technical", "专业", "专业技术", "1":
		return 1, nil
	case "public", "public-demand", "公需", "公需课", "2":
		return 2, nil
	default:
		return learningPositiveInt(value, "--kind")
	}
}

func learningPositiveInt(value, flag string) (int, *siteError) {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || parsed < 1 {
		return 0, &siteError{Code: "invalid_argument", Message: flag + " 必须是正整数或受支持的语义值"}
	}
	return parsed, nil
}

func learningNonNegativeNumber(value, flag string) (any, *siteError) {
	parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil || parsed < 0 || math.IsInf(parsed, 0) || math.IsNaN(parsed) {
		return nil, &siteError{Code: "invalid_argument", Message: flag + " 必须是非负数字"}
	}
	if parsed == math.Trunc(parsed) {
		return int(parsed), nil
	}
	return parsed, nil
}

func learningMap(value any, label string) (map[string]any, *siteError) {
	payload, ok := value.(map[string]any)
	if !ok {
		return nil, &siteError{Code: "parse_error", Message: label + "响应结构无效"}
	}
	return payload, nil
}

func learningList(value any) []map[string]any {
	rows := []map[string]any{}
	switch typed := value.(type) {
	case []any:
		for _, item := range typed {
			if row, ok := item.(map[string]any); ok {
				rows = append(rows, row)
			}
		}
	case []map[string]any:
		rows = append(rows, typed...)
	}
	return rows
}

func learningField(row map[string]any, keys ...string) any {
	for _, key := range keys {
		if value, ok := row[key]; ok {
			return value
		}
	}
	for key, value := range row {
		for _, wanted := range keys {
			if strings.EqualFold(key, wanted) {
				return value
			}
		}
	}
	return nil
}

func learningText(value any) string {
	if value == nil {
		return ""
	}
	if text, ok := value.(string); ok {
		return text
	}
	return fmt.Sprint(value)
}

func learningIdentifier(value any) string {
	return strings.TrimSpace(learningText(value))
}

func learningNumber(value any) any {
	if value == nil {
		return nil
	}
	text := strings.TrimSpace(learningText(value))
	if text == "" {
		return nil
	}
	parsed, err := strconv.ParseFloat(text, 64)
	if err != nil || math.IsInf(parsed, 0) || math.IsNaN(parsed) {
		return value
	}
	if parsed == math.Trunc(parsed) {
		return int(parsed)
	}
	return parsed
}

func learningCourse(row map[string]any, includeVideo bool) map[string]any {
	return map[string]any{
		"id":             learningIdentifier(learningField(row, "CourseId", "Id", "courseId", "id")),
		"source_id":      learningIdentifier(learningField(row, "YlxCourseId", "YlxCourseID", "sourceId")),
		"code":           learningText(learningField(row, "CourseCode", "courseCode", "code")),
		"name":           learningText(learningField(row, "CourseName", "courseName", "name")),
		"teacher":        learningText(learningField(row, "ExpertName", "expertName", "Teacher", "teacher")),
		"price":          learningNumber(learningField(row, "Price", "price")),
		"classes":        learningNumber(learningField(row, "ClassesNum", "classesNum")),
		"learning_hours": learningNumber(learningField(row, "LearningHours", "learningHours")),
		"class_hours":    learningNumber(learningField(row, "ClassHours", "classHours")),
		"type_id":        learningNumber(learningField(row, "TypeId", "typeId", "CourseType", "courseType")),
		"image":          learningText(learningField(row, "Image", "ImageUrl", "image", "imageUrl")),
		"raw":            learningSafeRaw(row, includeVideo),
	}
}

func learningClass(row map[string]any, includeVideo bool) map[string]any {
	result := map[string]any{
		"id":       learningIdentifier(learningField(row, "ClassId", "classId", "Id", "id")),
		"name":     learningText(learningField(row, "ClassName", "className", "name")),
		"number":   learningNumber(learningField(row, "Numbers", "numbers", "Number", "number")),
		"teacher":  learningText(learningField(row, "Author", "author", "ExpertName", "teacher")),
		"duration": learningNumber(learningField(row, "Timelength", "timeLength", "duration")),
		"size":     learningNumber(learningField(row, "Size", "size")),
		"state":    learningNumber(learningField(row, "State", "state")),
		"raw":      learningSafeRaw(row, includeVideo),
	}
	if includeVideo {
		result["video_url"] = learningText(learningField(row, "VideoPath", "videoPath", "VideoUrl", "video_url"))
	}
	return result
}

func learningNotice(row map[string]any) map[string]any {
	return map[string]any{
		"id":           learningIdentifier(learningField(row, "i_id", "id", "Id")),
		"title":        learningText(learningField(row, "s_title", "title", "Title")),
		"created_at":   learningText(learningField(row, "s_createTime", "s_create_time", "createTime", "created_at")),
		"content_html": learningText(learningField(row, "s_content", "content", "content_html")),
		"raw":          learningSafeRaw(row, false),
	}
}

func learningSafeRaw(value any, includeVideo bool) any {
	switch typed := value.(type) {
	case map[string]any:
		result := make(map[string]any, len(typed))
		for key, item := range typed {
			if !includeVideo && (strings.EqualFold(key, "VideoPath") || strings.EqualFold(key, "VideoUrl") || strings.EqualFold(key, "video_url")) {
				continue
			}
			result[key] = learningSafeRaw(item, includeVideo)
		}
		return result
	case []any:
		result := make([]any, len(typed))
		for index, item := range typed {
			result[index] = learningSafeRaw(item, includeVideo)
		}
		return result
	default:
		return value
	}
}

func learningRejection(payload map[string]any) *siteError {
	for _, key := range []string{"StatusCode", "statusCode", "Code", "code"} {
		value := strings.ToLower(strings.TrimSpace(learningText(learningField(payload, key))))
		if value == "" || value == "0" || value == "200" || value == "ok" || value == "success" || value == "succeed" {
			continue
		}
		return &siteError{Code: "business_rejected", Message: "继续教育接口拒绝请求: " + learningText(learningField(payload, "Msg", "Info", "Detail", "message", "msg")), Details: map[string]any{"submitted": false, "confirmed": false, "evidence": "rejected", "code": learningField(payload, key)}}
	}
	return nil
}

func learningResult(platform learningPlatform, operation, evidence string) map[string]any {
	return map[string]any{
		"ok": true, "submitted": false, "confirmed": true, "evidence": evidence,
		"service": platform.command, "operation": operation, "site": platform.siteService,
		"api": map[string]any{
			"course_list": platform.courseBase + platform.courseListPath,
			"category":    platform.courseBase + platform.categoryPath,
			"detail":      platform.detailBase + "/GetResourceByCode/{course_code}",
			"notice":      platform.noticeBase + "/api/Notice/GetNoticeListPage",
			"response":    "JSON",
		},
	}
}
