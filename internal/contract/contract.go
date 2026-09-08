package contract

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

// Normalize adds the result fields shared by every successful command.
func Normalize(raw []byte) ([]byte, error) {
	result, err := object(raw)
	if err != nil {
		return nil, err
	}
	if err := validateConfidence(result); err != nil {
		return nil, err
	}
	var ok bool
	if value, exists := result["ok"]; exists {
		if err := json.Unmarshal(value, &ok); err != nil {
			return nil, fmt.Errorf("结果字段 ok 无效: %w", err)
		}
		if !ok && !pending(result) {
			return nil, fmt.Errorf("成功进程返回了 ok=false")
		}
	} else {
		ok = true
		result["ok"] = json.RawMessage("true")
	}
	if _, exists := result["submitted"]; !exists {
		result["submitted"] = json.RawMessage("false")
	}
	var submitted bool
	if err := json.Unmarshal(result["submitted"], &submitted); err != nil {
		return nil, fmt.Errorf("结果字段 submitted 无效: %w", err)
	}
	if _, exists := result["confirmed"]; !exists {
		if submitted {
			return nil, fmt.Errorf("写操作缺少 confirmed 成功证据")
		}
		result["confirmed"] = json.RawMessage(fmt.Sprintf("%t", ok))
	}
	var confirmed bool
	if err := json.Unmarshal(result["confirmed"], &confirmed); err != nil {
		return nil, fmt.Errorf("结果字段 confirmed 无效: %w", err)
	}
	if submitted && !confirmed {
		return nil, fmt.Errorf("写操作未取得成功证据")
	}
	if _, exists := result["evidence"]; !exists {
		evidence := "confirmed"
		if !ok {
			evidence = "pending"
		}
		result["evidence"] = json.RawMessage(fmt.Sprintf("%q", evidence))
	}
	return json.Marshal(result)
}

// NormalizeError keeps the same fields available when a command fails.
func NormalizeError(raw []byte) ([]byte, error) {
	result, err := object(raw)
	if err != nil {
		return nil, err
	}
	result["ok"] = json.RawMessage("false")
	if _, exists := result["submitted"]; !exists {
		result["submitted"] = nestedBool(result, "details", "submitted", false)
	}
	if _, exists := result["confirmed"]; !exists {
		result["confirmed"] = nestedBool(result, "details", "confirmed", false)
	}
	if _, exists := result["evidence"]; !exists {
		var code string
		_ = json.Unmarshal(result["code"], &code)
		evidence := "unknown"
		if code == "mutation_rejected" || code == "business_rejected" {
			evidence = "rejected"
		}
		result["evidence"] = json.RawMessage(fmt.Sprintf("%q", evidence))
	}
	return json.Marshal(result)
}

func object(raw []byte) (map[string]json.RawMessage, error) {
	var result map[string]json.RawMessage
	decoder := json.NewDecoder(bytes.NewReader(raw))
	if err := decoder.Decode(&result); err != nil || result == nil {
		if err == nil {
			err = fmt.Errorf("结果必须是 JSON 对象")
		}
		return nil, fmt.Errorf("结果不是 JSON 对象: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("结果包含多个 JSON 值")
		}
		return nil, fmt.Errorf("结果 JSON 尾部无效: %w", err)
	}
	return result, nil
}

func nestedBool(result map[string]json.RawMessage, parent, field string, fallback bool) json.RawMessage {
	var values map[string]json.RawMessage
	if json.Unmarshal(result[parent], &values) == nil {
		if value, exists := values[field]; exists {
			var parsed bool
			if json.Unmarshal(value, &parsed) == nil {
				return value
			}
		}
	}
	return json.RawMessage(fmt.Sprintf("%t", fallback))
}

func validateConfidence(result map[string]json.RawMessage) error {
	for _, key := range []string{"page", "response"} {
		var page map[string]json.RawMessage
		if json.Unmarshal(result[key], &page) != nil || page == nil {
			continue
		}
		var confidence string
		if json.Unmarshal(page["confidence"], &confidence) == nil && confidence == "low" {
			evidence := page["confidence_evidence"]
			if len(bytes.TrimSpace(evidence)) <= 2 || bytes.Equal(bytes.TrimSpace(evidence), []byte("null")) {
				return fmt.Errorf("低置信度页面缺少具体依据")
			}
		}
	}
	return nil
}

func pending(result map[string]json.RawMessage) bool {
	var value bool
	if json.Unmarshal(result["pending"], &value) == nil && value {
		return true
	}
	var next string
	return json.Unmarshal(result["next"], &next) == nil && next != ""
}
