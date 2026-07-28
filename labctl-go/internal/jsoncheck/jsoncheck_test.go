package jsoncheck

import (
	"strings"
	"testing"
)

func TestDecodeRejectsDuplicateKeysRecursively(t *testing.T) {
	t.Parallel()

	var target map[string]any
	err := Decode([]byte(`{"outer":{"same":1,"same":2}}`), &target, false)
	if err == nil || !strings.Contains(err.Error(), "duplicate object key") {
		t.Fatalf("expected duplicate-key error, got %v", err)
	}
}

func TestDecodeCanChooseClosedOrOpenShapes(t *testing.T) {
	t.Parallel()

	type value struct {
		Known int `json:"known"`
	}
	data := []byte(`{"known":1,"extra":2}`)

	var open value
	if err := Decode(data, &open, false); err != nil {
		t.Fatalf("open decode failed: %v", err)
	}
	var closed value
	if err := Decode(data, &closed, true); err == nil {
		t.Fatal("strict decode must reject the unknown field")
	}
}
