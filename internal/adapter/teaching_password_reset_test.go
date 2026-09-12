package adapter

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
)

func TestTeachingPasswordResetProtocols(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "text/plain; charset=utf-8")
		switch request.URL.Path {
		case "/meol/findPasswdQuestionPreAccount.do":
			writer.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = fmt.Fprint(writer, `<ul class="model-select-option"><li data-option="401">问题一</li><li data-option="402">问题二</li><li data-option="403">问题三</li></ul>`)
		case "/meol/findPasswdQuestionAccount.do":
			if err := request.ParseForm(); err != nil || request.Form.Get("username") != "question-user" || request.Form.Get("questionId") != "401" || request.Form.Get("questionId2") != "402" || request.Form.Get("questionId3") != "403" || request.Form.Get("questionVal") != url.QueryEscape("答案一") || request.Form.Get("imgcode") != "captcha" {
				t.Errorf("question verification form was not mapped: %v", request.Form)
			}
			writer.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(writer, `{"status":10000,"code":"question-reset-code"}`)
		case "/meol/findPasswdQuestionReset.do":
			if err := request.ParseForm(); err != nil || request.Form.Get("code") != "question-reset-code" || request.Form.Get("firstPasswd") != "Abc#1234" || request.Form.Get("secondPasswd") != "Abc#1234" {
				t.Errorf("question reset form was not mapped: %v", request.Form)
			}
			_, _ = fmt.Fprint(writer, "30000")
		case "/meol/findPasswdMailBoxAccount.do":
			if err := request.ParseForm(); err != nil || request.Form.Get("username") != "email-user" || request.Form.Get("email") != "user@example.com" || request.Form.Get("imgcode") != "captcha" {
				t.Errorf("email request form was not mapped: %v", request.Form)
			}
			_, _ = fmt.Fprint(writer, "10000")
		case "/meol/findPasswdMailBoxReset.do":
			if err := request.ParseForm(); err != nil || request.Form.Get("code") != "mail-reset-code" || request.Form.Get("firstPasswd") != "Abc#1234" || request.Form.Get("secondPasswd") != "Abc#1234" {
				t.Errorf("email reset form was not mapped: %v", request.Form)
			}
			_, _ = fmt.Fprint(writer, "30000")
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	t.Setenv("CSUST_BASE_URL", server.URL)
	t.Setenv("CSUST_COOKIE_FILE", filepath.Join(t.TempDir(), "theol.cookies"))

	question := runIssueJSON(t, "teaching", "password-reset", "--method", "question", "--username", "question-user", "--question-one", "问题一", "--answer-one", "答案一", "--question-two", "问题二", "--answer-two", "答案二", "--question-three", "问题三", "--answer-three", "答案三", "--captcha", "captcha", "--new-password", "Abc#1234", "--password-confirm", "Abc#1234", "--yes")
	if question["reset"] != true || question["confirmed"] != true || question["method"] != "question" {
		t.Fatalf("question password reset was not confirmed: %#v", question)
	}
	if encoded := string(mustMarshalIssue(question)); encoded == "" || containsAny(encoded, "question-reset-code", "Abc#1234") {
		t.Fatalf("password reset response leaked secret material: %s", encoded)
	}

	emailRequest := runIssueJSON(t, "teaching", "password-reset", "--method", "email", "--username", "email-user", "--email", "user@example.com", "--captcha", "captcha", "--yes")
	if emailRequest["pending"] != true || emailRequest["next"] != "email-link" || emailRequest["confirmed"] != true {
		t.Fatalf("email password reset request was not confirmed: %#v", emailRequest)
	}
	emailReset := runIssueJSON(t, "teaching", "password-reset", "--method", "email", "--code", "mail-reset-code", "--new-password", "Abc#1234", "--password-confirm", "Abc#1234", "--yes")
	if emailReset["reset"] != true || emailReset["method"] != "email" || emailReset["confirmed"] != true {
		t.Fatalf("email password reset was not confirmed: %#v", emailReset)
	}
}

func containsAny(value string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(value, needle) {
			return true
		}
	}
	return false
}
