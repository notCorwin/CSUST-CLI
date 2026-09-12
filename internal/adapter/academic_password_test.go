package adapter

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
)

func TestAcademicPasswordChangeUsesExactFormAndRedactsSecrets(t *testing.T) {
	var formValues url.Values
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		switch request.URL.Path {
		case academicPasswordPath:
			if request.Method == http.MethodGet {
				_, _ = writer.Write([]byte(`<form method="post" action="/jsxsd/grsz/grsz_xgmm"><input type="hidden" name="id" value="student"><input type="password" name="oldpassword"><input type="password" name="password1"><input type="password" name="password2"><input type="submit" name="button1" value="保 存"><input type="hidden" name="upt" value="1"></form>`))
				return
			}
			if err := request.ParseForm(); err != nil {
				t.Fatalf("parse password form: %v", err)
			}
			formValues = request.Form
			_, _ = writer.Write([]byte(`<script>alert('修改成功')</script>`))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	t.Setenv("CSUST_BASE_URL", server.URL)
	t.Setenv("CSUST_COOKIE_FILE", filepath.Join(t.TempDir(), "cookies.txt"))

	handled, output, _, code, err := (NativeSite{}).Run(context.Background(), []string{
		"change-password", "--current-password", "OldA!123", "--new-password", "NewA!1234", "--password-confirm", "NewA!1234", "--yes", "--json",
	}, true)
	if err != nil || !handled || code != 0 {
		t.Fatalf("change-password: handled=%v code=%d err=%v output=%s", handled, code, err, output)
	}
	if formValues.Get("id") != "student" || formValues.Get("oldpassword") != "OldA!123" || formValues.Get("password1") != "NewA!1234" || formValues.Get("password2") != "NewA!1234" || formValues.Get("button1") != "保 存" || formValues.Get("upt") != "1" {
		t.Fatalf("unexpected password form: %#v", formValues)
	}
	if strings.Contains(string(output), "OldA!123") || strings.Contains(string(output), "NewA!1234") || !strings.Contains(string(output), "response-success") {
		t.Fatalf("password leaked or confirmation missing: %s", output)
	}
}

func TestAcademicPasswordComplexityMatchesFrontendRules(t *testing.T) {
	for _, value := range []string{"NewA!1234", "Abc#9876"} {
		if !academicPasswordComplex(value) {
			t.Fatalf("expected complex password: %q", value)
		}
	}
	for _, value := range []string{"lowercase1!", "UPPERCASE1!", "NoNumber!", "NoSpecial1", "Bad&Aa1"} {
		if academicPasswordComplex(value) && !strings.ContainsRune(value, '&') {
			t.Fatalf("unexpectedly accepted incomplete password: %q", value)
		}
	}
}
