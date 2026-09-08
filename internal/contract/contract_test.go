package contract

import (
	"encoding/json"
	"testing"
)

func TestNormalizeReadResult(t *testing.T) {
	raw, err := Normalize([]byte(`{"items":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatal(err)
	}
	if result["ok"] != true || result["submitted"] != false || result["confirmed"] != true {
		t.Fatalf("unexpected contract: %#v", result)
	}
}

func TestNormalizeRejectsUnconfirmedMutation(t *testing.T) {
	if _, err := Normalize([]byte(`{"submitted":true,"confirmed":false}`)); err == nil {
		t.Fatal("expected unconfirmed mutation to fail")
	}
}

func TestNormalizeRejectsTrailingJSON(t *testing.T) {
	if _, err := Normalize([]byte(`{"items":[]} {}`)); err == nil {
		t.Fatal("expected trailing JSON to fail")
	}
}

func TestNormalizeErrorKeepsMutationState(t *testing.T) {
	raw, err := NormalizeError([]byte(`{"error":"结果未知","code":"mutation_unverified","details":{"submitted":true}}`))
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatal(err)
	}
	if result["ok"] != false || result["submitted"] != true || result["confirmed"] != false || result["evidence"] != "unknown" {
		t.Fatalf("unexpected error contract: %#v", result)
	}
}

func TestNormalizeKeepsPendingFlow(t *testing.T) {
	raw, err := Normalize([]byte(`{"ok":false,"pending":true,"next":"second-auth"}`))
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatal(err)
	}
	if result["ok"] != false || result["confirmed"] != false || result["evidence"] != "pending" {
		t.Fatalf("unexpected pending contract: %#v", result)
	}
}

func TestNormalizeRejectsLowConfidenceWithoutEvidence(t *testing.T) {
	if _, err := Normalize([]byte(`{"response":{"kind":"dynamic","confidence":"low"}}`)); err == nil {
		t.Fatal("expected low-confidence page without evidence to fail")
	}
}

func TestNormalizeAcceptsLowConfidenceWithEvidence(t *testing.T) {
	if _, err := Normalize([]byte(`{"response":{"kind":"dynamic","confidence":"low","confidence_evidence":{"reason":"script-only"}}}`)); err != nil {
		t.Fatal(err)
	}
}
