package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

// Default API constants
const (
	DefaultAPIURL = "https://api.kriyatec.com/ai/v1/chat/completions"
	DefaultModel  = "gemini-3.6-flash"
	DefaultEffort = "medium"
)

// ChatMessage represents a single message turn in the conversation.
type ChatMessage struct {
	Role    string `json:"role"` // "system", "user", "assistant"
	Content string `json:"content"`
}

// ChatRequest represents the payload for the KriyaTec AI chat completion endpoint.
type ChatRequest struct {
	Model          string        `json:"model"`
	ConversationID string        `json:"conversation_id,omitempty"`
	Messages       []ChatMessage `json:"messages"`
	Effort         string        `json:"effort,omitempty"`
	SkipCache      bool          `json:"skip_cache"`
}

// ChatChoice represents a choice returned by the completion API.
type ChatChoice struct {
	Index        int         `json:"index"`
	Message      ChatMessage `json:"message"`
	FinishReason string      `json:"finish_reason"`
}

// ChatResponse represents the response from the KriyaTec AI endpoint.
type ChatResponse struct {
	ID             string       `json:"id"`
	Object         string       `json:"object"`
	Created        int64        `json:"created"`
	Model          string       `json:"model"`
	Choices        []ChatChoice `json:"choices"`
	Success        bool         `json:"success"`
	ConversationID string       `json:"conversation_id"`
	Response       string       `json:"response"`
	Error          string       `json:"error,omitempty"`
}

// Client defines the interface for communicating with the AI model backend.
type Client interface {
	Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error)
}

// HTTPClient implements Client using standard net/http.
type HTTPClient struct {
	apiURL     string
	model      string
	httpClient *http.Client
}

// NewHTTPClient creates an HTTPClient with configurable or environment-based settings.
func NewHTTPClient(apiURL, model string) *HTTPClient {
	if apiURL == "" {
		if envURL := os.Getenv("AI_API_URL"); envURL != "" {
			apiURL = envURL
		} else {
			apiURL = DefaultAPIURL
		}
	}
	if model == "" {
		if envModel := os.Getenv("AI_MODEL"); envModel != "" {
			model = envModel
		} else {
			model = DefaultModel
		}
	}
	return &HTTPClient{
		apiURL: apiURL,
		model:  model,
		httpClient: &http.Client{
			Timeout: 45 * time.Second,
		},
	}
}

// Chat sends a completion request to the KriyaTec AI endpoint.
func (c *HTTPClient) Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
	if req.Model == "" {
		req.Model = c.model
	}
	if req.Effort == "" {
		req.Effort = DefaultEffort
	}

	bodyBytes, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed marshaling AI request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.apiURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed creating HTTP request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("AI endpoint request failed: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed reading AI response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("AI endpoint returned HTTP %d: %s", resp.StatusCode, string(respBytes))
	}

	var chatResp ChatResponse
	if err := json.Unmarshal(respBytes, &chatResp); err != nil {
		return nil, fmt.Errorf("failed unmarshaling AI response: %w (raw: %s)", err, string(respBytes))
	}

	return &chatResp, nil
}
