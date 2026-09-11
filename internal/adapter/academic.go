package adapter

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var academicDayNames = []string{"星期一", "星期二", "星期三", "星期四", "星期五", "星期六", "星期日"}

func (a NativeSite) runAcademicCommand(ctx context.Context, args []string, jsonMode bool) (bool, []byte, []byte, int, error) {
	if len(args) == 0 || !academicCommand(args[0]) || containsHelp(args[1:]) {
		return false, nil, nil, 0, nil
	}
	result, err := a.executeAcademic(ctx, args)
	if err != nil {
		if jsonMode {
			return true, errorJSON(err), nil, 2, nil
		}
		return true, nil, []byte("错误: " + err.Error() + "\n"), 2, nil
	}
	if jsonMode {
		return true, mustJSON(result), nil, 0, nil
	}
	return true, []byte(renderAcademic(result)), nil, 0, nil
}

func academicCommand(value string) bool {
	switch value {
	case "schedule", "timetable", "grades", "scores", "profile", "personal", "personal-info", "account-settings", "graduation-conclusion", "graduation-status", "graduation-info-check", "graduate-info-check", "exams", "exam", "in-class-exams", "in-class-exam", "class-exams", "classrooms", "rooms", "selections", "selection", "course-results", "terms", "semesters", "semester-start", "teaching-calendar", "semester-calendar", "class-changes", "class-change-history", "course-selection", "course-select", "training-plan", "plan", "cultivation-plan", "training-progress", "training-plan-progress", "deferred-exam-applications", "deferred-exam-application", "deferred-exam-registration", "exempt-exam-applications", "exempt-exam-application", "graduate-exam-registration", "grade-recognition-applications", "grade-recognition-application", "grade-review-applications", "grade-confirmation", "grade-confirmation-status", "enrollment-proof-applications", "enrollment-proof-application", "enrollment-status-changes", "academic-status-changes", "status-change-history", "drop-course-applications", "drop-course-application", "student-status-changes", "student-status-management", "student-status-change-history", "second-class-credits", "innovation-credits", "second-class-credit-query", "second-class-credit-applications", "innovation-credit-applications", "second-class-credit-application", "innovation-credit-application", "second-class-credit-workflow", "status-warnings", "academic-warnings", "announcements", "notices", "received-announcements", "messages", "received-messages", "announcement", "notice", "announcement-detail", "message", "message-detail", "message-reply", "retake-courses", "retake-registration", "classroom-request", "room-request", "minor", "minor-registration", "evaluation", "evaluate":
		return true
	default:
		return false
	}
}

func (a NativeSite) executeAcademic(ctx context.Context, args []string) (map[string]any, *siteError) {
	switch args[0] {
	case "schedule", "timetable":
		return a.academicSchedule(ctx, args[1:])
	case "grades", "scores":
		return a.academicGrades(ctx, args[1:])
	case "profile", "personal":
		body, pageURL, err := a.academicPage(ctx, "GET", "/jsxsd/grxx/xsxx", nil, nil)
		if err != nil {
			return nil, err
		}
		return academicWrap(parseProfilePage(body, pageURL)), nil
	case "personal-info", "account-settings":
		return a.academicPersonalInfo(ctx, args[1:])
	case "graduation-conclusion", "graduation-status":
		body, pageURL, err := a.academicPage(ctx, "GET", "/jsxsd/bygl/bygl_ckxsList", nil, nil)
		if err != nil {
			return nil, err
		}
		return academicWrap(parseGraduationConclusionPage(body, pageURL)), nil
	case "graduation-info-check", "graduate-info-check":
		return a.academicGraduationInfoCheck(ctx)
	case "exams", "exam":
		return a.academicExams(ctx, args[1:])
	case "in-class-exams", "in-class-exam", "class-exams":
		return a.academicInClassExams(ctx, args[1:])
	case "classrooms", "rooms":
		return a.academicClassrooms(ctx, args[1:])
	case "selections", "selection", "course-results":
		return a.academicSelections(ctx, args[1:])
	case "terms", "semesters":
		return a.academicTerms(ctx, args[1:])
	case "semester-start":
		return a.academicSemesterStart(ctx, args[1:])
	case "teaching-calendar", "semester-calendar":
		return a.academicTeachingCalendar(ctx, args[1:])
	case "course-selection", "course-select":
		return a.academicCourseSelection(ctx, args[1:])
	case "training-plan", "plan", "cultivation-plan":
		return a.academicTrainingPlan(ctx, args[1:])
	case "training-progress", "training-plan-progress":
		return a.academicTrainingProgress(ctx, args[1:])
	case "deferred-exam-applications", "deferred-exam-application":
		return a.academicDeferredExamApplications(ctx, args[1:])
	case "exempt-exam-applications", "exempt-exam-application":
		return a.academicExemptExamApplications(ctx, args[1:])
	case "deferred-exam-registration":
		return a.academicDeferredExamRegistration(ctx, args[1:])
	case "graduate-exam-registration":
		return a.academicGraduateExamRegistration(ctx, args[1:])
	case "grade-recognition-applications", "grade-recognition-application", "grade-review-applications":
		return a.academicStructuredPageWithField(ctx, args[1:], "grade-recognition-applications", "/jsxsd/kscj/cjfh_list", academicGradeRecognitionField)
	case "grade-confirmation", "grade-confirmation-status":
		return a.academicGradeConfirmation(ctx)
	case "class-changes", "class-change-history":
		return a.academicClassChanges(ctx, args[1:])
	case "enrollment-proof-applications", "enrollment-proof-application":
		return a.academicStructuredPageWithField(ctx, args[1:], "enrollment-proof-applications", "/jsxsd/kscj/xjzdzmsq_query", academicEnrollmentProofField)
	case "enrollment-status-changes", "academic-status-changes", "status-change-history":
		return a.academicStructuredPageWithField(ctx, args[1:], "enrollment-status-changes", "/jsxsd/xsxj/xsydxx.do", academicEnrollmentStatusChangeField)
	case "drop-course-applications", "drop-course-application":
		return a.academicStructuredPageWithField(ctx, args[1:], "drop-course-applications", "/jsxsd/xkgl/xstk_list", academicDropCourseField)
	case "student-status-changes", "student-status-management", "student-status-change-history":
		return a.academicStructuredPageWithField(ctx, args[1:], "student-status-change-history", "/jsxsd/xsxj/xjxxgl.do", academicStudentStatusChangeField)
	case "second-class-credits", "innovation-credits", "second-class-credit-query":
		return a.academicStructuredPageWithField(ctx, args[1:], "second-class-credits", "/jsxsd/pyfa/cxxf_query", academicSecondClassCreditField)
	case "second-class-credit-applications", "innovation-credit-applications":
		return a.academicSecondClassCreditApplications(ctx, args[1:])
	case "second-class-credit-application", "innovation-credit-application", "second-class-credit-workflow":
		return a.academicSecondClassCreditApplication(ctx, args[1:])
	case "status-warnings", "academic-warnings":
		return a.academicStructuredPageWithField(ctx, args[1:], "status-warnings", "/jsxsd/xsxj/xsyjxx.do", academicStatusWarningField)
	case "announcements", "notices", "received-announcements":
		return a.academicAnnouncements(ctx, args[1:])
	case "announcement", "notice", "announcement-detail":
		return a.academicAnnouncement(ctx, args[1:])
	case "messages", "received-messages":
		return a.academicMessages(ctx, args[1:])
	case "message", "message-detail":
		if len(args) > 1 && args[1] == "reply" {
			return a.academicMessageReply(ctx, args[2:])
		}
		return a.academicMessage(ctx, args[1:])
	case "message-reply":
		return a.academicMessageReply(ctx, args[1:])
	case "retake-courses", "retake-registration":
		return a.academicRetakeCourses(ctx, args[1:])
	case "classroom-request", "room-request":
		return a.academicStructuredPage(ctx, args[1:], "classroom-request", "/jsxsd/kbxx/jsjy_query")
	case "minor", "minor-registration":
		return a.academicStructuredPage(ctx, args[1:], "minor-registration", "/jsxsd/fxgl/fxbmxx_query")
	case "evaluation", "evaluate":
		return a.runAcademicEvaluation(ctx, args[1:])
	default:
		return nil, &siteError{Code: "invalid_argument", Message: "未知教务命令"}
	}
}

const academicPersonalInfoPath = "/jsxsd/grsz/grsz_xggrxx.do"

func (a NativeSite) academicPersonalInfo(ctx context.Context, args []string) (map[string]any, *siteError) {
	operation := "get"
	if len(args) > 0 && !strings.HasPrefix(args[0], "--") {
		operation, args = strings.ToLower(args[0]), args[1:]
	}
	switch operation {
	case "get", "show":
		body, pageURL, err := a.academicPage(ctx, "GET", academicPersonalInfoPath, nil, nil)
		if err != nil {
			return nil, err
		}
		return academicPersonalInfoSnapshot(body, pageURL, "get")
	case "update", "save":
		return a.academicPersonalInfoUpdate(ctx, args)
	default:
		return nil, &siteError{Code: "invalid_argument", Message: "personal-info 只支持 get 或 update"}
	}
}

func academicPersonalInfoSnapshot(source, pageURL, operation string) (map[string]any, *siteError) {
	document, parseErr := parsePage(source)
	if parseErr != nil {
		return nil, &siteError{Code: "parse_error", Message: parseErr.Error()}
	}
	values := academicPersonalInfoValues(document)
	page, pageErr := pageInspect(source, pageURL)
	if pageErr != nil {
		return nil, pageErr
	}
	return academicWrap(map[string]any{
		"kind": "personal-info", "operation": operation, "path": academicPersonalInfoPath,
		"account": nullableString(values["account"]), "real_name": nullableString(values["realName"]),
		"online_classroom": academicPersonalInfoOnlineClassroom(values["sfzczxkt"]),
		"page_size":        nullableString(values["pageSize"]),
		"password_protection": map[string]any{
			"question_1_set": values["pwdQuestion1"] != "", "answer_1_set": values["pwdAnswer1"] != "",
			"question_2_set": values["pwdQuestion2"] != "", "answer_2_set": values["pwdAnswer2"] != "",
		},
		"page": page,
	}), nil
}

func academicPersonalInfoValues(document *pageNode) map[string]string {
	values := map[string]string{}
	for _, tag := range []string{"input", "select", "textarea"} {
		for _, node := range document.findAll(tag) {
			name := node.attr("name")
			switch name {
			case "account", "realName", "pwdQuestion1", "pwdAnswer1", "pwdQuestion2", "pwdAnswer2", "sfzczxkt", "pageSize":
				values[name] = pageElementValue(node)
			}
		}
	}
	return values
}

func academicPersonalInfoOnlineClassroom(value string) any {
	switch strings.TrimSpace(value) {
	case "1":
		return true
	case "0":
		return false
	default:
		return nil
	}
}

func academicPersonalInfoOnlineClassroomValue(value string) (string, *siteError) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "enabled", "enable", "on", "true", "yes", "是":
		return "1", nil
	case "0", "disabled", "disable", "off", "false", "no", "否":
		return "0", nil
	default:
		return "", &siteError{Code: "invalid_argument", Message: "--online-classroom 只能是 enabled 或 disabled"}
	}
}

func (a NativeSite) academicPersonalInfoUpdate(ctx context.Context, args []string) (map[string]any, *siteError) {
	allowed := map[string]bool{
		"--real-name": true, "--page-size": true, "--online-classroom": true,
		"--password-question-1": true, "--password-answer-1": true,
		"--password-question-2": true, "--password-answer-2": true,
	}
	for index := 0; index < len(args); index++ {
		arg := args[index]
		if !strings.HasPrefix(arg, "--") {
			return nil, &siteError{Code: "invalid_argument", Message: "personal-info update 不接受位置参数: " + arg}
		}
		name, _, inline := strings.Cut(arg, "=")
		if name != "--yes" && name != "--json" && !allowed[name] {
			return nil, &siteError{Code: "invalid_argument", Message: "personal-info update 不支持参数: " + name}
		}
		if name != "--yes" && name != "--json" && !inline {
			if index+1 >= len(args) || strings.HasPrefix(args[index+1], "--") {
				return nil, &siteError{Code: "invalid_argument", Message: name + " 缺少参数值"}
			}
			index++
		}
	}
	if !flagPresent(args, "--yes") {
		return nil, &siteError{Code: "confirmation_required", Message: "修改个人资料会改变账号设置，请加 --yes"}
	}
	changed := make([]string, 0, len(allowed))
	updates := map[string]string{}
	setUpdate := func(flag, field string) {
		if flagPresent(args, flag) {
			updates[field] = flagValue(args, flag)
			changed = append(changed, strings.TrimPrefix(flag, "--"))
		}
	}
	setUpdate("--real-name", "realName")
	setUpdate("--page-size", "pageSize")
	setUpdate("--password-question-1", "pwdQuestion1")
	setUpdate("--password-answer-1", "pwdAnswer1")
	setUpdate("--password-question-2", "pwdQuestion2")
	setUpdate("--password-answer-2", "pwdAnswer2")
	if flagPresent(args, "--online-classroom") {
		value, valueErr := academicPersonalInfoOnlineClassroomValue(flagValue(args, "--online-classroom"))
		if valueErr != nil {
			return nil, valueErr
		}
		updates["sfzczxkt"] = value
		changed = append(changed, "online-classroom")
	}
	if len(changed) == 0 {
		return nil, &siteError{Code: "invalid_argument", Message: "至少提供一个需要修改的个人资料参数"}
	}
	queryBody, queryURL, queryErr := a.academicPage(ctx, "GET", academicPersonalInfoPath, nil, nil)
	if queryErr != nil {
		return nil, queryErr
	}
	queryDocument, parseErr := parsePage(queryBody)
	if parseErr != nil {
		return nil, &siteError{Code: "parse_error", Message: parseErr.Error()}
	}
	current := academicPersonalInfoValues(queryDocument)
	for field, value := range current {
		if _, exists := updates[field]; !exists {
			updates[field] = value
		}
	}
	if updates["realName"] == "" || len([]rune(updates["realName"])) > 32 {
		return nil, &siteError{Code: "invalid_argument", Message: "--real-name 不能为空且不能超过 32 个字符"}
	}
	pageSize, sizeErr := strconv.Atoi(updates["pageSize"])
	if sizeErr != nil || pageSize < 1 || pageSize > 260 {
		return nil, &siteError{Code: "invalid_argument", Message: "--page-size 必须是 1 到 260 的整数"}
	}
	for _, field := range []string{"pwdQuestion1", "pwdAnswer1", "pwdQuestion2", "pwdAnswer2"} {
		if len([]rune(updates[field])) > 50 {
			return nil, &siteError{Code: "invalid_argument", Message: field + " 不能超过 50 个字符"}
		}
	}
	data := []pair{
		{"realName", updates["realName"]}, {"pwdQuestion1", updates["pwdQuestion1"]}, {"pwdAnswer1", updates["pwdAnswer1"]},
		{"pwdQuestion2", updates["pwdQuestion2"]}, {"pwdAnswer2", updates["pwdAnswer2"]}, {"sfzczxkt", updates["sfzczxkt"]},
		{"pageSize", updates["pageSize"]}, {"edit", "1"},
	}
	result, submitErr := a.executeAcademicRequestWithRecovery(ctx, siteRequest{
		Service: "academic", Path: academicPersonalInfoPath, Method: "POST", CookieFile: academicCookiePath(), Data: data,
		Headers: []pair{{"Referer", queryURL}}, RequireLogin: true, Yes: true,
	})
	if submitErr != nil {
		return nil, submitErr
	}
	responseBody, _, bodyErr := loginBody(result)
	if bodyErr != nil {
		return nil, bodyErr
	}
	success, known := pageFeedback(responseBody, "text/html")
	if known && !success {
		return nil, &siteError{Code: "mutation_rejected", Message: "个人资料保存被服务端拒绝", Details: map[string]any{"submitted": true, "confirmed": false, "changed_fields": changed}}
	}
	if success {
		return academicWrap(map[string]any{
			"kind": "personal-info-update", "operation": "update", "path": academicPersonalInfoPath,
			"changed_fields": changed, "submitted": true, "confirmed": true, "evidence": "response-success",
		}), nil
	}
	verifyBody, _, verifyErr := a.academicPage(ctx, "GET", academicPersonalInfoPath, nil, nil)
	if verifyErr != nil {
		return nil, verifyErr
	}
	verifyDocument, verifyParseErr := parsePage(verifyBody)
	if verifyParseErr != nil {
		return nil, &siteError{Code: "parse_error", Message: verifyParseErr.Error()}
	}
	verified := true
	for field, value := range updates {
		if currentValue, ok := verifyPersonalInfoValue(verifyDocument, field); ok && currentValue != value {
			verified = false
			break
		}
	}
	if !verified {
		return nil, &siteError{Code: "mutation_unverified", Message: "个人资料已提交但回读结果不匹配", Details: map[string]any{"submitted": true, "confirmed": false, "changed_fields": changed}}
	}
	return academicWrap(map[string]any{
		"kind": "personal-info-update", "operation": "update", "path": academicPersonalInfoPath,
		"changed_fields": changed, "submitted": true, "confirmed": true, "evidence": "readback",
	}), nil
}

func verifyPersonalInfoValue(document *pageNode, field string) (string, bool) {
	values := academicPersonalInfoValues(document)
	value, ok := values[field]
	return value, ok
}

func (a NativeSite) academicCourseSelection(ctx context.Context, args []string) (map[string]any, *siteError) {
	scope := firstNonEmpty(flagValue(args, "--scope"), "center")
	scope = strings.ToLower(strings.TrimSpace(scope))
	paths := map[string]string{
		"center":        "/jsxsd/xsxk/xklc_list",
		"cross-major":   "/jsxsd/xsxk/xklc_list",
		"special":       "/jsxsd/tsxk/tsxk_sqlist",
		"special-query": "/jsxsd/tsxk/tsxk_cxlist",
	}
	path, found := paths[strings.ToLower(scope)]
	if !found {
		return nil, &siteError{Code: "invalid_argument", Message: "--scope 只能是 center、cross-major、special 或 special-query"}
	}
	entryPath := ""
	body, pageURL, err := a.academicPage(ctx, "GET", path, nil, nil)
	if err != nil {
		return nil, err
	}
	if scope == "cross-major" {
		entryPath = path
		document, parseErr := parsePage(body)
		if parseErr != nil {
			return nil, &siteError{Code: "parse_error", Message: parseErr.Error()}
		}
		path = academicSelectionEntry(document, pageURL, "跨专业选修")
		if path == "" {
			page, pageErr := pageInspect(body, pageURL)
			if pageErr != nil {
				return nil, pageErr
			}
			return academicWrap(map[string]any{"scope": scope, "entry_path": entryPath, "path": nil, "keyword": nullableString(strings.TrimSpace(flagValue(args, "--keyword"))), "items": []map[string]any{}, "item_count": 0, "entry_found": false, "page": page}), nil
		}
		body, pageURL, err = a.academicPage(ctx, "GET", path, nil, nil)
		if err != nil {
			return nil, err
		}
	}
	document, parseErr := parsePage(body)
	if parseErr != nil {
		return nil, &siteError{Code: "parse_error", Message: parseErr.Error()}
	}
	keyword := strings.TrimSpace(flagValue(args, "--keyword"))
	items := academicCourseRows(document, keyword)
	if scope == "center" {
		items = academicSelectionWindowRows(document, keyword, pageURL)
	}
	if len(items) == 0 && !noAcademicData(document) && len(document.findAll("table")) == 0 {
		return nil, &siteError{Code: "parse_error", Message: "未找到选课表；请使用 web get 查看页面结构"}
	}
	page, pageErr := pageInspect(body, pageURL)
	if pageErr != nil {
		return nil, pageErr
	}
	return academicWrap(map[string]any{
		"scope": scope, "entry_path": nullableString(entryPath), "path": path, "keyword": nullableString(keyword),
		"items": items, "item_count": len(items), "page": page,
	}), nil
}

func academicSelectionWindowRows(document *pageNode, keyword, pageURL string) []map[string]any {
	table := academicTable(document, "tbKxkc")
	if table == nil {
		return []map[string]any{}
	}
	rows := directTableRows(table)
	if len(rows) == 0 {
		return []map[string]any{}
	}
	header := rowValues(rows[0])
	if len(directCells(rows[0])) > 0 && directCells(rows[0])[0].tag == "th" {
		rows = rows[1:]
	}
	items := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		values := rowValues(row)
		text := strings.TrimSpace(pageDisplayText(row))
		if len(values) == 0 || allEmpty(values) || academicNoDataRow(text) || (keyword != "" && !strings.Contains(strings.ToLower(text), strings.ToLower(keyword))) {
			continue
		}
		item := map[string]any{"index": len(items) + 1, "cells": values, "text": text}
		for index, title := range header {
			if name := academicSelectionWindowField(title); name != "" && index < len(values) {
				item[name] = values[index]
			}
		}
		for _, link := range row.findAll("a") {
			target := resolvePageURL(pageURL, link.attr("href"))
			if target == "" {
				continue
			}
			if path, pathErr := academicPath(target); pathErr == nil {
				item["entry_path"] = path
				break
			}
		}
		items = append(items, item)
	}
	return items
}

func academicSelectionEntry(document *pageNode, pageURL, keyword string) string {
	for _, table := range document.findAll("table") {
		for _, row := range directTableRows(table) {
			if !strings.Contains(pageDisplayText(row), keyword) {
				continue
			}
			for _, link := range row.findAll("a") {
				href := link.attr("href")
				if href == "" {
					continue
				}
				if target := resolvePageURL(pageURL, href); target != "" {
					path, pathErr := academicPath(target)
					if pathErr == nil {
						return path
					}
				}
			}
		}
	}
	return ""
}

func academicCourseRows(document *pageNode, keyword string) []map[string]any {
	return academicStructuredRows(document, keyword, academicCourseField)
}

func (a NativeSite) academicStructuredPage(ctx context.Context, args []string, kind, path string) (map[string]any, *siteError) {
	return a.academicStructuredPageWithField(ctx, args, kind, path, academicPageField)
}

func (a NativeSite) academicStructuredPageWithField(ctx context.Context, args []string, kind, path string, field func(string) string) (map[string]any, *siteError) {
	body, pageURL, err := a.academicPage(ctx, "GET", path, nil, nil)
	if err != nil {
		return nil, err
	}
	document, parseErr := parsePage(body)
	if parseErr != nil {
		return nil, &siteError{Code: "parse_error", Message: parseErr.Error()}
	}
	keyword := strings.TrimSpace(flagValue(args, "--keyword"))
	items := academicStructuredRows(document, keyword, field)
	if len(items) == 0 && !noAcademicData(document) && len(document.findAll("table")) == 0 {
		return nil, &siteError{Code: "parse_error", Message: kind + " 页面未包含可解析表格；请使用 web get 查看页面结构"}
	}
	page, pageErr := pageInspect(body, pageURL)
	if pageErr != nil {
		return nil, pageErr
	}
	return academicWrap(map[string]any{
		"kind": kind, "path": path, "keyword": nullableString(keyword),
		"items": items, "item_count": len(items), "page": page,
	}), nil
}

func (a NativeSite) academicGraduationInfoCheck(ctx context.Context) (map[string]any, *siteError) {
	const path = "/jsxsd/bygl/bysxx"
	body, pageURL, err := a.academicPage(ctx, "GET", path, nil, nil)
	if err != nil {
		return nil, err
	}
	document, parseErr := parsePage(body)
	if parseErr != nil {
		return nil, &siteError{Code: "parse_error", Message: parseErr.Error()}
	}
	result := parseGraduationInfoCheckPage(document, pageURL)
	page, pageErr := pageInspect(body, pageURL)
	if pageErr != nil {
		return nil, pageErr
	}
	result["page"] = page
	return academicWrap(result), nil
}

func (a NativeSite) academicDeferredExamApplications(ctx context.Context, args []string) (map[string]any, *siteError) {
	const queryPath = "/jsxsd/kscj/hksq_query"
	const listPath = "/jsxsd/kscj/hksq_list"
	queryBody, queryURL, err := a.academicPage(ctx, "GET", queryPath, nil, nil)
	if err != nil {
		return nil, err
	}
	queryDocument, parseErr := parsePage(queryBody)
	if parseErr != nil {
		return nil, &siteError{Code: "parse_error", Message: parseErr.Error()}
	}
	term := strings.TrimSpace(flagValue(args, "--term"))
	if term == "" {
		term = selectedOptionPage(queryDocument, "xnxqid")
	}
	activity := strings.TrimSpace(flagValue(args, "--activity"))
	activityID := ""
	if activity != "" {
		var activityErr *siteError
		activityID, activity, activityErr = a.academicDeferredExamActivity(ctx, term, activity, queryURL)
		if activityErr != nil {
			return nil, activityErr
		}
	}
	status, statusID, statusErr := deferredExamStatus(flagValue(args, "--status"))
	if statusErr != nil {
		return nil, statusErr
	}
	course := strings.TrimSpace(flagValue(args, "--course"))
	data := []pair{{"xnxqid", term}, {"cj0701id", activityID}, {"kch", course}, {"iswfmes", statusID}}
	body, pageURL, err := a.academicPage(ctx, "POST", listPath, data, []pair{{"Referer", queryURL}})
	if err != nil {
		return nil, err
	}
	document, parseErr := parsePage(body)
	if parseErr != nil {
		return nil, &siteError{Code: "parse_error", Message: parseErr.Error()}
	}
	keyword := strings.TrimSpace(flagValue(args, "--keyword"))
	items := academicStructuredRowsWithLinks(document, keyword, academicDeferredExamField, pageURL)
	if len(items) == 0 && !noAcademicData(document) && len(document.findAll("table")) == 0 {
		return nil, &siteError{Code: "parse_error", Message: "缓考申请页面未包含可解析表格；请使用 web get 查看页面结构"}
	}
	page, pageErr := pageInspect(body, pageURL)
	if pageErr != nil {
		return nil, pageErr
	}
	return academicWrap(map[string]any{
		"kind": "deferred-exam-applications", "path": listPath,
		"term": nullableString(term), "activity": nullableString(activity), "course": nullableString(course), "status": nullableString(status),
		"items": items, "item_count": len(items), "page": page,
	}), nil
}

func (a NativeSite) academicExemptExamApplications(ctx context.Context, args []string) (map[string]any, *siteError) {
	const queryPath = "/jsxsd/kscj/mksq_query"
	const listPath = "/jsxsd/kscj/mksq_list"
	queryBody, queryURL, err := a.academicPage(ctx, "GET", queryPath, nil, nil)
	if err != nil {
		return nil, err
	}
	queryDocument, parseErr := parsePage(queryBody)
	if parseErr != nil {
		return nil, &siteError{Code: "parse_error", Message: parseErr.Error()}
	}
	term := strings.TrimSpace(flagValue(args, "--term"))
	if term == "" {
		term = selectedOptionPage(queryDocument, "xnxqid")
	}
	course := strings.TrimSpace(flagValue(args, "--course"))
	assessmentMethod, assessmentMethodID, methodErr := exemptExamAssessmentMethod(flagValue(args, "--assessment-method"))
	if methodErr != nil {
		return nil, methodErr
	}
	body, pageURL, err := a.academicPage(ctx, "POST", listPath, []pair{{"xnxqid", term}, {"kcmc", course}, {"ksfs", assessmentMethodID}}, []pair{{"Referer", queryURL}})
	if err != nil {
		return nil, err
	}
	document, parseErr := parsePage(body)
	if parseErr != nil {
		return nil, &siteError{Code: "parse_error", Message: parseErr.Error()}
	}
	keyword := strings.TrimSpace(flagValue(args, "--keyword"))
	items := academicStructuredRowsWithLinks(document, keyword, academicExemptExamField, pageURL)
	if len(items) == 0 && !noAcademicData(document) && len(document.findAll("table")) == 0 {
		return nil, &siteError{Code: "parse_error", Message: "免考申请页面未包含可解析表格；请使用 web get 查看页面结构"}
	}
	page, pageErr := pageInspect(body, pageURL)
	if pageErr != nil {
		return nil, pageErr
	}
	return academicWrap(map[string]any{
		"kind": "exempt-exam-applications", "path": listPath, "term": nullableString(term),
		"course": nullableString(course), "assessment_method": nullableString(assessmentMethod), "keyword": nullableString(keyword),
		"items": items, "item_count": len(items), "page": page,
	}), nil
}

func exemptExamAssessmentMethod(value string) (string, string, *siteError) {
	value = strings.TrimSpace(value)
	switch strings.ToLower(value) {
	case "", "all", "全部":
		return "", "", nil
	case "exam", "考试":
		return "考试", "1", nil
	case "other", "其它", "其他":
		return "其它", "2", nil
	case "assessment", "考查":
		return "考查", "3", nil
	case "no-exam", "不考试":
		return "不考试", "8", nil
	default:
		return "", "", &siteError{Code: "invalid_argument", Message: "--assessment-method 只能是 exam、other、assessment、no-exam 或 all"}
	}
}

func (a NativeSite) academicGraduateExamRegistration(ctx context.Context, args []string) (map[string]any, *siteError) {
	const queryPath = "/jsxsd/xsks/bysckbm_query"
	const listPath = "/jsxsd/xsks/bysckbm_list"
	queryBody, queryURL, err := a.academicPage(ctx, "GET", queryPath, nil, nil)
	if err != nil {
		return nil, err
	}
	queryDocument, parseErr := parsePage(queryBody)
	if parseErr != nil {
		return nil, &siteError{Code: "parse_error", Message: parseErr.Error()}
	}
	term := strings.TrimSpace(flagValue(args, "--term"))
	if term == "" {
		term = selectedOptionPage(queryDocument, "xnxqid")
	}
	examProject, examProjectID, optionErr := academicNamedPageOption(queryDocument, "kw0401id", flagValue(args, "--exam-project"), "毕业生插考项目")
	if optionErr != nil {
		return nil, optionErr
	}
	campus, campusID, campusErr := graduateExamCampus(flagValue(args, "--campus"))
	if campusErr != nil {
		return nil, campusErr
	}
	data := []pair{{"xnxqid", term}, {"kw0401id", examProjectID}, {"qssj", strings.TrimSpace(flagValue(args, "--start"))}, {"jssj", strings.TrimSpace(flagValue(args, "--end"))}, {"xq", campusID}}
	body, pageURL, err := a.academicPage(ctx, "POST", listPath, data, []pair{{"Referer", queryURL}})
	if err != nil {
		return nil, err
	}
	document, parseErr := parsePage(body)
	if parseErr != nil {
		return nil, &siteError{Code: "parse_error", Message: parseErr.Error()}
	}
	items := academicStructuredRowsWithLinks(document, "", academicGraduateExamField, pageURL)
	page, pageErr := pageInspect(body, pageURL)
	if pageErr != nil {
		return nil, pageErr
	}
	statusMessage := ""
	if messages, ok := page["messages"].([]string); ok && len(messages) > 0 {
		statusMessage = messages[0]
	}
	if len(items) == 0 && statusMessage == "" && len(document.findAll("table")) == 0 && page["kind"] == "html" {
		return nil, &siteError{Code: "parse_error", Message: "毕业生插考查询未返回记录或状态消息；请使用 web get 查看页面结构"}
	}
	return academicWrap(map[string]any{
		"kind": "graduate-exam-registration", "path": listPath, "term": nullableString(term),
		"exam_project": nullableString(examProject), "campus": nullableString(campus),
		"start": nullableString(strings.TrimSpace(flagValue(args, "--start"))), "end": nullableString(strings.TrimSpace(flagValue(args, "--end"))),
		"status_message": nullableString(statusMessage), "items": items, "item_count": len(items), "page": page,
	}), nil
}

func (a NativeSite) academicDeferredExamRegistration(ctx context.Context, args []string) (map[string]any, *siteError) {
	const queryPath = "/jsxsd/kscj/hkbm_query"
	const listPath = "/jsxsd/kscj/hkbm_list"
	queryBody, queryURL, err := a.academicPage(ctx, "GET", queryPath, nil, nil)
	if err != nil {
		return nil, err
	}
	queryDocument, parseErr := parsePage(queryBody)
	if parseErr != nil {
		return nil, &siteError{Code: "parse_error", Message: parseErr.Error()}
	}
	term := strings.TrimSpace(flagValue(args, "--term"))
	if term == "" {
		term = selectedOptionPage(queryDocument, "xnxq")
	}
	examProject, examProjectID, optionErr := academicNamedPageOption(queryDocument, "ksxm", flagValue(args, "--exam-project"), "缓考考试项目")
	if optionErr != nil {
		return nil, optionErr
	}
	campus, campusID, campusErr := graduateExamCampus(flagValue(args, "--campus"))
	if campusErr != nil {
		return nil, campusErr
	}
	body, pageURL, err := a.academicPage(ctx, "POST", listPath, []pair{{"xnxq", term}, {"ksxm", examProjectID}, {"kcxq", campusID}}, []pair{{"Referer", queryURL}})
	if err != nil {
		return nil, err
	}
	document, parseErr := parsePage(body)
	if parseErr != nil {
		return nil, &siteError{Code: "parse_error", Message: parseErr.Error()}
	}
	items := academicStructuredRowsWithLinks(document, "", academicGraduateExamField, pageURL)
	page, pageErr := pageInspect(body, pageURL)
	if pageErr != nil {
		return nil, pageErr
	}
	statusMessage := academicRegistrationStatusMessage(document, page)
	if len(items) == 0 && statusMessage == "" && len(document.findAll("table")) == 0 && page["kind"] == "html" {
		return nil, &siteError{Code: "parse_error", Message: "缓考报名查询未返回记录或状态消息；请使用 web get 查看页面结构"}
	}
	return academicWrap(map[string]any{
		"kind": "deferred-exam-registration", "path": listPath, "term": nullableString(term),
		"exam_project": nullableString(examProject), "campus": nullableString(campus),
		"status_message": nullableString(statusMessage), "items": items, "item_count": len(items), "page": page,
	}), nil
}

func (a NativeSite) academicGradeConfirmation(ctx context.Context) (map[string]any, *siteError) {
	const path = "/jsxsd/kscj/cjqr_list"
	body, pageURL, err := a.academicPage(ctx, "GET", path, nil, nil)
	if err != nil {
		return nil, err
	}
	document, parseErr := parsePage(body)
	if parseErr != nil {
		return nil, &siteError{Code: "parse_error", Message: parseErr.Error()}
	}
	page, pageErr := pageInspect(body, pageURL)
	if pageErr != nil {
		return nil, pageErr
	}
	text := strings.TrimSpace(pageDisplayText(document))
	statusMessage := ""
	for _, marker := range []string{"当前学年学期不在时间范围内", "成绩确认时间未到"} {
		if index := strings.Index(text, marker); index >= 0 {
			statusMessage = strings.TrimSpace(text[index:])
			break
		}
	}
	return academicWrap(map[string]any{
		"kind": "grade-confirmation", "path": path, "status_message": nullableString(statusMessage), "text": text, "page": page,
	}), nil
}

func (a NativeSite) academicClassChanges(ctx context.Context, args []string) (map[string]any, *siteError) {
	const queryPath = "/jsxsd/xskb/xskb_ttkmx.do"
	const listPath = "/jsxsd/xskb/loadTtkMxList"
	queryBody, queryURL, err := a.academicPage(ctx, "GET", queryPath, nil, nil)
	if err != nil {
		return nil, err
	}
	queryDocument, parseErr := parsePage(queryBody)
	if parseErr != nil {
		return nil, &siteError{Code: "parse_error", Message: parseErr.Error()}
	}
	term := strings.TrimSpace(flagValue(args, "--term"))
	if term == "" {
		term = selectedOptionPage(queryDocument, "xnxqid")
	}
	body, pageURL, err := a.academicPage(ctx, "POST", listPath, []pair{{"xnxqid", term}}, []pair{{"Referer", queryURL}})
	if err != nil {
		return nil, err
	}
	document, parseErr := parsePage(body)
	if parseErr != nil {
		return nil, &siteError{Code: "parse_error", Message: parseErr.Error()}
	}
	keyword := strings.TrimSpace(flagValue(args, "--keyword"))
	items := academicStructuredRowsWithLinks(document, keyword, academicClassChangeField, pageURL)
	if len(items) == 0 && !noAcademicData(document) && len(document.findAll("table")) == 0 {
		return nil, &siteError{Code: "parse_error", Message: "调停课查询未包含可解析表格；请使用 web get 查看页面结构"}
	}
	page, pageErr := pageInspect(body, pageURL)
	if pageErr != nil {
		return nil, pageErr
	}
	return academicWrap(map[string]any{
		"kind": "class-changes", "path": listPath, "term": nullableString(term), "keyword": nullableString(keyword),
		"items": items, "item_count": len(items), "page": page,
	}), nil
}

func academicNamedPageOption(document *pageNode, id, wanted, description string) (string, string, *siteError) {
	wanted = strings.TrimSpace(wanted)
	if wanted == "" {
		return "", "", nil
	}
	options := pageOptions(document, id)
	for _, option := range options {
		label := strings.TrimSpace(fmt.Sprint(option["label"]))
		if label == wanted {
			return label, strings.TrimSpace(fmt.Sprint(option["value"])), nil
		}
	}
	matches := make([]map[string]any, 0)
	for _, option := range options {
		label := strings.TrimSpace(fmt.Sprint(option["label"]))
		if label != "" && strings.Contains(strings.ToLower(label), strings.ToLower(wanted)) {
			matches = append(matches, option)
		}
	}
	if len(matches) == 1 {
		return strings.TrimSpace(fmt.Sprint(matches[0]["label"])), strings.TrimSpace(fmt.Sprint(matches[0]["value"])), nil
	}
	if len(matches) > 1 {
		choices := make([]string, 0, len(matches))
		for _, option := range matches {
			choices = append(choices, strings.TrimSpace(fmt.Sprint(option["label"])))
		}
		return "", "", &siteError{Code: "ambiguous_target", Message: description + "名称对应多个项目，请使用完整名称", Details: map[string]any{"target": wanted, "choices": choices}}
	}
	return "", "", &siteError{Code: "not_found", Message: "当前页面找不到对应" + description, Details: map[string]any{"target": wanted, "choices": options}}
}

func academicRegistrationStatusMessage(document *pageNode, page map[string]any) string {
	if messages, ok := page["messages"].([]string); ok && len(messages) > 0 {
		return messages[0]
	}
	text := strings.TrimSpace(pageDisplayText(document))
	for _, marker := range []string{"当前不在报名时间范围内或未启用报名", "未查询到数据", "暂无数据"} {
		if index := strings.Index(text, marker); index >= 0 {
			status := strings.TrimSpace(text[index:])
			if len(status) <= 160 {
				return status
			}
			return marker
		}
	}
	if len(text) <= 240 {
		return text
	}
	return ""
}

func graduateExamCampus(value string) (string, string, *siteError) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "all", "全部":
		return "", "", nil
	case "yuntang", "云塘", "云塘校区", "1":
		return "云塘校区", "1", nil
	case "jinpenling", "金盆岭", "金盆岭校区", "2":
		return "金盆岭校区", "2", nil
	default:
		return "", "", &siteError{Code: "invalid_argument", Message: "--campus 只能是 yuntang、jinpenling 或 all"}
	}
}

func (a NativeSite) academicDeferredExamActivity(ctx context.Context, term, wanted, referer string) (string, string, *siteError) {
	path := "/jsxsd/kscj/hksq_query_ajax?xnxq01id=" + url.QueryEscape(term)
	result, err := a.executeAcademicRequestWithRecovery(ctx, siteRequest{
		Service: "academic", Path: path, Method: "GET", CookieFile: academicCookiePath(),
		Headers: []pair{{"Referer", referer}}, RequireLogin: true, ReadOnly: true, Yes: true, RawJSON: true,
	})
	if err != nil {
		return "", "", err
	}
	var options []struct {
		ID   string `json:"cj0701id"`
		Name string `json:"cjlrmc"`
	}
	encoded := []byte(nil)
	if value, ok := businessData(result); ok {
		var marshalErr error
		encoded, marshalErr = json.Marshal(value)
		if marshalErr != nil {
			return "", "", &siteError{Code: "parse_error", Message: "缓考活动接口响应无法读取: " + marshalErr.Error()}
		}
	} else if response, ok := result["response"].(map[string]any); ok {
		body, _ := response["body_internal"].(string)
		if body == "" {
			body, _ = response["body"].(string)
		}
		encoded = []byte(body)
	}
	if parseErr := json.Unmarshal(encoded, &options); parseErr != nil {
		return "", "", &siteError{Code: "parse_error", Message: "缓考活动接口响应不是有效 JSON: " + parseErr.Error()}
	}
	wantedLower := strings.ToLower(strings.TrimSpace(wanted))
	for _, option := range options {
		if option.ID != "" && strings.TrimSpace(option.Name) == wanted {
			return option.ID, strings.TrimSpace(option.Name), nil
		}
	}
	matches := make([]struct {
		id   string
		name string
	}, 0)
	for _, option := range options {
		name := strings.TrimSpace(option.Name)
		if option.ID != "" && (name == wanted || strings.Contains(strings.ToLower(name), wantedLower)) {
			matches = append(matches, struct {
				id   string
				name string
			}{id: option.ID, name: name})
		}
	}
	if len(matches) == 1 {
		return matches[0].id, matches[0].name, nil
	}
	if len(matches) > 1 {
		choices := make([]string, 0, len(matches))
		for _, match := range matches {
			choices = append(choices, match.name)
		}
		return "", "", &siteError{Code: "ambiguous_target", Message: "缓考活动名称对应多个活动，请使用完整名称", Details: map[string]any{"activity": wanted, "choices": choices}}
	}
	choices := make([]string, 0, len(options))
	for _, option := range options {
		if name := strings.TrimSpace(option.Name); name != "" {
			choices = append(choices, name)
		}
	}
	return "", "", &siteError{Code: "not_found", Message: "当前学期找不到对应缓考活动", Details: map[string]any{"activity": wanted, "choices": choices}}
}

func deferredExamStatus(value string) (string, string, *siteError) {
	value = strings.TrimSpace(value)
	switch strings.ToLower(value) {
	case "", "all", "全部":
		return "", "", nil
	case "pending", "待审":
		return "pending", "3", nil
	case "reviewing", "审核中":
		return "reviewing", "2", nil
	case "approved", "通过":
		return "approved", "1", nil
	case "rejected", "不通过":
		return "rejected", "0", nil
	default:
		return "", "", &siteError{Code: "invalid_argument", Message: "--status 只能是 pending、reviewing、approved、rejected 或 all"}
	}
}

func (a NativeSite) academicSecondClassCreditApplications(ctx context.Context, args []string) (map[string]any, *siteError) {
	const path = "/jsxsd/pyfa/cxxfsb_query"
	body, pageURL, err := a.academicPage(ctx, "GET", path, nil, nil)
	if err != nil {
		return nil, err
	}
	document, parseErr := parsePage(body)
	if parseErr != nil {
		return nil, &siteError{Code: "parse_error", Message: parseErr.Error()}
	}
	keyword := strings.TrimSpace(flagValue(args, "--keyword"))
	items := academicSecondClassCreditApplicationRows(document, keyword, pageURL)
	if len(items) == 0 && !noAcademicData(document) && len(document.findAll("table")) == 0 {
		return nil, &siteError{Code: "parse_error", Message: "第二课堂学分申报页面未包含可解析表格；请使用 web get 查看页面结构"}
	}
	page, pageErr := pageInspect(body, pageURL)
	if pageErr != nil {
		return nil, pageErr
	}
	return academicWrap(map[string]any{
		"kind": "second-class-credit-applications", "path": path, "keyword": nullableString(keyword),
		"items": items, "item_count": len(items), "page": page,
	}), nil
}

func academicSecondClassCreditApplicationRows(document *pageNode, keyword, pageURL string) []map[string]any {
	items := academicStructuredRowsWithLinks(document, keyword, academicSecondClassCreditField, pageURL)
	for _, item := range items {
		workflowPath, _ := item["detail_path"].(string)
		if parsed, parseErr := url.Parse(workflowPath); parseErr == nil {
			if id := parsed.Query().Get("cxxf04id"); id != "" {
				item["application_id"] = id
				item["workflow_path"] = workflowPath
				delete(item, "detail_path")
			}
		}
	}
	return items
}

func (a NativeSite) academicSecondClassCreditApplication(ctx context.Context, args []string) (map[string]any, *siteError) {
	id := strings.TrimSpace(flagValue(args, "--id"))
	if id == "" || strings.ContainsAny(id, "/?#&") {
		return nil, &siteError{Code: "invalid_argument", Message: "second-class-credit-application 必须提供不含路径的 --id"}
	}
	path := "/jsxsd/pyfa/cxxfsb_shyj?cxxf04id=" + url.QueryEscape(id)
	body, pageURL, err := a.academicPage(ctx, "GET", path, nil, nil)
	if err != nil {
		return nil, err
	}
	document, parseErr := parsePage(body)
	if parseErr != nil {
		return nil, &siteError{Code: "parse_error", Message: parseErr.Error()}
	}
	detail, detailErr := parseSecondClassCreditApplicationPage(document, pageURL)
	if detailErr != nil {
		return nil, detailErr
	}
	detail["kind"], detail["application_id"] = "second-class-credit-application", id
	page, detailPageErr := pageInspect(body, pageURL)
	if detailPageErr != nil {
		return nil, detailPageErr
	}
	detail["page"] = page
	return academicWrap(detail), nil
}

func parseSecondClassCreditApplicationPage(document *pageNode, pageURL string) (map[string]any, *siteError) {
	fields := map[string]string{}
	for _, table := range document.findAll("table") {
		for _, row := range directTableRows(table) {
			values := rowValues(row)
			if len(values) < 2 {
				continue
			}
			for index := 0; index+1 < len(values); index += 2 {
				label := strings.Trim(strings.TrimSpace(values[index]), " ：:")
				value := strings.TrimSpace(values[index+1])
				if label == "" || value == "" {
					continue
				}
				fields[label] = value
			}
		}
	}
	if len(fields) == 0 {
		return nil, &siteError{Code: "parse_error", Message: "未找到第二课堂学分申报流程详情"}
	}
	result := map[string]any{
		"url":    safeSiteURL(mustParseURL(pageURL)),
		"fields": fields,
		"text":   pageDisplayText(document),
	}
	for label, name := range map[string]string{
		"项目获得时间": "project_time",
		"审核状态":   "review_history",
		"认定状态":   "recognition_history",
	} {
		if value := fields[label]; value != "" {
			result[name] = value
		}
	}
	return result, nil
}

func (a NativeSite) academicAnnouncements(ctx context.Context, args []string) (map[string]any, *siteError) {
	const path = "/jsxsd/ggly/ysgg_query"
	body, pageURL, err := a.academicPage(ctx, "GET", path, nil, nil)
	if err != nil {
		return nil, err
	}
	document, parseErr := parsePage(body)
	if parseErr != nil {
		return nil, &siteError{Code: "parse_error", Message: parseErr.Error()}
	}
	keyword := strings.TrimSpace(flagValue(args, "--keyword"))
	items := academicStructuredRowsWithLinks(document, keyword, academicAnnouncementField, pageURL)
	if len(items) == 0 && !noAcademicData(document) && len(document.findAll("table")) == 0 {
		return nil, &siteError{Code: "parse_error", Message: "公告页面未包含可解析表格；请使用 web get 查看页面结构"}
	}
	page, pageErr := pageInspect(body, pageURL)
	if pageErr != nil {
		return nil, pageErr
	}
	return academicWrap(map[string]any{
		"kind": "announcements", "path": path, "keyword": nullableString(keyword),
		"items": items, "item_count": len(items), "page": page,
	}), nil
}

func (a NativeSite) academicMessages(ctx context.Context, args []string) (map[string]any, *siteError) {
	const path = "/jsxsd/ggly/ysly_query"
	body, pageURL, err := a.academicPage(ctx, "GET", path, nil, nil)
	if err != nil {
		return nil, err
	}
	document, parseErr := parsePage(body)
	if parseErr != nil {
		return nil, &siteError{Code: "parse_error", Message: parseErr.Error()}
	}
	keyword := strings.TrimSpace(flagValue(args, "--keyword"))
	items := academicMessageRows(document, keyword, pageURL)
	if len(items) == 0 && !noAcademicData(document) && len(document.findAll("table")) == 0 {
		return nil, &siteError{Code: "parse_error", Message: "留言页面未包含可解析表格；请使用 web get 查看页面结构"}
	}
	page, pageErr := pageInspect(body, pageURL)
	if pageErr != nil {
		return nil, pageErr
	}
	return academicWrap(map[string]any{
		"kind": "messages", "path": path, "keyword": nullableString(keyword),
		"items": items, "item_count": len(items), "page": page,
	}), nil
}

func academicMessageRows(document *pageNode, keyword, pageURL string) []map[string]any {
	items := academicStructuredRowsWithLinks(document, keyword, academicAnnouncementField, pageURL)
	for _, item := range items {
		if id, ok := item["announcement_id"]; ok {
			item["message_id"] = id
			delete(item, "announcement_id")
		}
	}
	return items
}

func (a NativeSite) academicAnnouncement(ctx context.Context, args []string) (map[string]any, *siteError) {
	id := strings.TrimSpace(flagValue(args, "--id"))
	if id == "" || strings.ContainsAny(id, "/?#&") {
		return nil, &siteError{Code: "invalid_argument", Message: "announcement 必须提供不含路径的 --id"}
	}
	path := "/jsxsd/ggly/ggly_show?ggid=" + url.QueryEscape(id)
	body, pageURL, err := a.academicPage(ctx, "GET", path, nil, nil)
	if err != nil {
		return nil, err
	}
	document, parseErr := parsePage(body)
	if parseErr != nil {
		return nil, &siteError{Code: "parse_error", Message: parseErr.Error()}
	}
	page, pageErr := pageInspect(body, pageURL)
	if pageErr != nil {
		return nil, pageErr
	}
	return academicWrap(map[string]any{
		"kind": "announcement", "announcement_id": id, "content": pageDisplayText(document), "page": page,
	}), nil
}

func (a NativeSite) academicMessage(ctx context.Context, args []string) (map[string]any, *siteError) {
	id := strings.TrimSpace(flagValue(args, "--id"))
	if id == "" || strings.ContainsAny(id, "/?#&") {
		return nil, &siteError{Code: "invalid_argument", Message: "message 必须提供不含路径的 --id"}
	}
	path := "/jsxsd/ggly/ggly_show?dpt=ly&ggid=" + url.QueryEscape(id)
	body, pageURL, err := a.academicPage(ctx, "GET", path, nil, nil)
	if err != nil {
		return nil, err
	}
	document, parseErr := parsePage(body)
	if parseErr != nil {
		return nil, &siteError{Code: "parse_error", Message: parseErr.Error()}
	}
	page, pageErr := pageInspect(body, pageURL)
	if pageErr != nil {
		return nil, pageErr
	}
	return academicWrap(map[string]any{
		"kind": "message", "message_id": id, "content": pageDisplayText(document), "page": page,
	}), nil
}

func (a NativeSite) academicMessageReply(ctx context.Context, args []string) (map[string]any, *siteError) {
	id := strings.TrimSpace(flagValue(args, "--id"))
	content := flagValue(args, "--content")
	if id == "" || strings.ContainsAny(id, "/?#&") {
		return nil, &siteError{Code: "invalid_argument", Message: "message reply 必须提供不含路径的 --id"}
	}
	if strings.TrimSpace(content) == "" {
		return nil, &siteError{Code: "invalid_argument", Message: "message reply 必须提供 --content"}
	}
	if len([]rune(content)) > 2000 {
		return nil, &siteError{Code: "invalid_argument", Message: "--content 不能超过 2000 个字符"}
	}
	if !flagPresent(args, "--yes") {
		return nil, &siteError{Code: "confirmation_required", Message: "回复留言会修改账号数据，请加 --yes"}
	}
	detailPath := "/jsxsd/ggly/ggly_show?dpt=ly&ggid=" + url.QueryEscape(id)
	_, detailURL, detailErr := a.academicPage(ctx, "GET", detailPath, nil, nil)
	if detailErr != nil {
		return nil, detailErr
	}
	result, submitErr := a.executeAcademicRequestWithRecovery(ctx, siteRequest{
		Service:      "academic",
		Path:         "/jsxsd/ggly/lyhf_save",
		Method:       "POST",
		CookieFile:   academicCookiePath(),
		Data:         []pair{{"ggid", id}, {"xmms", content}},
		Headers:      []pair{{"Referer", detailURL}},
		RequireLogin: true,
		Yes:          true,
	})
	if submitErr != nil {
		return nil, submitErr
	}
	return academicWrap(map[string]any{
		"kind": "message-reply", "message_id": id, "content_length": len([]rune(content)),
		"request":  map[string]any{"method": "POST", "path": "/jsxsd/ggly/lyhf_save", "fields": []string{"ggid", "xmms"}},
		"response": result["response"], "submitted": true, "confirmed": true, "evidence": "reply-response-confirmed",
	}), nil
}

func (a NativeSite) academicRetakeCourses(ctx context.Context, args []string) (map[string]any, *siteError) {
	const path = "/jsxsd/kscj/cxbmxk_query"
	body, pageURL, err := a.academicPage(ctx, "GET", path, nil, nil)
	if err != nil {
		return nil, err
	}
	document, parseErr := parsePage(body)
	if parseErr != nil {
		return nil, &siteError{Code: "parse_error", Message: parseErr.Error()}
	}
	keyword := strings.TrimSpace(flagValue(args, "--keyword"))
	items := academicRetakeRows(document, keyword)
	if len(items) == 0 && !noAcademicData(document) && len(document.findAll("table")) == 0 {
		return nil, &siteError{Code: "parse_error", Message: "重修报名页面未包含可解析表格；请使用 web get 查看页面结构"}
	}
	page, pageErr := pageInspect(body, pageURL)
	if pageErr != nil {
		return nil, pageErr
	}
	statusMessage := ""
	if messages, ok := page["messages"].([]string); ok && len(messages) > 0 {
		statusMessage = messages[0]
	}
	return academicWrap(map[string]any{
		"kind": "retake-courses", "path": path, "keyword": nullableString(keyword),
		"status_message": nullableString(statusMessage), "items": items, "item_count": len(items), "page": page,
	}), nil
}

func academicRetakeRows(document *pageNode, keyword string) []map[string]any {
	var rows []*pageNode
	for _, table := range document.findAll("table") {
		candidate := directTableRows(table)
		if len(candidate) > 0 && containsValue(rowValues(candidate[0]), "重修报名类别") {
			rows = candidate
			break
		}
	}
	if len(rows) == 0 {
		return []map[string]any{}
	}
	header := rowValues(rows[0])
	if cells := directCells(rows[0]); len(cells) > 0 && cells[0].tag == "th" {
		rows = rows[1:]
	}
	items := make([]map[string]any, 0)
	lastItem := -1
	for _, row := range rows {
		values := rowValues(row)
		text := strings.TrimSpace(pageDisplayText(row))
		if len(values) < len(header) {
			if lastItem >= 0 {
				for name, value := range academicRetakeDetailFields(text) {
					items[lastItem][name] = value
				}
			}
			continue
		}
		if allEmpty(values) || academicNoDataRow(text) || (keyword != "" && !strings.Contains(strings.ToLower(text), strings.ToLower(keyword))) {
			lastItem = -1
			continue
		}
		item := map[string]any{"index": len(items) + 1, "cells": values, "text": text}
		for index, title := range header {
			if name := academicRetakeField(title); name != "" && index < len(values) {
				item[name] = values[index]
			}
		}
		items = append(items, item)
		lastItem = len(items) - 1
	}
	return items
}

func academicRetakeDetailFields(value string) map[string]string {
	fields := make(map[string]string)
	for _, part := range strings.Split(value, ";") {
		index := strings.IndexAny(part, ":：")
		if index < 0 {
			continue
		}
		label, fieldValue := strings.TrimSpace(part[:index]), strings.TrimSpace(part[index+1:])
		name := map[string]string{
			"课程编号": "course_id", "考试性质": "exam_nature", "课程属性": "course_attribute",
			"课程性质": "course_nature", "成绩标识": "score_mark", "上课校区": "campus",
			"上课班级": "class", "上课教师": "teacher", "教师院系": "department",
		}[label]
		if name != "" {
			fields[name] = fieldValue
		}
	}
	return fields
}

func (a NativeSite) academicTrainingPlan(ctx context.Context, args []string) (map[string]any, *siteError) {
	body, pageURL, err := a.academicPage(ctx, "POST", "/jsxsd/pyfa/pyfa_query", nil, nil)
	if err != nil {
		return nil, err
	}
	document, parseErr := parsePage(body)
	if parseErr != nil {
		return nil, &siteError{Code: "parse_error", Message: parseErr.Error()}
	}
	keyword := strings.TrimSpace(flagValue(args, "--keyword"))
	items := academicStructuredRows(document, keyword, academicTrainingPlanField)
	if len(items) == 0 && !noAcademicData(document) && len(document.findAll("table")) == 0 {
		return nil, &siteError{Code: "parse_error", Message: "培养方案页面未包含可解析表格；请使用 web get 查看页面结构"}
	}
	page, pageErr := pageInspect(body, pageURL)
	if pageErr != nil {
		return nil, pageErr
	}
	return academicWrap(map[string]any{
		"kind": "training-plan", "path": "/jsxsd/pyfa/pyfa_query", "keyword": nullableString(keyword),
		"items": items, "item_count": len(items), "page": page,
	}), nil
}

var trainingProgressSummaryPattern = regexp.MustCompile(`(必修|选修|实践环节)\s*[（(]\s*应修\s*([^/）)]+)\s*/\s*已修\s*([^）)]+)`)

func (a NativeSite) academicTrainingProgress(ctx context.Context, args []string) (map[string]any, *siteError) {
	body, pageURL, err := a.academicPage(ctx, "GET", "/jsxsd/pyfa/topyfamx", nil, nil)
	if err != nil {
		return nil, err
	}
	document, parseErr := parsePage(body)
	if parseErr != nil {
		return nil, &siteError{Code: "parse_error", Message: parseErr.Error()}
	}
	keyword := strings.TrimSpace(flagValue(args, "--keyword"))
	items := academicTrainingProgressRows(document, keyword)
	if len(items) == 0 && !noAcademicData(document) && len(document.findAll("table")) == 0 {
		return nil, &siteError{Code: "parse_error", Message: "培养方案完成情况页面未包含可解析表格；请使用 web get 查看页面结构"}
	}
	page, pageErr := pageInspect(body, pageURL)
	if pageErr != nil {
		return nil, pageErr
	}
	return academicWrap(map[string]any{
		"kind": "training-progress", "path": "/jsxsd/pyfa/topyfamx", "keyword": nullableString(keyword),
		"credit_summary": academicTrainingProgressSummary(pageDisplayText(document)),
		"items":          items, "item_count": len(items), "page": page,
	}), nil
}

func academicTrainingProgressSummary(text string) map[string]map[string]string {
	result := map[string]map[string]string{}
	for _, match := range trainingProgressSummaryPattern.FindAllStringSubmatch(text, -1) {
		if len(match) < 4 {
			continue
		}
		result[strings.TrimSpace(match[1])] = map[string]string{
			"required": strings.TrimSpace(match[2]),
			"earned":   strings.TrimSpace(match[3]),
		}
	}
	return result
}

var trainingProgressCourseCode = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{4,}$`)

func academicTrainingProgressRows(document *pageNode, keyword string) []map[string]any {
	var rows []*pageNode
	for _, table := range document.findAll("table") {
		candidate := directTableRows(table)
		for _, row := range candidate {
			values := rowValues(row)
			if containsValue(values, "课程体系") && containsValue(values, "完成情况") && len(candidate) > len(rows) {
				rows = candidate
				break
			}
		}
	}
	if len(rows) == 0 {
		return []map[string]any{}
	}
	dataStart := 0
	for index, row := range rows {
		values := rowValues(row)
		if !containsValue(values, "课程体系") || !containsValue(values, "完成情况") {
			continue
		}
		dataStart = index + 1
		if dataStart < len(rows) && containsValue(rowValues(rows[dataStart]), "讲课学时") {
			dataStart++
		}
		break
	}
	items := make([]map[string]any, 0)
	category := ""
	for _, row := range rows[dataStart:] {
		values := rowValues(row)
		text := strings.TrimSpace(pageDisplayText(row))
		if len(values) == 0 || allEmpty(values) {
			continue
		}
		codeIndex := -1
		for index, value := range values {
			if trainingProgressCourseCode.MatchString(strings.TrimSpace(value)) {
				codeIndex = index
				break
			}
		}
		if codeIndex < 0 {
			if current := trainingProgressCategory(values); current != "" {
				category = current
			}
			continue
		}
		if keyword != "" && !strings.Contains(strings.ToLower(text), strings.ToLower(keyword)) {
			continue
		}
		item := map[string]any{
			"index":      len(items) + 1,
			"cells":      values,
			"text":       text,
			"course_id":  values[codeIndex],
			"course":     trainingProgressValue(values, codeIndex+1),
			"completion": trainingProgressValue(values, codeIndex+2),
		}
		if current := trainingProgressCategory(values); current != "" {
			category = current
		}
		if category != "" {
			item["curriculum"] = category
		}
		if codeIndex >= 2 {
			item["selection_group"] = trainingProgressValue(values, 1)
		} else if codeIndex == 1 && strings.TrimSpace(values[0]) != "" && trainingProgressCategory(values) == "" {
			item["selection_group"] = values[0]
		}
		item["course_nature"] = trainingProgressValue(values, codeIndex+3)
		item["course_attribute"] = trainingProgressValue(values, codeIndex+4)
		item["credit"] = trainingProgressValue(values, codeIndex+5)
		item["lecture_hours"] = trainingProgressValue(values, codeIndex+6)
		item["computer_hours"] = trainingProgressValue(values, codeIndex+7)
		item["other_hours"] = trainingProgressValue(values, codeIndex+8)
		item["experiment_hours"] = trainingProgressValue(values, codeIndex+9)
		item["practice_hours"] = trainingProgressValue(values, codeIndex+10)
		item["total_hours"] = trainingProgressValue(values, codeIndex+11)
		if len(values) > codeIndex+11 {
			item["offered_term"] = values[len(values)-1]
		}
		items = append(items, item)
	}
	return items
}

func containsValue(values []string, wanted string) bool {
	for _, value := range values {
		if strings.Contains(strings.ReplaceAll(value, " ", ""), wanted) {
			return true
		}
	}
	return false
}

func trainingProgressValue(values []string, index int) string {
	if index < 0 || index >= len(values) {
		return ""
	}
	return values[index]
}

func trainingProgressCategory(values []string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		for _, category := range []string{"必修", "选修", "实践环节"} {
			if strings.HasPrefix(value, category) && strings.Contains(value, "应修") {
				return category
			}
		}
	}
	return ""
}

func academicStructuredRows(document *pageNode, keyword string, field func(string) string) []map[string]any {
	return academicStructuredRowsWithLinks(document, keyword, field, "")
}

func academicStructuredRowsWithLinks(document *pageNode, keyword string, field func(string) string, pageURL string) []map[string]any {
	table := (*pageNode)(nil)
	score := 0
	for _, candidate := range document.findAll("table") {
		text := pageDisplayText(candidate)
		current := len(directTableRows(candidate))
		if strings.Contains(text, "课程") {
			current += 5
		}
		if strings.Contains(text, "学分") || strings.Contains(text, "教师") || strings.Contains(text, "选课") || strings.Contains(text, "申请") || strings.Contains(text, "教室") || strings.Contains(text, "辅修") {
			current += 2
		}
		if current > score {
			table, score = candidate, current
		}
	}
	if table == nil {
		return []map[string]any{}
	}
	rows := directTableRows(table)
	if len(rows) == 0 {
		return []map[string]any{}
	}
	header := rowValues(rows[0])
	headerCount := 0
	for _, title := range header {
		if field(title) != "" {
			headerCount++
		}
	}
	headerRow := len(directCells(rows[0])) > 0 && (directCells(rows[0])[0].tag == "th" || countGradeHeaders(header) > 0 || countTextHeaders(header) > 0 || headerCount > 0)
	if headerRow {
		rows = rows[1:]
	}
	result := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		values := rowValues(row)
		text := strings.TrimSpace(pageDisplayText(row))
		if len(values) == 0 || allEmpty(values) || academicNoDataRow(text) || (keyword != "" && !strings.Contains(strings.ToLower(text), strings.ToLower(keyword))) {
			continue
		}
		item := map[string]any{"index": len(result) + 1, "cells": values, "text": text}
		if headerRow {
			for index, title := range header {
				if name := field(title); name != "" && index < len(values) {
					item[name] = values[index]
				}
			}
		}
		if pageURL != "" {
			for _, link := range row.findAll("a") {
				target := academicLinkTarget(link, pageURL)
				if target == "" {
					continue
				}
				path, _ := academicPath(target)
				item["detail_path"] = path
				if parsed, parseErr := url.Parse(path); parseErr == nil {
					if id := parsed.Query().Get("ggid"); id != "" {
						item["announcement_id"] = id
					}
				}
				if path != "" {
					break
				}
			}
		}
		result = append(result, item)
	}
	return result
}

func academicNoDataRow(value string) bool {
	value = strings.TrimSpace(value)
	return value == "未查询到数据" || value == "暂无数据" || value == "无数据"
}

func academicPageField(value string) string {
	value = regexp.MustCompile(`\s+`).ReplaceAllString(value, "")
	if field := academicCourseField(value); field != "" {
		return field
	}
	switch {
	case strings.Contains(value, "申请编号") || value == "编号":
		return "id"
	case strings.Contains(value, "教室"):
		return "room"
	case strings.Contains(value, "教学楼"):
		return "building"
	case strings.Contains(value, "校区"):
		return "campus"
	case strings.Contains(value, "申请日期") || strings.Contains(value, "借用日期") || value == "日期":
		return "date"
	case strings.Contains(value, "开始节") || strings.Contains(value, "起始节"):
		return "section_start"
	case strings.Contains(value, "结束节") || strings.Contains(value, "终止节"):
		return "section_end"
	case strings.Contains(value, "申请人"):
		return "applicant"
	case strings.Contains(value, "申请时间") || strings.Contains(value, "提交时间"):
		return "submitted_at"
	case strings.Contains(value, "状态"):
		return "status"
	case strings.Contains(value, "专业"):
		return "major"
	default:
		return ""
	}
}

func academicTrainingPlanField(value string) string {
	value = regexp.MustCompile(`\s+`).ReplaceAllString(value, "")
	switch {
	case strings.Contains(value, "开课学期") || strings.Contains(value, "计划学期") || value == "学期":
		return "term"
	case strings.Contains(value, "开课单位") || strings.Contains(value, "开课院系"):
		return "department"
	case strings.Contains(value, "考核方式"):
		return "assessment_method"
	case strings.Contains(value, "课程属性"):
		return "course_attribute"
	case strings.Contains(value, "是否考试"):
		return "exam"
	default:
		return academicPageField(value)
	}
}

func academicSecondClassCreditField(value string) string {
	value = regexp.MustCompile(`\s+`).ReplaceAllString(value, "")
	switch {
	case strings.Contains(value, "组织方式"):
		return "organization"
	case strings.Contains(value, "学年学期"):
		return "term"
	case strings.Contains(value, "分类名称"):
		return "category"
	case strings.Contains(value, "获得项目时间"):
		return "project_time"
	case strings.Contains(value, "认定学分"):
		return "recognized_credit"
	case strings.Contains(value, "审核状态"):
		return "review_status"
	case strings.Contains(value, "认定状态"):
		return "recognition_status"
	case strings.Contains(value, "项目编号"):
		return "project_id"
	case value == "学号":
		return "student_id"
	case value == "姓名":
		return "name"
	case strings.Contains(value, "学分类型"):
		return "credit_type"
	case value == "学分":
		return "credit"
	case value == "备注":
		return "note"
	case value == "操作":
		return "action"
	default:
		return academicPageField(value)
	}
}

func academicDeferredExamField(value string) string {
	value = regexp.MustCompile(`\s+`).ReplaceAllString(value, "")
	switch {
	case strings.Contains(value, "学年学期"):
		return "term"
	case strings.Contains(value, "课程编号") || strings.Contains(value, "课程代码"):
		return "course_id"
	case strings.Contains(value, "课程名称") || value == "课程":
		return "course"
	case strings.Contains(value, "学时"):
		return "hours"
	case value == "学分":
		return "credit"
	case strings.Contains(value, "考试方式"):
		return "assessment_method"
	case strings.Contains(value, "成绩标识"):
		return "score_mark"
	case strings.Contains(value, "缓考原因"):
		return "reason"
	case strings.Contains(value, "审核状态"):
		return "status"
	case strings.Contains(value, "申请时间"):
		return "submitted_at"
	case value == "操作":
		return "action"
	default:
		return academicPageField(value)
	}
}

func academicInClassExamField(value string) string {
	value = regexp.MustCompile(`\s+`).ReplaceAllString(value, "")
	switch {
	case strings.Contains(value, "学年学期"):
		return "term"
	case strings.Contains(value, "课程编号") || strings.Contains(value, "课程代码"):
		return "course_id"
	case strings.Contains(value, "课程名称") || value == "课程":
		return "course"
	case strings.Contains(value, "考试周次"):
		return "exam_week"
	case strings.Contains(value, "考试星期"):
		return "exam_weekday"
	case strings.Contains(value, "考试节次"):
		return "exam_section"
	case strings.Contains(value, "监考老师") || strings.Contains(value, "监考教师"):
		return "invigilator"
	case strings.Contains(value, "考试教室") || value == "教室":
		return "room"
	case strings.Contains(value, "考试时间"):
		return "exam_time"
	case strings.Contains(value, "考试类型"):
		return "exam_type"
	default:
		return academicPageField(value)
	}
}

func academicEnrollmentProofField(value string) string {
	value = regexp.MustCompile(`\s+`).ReplaceAllString(value, "")
	switch {
	case value == "学号":
		return "student_id"
	case value == "姓名":
		return "name"
	case value == "性别":
		return "gender"
	case value == "籍贯":
		return "native_place"
	case value == "民族":
		return "ethnicity"
	case strings.Contains(value, "培养层次"):
		return "study_level"
	case strings.Contains(value, "入学日期"):
		return "enrollment_date"
	case strings.Contains(value, "身份证号"):
		return "id_number"
	case strings.Contains(value, "出生日期"):
		return "birth_date"
	case value == "备注":
		return "note"
	case strings.Contains(value, "申请时间"):
		return "applied_at"
	default:
		return academicPageField(value)
	}
}

func academicExemptExamField(value string) string {
	value = regexp.MustCompile(`\s+`).ReplaceAllString(value, "")
	switch {
	case strings.Contains(value, "考试性质"):
		return "exam_nature"
	case strings.Contains(value, "考试状态"):
		return "exam_status"
	case strings.Contains(value, "审核状态"):
		return "review_status"
	case strings.Contains(value, "免考原因") || value == "原因":
		return "reason"
	case strings.Contains(value, "上课院系"):
		return "department"
	case strings.Contains(value, "班级名称"):
		return "class"
	case strings.Contains(value, "课程属性"):
		return "course_attribute"
	case value == "姓名":
		return "name"
	default:
		return academicDeferredExamField(value)
	}
}

func academicEnrollmentStatusChangeField(value string) string {
	value = regexp.MustCompile(`\s+`).ReplaceAllString(value, "")
	switch {
	case strings.Contains(value, "原班级"):
		return "previous_class"
	case strings.Contains(value, "原学籍"):
		return "previous_enrollment_status"
	case strings.Contains(value, "原在校"):
		return "previous_school_status"
	case strings.Contains(value, "新学院"):
		return "new_college"
	case strings.Contains(value, "新专业"):
		return "new_major"
	case strings.Contains(value, "新班级"):
		return "new_class"
	case strings.Contains(value, "新学籍"):
		return "new_enrollment_status"
	case strings.Contains(value, "新状态"):
		return "new_status"
	case strings.Contains(value, "新在校"):
		return "new_school_status"
	case strings.Contains(value, "异动类别"):
		return "change_type"
	case strings.Contains(value, "终审状态"):
		return "final_review_status"
	case value == "详情":
		return "detail"
	default:
		return academicPageField(value)
	}
}

func academicGraduateExamField(value string) string {
	value = regexp.MustCompile(`\s+`).ReplaceAllString(value, "")
	switch {
	case strings.Contains(value, "学年学期"):
		return "term"
	case strings.Contains(value, "课程编号") || strings.Contains(value, "课程代码"):
		return "course_id"
	case strings.Contains(value, "课程名称") || value == "课程":
		return "course"
	case strings.Contains(value, "考试性质"):
		return "exam_nature"
	case strings.Contains(value, "考试方式"):
		return "assessment_method"
	case strings.Contains(value, "考试时间") || strings.Contains(value, "考试日期"):
		return "exam_time"
	case strings.Contains(value, "考场") || strings.Contains(value, "教室"):
		return "room"
	case strings.Contains(value, "校区"):
		return "campus"
	case strings.Contains(value, "报名状态") || strings.Contains(value, "审核状态"):
		return "status"
	case value == "操作":
		return "action"
	default:
		return academicPageField(value)
	}
}

func academicGradeRecognitionField(value string) string {
	value = regexp.MustCompile(`\s+`).ReplaceAllString(value, "")
	switch {
	case strings.Contains(value, "学年学期"):
		return "term"
	case strings.Contains(value, "课程编号") || strings.Contains(value, "课程代码"):
		return "course_id"
	case strings.Contains(value, "课程名称") || value == "课程":
		return "course"
	case value == "学分":
		return "credit"
	case strings.Contains(value, "总学时") || strings.Contains(value, "学时"):
		return "hours"
	case strings.Contains(value, "成绩项目"):
		return "grade_item"
	case strings.Contains(value, "原成绩"):
		return "original_score"
	case strings.Contains(value, "申请时间"):
		return "submitted_at"
	case strings.Contains(value, "审核状态"):
		return "review_status"
	case value == "操作":
		return "action"
	default:
		return academicPageField(value)
	}
}

func academicClassChangeField(value string) string {
	value = regexp.MustCompile(`\s+`).ReplaceAllString(value, "")
	switch {
	case strings.Contains(value, "课程编号") || strings.Contains(value, "课程代码"):
		return "course_id"
	case strings.Contains(value, "课程名称") || value == "课程":
		return "course"
	case strings.Contains(value, "上课教师") || value == "教师":
		return "teacher"
	case strings.Contains(value, "上课班级") || value == "班级":
		return "class"
	case strings.Contains(value, "开课院系"):
		return "department"
	case strings.Contains(value, "调课类型"):
		return "change_type"
	case strings.Contains(value, "调前时间"):
		return "original_time"
	case strings.Contains(value, "调前地点"):
		return "original_room"
	case strings.Contains(value, "调前周次") || strings.Contains(value, "上课周次"):
		return "original_weeks"
	case strings.Contains(value, "调整周次"):
		return "adjustment_weeks"
	case strings.Contains(value, "调后周次"):
		return "new_weeks"
	case strings.Contains(value, "调后时间"):
		return "new_time"
	case strings.Contains(value, "调后地点"):
		return "new_room"
	case strings.Contains(value, "调课方式"):
		return "change_method"
	case value == "状态":
		return "status"
	default:
		return academicPageField(value)
	}
}

func academicDropCourseField(value string) string {
	value = regexp.MustCompile(`\s+`).ReplaceAllString(value, "")
	switch {
	case strings.Contains(value, "课程名称") || value == "课程":
		return "course"
	case strings.Contains(value, "课程编号") || strings.Contains(value, "课程代码"):
		return "course_id"
	case strings.Contains(value, "授课教师") || strings.Contains(value, "教师"):
		return "teacher"
	case strings.Contains(value, "总学时") || strings.Contains(value, "学时"):
		return "hours"
	case value == "学分":
		return "credit"
	case strings.Contains(value, "课程属性"):
		return "course_attribute"
	case strings.Contains(value, "课程性质"):
		return "course_nature"
	case strings.Contains(value, "审核状态"):
		return "status"
	case value == "操作":
		return "action"
	default:
		return academicPageField(value)
	}
}

func academicStudentStatusChangeField(value string) string {
	value = regexp.MustCompile(`\s+`).ReplaceAllString(value, "")
	switch {
	case value == "学号":
		return "student_id"
	case value == "姓名":
		return "name"
	case strings.Contains(value, "修改字段"):
		return "changed_field"
	case strings.Contains(value, "修改信息"):
		return "change_detail"
	case strings.Contains(value, "审核状态"):
		return "review_status"
	case strings.Contains(value, "修改时间"):
		return "modified_at"
	case value == "操作":
		return "action"
	default:
		return academicPageField(value)
	}
}

func academicStatusWarningField(value string) string {
	value = regexp.MustCompile(`\s+`).ReplaceAllString(value, "")
	switch {
	case strings.Contains(value, "预警学期"):
		return "term"
	case strings.Contains(value, "预警名称"):
		return "warning"
	case strings.Contains(value, "预警条件"):
		return "condition"
	case strings.Contains(value, "处理结果"):
		return "result"
	case strings.Contains(value, "提示信息"):
		return "message"
	case strings.Contains(value, "对象名称"):
		return "object"
	case strings.Contains(value, "实际值"):
		return "actual_value"
	default:
		return academicPageField(value)
	}
}

func academicAnnouncementField(value string) string {
	value = regexp.MustCompile(`\s+`).ReplaceAllString(value, "")
	switch {
	case strings.Contains(value, "标题"):
		return "title"
	case strings.Contains(value, "类别"):
		return "category"
	case strings.Contains(value, "发送人"):
		return "sender"
	case strings.Contains(value, "发送时间"):
		return "sent_at"
	case value == "操作":
		return "action"
	default:
		return academicPageField(value)
	}
}

func academicRetakeField(value string) string {
	value = regexp.MustCompile(`\s+`).ReplaceAllString(value, "")
	switch {
	case strings.Contains(value, "是否报名"):
		return "enrolled"
	case strings.Contains(value, "上课院审"):
		return "class_review"
	case strings.Contains(value, "开课院审"):
		return "course_review"
	case strings.Contains(value, "取得资格"):
		return "eligible"
	case value == "学年学期":
		return "term"
	case value == "开课学期":
		return "course_term"
	case value == "课程名称":
		return "course"
	case value == "学时":
		return "hours"
	case value == "学分":
		return "credit"
	case value == "最好成绩":
		return "best_score"
	case strings.Contains(value, "替代课程编号"):
		return "substitute_course_id"
	case strings.Contains(value, "替代课程名称"):
		return "substitute_course"
	case strings.Contains(value, "替代课程学时"):
		return "substitute_hours"
	case strings.Contains(value, "替代课程学分"):
		return "substitute_credit"
	case strings.Contains(value, "是否选课"):
		return "selected"
	case strings.Contains(value, "是否收费"):
		return "fee_required"
	case strings.Contains(value, "是否缴费"):
		return "paid"
	case strings.Contains(value, "重修报名类别"):
		return "registration_type"
	case value == "操作":
		return "action"
	default:
		return ""
	}
}

func academicSelectionWindowField(value string) string {
	value = regexp.MustCompile(`\s+`).ReplaceAllString(value, "")
	switch value {
	case "学年学期":
		return "term"
	case "选课名称":
		return "selection_name"
	case "选课时间":
		return "selection_time"
	case "操作":
		return "action"
	default:
		return academicCourseField(value)
	}
}

func academicCourseField(value string) string {
	value = regexp.MustCompile(`\s+`).ReplaceAllString(value, "")
	switch {
	case strings.Contains(value, "课程代码") || strings.Contains(value, "课程编号") || strings.Contains(value, "课号"):
		return "course_id"
	case strings.Contains(value, "课程名称") || value == "课程" || strings.Contains(value, "科目名称"):
		return "course"
	case strings.Contains(value, "教师") || strings.Contains(value, "老师"):
		return "teacher"
	case strings.Contains(value, "学分"):
		return "credit"
	case strings.Contains(value, "学时"):
		return "hours"
	case strings.Contains(value, "课程性质"):
		return "course_nature"
	case strings.Contains(value, "课程类别") || strings.Contains(value, "类别"):
		return "course_category"
	case strings.Contains(value, "状态"):
		return "status"
	default:
		return ""
	}
}

func (a NativeSite) academicPage(ctx context.Context, method, path string, data []pair, headers []pair) (string, string, *siteError) {
	request := siteRequest{Service: "academic", Path: path, Method: method, CookieFile: academicCookiePath(), Data: data, Headers: headers, RequireLogin: true, Yes: true, ReadOnly: true}
	if method == "GET" {
		request.Data = nil
	}
	result, err := a.executeAcademicRequestWithRecovery(ctx, request)
	if err != nil {
		return "", "", err
	}
	response, ok := result["response"].(map[string]any)
	if !ok {
		return "", "", &siteError{Code: "parse_error", Message: "教务响应不是页面或 JSON"}
	}
	body, _ := response["body"].(string)
	if internal, ok := response["body_internal"].(string); ok {
		body = internal
	}
	pageURL, _ := response["raw_url"].(string)
	if pageURL == "" {
		pageURL, _ = response["url"].(string)
	}
	if pageURL == "" {
		target, _, resolveErr := resolveSite(request)
		if resolveErr != nil {
			return "", "", resolveErr
		}
		pageURL = target.String()
	}
	return body, pageURL, nil
}

func (a NativeSite) academicSchedule(ctx context.Context, args []string) (map[string]any, *siteError) {
	term := flagValue(args, "--term")
	weekText := flagValue(args, "--week")
	week := 0
	if weekText != "" {
		parsed, err := strconv.Atoi(weekText)
		if err != nil || parsed < 1 {
			return nil, &siteError{Code: "invalid_argument", Message: "--week 必须是正整数"}
		}
		week = parsed
	}
	schemeID := flagValue(args, "--scheme-id")
	data := []pair{{"jx0404id", ""}, {"cj0701id", ""}, {"zc", strconv.Itoa(week)}, {"demo", ""}, {"xnxq01id", term}, {"sfFD", "1"}, {"kbjcmsid", schemeID}}
	body, pageURL, err := a.academicPage(ctx, "POST", "/jsxsd/xskb/xskb_list.do", data, nil)
	if err != nil {
		return nil, err
	}
	document, parseErr := parsePage(body)
	if parseErr != nil {
		return nil, &siteError{Code: "parse_error", Message: parseErr.Error()}
	}
	if term == "" {
		term = selectedOptionPage(document, "xnxq01id")
	}
	items, dataErr := parseSchedulePage(document, term)
	if dataErr != nil {
		return nil, dataErr
	}
	if week > 0 {
		filtered := make([]map[string]any, 0)
		for _, item := range items {
			if containsInt(item["weeks"], week) {
				filtered = append(filtered, item)
			}
		}
		items = filtered
	}
	return academicWrap(map[string]any{"term": term, "week": nullableInt(week), "items": items, "url": safeSiteURL(mustParseURL(pageURL))}), nil
}

func (a NativeSite) academicGrades(ctx context.Context, args []string) (map[string]any, *siteError) {
	if len(args) > 0 && args[0] == "detail" {
		path := flagValue(args[1:], "--path")
		if path == "" {
			courseID, courseName := flagValue(args[1:], "--course-id"), flagValue(args[1:], "--course-name")
			if courseID == "" && courseName == "" {
				return nil, &siteError{Code: "invalid_argument", Message: "grades detail 必须提供 --course-id 或 --course-name"}
			}
			term := flagValue(args[1:], "--term")
			rows, _, _, listErr := a.academicGradeRows(ctx, term, flagValue(args[1:], "--course-nature"), "", flagValue(args[1:], "--study-mode-id"), firstNonEmpty(flagValue(args[1:], "--display"), "all"))
			if listErr != nil {
				return nil, listErr
			}
			for _, row := range rows {
				if (courseID != "" && fmt.Sprint(row["course_id"]) == courseID) || (courseName != "" && strings.Contains(fmt.Sprint(row["course"]), courseName)) {
					path = fmt.Sprint(row["grade_detail_url"])
					break
				}
			}
			if path == "" || path == "<nil>" {
				return nil, &siteError{Code: "not_found", Message: "成绩列表中找不到对应课程详情"}
			}
			var pathErr *siteError
			path, pathErr = academicPath(path)
			if pathErr != nil {
				return nil, pathErr
			}
		}
		body, pageURL, err := a.academicPage(ctx, "GET", path, nil, nil)
		if err != nil {
			return nil, err
		}
		document, parseErr := parsePage(body)
		if parseErr != nil {
			return nil, &siteError{Code: "parse_error", Message: parseErr.Error()}
		}
		detail, detailErr := parseGradeDetailPage(document, pageURL)
		if detailErr != nil {
			return nil, detailErr
		}
		return academicWrap(detail), nil
	}
	term, nature, course, display, study := flagValue(args, "--term"), flagValue(args, "--course-nature"), flagValue(args, "--course-name"), flagValue(args, "--display"), flagValue(args, "--study-mode-id")
	items, term, summary, dataErr := a.academicGradeRows(ctx, term, nature, course, study, display)
	if dataErr != nil {
		return nil, dataErr
	}
	return academicWrap(map[string]any{"term": nullableString(term), "summary": summary, "items": items}), nil
}

func (a NativeSite) academicGradeRows(ctx context.Context, term, nature, course, study, display string) ([]map[string]any, string, map[string]string, *siteError) {
	if study == "" {
		study = "2"
	}
	if display == "" {
		display = "all"
	}
	if display != "all" && display != "best" {
		return nil, "", nil, &siteError{Code: "invalid_argument", Message: "--display 只能是 all 或 best"}
	}
	_, queryURL, queryErr := a.academicPage(ctx, "GET", "/jsxsd/kscj/cjcx_query", nil, nil)
	if queryErr != nil {
		return nil, "", nil, queryErr
	}
	data := []pair{{"kksj", term}, {"kcxz", nature}, {"kcmc", course}, {"xsfs", map[bool]string{true: "max", false: "all"}[display == "best"]}, {"fxkc", study}}
	body, pageURL, err := a.academicPage(ctx, "POST", "/jsxsd/kscj/cjcx_list", data, []pair{{"Referer", queryURL}})
	if err != nil {
		return nil, "", nil, err
	}
	document, parseErr := parsePage(body)
	if parseErr != nil {
		return nil, "", nil, &siteError{Code: "parse_error", Message: parseErr.Error()}
	}
	items, dataErr := parseGradesPage(document, pageURL)
	if dataErr != nil {
		return nil, "", nil, dataErr
	}
	return items, term, parseGradeSummary(document), nil
}

func parseGradeSummary(document *pageNode) map[string]string {
	text := pageDisplayText(document)
	patterns := map[string]*regexp.Regexp{
		"earned_credit":           regexp.MustCompile(`已获得总学分\s*[:：]\s*([0-9]+(?:\.[0-9]+)?)`),
		"required_credit":         regexp.MustCompile(`其中必修\s*([0-9]+(?:\.[0-9]+)?)`),
		"general_elective_credit": regexp.MustCompile(`公选\s*([0-9]+(?:\.[0-9]+)?)`),
		"elective_credit":         regexp.MustCompile(`选修\s*([0-9]+(?:\.[0-9]+)?)`),
		"average_grade_point":     regexp.MustCompile(`平均学分绩点\s*[:：]\s*([0-9]+(?:\.[0-9]+)?)`),
		"average_score":           regexp.MustCompile(`平均成绩\s*[:：]\s*([0-9]+(?:\.[0-9]+)?)`),
	}
	result := make(map[string]string, len(patterns))
	for name, pattern := range patterns {
		if match := pattern.FindStringSubmatch(text); len(match) > 1 {
			result[name] = match[1]
		}
	}
	return result
}

func academicPath(value string) (string, *siteError) {
	parsed, err := url.Parse(value)
	if err != nil || parsed.User != nil {
		return "", &siteError{Code: "invalid_path", Message: "成绩详情地址无效"}
	}
	path := parsed.EscapedPath()
	if path == "" {
		path = "/"
	}
	if parsed.RawQuery != "" {
		path += "?" + parsed.RawQuery
	}
	return path, nil
}

func (a NativeSite) academicExams(ctx context.Context, args []string) (map[string]any, *siteError) {
	term := flagValue(args, "--term")
	queryBody, queryURL, err := a.academicPage(ctx, "GET", "/jsxsd/xsks/xsksap_query", nil, nil)
	if err != nil {
		return nil, err
	}
	queryDoc, parseErr := parsePage(queryBody)
	if parseErr != nil {
		return nil, &siteError{Code: "parse_error", Message: parseErr.Error()}
	}
	if term == "" {
		term = selectedOptionPage(queryDoc, "xnxqid")
	}
	data := []pair{{"xqlbmc", flagValue(args, "--category")}, {"xnxqid", term}, {"xqlb", flagValue(args, "--category-id")}}
	body, pageURL, err := a.academicPage(ctx, "POST", "/jsxsd/xsks/xsksap_list", data, []pair{{"Referer", queryURL}})
	if err != nil {
		return nil, err
	}
	document, parseErr := parsePage(body)
	if parseErr != nil {
		return nil, &siteError{Code: "parse_error", Message: parseErr.Error()}
	}
	items, dataErr := parseExamPage(document, pageURL)
	if dataErr != nil {
		return nil, dataErr
	}
	return academicWrap(map[string]any{
		"kind": "exams", "path": "/jsxsd/xsks/xsksap_list", "term": nullableString(term),
		"items": items, "item_count": len(items), "url": safeSiteURL(mustParseURL(pageURL)),
	}), nil
}

func (a NativeSite) academicInClassExams(ctx context.Context, args []string) (map[string]any, *siteError) {
	const queryPath = "/jsxsd/xsks/xsstk_query"
	const listPath = "/jsxsd/xsks/xsstk_list"
	queryBody, queryURL, err := a.academicPage(ctx, "GET", queryPath, nil, nil)
	if err != nil {
		return nil, err
	}
	queryDocument, parseErr := parsePage(queryBody)
	if parseErr != nil {
		return nil, &siteError{Code: "parse_error", Message: parseErr.Error()}
	}
	term := strings.TrimSpace(flagValue(args, "--term"))
	if term == "" {
		term = selectedOptionPage(queryDocument, "xnxqid")
	}
	examType, examTypeID, optionErr := academicNamedPageOption(queryDocument, "xqlb", flagValue(args, "--exam-type"), "考试类型")
	if optionErr != nil {
		return nil, optionErr
	}
	body, pageURL, err := a.academicPage(ctx, "POST", listPath, []pair{
		{"xnxqid", term}, {"xqlb", examTypeID}, {"xqlbmc", examType},
	}, []pair{{"Referer", queryURL}})
	if err != nil {
		return nil, err
	}
	document, parseErr := parsePage(body)
	if parseErr != nil {
		return nil, &siteError{Code: "parse_error", Message: parseErr.Error()}
	}
	items := academicStructuredRows(document, "", academicInClassExamField)
	if len(items) == 0 && !noAcademicData(document) && len(document.findAll("table")) == 0 {
		return nil, &siteError{Code: "parse_error", Message: "随堂考查询未包含可解析表格；请使用 web get 查看页面结构"}
	}
	page, pageErr := pageInspect(body, pageURL)
	if pageErr != nil {
		return nil, pageErr
	}
	return academicWrap(map[string]any{
		"kind": "in-class-exams", "path": listPath, "term": nullableString(term), "exam_type": nullableString(examType),
		"items": items, "item_count": len(items), "page": page,
	}), nil
}

func (a NativeSite) academicClassrooms(ctx context.Context, args []string) (map[string]any, *siteError) {
	campus := strings.ToLower(flagValue(args, "--campus"))
	campusID := map[string]string{"yuntang": "1", "云塘": "1", "1": "1", "jinpenling": "2", "金盆岭": "2", "2": "2"}[campus]
	if campusID == "" {
		return nil, &siteError{Code: "invalid_argument", Message: "--campus 只能是 yuntang、jinpenling、1 或 2"}
	}
	week, weekday, section := flagInt(args, "--week"), flagInt(args, "--weekday"), flagInt(args, "--section")
	if week < 1 || weekday < 1 || weekday > 7 || section < 1 || section > 5 {
		return nil, &siteError{Code: "invalid_argument", Message: "--week、--weekday、--section 参数范围无效"}
	}
	sectionStart := []string{"", "01", "03", "05", "07", "09"}[section]
	sectionEnd := []string{"", "02", "04", "06", "08", "10"}[section]
	data := []pair{{"xnxqh", flagValue(args, "--term")}, {"skyx", flagValue(args, "--department")}, {"xqid", campusID}, {"jzwid", flagValue(args, "--building")}, {"gnq", flagValue(args, "--area")}, {"skjsid", ""}, {"skjs", ""}, {"zc1", strconv.Itoa(week)}, {"zc2", strconv.Itoa(week)}, {"skxq1", strconv.Itoa(weekday)}, {"skxq2", strconv.Itoa(weekday)}, {"jc1", sectionStart}, {"jc2", sectionEnd}}
	body, pageURL, err := a.academicPage(ctx, "POST", "/jsxsd/kbcx/kbxx_classroom_ifr", data, nil)
	if err != nil {
		return nil, err
	}
	document, parseErr := parsePage(body)
	if parseErr != nil {
		return nil, &siteError{Code: "parse_error", Message: parseErr.Error()}
	}
	items, dataErr := parseClassroomPage(document, pageURL)
	if dataErr != nil {
		return nil, dataErr
	}
	return academicWrap(map[string]any{"campus": campusID, "week": week, "weekday": weekday, "section": section, "items": items}), nil
}

func (a NativeSite) academicSelections(ctx context.Context, args []string) (map[string]any, *siteError) {
	term := flagValue(args, "--term")
	queryBody, queryURL, err := a.academicPage(ctx, "GET", "/jsxsd/xkgl/xsxkjgcx", nil, nil)
	if err != nil {
		return nil, err
	}
	queryDoc, parseErr := parsePage(queryBody)
	if parseErr != nil {
		return nil, &siteError{Code: "parse_error", Message: parseErr.Error()}
	}
	if term == "" {
		term = selectedOptionPage(queryDoc, "xnxqid")
	}
	body, pageURL, err := a.academicPage(ctx, "POST", "/jsxsd/xkgl/loadXsxkjgList", []pair{{"xnxqid", term}}, []pair{{"Referer", queryURL}})
	if err != nil {
		return nil, err
	}
	document, parseErr := parsePage(body)
	if parseErr != nil {
		return nil, &siteError{Code: "parse_error", Message: parseErr.Error()}
	}
	items, dataErr := parseSelectionPage(document, pageURL)
	if dataErr != nil {
		return nil, dataErr
	}
	return academicWrap(map[string]any{"term": term, "items": items}), nil
}

func (a NativeSite) academicTerms(ctx context.Context, args []string) (map[string]any, *siteError) {
	scope := flagValue(args, "--scope")
	paths := map[string][2]string{"schedule": {"/jsxsd/xskb/xskb_list.do", "xnxq01id"}, "grades": {"/jsxsd/kscj/cjcx_query", "kksj"}, "exams": {"/jsxsd/xsks/xsksap_query", "xnxqid"}, "selection": {"/jsxsd/xkgl/xsxkjgcx", "xnxqid"}, "semester-start": {"/jsxsd/jxzl/jxzl_query", "xnxq01id"}}
	item, ok := paths[scope]
	if !ok {
		return nil, &siteError{Code: "invalid_argument", Message: "--scope 无效"}
	}
	body, pageURL, err := a.academicPage(ctx, "GET", item[0], nil, nil)
	if err != nil {
		return nil, err
	}
	document, parseErr := parsePage(body)
	if parseErr != nil {
		return nil, &siteError{Code: "parse_error", Message: parseErr.Error()}
	}
	options := pageOptions(document, item[1])
	return academicWrap(map[string]any{"scope": scope, "url": pageURL, "terms": options, "selected": selectedOptionPage(document, item[1])}), nil
}

func (a NativeSite) academicSemesterStart(ctx context.Context, args []string) (map[string]any, *siteError) {
	term := flagValue(args, "--term")
	queryBody, queryURL, err := a.academicPage(ctx, "GET", "/jsxsd/jxzl/jxzl_query", nil, nil)
	if err != nil {
		return nil, err
	}
	queryDoc, parseErr := parsePage(queryBody)
	if parseErr != nil {
		return nil, &siteError{Code: "parse_error", Message: parseErr.Error()}
	}
	if term == "" {
		term = selectedOptionPage(queryDoc, "xnxq01id")
	}
	body, pageURL, err := a.academicPage(ctx, "POST", "/jsxsd/jxzl/jxzl_query", []pair{{"xnxq01id", term}}, []pair{{"Referer", queryURL}})
	if err != nil {
		return nil, err
	}
	document, parseErr := parsePage(body)
	if parseErr != nil {
		return nil, &siteError{Code: "parse_error", Message: parseErr.Error()}
	}
	result, dataErr := parseSemesterStartPage(document, pageURL)
	if dataErr != nil {
		return nil, dataErr
	}
	result["term"] = term
	return academicWrap(result), nil
}

func (a NativeSite) academicTeachingCalendar(ctx context.Context, args []string) (map[string]any, *siteError) {
	const path = "/jsxsd/jxzl/jxzl_query"
	term := strings.TrimSpace(flagValue(args, "--term"))
	queryBody, queryURL, err := a.academicPage(ctx, "GET", path, nil, nil)
	if err != nil {
		return nil, err
	}
	queryDocument, parseErr := parsePage(queryBody)
	if parseErr != nil {
		return nil, &siteError{Code: "parse_error", Message: parseErr.Error()}
	}
	if term == "" {
		term = selectedOptionPage(queryDocument, "xnxq01id")
	}
	body, pageURL := queryBody, queryURL
	if term != selectedOptionPage(queryDocument, "xnxq01id") {
		body, pageURL, err = a.academicPage(ctx, "POST", path, []pair{{"xnxq01id", term}}, []pair{{"Referer", queryURL}})
		if err != nil {
			return nil, err
		}
	}
	document, parseErr := parsePage(body)
	if parseErr != nil {
		return nil, &siteError{Code: "parse_error", Message: parseErr.Error()}
	}
	items, dataErr := parseTeachingCalendarPage(document)
	if dataErr != nil {
		return nil, dataErr
	}
	page, pageErr := pageInspect(body, pageURL)
	if pageErr != nil {
		return nil, pageErr
	}
	return academicWrap(map[string]any{"kind": "teaching-calendar", "path": path, "term": nullableString(term), "items": items, "item_count": len(items), "page": page}), nil
}

func academicWrap(value map[string]any) map[string]any {
	result := map[string]any{"ok": true, "submitted": false, "confirmed": true, "evidence": "confirmed"}
	for key, item := range value {
		result[key] = item
	}
	return result
}
func mustJSON(value map[string]any) []byte {
	encoded, _ := jsonMarshal(stripSiteInternal(value).(map[string]any))
	return append(encoded, '\n')
}
func jsonMarshal(value any) ([]byte, error) { return json.Marshal(value) }
func nullableInt(value int) any {
	if value == 0 {
		return nil
	}
	return value
}
func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}
func flagValue(args []string, flag string) string {
	for index, value := range args {
		if value == flag && index+1 < len(args) {
			return args[index+1]
		}
		if strings.HasPrefix(value, flag+"=") {
			return strings.TrimPrefix(value, flag+"=")
		}
	}
	return ""
}
func flagInt(args []string, flag string) int {
	value, _ := strconv.Atoi(flagValue(args, flag))
	return value
}
func containsInt(value any, wanted int) bool {
	values, ok := value.([]int)
	if !ok {
		if list, ok := value.([]any); ok {
			for _, item := range list {
				if number, ok := item.(int); ok && number == wanted {
					return true
				}
			}
		}
		return false
	}
	for _, item := range values {
		if item == wanted {
			return true
		}
	}
	return false
}

func directTableRows(table *pageNode) []*pageNode {
	if table == nil {
		return nil
	}
	result := make([]*pageNode, 0)
	for _, row := range table.findAll("tr") {
		if row.parent != nil && row.parent.tag == "tr" {
			continue
		}
		if len(directCells(row)) > 0 {
			result = append(result, row)
		}
	}
	return result
}
func directCells(row *pageNode) []*pageNode {
	result := make([]*pageNode, 0)
	if row == nil {
		return result
	}
	for _, child := range row.children {
		if child.tag == "td" || child.tag == "th" {
			result = append(result, child)
		}
	}
	return result
}
func academicTable(document *pageNode, ids ...string) *pageNode {
	for _, id := range ids {
		if table := document.first("table", id); table != nil {
			return table
		}
	}
	tables := document.findAll("table")
	if len(tables) == 1 {
		return tables[0]
	}
	for _, table := range tables {
		if len(directTableRows(table)) > 0 {
			return table
		}
	}
	return nil
}
func rowValues(row *pageNode) []string {
	values := make([]string, 0)
	for _, cell := range directCells(row) {
		values = append(values, pageDisplayText(cell))
	}
	return values
}
func noAcademicData(document *pageNode) bool {
	return strings.Contains(pageDisplayText(document), "未查询到数据")
}
func pageOptions(document *pageNode, id string) []map[string]any {
	selectNode := document.first("select", id)
	if selectNode == nil {
		for _, node := range document.findAll("select") {
			if node.attr("name") == id {
				selectNode = node
				break
			}
		}
	}
	if selectNode == nil || selectNode.disabled() {
		return []map[string]any{}
	}
	result := make([]map[string]any, 0)
	for _, option := range selectNode.findAll("option") {
		if pageDisplayText(option) == "" && option.attr("value") == "" {
			continue
		}
		result = append(result, map[string]any{"value": pageOptionValue(option), "label": pageDisplayText(option), "selected": option.has("selected"), "disabled": option.disabled()})
	}
	return result
}
func selectedOptionPage(document *pageNode, id string) string {
	for _, item := range pageOptions(document, id) {
		if item["selected"] == true {
			if value, ok := item["value"].(string); ok {
				return strings.TrimSpace(value)
			}
		}
	}
	options := pageOptions(document, id)
	for _, item := range options {
		if item["disabled"] != true {
			if value, ok := item["value"].(string); ok {
				return strings.TrimSpace(value)
			}
		}
	}
	return ""
}

func parseProfilePage(source, pageURL string) map[string]any {
	document, _ := parsePage(source)
	table := academicTable(document, "xjkpTable", "xjxxTable")
	rows := make([][]string, 0)
	fields := make([]map[string]string, 0)
	values := map[string]string{}
	semantic := map[string]string{}
	if table == nil {
		return map[string]any{"kind": "profile", "url": pageURL, "fields": fields, "values": values, "semantic": semantic, "rows": rows}
	}
	for _, row := range directTableRows(table) {
		items := rowValues(row)
		rows = append(rows, items)
		for _, value := range items {
			if name, item, ok := colonField(value); ok {
				fields = append(fields, map[string]string{"name": name, "value": item})
				values[name] = item
				if semanticName := profileSemanticField(name); semanticName != "" {
					semantic[semanticName] = item
				}
			}
		}
		for index := 0; index+1 < len(items); index += 2 {
			label := strings.Trim(strings.TrimSpace(items[index]), "：:")
			item := strings.TrimSpace(items[index+1])
			if label != "" && len(label) <= 40 && !strings.ContainsAny(label, "：:") && item != "" {
				fields = append(fields, map[string]string{"name": label, "value": item})
				values[label] = item
				if semanticName := profileSemanticField(label); semanticName != "" {
					if _, exists := semantic[semanticName]; !exists {
						semantic[semanticName] = item
					}
				}
			}
		}
	}
	return map[string]any{"kind": "profile", "url": safeSiteURL(mustParseURL(pageURL)), "fields": fields, "values": values, "semantic": semantic, "rows": rows}
}

func profileSemanticField(value string) string {
	return map[string]string{
		"院系": "college", "专业": "major", "学制": "duration_years", "班级": "class", "学号": "student_id",
		"姓名": "name", "性别": "gender", "姓名拼音": "name_pinyin", "出生日期": "birth_date", "本人电话": "phone",
		"民族": "ethnicity", "学习层次": "study_level", "入学日期": "enrollment_date", "入学考号": "entrance_exam_id",
		"身份证编号": "id_number", "备注": "note",
	}[strings.Join(strings.Fields(value), "")]
}

func parseGraduationConclusionPage(source, pageURL string) map[string]any {
	profile := parseProfilePage(source, pageURL)
	values, _ := profile["values"].(map[string]string)
	return map[string]any{
		"kind":                  "graduation-conclusion",
		"url":                   profile["url"],
		"name":                  graduationField(values, "姓名"),
		"enrollment_year":       graduationField(values, "入学年份"),
		"major":                 graduationField(values, "上课专业", "专业"),
		"graduation_conclusion": graduationField(values, "毕业结论"),
		"degree_conclusion":     graduationField(values, "学位结论"),
		"fields":                profile["fields"],
	}
}

func parseGraduationInfoCheckPage(document *pageNode, pageURL string) map[string]any {
	labels := map[string]string{
		"所属学院": "college",
		"所属专业": "major",
		"所在班级": "class",
		"培养层次": "study_level",
		"学制":   "duration_years",
		"性别":   "gender",
		"证件类型": "id_type",
		"证件号":  "id_number",
		"学号":   "student_id",
		"姓名":   "name",
		"姓名拼音": "name_pinyin",
	}
	fields := map[string]string{}
	statusMessage := ""
	for _, table := range document.findAll("table") {
		for _, row := range directTableRows(table) {
			values := rowValues(row)
			for _, value := range values {
				if strings.Contains(value, "注：") || strings.Contains(value, "注:") {
					statusMessage = strings.TrimSpace(value)
				}
			}
			for index := 0; index+1 < len(values); index += 2 {
				label := strings.Join(strings.Fields(strings.Trim(values[index], " ：:")), "")
				if name, ok := labels[label]; ok {
					fields[name] = strings.TrimSpace(values[index+1])
				}
			}
		}
	}
	result := map[string]any{
		"kind":           "graduation-info-check",
		"url":            safeSiteURL(mustParseURL(pageURL)),
		"fields":         fields,
		"status_message": nullableString(statusMessage),
		"text":           pageDisplayText(document),
	}
	for name, value := range fields {
		result[name] = value
	}
	return result
}

func graduationField(values map[string]string, names ...string) string {
	for key, value := range values {
		key = strings.Join(strings.Fields(key), "")
		for _, name := range names {
			if key == strings.Join(strings.Fields(name), "") {
				return value
			}
		}
	}
	return ""
}

func colonField(value string) (string, string, bool) {
	for index, delimiter := range value {
		if delimiter != ':' && delimiter != '：' {
			continue
		}
		name := strings.TrimSpace(value[:index])
		if name == "" {
			return "", "", false
		}
		return name, strings.TrimSpace(value[index+len(string(delimiter)):]), true
	}
	return "", "", false
}

func parseSchedulePage(document *pageNode, term string) ([]map[string]any, *siteError) {
	table := academicTable(document, "kbtable")
	if table == nil {
		if noAcademicData(document) {
			return []map[string]any{}, nil
		}
		return nil, &siteError{Code: "parse_error", Message: "未找到课表"}
	}
	rows := directTableRows(table)
	days := append([]string(nil), academicDayNames...)
	if len(rows) > 0 {
		found := make([]string, 0)
		for _, cell := range directCells(rows[0]) {
			if containsString(academicDayNames, pageDisplayText(cell)) {
				found = append(found, pageDisplayText(cell))
			}
		}
		if len(found) == 7 {
			days = found
		}
	}
	result := make([]map[string]any, 0)
	seen := map[string]bool{}
	for _, row := range rows[1:] {
		cells := directCells(row)
		if len(cells) == len(days)+1 && len(cells[0].findAll("div")) == 0 {
			cells = cells[1:]
		}
		sectionText := ""
		for _, header := range directCells(row) {
			if header.tag == "th" {
				sectionText = pageDisplayText(header)
				break
			}
		}
		for day, cell := range cells {
			if day >= len(days) {
				break
			}
			for _, item := range parseScheduleCellPage(cell, sectionText) {
				item["term"] = term
				item["weekday"] = days[day]
				key := fmt.Sprintf("%v|%v|%v|%v|%v|%v", item["course"], item["teacher"], item["room"], item["weekday"], item["weeks"], item["sections"])
				if !seen[key] {
					seen[key] = true
					result = append(result, item)
				}
			}
		}
	}
	return result, nil
}
func parseScheduleCellPage(cell *pageNode, fallback string) []map[string]any {
	result := make([]map[string]any, 0)
	for _, content := range cell.findAll("div") {
		class := " " + content.attr("class") + " "
		if !strings.Contains(class, " kbcontent ") && !strings.Contains(class, " kbcontent1 ") {
			continue
		}
		lines := pageBlockLines(content)
		if len(lines) == 0 {
			continue
		}
		teacher, room, spec := "", "", ""
		for _, node := range content.findAll("font") {
			switch node.attr("title") {
			case "老师":
				teacher = pageDisplayText(node)
			case "教室":
				room = pageDisplayText(node)
			case "周次(节次)":
				spec = pageDisplayText(node)
			}
		}
		weeks, sections := parseWeekSpecPage(spec, fallback)
		if len(weeks) == 0 {
			continue
		}
		result = append(result, map[string]any{"course": lines[0], "teacher": teacher, "room": room, "weeks": weeks, "sections": sections})
	}
	return result
}
func pageBlockLines(node *pageNode) []string {
	lines := make([]string, 0)
	var current strings.Builder
	var visit func(*pageNode)
	flush := func() {
		if value := strings.TrimSpace(strings.ReplaceAll(current.String(), "\u00a0", " ")); value != "" {
			lines = append(lines, value)
		}
		current.Reset()
	}
	visit = func(item *pageNode) {
		if item.tag == "#text" {
			current.WriteString(item.text)
			return
		}
		if item.tag == "script" || item.tag == "style" {
			return
		}
		if item.tag == "br" {
			flush()
			return
		}
		for _, child := range item.children {
			visit(child)
		}
		if item.tag == "div" || item.tag == "p" {
			flush()
		}
	}
	visit(node)
	flush()
	return lines
}
func parseWeekSpecPage(value, fallback string) ([]int, []int) {
	value = strings.ReplaceAll(strings.ReplaceAll(value, " ", ""), "，", ",")
	match := regexp.MustCompile(`(.+?)[(（](单周|双周|周)[)）](?:\[([^]]*)\])?`).FindStringSubmatch(value)
	if len(match) == 0 {
		return []int{}, parseIntsPage(fallback)
	}
	weeks := expandWeekRangePage(match[1], match[2])
	sections := parseIntsPage(fallback)
	if match[3] != "" {
		sections = parseIntsPage(match[3])
	}
	return weeks, sections
}
func parseIntsPage(value string) []int {
	numbers := regexp.MustCompile(`\d+`).FindAllString(value, -1)
	result := make([]int, 0)
	for _, item := range numbers {
		parsed, _ := strconv.Atoi(item)
		result = append(result, parsed)
	}
	return result
}
func expandWeekRangePage(value, parity string) []int {
	result := make([]int, 0)
	for _, part := range strings.Split(value, ",") {
		numbers := parseIntsPage(part)
		if len(numbers) == 1 {
			result = append(result, numbers[0])
		}
		if len(numbers) >= 2 {
			for item := numbers[0]; item <= numbers[1]; item++ {
				result = append(result, item)
			}
		}
	}
	filtered := make([]int, 0)
	seen := map[int]bool{}
	for _, item := range result {
		if parity == "单周" && item%2 == 0 || parity == "双周" && item%2 != 0 || seen[item] {
			continue
		}
		seen[item] = true
		filtered = append(filtered, item)
	}
	sort.Ints(filtered)
	return filtered
}
func parseGradesPage(document *pageNode, pageURL string) ([]map[string]any, *siteError) {
	table := academicTable(document, "dataList")
	if table == nil {
		if noAcademicData(document) {
			return []map[string]any{}, nil
		}
		return nil, &siteError{Code: "parse_error", Message: "未找到成绩表"}
	}
	rows := directTableRows(table)
	if len(rows) == 0 {
		return []map[string]any{}, nil
	}
	header := rowValues(rows[0])
	hasHeader := len(directCells(rows[0])) > 0 && (directCells(rows[0])[0].tag == "th" || countGradeHeaders(header) >= 2 || (len(header) > 0 && (header[0] == "序号" || header[0] == "序")))
	result := make([]map[string]any, 0)
	for index, row := range rows {
		if index == 0 && hasHeader {
			continue
		}
		values := rowValues(row)
		if len(values) < 6 || values[0] == "序号" || values[0] == "序" || allEmpty(values) {
			continue
		}
		item := map[string]any{"cells": values}
		if hasHeader {
			for column, title := range header {
				if field := gradeHeaderPage(title); field != "" && column < len(values) {
					item[field] = values[column]
				}
			}
		} else if len(values) >= 20 {
			names := []string{"semester", "course_id", "course", "group", "score", "score_mark", "credit", "hours", "grade_point", "general_elective", "original_score", "description", "note", "retake_semester", "assessment_method", "exam_type", "course_attribute", "course_nature", "course_category"}
			for column, name := range names {
				if column+1 < len(values) {
					item[name] = values[column+1]
				}
			}
		} else {
			item["course"], item["score"] = values[0], values[len(values)-1]
		}
		if anchor := row.first("a", ""); anchor != nil {
			if target := academicLinkTarget(anchor, pageURL); target != "" {
				item["grade_detail_url"] = safeSiteURL(mustParseURL(target))
			}
		}
		result = append(result, item)
	}
	return result, nil
}

func academicLinkTarget(link *pageNode, pageURL string) string {
	if link == nil {
		return ""
	}
	href := link.attr("href")
	if href == "" {
		return ""
	}
	candidates := []string{href}
	if strings.HasPrefix(strings.ToLower(href), "javascript:") {
		candidates = candidates[:0]
		for _, source := range []string{href, link.attr("onclick")} {
			for _, match := range pageEndpointLiteral.FindAllStringSubmatch(source, -1) {
				if len(match) > 1 {
					candidates = append(candidates, match[1])
				}
			}
		}
	}
	for _, candidate := range candidates {
		if target := resolvePageURL(pageURL, candidate); target != "" {
			if _, pathErr := academicPath(target); pathErr == nil {
				return target
			}
		}
	}
	return ""
}

func gradeHeaderPage(value string) string {
	value = regexp.MustCompile(`\s+`).ReplaceAllString(value, "")
	switch {
	case strings.Contains(value, "原始成绩"):
		return "original_score"
	case strings.Contains(value, "重修学期") || strings.Contains(value, "补考学期"):
		return "retake_semester"
	case strings.Contains(value, "学期"):
		return "semester"
	case strings.Contains(value, "课程代码") || strings.Contains(value, "课程编号") || strings.Contains(value, "课号"):
		return "course_id"
	case value == "课程" || strings.Contains(value, "课程名称") || strings.Contains(value, "科目名称"):
		return "course"
	case strings.Contains(value, "成绩") || strings.Contains(value, "分数") || strings.Contains(value, "得分"):
		return "score"
	case strings.Contains(value, "修读方式") || strings.Contains(value, "学习方式"):
		return "study_mode"
	case strings.Contains(value, "学分"):
		return "credit"
	case strings.Contains(value, "学时"):
		return "hours"
	case strings.Contains(value, "绩点"):
		return "grade_point"
	case strings.Contains(value, "课程性质"):
		return "course_nature"
	case strings.Contains(value, "课程属性"):
		return "course_attribute"
	case strings.Contains(value, "课程类别"):
		return "course_category"
	case strings.Contains(value, "考核方式") || strings.Contains(value, "考试方式"):
		return "assessment_method"
	}
	return ""
}
func countGradeHeaders(values []string) int {
	count := 0
	for _, value := range values {
		if gradeHeaderPage(value) != "" {
			count++
		}
	}
	return count
}
func parseGradeDetailPage(document *pageNode, pageURL string) (map[string]any, *siteError) {
	table := academicTable(document, "dataList")
	if table == nil {
		return nil, &siteError{Code: "parse_error", Message: "未找到成绩详情表"}
	}
	rows := directTableRows(table)
	if len(rows) < 2 {
		return nil, &siteError{Code: "parse_error", Message: "成绩详情表行数不足"}
	}
	headers, values := rowValues(rows[0]), rowValues(rows[1])
	fields := map[string]string{}
	for index, header := range headers {
		if header != "" && index < len(values) {
			fields[header] = values[index]
		}
	}
	return map[string]any{"url": safeSiteURL(mustParseURL(pageURL)), "fields": fields, "headers": headers, "cells": values, "text": pageDisplayText(rows[1])}, nil
}
func parseExamPage(document *pageNode, pageURL string) ([]map[string]any, *siteError) {
	table := academicTable(document, "dataList")
	if table == nil {
		if noAcademicData(document) {
			return []map[string]any{}, nil
		}
		return nil, &siteError{Code: "parse_error", Message: "未找到考试安排表"}
	}
	result := make([]map[string]any, 0)
	for _, row := range directTableRows(table) {
		values := rowValues(row)
		if len(values) < 10 || values[0] == "序号" || values[0] == "课程代码" || values[0] == "考试安排" {
			continue
		}
		item := map[string]any{"index": len(result) + 1, "sequence": values[0], "campus": valueAt(values, 1), "session": valueAt(values, 2), "course_id": valueAt(values, 3), "course": valueAt(values, 4), "teacher": valueAt(values, 5), "exam_time": valueAt(values, 6), "room": valueAt(values, 7), "seat": valueAt(values, 8), "admission_ticket": valueAt(values, 9), "remarks": valueAt(values, 10), "cells": values, "text": pageDisplayText(row)}
		if match := regexp.MustCompile(`(\d{4}[-/]\d{1,2}[-/]\d{1,2})\s+(\d{1,2}:\d{2})\s*[~～-]\s*(\d{1,2}:\d{2})`).FindStringSubmatch(valueAt(values, 6)); len(match) > 3 {
			item["date"], item["start_time"], item["end_time"] = match[1], match[2], match[3]
		}
		result = append(result, item)
	}
	_ = pageURL
	return result, nil
}
func parseClassroomPage(document *pageNode, pageURL string) ([]map[string]any, *siteError) {
	table := academicTable(document, "kbtable")
	if table == nil {
		if noAcademicData(document) {
			return []map[string]any{}, nil
		}
		return nil, &siteError{Code: "parse_error", Message: "未找到空闲教室表"}
	}
	result := make([]map[string]any, 0)
	for _, row := range directTableRows(table) {
		values := rowValues(row)
		if len(values) < 2 || containsString([]string{"教室", "教学楼", "教室名称"}, values[0]) {
			continue
		}
		occupied := false
		for _, value := range values[1:] {
			if strings.TrimSpace(value) != "" {
				occupied = true
				break
			}
		}
		if !occupied {
			result = append(result, map[string]any{"index": len(result) + 1, "room": values[0], "available": true, "cells": values, "text": pageDisplayText(row)})
		}
	}
	_ = pageURL
	return result, nil
}
func parseSelectionPage(document *pageNode, pageURL string) ([]map[string]any, *siteError) {
	table := academicTable(document, "dataList")
	if table == nil {
		if noAcademicData(document) {
			return []map[string]any{}, nil
		}
		return nil, &siteError{Code: "parse_error", Message: "未找到选课结果表"}
	}
	result := make([]map[string]any, 0)
	for _, row := range directTableRows(table) {
		values := rowValues(row)
		if len(values) < 8 || values[0] == "序号" {
			continue
		}
		result = append(result, map[string]any{"index": len(result) + 1, "sequence": values[0], "course": valueAt(values, 1), "course_id": valueAt(values, 2), "teacher": valueAt(values, 3), "hours": valueAt(values, 4), "credit": valueAt(values, 5), "course_attribute": valueAt(values, 6), "course_nature": valueAt(values, 7), "cells": values, "text": pageDisplayText(row)})
	}
	_ = pageURL
	return result, nil
}
func parseSemesterStartPage(document *pageNode, pageURL string) (map[string]any, *siteError) {
	table := academicTable(document, "kbtable")
	if table == nil {
		return nil, &siteError{Code: "parse_error", Message: "未找到学期起始日表"}
	}
	for _, row := range directTableRows(table) {
		values := rowValues(row)
		start := ""
		cells := directCells(row)
		if len(cells) > 1 {
			start = cells[1].attr("title")
		}
		if start == "" {
			start = regexp.MustCompile(`\d{4}年\d{1,2}月\d{1,2}日?`).FindString(strings.Join(values, " "))
		}
		if start != "" {
			return map[string]any{"url": safeSiteURL(mustParseURL(pageURL)), "start_date": start, "cells": values, "text": pageDisplayText(row)}, nil
		}
	}
	return nil, &siteError{Code: "parse_error", Message: "未找到学期起始日"}
}

func parseTeachingCalendarPage(document *pageNode) ([]map[string]any, *siteError) {
	table := academicTable(document)
	if table == nil {
		return nil, &siteError{Code: "parse_error", Message: "未找到教学周历表"}
	}
	rows := directTableRows(table)
	if len(rows) == 0 {
		return []map[string]any{}, nil
	}
	if strings.Contains(pageDisplayText(rows[0]), "星期日") {
		rows = rows[1:]
	}
	items := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		values := rowValues(row)
		if len(values) < 8 || !regexp.MustCompile(`^\d+$`).MatchString(strings.TrimSpace(values[0])) {
			continue
		}
		item := map[string]any{
			"week":      values[0],
			"sunday":    values[1],
			"monday":    values[2],
			"tuesday":   values[3],
			"wednesday": values[4],
			"thursday":  values[5],
			"friday":    values[6],
			"saturday":  values[7],
			"note":      valueAt(values, 8),
			"cells":     values,
			"text":      pageDisplayText(row),
		}
		items = append(items, item)
	}
	return items, nil
}

func valueAt(values []string, index int) string {
	if index >= 0 && index < len(values) {
		return values[index]
	}
	return ""
}
func allEmpty(values []string) bool {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return false
		}
	}
	return true
}
func containsString(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func renderAcademic(result map[string]any) string {
	if fields, ok := result["fields"].(map[string]string); ok {
		var builder strings.Builder
		for name, value := range fields {
			builder.WriteString(fmt.Sprintf("%s: %s\n", name, value))
		}
		return builder.String()
	}
	if start, ok := result["start_date"].(string); ok {
		return "学期起始日：" + start + "\n"
	}
	if items, ok := result["items"].([]map[string]any); ok {
		var builder strings.Builder
		for _, item := range items {
			label := make([]string, 0)
			for _, key := range []string{"semester", "course", "score", "credit", "grade_point", "exam_time", "room", "name"} {
				if value, ok := item[key].(string); ok && value != "" {
					label = append(label, value)
				}
			}
			builder.WriteString(fmt.Sprintf("[%v] %s\n", item["index"], strings.Join(label, " ")))
		}
		return builder.String()
	}
	encoded, _ := json.Marshal(result)
	return string(encoded) + "\n"
}
