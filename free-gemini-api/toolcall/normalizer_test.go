package toolcall

import (
	"testing"
)

func TestNormalizeToolCalls_JSON(t *testing.T) {
	rawText := `Here is the call: {"name": "get_weather", "arguments": {"location": "San Francisco", "units": "celsius"}}`
	cleaned, calls := NormalizeToolCalls(rawText)

	if len(calls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(calls))
	}

	call := calls[0]
	if call.Name != "get_weather" {
		t.Errorf("expected name 'get_weather', got '%s'", call.Name)
	}

	if call.ID == "" {
		t.Errorf("expected non-empty tool call ID")
	}

	if call.Arguments["location"] != "San Francisco" {
		t.Errorf("expected location 'San Francisco', got '%v'", call.Arguments["location"])
	}

	_ = cleaned
}

func TestNormalizeToolCalls_NestedJSON(t *testing.T) {
	rawText := `{"name": "complex_tool", "arguments": {"meta": {"deep": true, "count": 5}, "tags": ["a", "b"]}}`
	_, calls := NormalizeToolCalls(rawText)

	if len(calls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(calls))
	}

	call := calls[0]
	if call.Name != "complex_tool" {
		t.Errorf("expected name 'complex_tool', got '%s'", call.Name)
	}

	meta, ok := call.Arguments["meta"].(map[string]any)
	if !ok {
		t.Fatalf("expected meta argument to be map[string]any, got %T", call.Arguments["meta"])
	}

	if meta["deep"] != true {
		t.Errorf("expected meta.deep to be true, got %v", meta["deep"])
	}
}

func TestNormalizeToolCalls_XML(t *testing.T) {
	rawText := `<function=search>{"query": "golang testing"}</function>`
	_, calls := NormalizeToolCalls(rawText)

	if len(calls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(calls))
	}

	if calls[0].Name != "search" {
		t.Errorf("expected name 'search', got '%s'", calls[0].Name)
	}
}
