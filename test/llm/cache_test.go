package llm_test

import (
	"encoding/json"
	"testing"

	"github.com/vanpiyp/awp/internal/llm"
)

func TestCacheEphemeral(t *testing.T) {
	cc := llm.CacheEphemeral()
	if cc == nil {
		t.Fatalf("CacheEphemeral() = nil, want non-nil")
	}
	if cc.Type != "ephemeral" {
		t.Fatalf("Type = %q, want %q", cc.Type, "ephemeral")
	}
	if cc.TTL != "" {
		t.Fatalf("TTL = %q, want empty", cc.TTL)
	}
}

func TestCacheEphemeral1h(t *testing.T) {
	cc := llm.CacheEphemeral1h()
	if cc == nil {
		t.Fatalf("CacheEphemeral1h() = nil, want non-nil")
	}
	if cc.Type != "ephemeral" {
		t.Fatalf("Type = %q, want %q", cc.Type, "ephemeral")
	}
	if cc.TTL != "1h" {
		t.Fatalf("TTL = %q, want %q", cc.TTL, "1h")
	}
}

func TestCacheEphemeralReturnsDistinct(t *testing.T) {
	a := llm.CacheEphemeral()
	b := llm.CacheEphemeral()
	if a == b {
		t.Fatalf("CacheEphemeral() returned the same pointer twice; expected distinct pointers")
	}
	if *a != *b {
		t.Fatalf("pointers differ in value: %v vs %v", *a, *b)
	}
}

func TestCacheControlJSONRoundTrip(t *testing.T) {
	cc := llm.CacheEphemeral1h()
	data, err := json.Marshal(cc)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}
	want := `{"Type":"ephemeral","TTL":"1h"}`
	if string(data) != want {
		t.Fatalf("Marshal = %s, want %s", string(data), want)
	}

	var got llm.CacheControl
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if got != *cc {
		t.Fatalf("round-trip = %+v, want %+v", got, *cc)
	}
}

func TestCacheControlEphemeralJSON(t *testing.T) {
	cc := llm.CacheEphemeral()
	data, err := json.Marshal(cc)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}
	want := `{"Type":"ephemeral","TTL":""}`
	if string(data) != want {
		t.Fatalf("Marshal = %s, want %s", string(data), want)
	}
}
