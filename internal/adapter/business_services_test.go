package adapter

import (
	"bytes"
	"reflect"
	"strings"
	"testing"
)

func TestBusinessAdaptersKeepSemanticAndRawData(t *testing.T) {
	catalog := businessCatalogNames("onlinejudge")
	items := catalog["catalog"].([]map[string]any)
	if len(items) != 1 || items[0]["name"] != "onlinejudge" || items[0]["confidence_evidence"] == "" {
		t.Fatalf("unexpected business catalog: %#v", catalog)
	}

	result := businessResult(map[string]any{
		"response": map[string]any{"json": map[string]any{
			"error": nil,
			"data":  map[string]any{"total": float64(1), "results": []any{}},
		}},
	}, "onlinejudge", "problems")
	if _, ok := result["response"]; ok {
		t.Fatalf("JSON response was not unwrapped: %#v", result)
	}
	data, ok := result["data"].(map[string]any)
	if !ok || data["total"] != float64(1) {
		t.Fatalf("unexpected unwrapped data: %#v", result)
	}

	decoded, err := decodeJSAssignment(`const data = {"career":[{"career_talk_id":7,"meet_name":"招聘会"}]};`, "const data =")
	if err != nil || len(employmentItems(decoded, "career")) != 1 {
		t.Fatalf("embedded employment data was not decoded: %#v %v", decoded, err)
	}

	article := journalArticle(map[string]any{
		"file_no": "20260112", "title": "题目", "author_name": "作者", "doi": "10/test",
	}, "journal-transport", "jtkxygc")
	if article["id"] != "20260112" || article["title"] != "题目" || article["raw"] == nil {
		t.Fatalf("article lost semantic/raw fields: %#v", article)
	}

	if got := findID(map[string]any{"data": []any{map[string]any{"submission_id": float64(12)}}}); got != "12" {
		t.Fatalf("unexpected nested id: %q", got)
	}
	if got := recordOriginValue("湖南省"); got != "14" {
		t.Fatalf("unexpected province mapping: %q", got)
	}
	if role := journalRole("审稿人"); role != "reviewer" {
		t.Fatalf("unexpected journal role: %q", role)
	}
	if encrypted, err := journalEncryptedPasswordWithRandom("secret", bytes.NewReader(bytes.Repeat([]byte{1}, 126))); err != nil || len(encrypted) != 252 {
		t.Fatalf("unexpected journal RSA payload: len=%d err=%v", len(encrypted), err)
	}
	if got := strings.ToLower(sm3Hex("abc")); got != "66c7f0f462eeedd9d1f2d46bdc10e4e24167c4875cf2f7a2297da02b8f4ba8e0" {
		t.Fatalf("unexpected SM3 digest: %s", got)
	}
	if state, known := jsonBusinessState(map[string]any{"state": "success"}); !state || !known {
		t.Fatalf("state=success was not recognized: %v %v", state, known)
	}
	if !securityLoginForm(`<form id="loginForm"><input name="user[account]"></form>`) {
		t.Fatal("security login form was not recognized")
	}
	if !cmsLoginPage(`<form id="loginform"><input name="user"></form>`) {
		t.Fatal("CMS login form was not recognized")
	}
	if got := redactSiteJSON(map[string]any{"results": []any{1}, "password": "secret"}); !reflect.DeepEqual(got, map[string]any{"results": []any{1}, "password": "<redacted>"}) {
		t.Fatalf("sensitive redaction changed ordinary result fields: %#v", got)
	}
}
