package transformer

import (
	"encoding/json"
	"strings"
	"testing"
)

const testSessionID = "37c2c260-c037-4752-b118-6e3f81d50895"

func parseUserID(t *testing.T, body []byte) map[string]interface{} {
	t.Helper()
	var req map[string]interface{}
	if err := json.Unmarshal(body, &req); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	meta, ok := req["metadata"].(map[string]interface{})
	if !ok {
		t.Fatalf("metadata missing or not an object: %v", req["metadata"])
	}
	uidStr, ok := meta["user_id"].(string)
	if !ok {
		t.Fatalf("user_id missing or not a string: %v", meta["user_id"])
	}
	var uid map[string]interface{}
	if err := json.Unmarshal([]byte(uidStr), &uid); err != nil {
		t.Fatalf("unmarshal user_id string: %v", err)
	}
	return uid
}

func TestInjectMetadata_NoMetadata(t *testing.T) {
	body := []byte(`{"model":"claude-opus-4-8","messages":[]}`)
	out, changed := injectMetadata(body, testSessionID)
	if !changed {
		t.Fatal("expected changed=true when metadata absent")
	}
	uid := parseUserID(t, out)
	if uid["session_id"] != testSessionID {
		t.Errorf("session_id = %v, want %v", uid["session_id"], testSessionID)
	}
	if uid["account_uuid"] != "" {
		t.Errorf("account_uuid = %v, want empty", uid["account_uuid"])
	}
	dev, _ := uid["device_id"].(string)
	if len(dev) != 64 {
		t.Errorf("device_id len = %d, want 64", len(dev))
	}
}

func TestInjectMetadata_MetadataWithoutUserID(t *testing.T) {
	body := []byte(`{"metadata":{"other":"keep"},"messages":[]}`)
	out, changed := injectMetadata(body, testSessionID)
	if !changed {
		t.Fatal("expected changed=true when metadata lacks user_id")
	}
	var req map[string]interface{}
	if err := json.Unmarshal(out, &req); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	meta := req["metadata"].(map[string]interface{})
	if meta["other"] != "keep" {
		t.Errorf("existing metadata field lost: %v", meta["other"])
	}
	uid := parseUserID(t, out)
	if uid["session_id"] != testSessionID {
		t.Errorf("session_id = %v, want %v", uid["session_id"], testSessionID)
	}
}

func TestInjectMetadata_ExistingUserIDUnchanged(t *testing.T) {
	body := []byte(`{"metadata":{"user_id":"preset"},"messages":[]}`)
	out, changed := injectMetadata(body, testSessionID)
	if changed {
		t.Fatal("expected changed=false when user_id already present")
	}
	if string(out) != string(body) {
		t.Errorf("body should be byte-identical, got %s", out)
	}
}

func TestInjectMetadata_NonObjectMetadataUnchanged(t *testing.T) {
	body := []byte(`{"metadata":"scalar","messages":[]}`)
	out, changed := injectMetadata(body, testSessionID)
	if changed {
		t.Fatal("expected changed=false when metadata is not an object")
	}
	if string(out) != string(body) {
		t.Errorf("body should be byte-identical, got %s", out)
	}
}

func TestDeriveDeviceID_Deterministic(t *testing.T) {
	a := deriveDeviceID(testSessionID)
	b := deriveDeviceID(testSessionID)
	if a != b {
		t.Errorf("device_id not deterministic: %q vs %q", a, b)
	}
	if len(a) != 64 {
		t.Errorf("device_id len = %d, want 64", len(a))
	}
	if deriveDeviceID("other-session") == a {
		t.Error("device_id should differ for different session IDs")
	}
}

func TestBuildUserID_KeyOrder(t *testing.T) {
	s := buildUserID(testSessionID)
	di := strings.Index(s, `"device_id"`)
	ai := strings.Index(s, `"account_uuid"`)
	si := strings.Index(s, `"session_id"`)
	if !(di >= 0 && di < ai && ai < si) {
		t.Errorf("key order wrong: device_id=%d account_uuid=%d session_id=%d in %s", di, ai, si, s)
	}
}
