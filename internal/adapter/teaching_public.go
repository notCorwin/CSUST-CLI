package adapter

import (
	"context"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

var teachingPublicPagePattern = regexp.MustCompile(`共\s*(\d+)\s*页`)
var teachingPublicCoursePattern = regexp.MustCompile(`(?s)课程编号\s*[:：]\s*(.*?)\s*主讲教师\s*[:：]\s*(.*)`)
var teachingPublicNoticeCountPattern = regexp.MustCompile(`查询到\s*(\d+)\s*条记录`)
var teachingPublicNoticeMetaPattern = regexp.MustCompile(`发布人\s*[:：]\s*(.*?)\s+发布时间\s*[:：]\s*(.*)`)

func (a NativeSite) teachingPublicNotices(ctx context.Context, args []string) (map[string]any, *siteError) {
	if err := teachingPublicValidateArgs(args, map[string]bool{"--keyword": true, "--match": true, "--department-id": true, "--page": true}); err != nil {
		return nil, err
	}
	keyword, _, valueErr := businessValue(args, "--keyword")
	if valueErr != nil {
		return nil, valueErr
	}
	match, _, valueErr := businessValue(args, "--match")
	if valueErr != nil {
		return nil, valueErr
	}
	departmentID, _, valueErr := businessValue(args, "--department-id")
	if valueErr != nil {
		return nil, valueErr
	}
	page, pageErr := businessInt(args, "--page", 1)
	if pageErr != nil {
		return nil, pageErr
	}
	matchValue, matchErr := teachingPublicNoticeMatch(match)
	if matchErr != nil {
		return nil, matchErr
	}
	params := []pair{{"deptId", firstNonEmpty(strings.TrimSpace(departmentID), "0")}}
	if strings.TrimSpace(keyword) != "" {
		params = append(params, pair{"s_keyword", strings.TrimSpace(keyword)}, pair{"s_keywordrealation", matchValue})
	}
	if page > 1 {
		params = append(params, pair{"s_gotopage", strconv.Itoa(page)})
	}
	result, requestErr := (NativeSite{}).businessGet(ctx, "theol", "/meol/homepage/common/inform_all.jsp", params, businessRequestOptions{})
	if requestErr != nil {
		return nil, requestErr
	}
	document, parseErr := parsePage(businessBody(result))
	if parseErr != nil {
		return nil, &siteError{Code: "parse_error", Message: "网络教学平台通知页面解析失败: " + parseErr.Error()}
	}
	items := teachingPublicNoticesFromPage(document, safeResponseURL(result))
	return map[string]any{
		"ok": true, "submitted": false, "confirmed": true,
		"evidence": "THEOL inform_all.jsp 公开通知列表及分页", "service": teachingServiceName,
		"operation": "public-notices", "keyword": strings.TrimSpace(keyword), "match": firstNonEmpty(strings.TrimSpace(match), "fuzzy"),
		"department_id": strings.TrimSpace(departmentID), "page": page, "page_size": len(items),
		"total": teachingPublicNoticeTotal(document), "data": items,
	}, nil
}

func teachingPublicNoticeMatch(value string) (string, *siteError) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "fuzzy", "模糊":
		return "0", nil
	case "exact", "精确":
		return "1", nil
	default:
		return "", &siteError{Code: "invalid_argument", Message: "--match 只能是 fuzzy、exact、模糊或精确"}
	}
}

func (a NativeSite) teachingPublicNotice(ctx context.Context, args []string) (map[string]any, *siteError) {
	if err := teachingPublicValidateArgs(args, map[string]bool{"--id": true}); err != nil {
		return nil, err
	}
	id, found, valueErr := businessValue(args, "--id")
	if valueErr != nil {
		return nil, valueErr
	}
	if !found || strings.TrimSpace(id) == "" {
		return nil, &siteError{Code: "invalid_argument", Message: "public-notice 必须提供 --id"}
	}
	id = strings.TrimSpace(id)
	result, requestErr := (NativeSite{}).businessGet(ctx, "theol", "/meol/common/inform/message_content.jsp", []pair{{"nid", id}}, businessRequestOptions{})
	if requestErr != nil {
		return nil, requestErr
	}
	document, parseErr := parsePage(businessBody(result))
	if parseErr != nil {
		return nil, &siteError{Code: "parse_error", Message: "网络教学平台通知详情页面解析失败: " + parseErr.Error()}
	}
	article := libraryRemoteNodeWithClass(document, "article")
	if article == nil {
		return nil, &siteError{Code: "parse_error", Message: "网络教学平台通知详情缺少正文"}
	}
	title := strings.TrimSpace(pageDisplayText(libraryRemoteNodeWithClass(article, "atitle")))
	metadata := strings.TrimSpace(pageDisplayText(libraryRemoteNodeWithClass(article, "adate")))
	publisher, publishedAt := "", ""
	if match := teachingPublicNoticeMetaPattern.FindStringSubmatch(metadata); len(match) > 2 {
		publisher, publishedAt = strings.TrimSpace(match[1]), strings.TrimSpace(match[2])
	}
	body := libraryRemoteNodeWithClass(article, "abody")
	contentHTML := teachingPublicNoticeContent(body)
	links := make([]map[string]any, 0)
	if body != nil {
		for _, link := range body.findAll("a") {
			if href := strings.TrimSpace(link.attr("href")); href != "" {
				links = append(links, pageLink(link, safeResponseURL(result)))
			}
		}
	}
	return map[string]any{
		"ok": true, "submitted": false, "confirmed": true,
		"evidence": "THEOL message_content.jsp 公开通知详情页面", "service": teachingServiceName,
		"operation": "public-notice", "id": id, "title": title, "publisher": publisher,
		"published_at": publishedAt, "content_html": contentHTML, "links": links,
		"url": safeSiteURL(mustParseURL(safeResponseURL(result))),
	}, nil
}

func teachingPublicNoticeContent(body *pageNode) string {
	if body == nil {
		return ""
	}
	for _, input := range body.findAll("input") {
		if strings.HasSuffix(strings.TrimSpace(input.attr("name")), "_content") {
			return strings.TrimSpace(input.attr("value"))
		}
	}
	return strings.TrimSpace(pageDisplayText(body))
}

func teachingPublicNoticesFromPage(document *pageNode, pageURL string) []map[string]any {
	table := libraryRemoteNodeWithClass(document, "datatable")
	if table == nil {
		return []map[string]any{}
	}
	items := make([]map[string]any, 0)
	for _, row := range table.findAll("tr") {
		cells := row.findAll("td")
		if len(cells) < 2 {
			continue
		}
		var link *pageNode
		for _, candidate := range cells[0].findAll("a") {
			if strings.Contains(candidate.attr("href"), "message_content.jsp") {
				link = candidate
				break
			}
		}
		if link == nil {
			continue
		}
		parsed, parseErr := url.Parse(resolvePageURL(pageURL, link.attr("href")))
		if parseErr != nil || parsed.Query().Get("nid") == "" {
			continue
		}
		items = append(items, map[string]any{
			"id": parsed.Query().Get("nid"), "title": strings.TrimSpace(pageDisplayText(link)),
			"published_at": strings.TrimSpace(pageDisplayText(cells[1])), "url": safeSiteURL(parsed),
		})
	}
	return items
}

func teachingPublicNoticeTotal(document *pageNode) int {
	match := teachingPublicNoticeCountPattern.FindStringSubmatch(pageDisplayText(document))
	if len(match) < 2 {
		return 0
	}
	total, _ := strconv.Atoi(match[1])
	return total
}

func (a NativeSite) teachingPublicCourses(ctx context.Context, args []string) (map[string]any, *siteError) {
	if err := teachingPublicValidateArgs(args, map[string]bool{"--keyword": true, "--department-id": true, "--page": true}); err != nil {
		return nil, err
	}
	keyword, _, valueErr := businessValue(args, "--keyword")
	if valueErr != nil {
		return nil, valueErr
	}
	departmentID, _, valueErr := businessValue(args, "--department-id")
	if valueErr != nil {
		return nil, valueErr
	}
	page, pageErr := businessInt(args, "--page", 1)
	if pageErr != nil {
		return nil, pageErr
	}

	result, requestErr := a.teachingPublicCoursePage(ctx, keyword, departmentID, page)
	if requestErr != nil {
		return nil, requestErr
	}
	body := businessBody(result)
	document, parseErr := parsePage(body)
	if parseErr != nil {
		return nil, &siteError{Code: "parse_error", Message: "网络教学平台公开课程页面解析失败: " + parseErr.Error()}
	}
	items := teachingPublicCoursesFromPage(document, safeResponseURL(result))
	return map[string]any{
		"ok": true, "submitted": false, "confirmed": true,
		"evidence": "THEOL course.do 公开课程页面及分页", "service": teachingServiceName,
		"operation": "public-courses", "keyword": strings.TrimSpace(keyword),
		"department_id": strings.TrimSpace(departmentID), "page": page, "page_size": len(items),
		"total_pages": teachingPublicPageCount(document), "data": items,
	}, nil
}

func (a NativeSite) teachingPublicCoursePage(ctx context.Context, keyword, departmentID string, page int) (map[string]any, *siteError) {
	departmentID = strings.TrimSpace(departmentID)
	keyword = strings.TrimSpace(keyword)
	if keyword == "" {
		params := []pair{{"deptId", firstNonEmpty(departmentID, "0")}}
		if page > 1 {
			params = append(params, pair{"s_gotopage", strconv.Itoa(page)})
		}
		return (NativeSite{}).businessGet(ctx, "theol", "/meol/course.do", params, businessRequestOptions{})
	}

	initial, requestErr := (NativeSite{}).businessGet(ctx, "theol", "/meol/course.do", []pair{{"deptId", firstNonEmpty(departmentID, "0")}}, businessRequestOptions{})
	if requestErr != nil {
		return nil, requestErr
	}
	fields, fieldsErr := teachingPublicSearchFields(businessBody(initial), "course.do")
	if fieldsErr != nil {
		return nil, fieldsErr
	}
	fields = setFormField(fields, "deptId", firstNonEmpty(departmentID, "0"))
	fields = setFormField(fields, "s_keywordrealation", "0")
	fields = setFormField(fields, "s_keyword", keyword)
	result, requestErr := businessRequest(ctx, "theol", "POST", "/meol/course.do", nil, fields, []pair{{"Referer", safeResponseURL(initial)}}, businessRequestOptions{allowBusinessFailure: true}, true, true)
	if requestErr != nil {
		return nil, requestErr
	}
	if page == 1 {
		return result, nil
	}
	return (NativeSite{}).businessGet(ctx, "theol", "/meol/course.do", []pair{{"s_gotopage", strconv.Itoa(page)}, {"deptId", firstNonEmpty(departmentID, "0")}}, businessRequestOptions{})
}

func (a NativeSite) teachingPublicTeachers(ctx context.Context, args []string) (map[string]any, *siteError) {
	if err := teachingPublicValidateArgs(args, map[string]bool{"--keyword": true, "--department-id": true, "--page": true}); err != nil {
		return nil, err
	}
	keyword, _, valueErr := businessValue(args, "--keyword")
	if valueErr != nil {
		return nil, valueErr
	}
	departmentID, _, valueErr := businessValue(args, "--department-id")
	if valueErr != nil {
		return nil, valueErr
	}
	page, pageErr := businessInt(args, "--page", 1)
	if pageErr != nil {
		return nil, pageErr
	}

	result, requestErr := a.teachingPublicTeacherPage(ctx, keyword, departmentID, page)
	if requestErr != nil {
		return nil, requestErr
	}
	document, parseErr := parsePage(businessBody(result))
	if parseErr != nil {
		return nil, &siteError{Code: "parse_error", Message: "网络教学平台公开教师页面解析失败: " + parseErr.Error()}
	}
	items := teachingPublicTeachersFromPage(document, safeResponseURL(result))
	return map[string]any{
		"ok": true, "submitted": false, "confirmed": true,
		"evidence": "THEOL teacher.do 公开教师页面及分页", "service": teachingServiceName,
		"operation": "public-teachers", "keyword": strings.TrimSpace(keyword),
		"department_id": strings.TrimSpace(departmentID), "page": page, "page_size": len(items),
		"total_pages": teachingPublicPageCount(document), "data": items,
	}, nil
}

func (a NativeSite) teachingPublicTeacherPage(ctx context.Context, keyword, departmentID string, page int) (map[string]any, *siteError) {
	departmentID = strings.TrimSpace(departmentID)
	keyword = strings.TrimSpace(keyword)
	if keyword == "" {
		params := []pair{{"deptId", firstNonEmpty(departmentID, "0")}}
		if page > 1 {
			params = append(params, pair{"s_gotopage", strconv.Itoa(page)})
		}
		return (NativeSite{}).businessGet(ctx, "theol", "/meol/teacher.do", params, businessRequestOptions{})
	}

	initial, requestErr := (NativeSite{}).businessGet(ctx, "theol", "/meol/teacher.do", []pair{{"deptId", firstNonEmpty(departmentID, "0")}}, businessRequestOptions{})
	if requestErr != nil {
		return nil, requestErr
	}
	fields, fieldsErr := teachingPublicSearchFields(businessBody(initial), "teacher.do")
	if fieldsErr != nil {
		return nil, fieldsErr
	}
	fields = setFormField(fields, "deptId", firstNonEmpty(departmentID, "0"))
	fields = setFormField(fields, "s_keywordrealation", "0")
	fields = setFormField(fields, "s_keyword", keyword)
	result, requestErr := businessRequest(ctx, "theol", "POST", "/meol/teacher.do", nil, fields, []pair{{"Referer", safeResponseURL(initial)}}, businessRequestOptions{allowBusinessFailure: true}, true, true)
	if requestErr != nil {
		return nil, requestErr
	}
	if page == 1 {
		return result, nil
	}
	return (NativeSite{}).businessGet(ctx, "theol", "/meol/teacher.do", []pair{{"s_gotopage", strconv.Itoa(page)}, {"deptId", firstNonEmpty(departmentID, "0")}}, businessRequestOptions{})
}

func (a NativeSite) teachingPublicTeacher(ctx context.Context, args []string) (map[string]any, *siteError) {
	if err := teachingPublicValidateArgs(args, map[string]bool{"--id": true}); err != nil {
		return nil, err
	}
	id, found, valueErr := businessValue(args, "--id")
	if valueErr != nil {
		return nil, valueErr
	}
	if !found || strings.TrimSpace(id) == "" {
		return nil, &siteError{Code: "invalid_argument", Message: "public-teacher 必须提供 --id"}
	}
	result, requestErr := (NativeSite{}).businessGet(ctx, "theol", "/meol/teacherLesson.do", []pair{{"uid", strings.TrimSpace(id)}}, businessRequestOptions{})
	if requestErr != nil {
		return nil, requestErr
	}
	document, parseErr := parsePage(businessBody(result))
	if parseErr != nil {
		return nil, &siteError{Code: "parse_error", Message: "网络教学平台教师详情页面解析失败: " + parseErr.Error()}
	}
	items := teachingPublicCoursesFromTeacherPage(document, safeResponseURL(result))
	return map[string]any{
		"ok": true, "submitted": false, "confirmed": true,
		"evidence": "THEOL teacherLesson.do 公开教师详情页面", "service": teachingServiceName,
		"operation": "public-teacher", "id": strings.TrimSpace(id), "data": items,
	}, nil
}

func (a NativeSite) teachingPublicDepartments(ctx context.Context, args []string) (map[string]any, *siteError) {
	if err := teachingPublicValidateArgs(args, nil); err != nil {
		return nil, err
	}
	result, requestErr := (NativeSite{}).businessGet(ctx, "theol", "/meol/course.do", nil, businessRequestOptions{})
	if requestErr != nil {
		return nil, requestErr
	}
	document, parseErr := parsePage(businessBody(result))
	if parseErr != nil {
		return nil, &siteError{Code: "parse_error", Message: "网络教学平台院系页面解析失败: " + parseErr.Error()}
	}
	items := teachingPublicDepartmentsFromPage(document, safeResponseURL(result))
	return map[string]any{
		"ok": true, "submitted": false, "confirmed": true,
		"evidence": "THEOL course.do 公开院系筛选项", "service": teachingServiceName,
		"operation": "public-departments", "data": items,
	}, nil
}

func teachingPublicValidateArgs(args []string, allowed map[string]bool) *siteError {
	for index := 0; index < len(args); index++ {
		arg, _, inline := splitInline(args[index])
		if arg == "--json" {
			continue
		}
		if !strings.HasPrefix(arg, "--") || allowed == nil || !allowed[arg] {
			return &siteError{Code: "invalid_argument", Message: "teaching 公开查询参数无效: " + args[index]}
		}
		if !inline {
			if index+1 >= len(args) || strings.HasPrefix(args[index+1], "--") {
				return &siteError{Code: "invalid_argument", Message: args[index] + " 缺少参数值"}
			}
			index++
		}
	}
	return nil
}

func teachingPublicSearchFields(body, path string) ([]pair, *siteError) {
	document, parseErr := parsePage(body)
	if parseErr != nil {
		return nil, &siteError{Code: "parse_error", Message: "网络教学平台查询表单解析失败: " + parseErr.Error()}
	}
	for _, form := range document.findAll("form") {
		action := form.attr("action")
		if !strings.Contains(action, path) {
			continue
		}
		for _, input := range form.findAll("input") {
			if input.attr("name") == "searchtoken" {
				return pageFormFields(form, nil, document), nil
			}
		}
	}
	return nil, &siteError{Code: "parse_error", Message: "网络教学平台查询页面缺少搜索表单"}
}

func teachingPublicCoursesFromPage(document *pageNode, pageURL string) []map[string]any {
	container := libraryRemoteNodeWithClass(document, "blockPicText")
	if container == nil {
		return []map[string]any{}
	}
	items := make([]map[string]any, 0)
	for _, row := range container.findAll("li") {
		link := libraryRemoteNodeWithClass(row, "wrapa")
		if link == nil {
			continue
		}
		fullURL := resolvePageURL(pageURL, link.attr("href"))
		parsed, parseErr := url.Parse(fullURL)
		if parseErr != nil || parsed.Query().Get("courseId") == "" {
			continue
		}
		info := pageDisplayText(libraryRemoteNodeWithClass(row, "content"))
		code, teacher := "", ""
		if match := teachingPublicCoursePattern.FindStringSubmatch(info); len(match) > 2 {
			code, teacher = strings.TrimSpace(match[1]), strings.TrimSpace(match[2])
		}
		items = append(items, map[string]any{
			"id": parsed.Query().Get("courseId"), "title": strings.TrimSpace(pageDisplayText(libraryRemoteNodeWithClass(row, "name"))),
			"course_code": code, "teacher": teacher, "url": safeSiteURL(parsed),
		})
	}
	return items
}

func teachingPublicTeachersFromPage(document *pageNode, pageURL string) []map[string]any {
	container := libraryRemoteNodeWithClass(document, "teanamewrap")
	if container == nil {
		return []map[string]any{}
	}
	items := make([]map[string]any, 0)
	for _, row := range container.findAll("li") {
		var anchor *pageNode
		for _, candidate := range row.findAll("a") {
			if strings.Contains(candidate.attr("href"), "teacherLesson.do") {
				anchor = candidate
				break
			}
		}
		if anchor == nil {
			continue
		}
		parsed, parseErr := url.Parse(resolvePageURL(pageURL, anchor.attr("href")))
		if parseErr != nil || parsed.Query().Get("uid") == "" {
			continue
		}
		info := libraryRemoteNodeWithClass(row, "teainfo")
		department := ""
		if info != nil {
			if node := info.first("p", ""); node != nil {
				department = strings.TrimSpace(pageDisplayText(node))
			}
		}
		name := strings.TrimSpace(pageDisplayText(info))
		if department != "" {
			name = strings.TrimSpace(strings.TrimSuffix(name, department))
		}
		photo := ""
		if image := row.first("img", ""); image != nil {
			photo = safeSiteURL(mustParseURL(resolvePageURL(pageURL, image.attr("src"))))
		}
		items = append(items, map[string]any{"id": parsed.Query().Get("uid"), "name": name, "department": department, "photo": photo, "url": safeSiteURL(parsed)})
	}
	return items
}

func teachingPublicCoursesFromTeacherPage(document *pageNode, pageURL string) []map[string]any {
	items := make([]map[string]any, 0)
	for _, anchor := range document.findAll("a") {
		if !strings.Contains(anchor.attr("href"), "course_index.jsp") {
			continue
		}
		parsed, parseErr := url.Parse(resolvePageURL(pageURL, anchor.attr("href")))
		if parseErr != nil || parsed.Query().Get("courseId") == "" {
			continue
		}
		text := pageDisplayText(anchor)
		code, teacher := "", ""
		if match := teachingPublicCoursePattern.FindStringSubmatch(text); len(match) > 2 {
			code, teacher = strings.TrimSpace(match[1]), strings.TrimSpace(match[2])
		}
		title := strings.TrimSpace(text)
		if match := teachingPublicCoursePattern.FindStringSubmatch(text); len(match) > 2 {
			title = strings.TrimSpace(strings.TrimSuffix(text, match[0]))
		}
		items = append(items, map[string]any{"id": parsed.Query().Get("courseId"), "title": title, "course_code": code, "teacher": teacher, "url": safeSiteURL(parsed)})
	}
	return items
}

func teachingPublicDepartmentsFromPage(document *pageNode, pageURL string) []map[string]any {
	container := libraryRemoteNodeWithClass(document, "all-course-ul")
	if container == nil {
		return []map[string]any{}
	}
	items := make([]map[string]any, 0)
	for _, anchor := range container.findAll("a") {
		parsed, parseErr := url.Parse(resolvePageURL(pageURL, anchor.attr("href")))
		if parseErr != nil || parsed.Query().Get("deptId") == "" {
			continue
		}
		items = append(items, map[string]any{"id": parsed.Query().Get("deptId"), "name": strings.TrimSpace(pageDisplayText(anchor))})
	}
	return items
}

func teachingPublicPageCount(document *pageNode) int {
	navigation := libraryRemoteNodeWithClass(document, "navigation")
	if navigation == nil {
		return 0
	}
	match := teachingPublicPagePattern.FindStringSubmatch(pageDisplayText(navigation))
	if len(match) < 2 {
		return 0
	}
	pages, _ := strconv.Atoi(match[1])
	return pages
}
