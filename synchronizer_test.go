package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestValidateOperation(t *testing.T) {
	cssAnchor := anchor{Type: "css", Selector: "#btn"}
	cases := []struct {
		name    string
		op      operation
		wantErr bool
	}{
		{"click", operation{Type: "click", Anchor: cssAnchor}, false},
		{"input with value", operation{Type: "input", Anchor: cssAnchor, Value: json.RawMessage(`"hi"`)}, false},
		{"empty value is kept", operation{Type: "input", Anchor: cssAnchor, Value: json.RawMessage(`""`)}, false},
		{"unknown type", operation{Type: "hover", Anchor: cssAnchor}, true},
		{"unknown anchor type", operation{Type: "click", Anchor: anchor{Type: "xpath", Selector: "//x"}}, true},
		{"empty selector", operation{Type: "click", Anchor: anchor{Type: "css"}}, true},
		{"selector at limit", operation{Type: "click", Anchor: anchor{Type: "css", Selector: strings.Repeat("a", 2048)}}, false},
		{"selector over limit", operation{Type: "click", Anchor: anchor{Type: "css", Selector: strings.Repeat("a", 2049)}}, true},
		{"value at limit", operation{Type: "input", Anchor: cssAnchor, Value: json.RawMessage(`"` + strings.Repeat("a", 9998) + `"`)}, false},
		{"value over limit", operation{Type: "input", Anchor: cssAnchor, Value: json.RawMessage(`"` + strings.Repeat("a", 9999) + `"`)}, true},
	}
	for _, c := range cases {
		if err := validateOperation(&c.op); (err != nil) != c.wantErr {
			t.Errorf("%s: validateOperation error = %v, wantErr %v", c.name, err, c.wantErr)
		}
	}
}

func TestErrorCode(t *testing.T) {
	cases := []struct {
		err  error
		want string
	}{
		{fmt.Errorf("ANCHOR_TIMEOUT: context deadline exceeded"), "ANCHOR_TIMEOUT"},
		{fmt.Errorf("INVALID_LOCATOR"), "INVALID_LOCATOR"},
		{fmt.Errorf("AMBIGUOUS"), "AMBIGUOUS"},
		{fmt.Errorf("NOT_FOUND"), "NOT_FOUND"},
		{fmt.Errorf("something else"), "REPLAY_FAILED"},
	}
	for _, c := range cases {
		if got := errorCode(c.err); got != c.want {
			t.Errorf("errorCode(%v) = %q, want %q", c.err, got, c.want)
		}
	}
}
