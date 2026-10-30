package api

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"[github.com/gofiber/fiber/v3](https://github.com/gofiber/fiber/v3)"
)

const (
	AnthropicVersion    = "2023-06-01"
	DefaultAnthropicURL = "[https://api.anthropic.com/v1/messages](https://api.anthropic.com/v1/messages)"
)

type AnthropicMessage struct {
	Role    string      `json:"role"`
	Content interface{} `json:"content"`
}

type AnthropicTool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	InputSchema map[string]interface{} `json:"input_schema"`
}

type AnthropicRequest struct {
	Model       string             `json:"model"`
	Messages    []AnthropicMessage `json:"messages"`
	System      string             `json:"system,omitempty"`
	MaxTokens   int                `json:"max_tokens"`
	Stream      bool               `json:"stream"`
	Temperature *float64           `json:"temperature,omitempty"`
	Tools       []AnthropicTool    `json:"tools,omitempty"`
}

type AnthropicResponse struct {
	ID           string `json:"id"`
	Type         string `json:"type"`
	Role         string `json:"role"`
	Model        string `json:"model"`
	StopReason   string `json:"stop_reason"`
	StopSequence string `json:"stop_sequence"`
	Usage        struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
	Content []struct {
		Type  string                 `json:"type"`
		Text  string                 `json:"text,omitempty"`
		ID    string                 `json:"id,omitempty"`
		Name  string                 `json:"name,omitempty"`
		Input map[string]interface{} `json:"input,omitempty"`
	} `json:"content"`
}

type AnthropicStreamEvent struct {
	Type  string `json:"type"`
	Index int    `json:"index,omitempty"`
	Delta struct {
		Type         string `json:"type,omitempty"`
		Text         string `json:"text,omitempty"`
		StopReason   string `json:"stop_reason,omitempty"`
		StopSequence string `json:"stop_sequence,omitempty"`
	} `json:"delta,omitempty"`
	Message struct {
		ID    string `json:"id"`
		Model string `json:"model"`
		Usage struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	} `json:"message,omitempty"`
}

// ConvertOpenAIToAnthropic translates OpenAI payload to Anthropic format
func ConvertOpenAIToAnthropic(openAiReq OpenAIChatCompletionRequest) (*AnthropicRequest, error) {
	antReq := &AnthropicRequest{
		Model:     MapClaudeModelAlias(openAiReq.Model),
		MaxTokens: 4096,
		Stream:    openAiReq.Stream,
	}

	if openAiReq.MaxTokens > 0 {
		antReq.MaxTokens = openAiReq.MaxTokens
	}
	if openAiReq.Temperature != nil {
		antReq.Temperature = openAiReq.Temperature
	}

	var systemPrompts []string
	var antMessages []AnthropicMessage

	for _, msg := range openAiReq.Messages {
		contentStr := ExtractContentString(msg.Content)
		switch strings.ToLower(msg.Role) {
		case "system":
			if contentStr != "" {
				systemPrompts = append(systemPrompts, contentStr)
			}
		case "user", "assistant":
			antMessages = append(antMessages, AnthropicMessage{
				Role:    msg.Role,
				Content: contentStr,
			})
		}
	}

	if len(systemPrompts) > 0 {
		antReq.System = strings.Join(systemPrompts, "\n\n")
	}
	antReq.Messages = antMessages

	if len(openAiReq.Tools) > 0 {
		for _, t := range openAiReq.Tools {
			if t.Type == "function" {
				antReq.Tools = append(antReq.Tools, AnthropicTool{
					Name:        t.Function.Name,
					Description: t.Function.Description,
					InputSchema: t.Function.Parameters,
				})
			}
		}
	}

	return antReq, nil
}

func MapClaudeModelAlias(model string) string {
	clean := strings.ToLower(strings.TrimSpace(model))
	switch {
	case strings.Contains(clean, "sonnet"):
		return "claude-3-5-sonnet-20241022"
	case strings.Contains(clean, "haiku"):
		return "claude-3-5-haiku-20241022"
	case strings.Contains(clean, "opus"):
		return "claude-3-opus-20240229"
	default:
		if strings.HasPrefix(clean, "claude-") {
			return clean
		}
		return "claude-3-5-sonnet-20241022"
	}
}

func ExtractContentString(content interface{}) string {
	switch v := content.(type) {
	case string:
		return v
	case []interface{}:
		var parts []string
		for _, item := range v {
			if m, ok := item.(map[string]interface{}); ok {
				if t, ok := m["text"].(string); ok {
					parts = append(parts, t)
				}
			}
		}
		return strings.Join(parts, "\n")
	default:
		return ""
	}
}

// HandleAnthropicChatCompletions processes OpenAI requests via Anthropic
func HandleAnthropicChatCompletions(c fiber.Ctx, apiKey string) error {
	var openAiReq OpenAIChatCompletionRequest
	if err := c.Bind().JSON(&openAiReq); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": fiber.Map{
				"message": "Invalid JSON body",
				"type":    "invalid_request_error",
			},
		})
	}

	antReq, err := ConvertOpenAIToAnthropic(openAiReq)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": fiber.Map{
				"message": err.Error(),
				"type":    "invalid_request_error",
			},
		})
	}

	reqBody, err := json.Marshal(antReq)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to encode request"})
	}

	ctx, cancel := context.WithTimeout(c.Context(), 5*time.Minute)
	defer cancel()

	httpReq, err := http.NewRequestWithContext(ctx, "POST", DefaultAnthropicURL, bytes.NewReader(reqBody))
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to create request"})
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", apiKey)
	httpReq.Header.Set("anthropic-version", AnthropicVersion)

	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Do(httpReq)
	if err != nil {
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{
			"error": fiber.Map{
				"message": fmt.Sprintf("Anthropic API connection failed: %v", err),
				"type":    "api_error",
			},
		})
	}

	if resp.StatusCode != http.StatusOK {
		respBytes, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return c.Status(resp.StatusCode).Send(respBytes)
	}

	if openAiReq.Stream {
		c.Set("Content-Type", "text/event-stream")
		c.Set("Cache-Control", "no-cache")
		c.Set("Connection", "keep-alive")

		c.Context().SetBodyStreamWriter(func(w *bufio.Writer) {
			defer resp.Body.Close()
			scanner := bufio.NewScanner(resp.Body)
			reqID := fmt.Sprintf("chatcmpl-claude-%d", time.Now().UnixNano())

			for scanner.Scan() {
				line := scanner.Text()
				if !strings.HasPrefix(line, "data: ") {
					continue
				}
				dataStr := strings.TrimPrefix(line, "data: ")

				var event AnthropicStreamEvent
				if err := json.Unmarshal([]byte(dataStr), &event); err != nil {
					continue
				}

				if event.Type == "content_block_delta" && event.Delta.Text != "" {
					chunk := OpenAIChatCompletionChunk{
						ID:      reqID,
						Object:  "chat.completion.chunk",
						Created: time.Now().Unix(),
						Model:   openAiReq.Model,
						Choices: []OpenAIStreamChoice{
							{
								Index: 0,
								Delta: OpenAIDelta{
									Content: event.Delta.Text,
								},
							},
						},
					}
					chunkBytes, _ := json.Marshal(chunk)
					_, _ = fmt.Fprintf(w, "data: %s\n\n", string(chunkBytes))
					_ = w.Flush()
				}
			}
			_, _ = fmt.Fprintf(w, "data: [DONE]\n\n")
			_ = w.Flush()
		})
		return nil
	}

	defer resp.Body.Close()
	var antResp AnthropicResponse
	if err := json.NewDecoder(resp.Body).Decode(&antResp); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to parse Anthropic response"})
	}

	var responseText string
	for _, block := range antResp.Content {
		if block.Type == "text" {
			responseText += block.Text
		}
	}

	openAiResp := OpenAIChatCompletionResponse{
		ID:      antResp.ID,
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   openAiReq.Model,
		Choices: []OpenAIChoice{
			{
				Index: 0,
				Message: OpenAIMessage{
					Role:    "assistant",
					Content: responseText,
				},
				FinishReason: MapAnthropicFinishReason(antResp.StopReason),
			},
		},
		Usage: OpenAIUsage{
			PromptTokens:     antResp.Usage.InputTokens,
			CompletionTokens: antResp.Usage.OutputTokens,
			TotalTokens:      antResp.Usage.InputTokens + antResp.Usage.OutputTokens,
		},
	}

	return c.JSON(openAiResp)
}

func MapAnthropicFinishReason(reason string) string {
	switch reason {
	case "end_turn":
		return "stop"
	case "max_tokens":
		return "length"
	case "tool_use":
		return "tool_calls"
	default:
		return "stop"
	}
}
