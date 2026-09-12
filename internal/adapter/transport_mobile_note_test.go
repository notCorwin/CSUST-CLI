package adapter

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func TestTransportMobileNoteDetailReplyAndDelete(t *testing.T) {
	const token, userID = "note-token", "user-1"
	row := map[string]any{
		"_id": "note-1", "name": "项目沟通", "version": "v1", "dateModified": "2026-09-12T00:00:00Z",
		"participants": []any{map[string]any{"user": userID}},
		"orderMax":     1.0,
		"detail":       []any{map[string]any{"_id": "message-1", "order": 1.0, "content": "旧消息", "sender": userID}},
	}
	var replyBody, deleteBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		if request.Header.Get("token") != token {
			t.Errorf("missing note token: %q", request.Header.Get("token"))
		}
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/api/user/getUserInfo":
			_, _ = fmt.Fprint(writer, `{"code":200,"success":true,"data":{"_id":"user-1","code":"T001","name":"交通老师"}}`)
		case request.Method == http.MethodPut && request.URL.Path == "/api/table/note":
			var body map[string]any
			if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if body["filter"].(map[string]any)["_id"] != "note-1" {
				t.Errorf("note detail filter was not mapped: %#v", body)
			}
			_ = json.NewEncoder(writer).Encode(map[string]any{"code": 200, "success": true, "data": map[string]any{"total": 1, "records": []any{row}}})
		case request.Method == http.MethodPut && request.URL.Path == "/api/table/note/note-1":
			var body map[string]any
			if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if push, ok := body["$push"].(map[string]any); ok {
				replyBody = body
				message := push["detail"].(map[string]any)
				message["_id"] = "message-2"
				row["detail"] = append(row["detail"].([]any), message)
				row["orderMax"], row["version"] = message["order"], "v2"
			} else {
				deleteBody = body
				pull := body["$pull"].(map[string]any)["detail"].(map[string]any)["_id"]
				allMessages := row["detail"].([]any)
				messages := make([]any, 0, len(allMessages))
				for _, item := range allMessages {
					if item.(map[string]any)["_id"] != pull {
						messages = append(messages, item)
					}
				}
				row["detail"], row["version"] = messages, "v3"
			}
			_, _ = fmt.Fprint(writer, `{"code":200,"success":true}`)
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	t.Setenv("CSUST_BASE_URL", server.URL)
	t.Setenv("CSUST_COOKIE_FILE", filepath.Join(t.TempDir(), "transport.cookies.txt"))

	detail := runIssueJSON(t, "transport-mobile", "note", "--access-token", token, "--id", "note-1")
	if detail["confirmed"] != true || detail["note"].(map[string]any)["messages"] == nil {
		t.Fatalf("note detail was not mapped: %#v", detail)
	}
	reply := runIssueJSON(t, "transport-mobile", "note-reply", "--access-token", token, "--id", "note-1", "--content", "新消息", "--yes")
	if reply["submitted"] != true || reply["confirmed"] != true || reply["message_order"] != float64(2) {
		t.Fatalf("note reply was not confirmed: %#v", reply)
	}
	if replyBody["version"] != "v1" || replyBody["orderMax"] != float64(2) || replyBody["$push"].(map[string]any)["detail"].(map[string]any)["sender"] != userID {
		t.Fatalf("note reply body was not mapped: %#v", replyBody)
	}
	deleted := runIssueJSON(t, "transport-mobile", "note-delete", "--access-token", token, "--id", "note-1", "--message-id", "message-1", "--yes")
	if deleted["submitted"] != true || deleted["confirmed"] != true || deleteBody["version"] != "v2" {
		t.Fatalf("note delete was not confirmed: %#v body=%#v row=%#v", deleted, deleteBody, row)
	}
}

func TestTransportMobileNoteCreate(t *testing.T) {
	const token, userID, recipient = "note-create-token", "user-1", "user-2"
	var entity map[string]any
	var row map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		if request.Header.Get("token") != token {
			t.Errorf("missing note-create token: %q", request.Header.Get("token"))
		}
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/api/user/getUserInfo":
			_, _ = fmt.Fprint(writer, `{"code":200,"success":true,"data":{"_id":"user-1"}}`)
		case request.Method == http.MethodPost && request.URL.Path == "/api/table/note":
			if err := json.NewDecoder(request.Body).Decode(&entity); err != nil {
				t.Fatal(err)
			}
			row = map[string]any{"_id": "note-new", "version": "v1", "orderMax": 1.0, "detail": entity["detail"], "participants": entity["participants"]}
			_, _ = fmt.Fprint(writer, `{"code":200,"success":true,"data":{"_id":"note-new","version":"v1"}}`)
		case request.Method == http.MethodPut && request.URL.Path == "/api/table/note":
			_ = json.NewEncoder(writer).Encode(map[string]any{"code": 200, "success": true, "data": map[string]any{"total": 1, "records": []any{row}}})
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	t.Setenv("CSUST_BASE_URL", server.URL)
	t.Setenv("CSUST_COOKIE_FILE", filepath.Join(t.TempDir(), "transport.cookies.txt"))

	result := runIssueJSON(t, "transport-mobile", "note-create", "--access-token", token, "--recipient-id", recipient, "--content", "新建消息", "--yes")
	if result["submitted"] != true || result["confirmed"] != true || result["id"] != "note-new" || result["recipient_id"] != recipient {
		t.Fatalf("note create was not confirmed: %#v", result)
	}
	if entity["code"] != "__auto__note" || entity["creater"] != userID || entity["status"] != "进行中" || entity["orderMax"] != float64(1) {
		t.Fatalf("note create entity was not mapped: %#v", entity)
	}
	participants := entity["participants"].([]any)
	if participants[0].(map[string]any)["user"] != userID || participants[1].(map[string]any)["user"] != recipient {
		t.Fatalf("note participants were not mapped: %#v", participants)
	}
}
