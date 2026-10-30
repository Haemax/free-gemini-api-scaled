package api

import (
	"testing"
)

func TestConvertOpenAIToAnthropic_SystemAndMessages(t *testing.T) {
	temp := 0.7
	req := OpenAIChatCompletionRequest{
		Model:       "claude-3.5-sonnet",
		MaxTokens:   2048,
		Temperature: &temp,
		Messages: []OpenAIMessage{
			{Role: "system", Content: "You are a helpful software architect."},
			{Role: "user", Content: "Explain clean architecture in 2 sentences."},
		},
	}

	antReq, err := ConvertOpenAIToAnthropic(req)
	if err != nil {
		t.Fatalf("Unexpected conversion error: %v", err)
	}

	if antReq.Model != "claude-3-5-sonnet-20241022" {
		t.Errorf("Expected model 'claude-3-5-sonnet-20241022', got '%s'", antReq.Model)
	}
	if antReq.System != "You are a helpful software architect." {
		t.Errorf("Expected system prompt preserved, got '%s'", antReq.System)
	}
	if len(antReq.Messages) != 1 {
		t.Fatalf("Expected 1 non-system message, got %d", len(antReq.Messages))
	}
	if antReq.Messages[0].Role != "user" || antReq.Messages[0].Content != "Explain clean architecture in 2 sentences." {
		t.Errorf("Message conversion mismatch: %+v", antReq.Messages[0])
	}
}

func TestMapClaudeModelAlias(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"claude-3.5-sonnet", "claude-3-5-sonnet-20241022"},
		{"claude-3-5-haiku", "claude-3-5-haiku-20241022"},
		{"opus", "claude-3-opus-20240229"},
		{"claude-3-7-sonnet-custom", "claude-3-7-sonnet-custom"},
	}

	for _, tt := range tests {
		result := MapClaudeModelAlias(tt.input)
		if result != tt.expected {
			t.Errorf("MapClaudeModelAlias(%s) = %s; want %s", tt.input, result, tt.expected)
		}
	}
}
