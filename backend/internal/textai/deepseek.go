package textai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"unicode/utf8"

	"gamegen/backend/internal/platform/config"
)

const (
	chatCompletionsPath = "/chat/completions"
	maxResponseBytes    = 1 << 20
	maxLoveLetterRunes  = 1000
)

var (
	ErrNotConfigured   = errors.New("text AI is not configured")
	ErrTimeout         = errors.New("text AI request timed out")
	ErrUnavailable     = errors.New("text AI provider is unavailable")
	ErrInvalidResponse = errors.New("text AI returned an invalid response")
)

type LoveLetterPolisher interface {
	Configured() bool
	PolishLoveLetter(context.Context, string, string) (string, error)
}

type DeepSeekPolisher struct {
	config     config.AITextConfig
	httpClient *http.Client
}

func NewDeepSeekPolisher(cfg config.AITextConfig) *DeepSeekPolisher {
	return &DeepSeekPolisher{
		config:     cfg,
		httpClient: &http.Client{Timeout: cfg.Timeout},
	}
}

func (polisher *DeepSeekPolisher) Configured() bool {
	return strings.TrimSpace(polisher.config.APIKey) != "" &&
		strings.TrimSpace(polisher.config.BaseURL) != "" &&
		strings.TrimSpace(polisher.config.Model) != ""
}

type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Thinking    thinkingMode  `json:"thinking"`
	Temperature float64       `json:"temperature"`
	MaxTokens   int           `json:"max_tokens"`
	Stream      bool          `json:"stream"`
	UserID      string        `json:"user_id,omitempty"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type thinkingMode struct {
	Type string `json:"type"`
}

type chatResponse struct {
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
}

func (polisher *DeepSeekPolisher) PolishLoveLetter(ctx context.Context, text, userID string) (string, error) {
	if !polisher.Configured() {
		return "", ErrNotConfigured
	}
	payload := chatRequest{
		Model: polisher.config.Model,
		Messages: []chatMessage{
			{Role: "system", Content: loveLetterSystemPrompt},
			{Role: "user", Content: strings.TrimSpace(text)},
		},
		Thinking:    thinkingMode{Type: "disabled"},
		Temperature: 0.6,
		MaxTokens:   polisher.config.MaxOutputTokens,
		Stream:      false,
		UserID:      userID,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("encode DeepSeek request: %w", err)
	}
	endpoint := strings.TrimRight(polisher.config.BaseURL, "/") + chatCompletionsPath
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("create DeepSeek request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+polisher.config.APIKey)
	request.Header.Set("Content-Type", "application/json")

	response, err := polisher.httpClient.Do(request)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return "", ErrTimeout
		}
		return "", fmt.Errorf("%w: request failed", ErrUnavailable)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, maxResponseBytes))
		return "", fmt.Errorf("%w: HTTP %d", ErrUnavailable, response.StatusCode)
	}

	var result chatResponse
	decoder := json.NewDecoder(io.LimitReader(response.Body, maxResponseBytes))
	if err := decoder.Decode(&result); err != nil {
		return "", fmt.Errorf("%w: decode response", ErrInvalidResponse)
	}
	if len(result.Choices) == 0 {
		return "", fmt.Errorf("%w: response has no choices", ErrInvalidResponse)
	}
	polished := strings.TrimSpace(result.Choices[0].Message.Content)
	if polished == "" || !utf8.ValidString(polished) || utf8.RuneCountInString(polished) > maxLoveLetterRunes {
		return "", fmt.Errorf("%w: polished letter is empty or too long", ErrInvalidResponse)
	}
	return polished, nil
}

const loveLetterSystemPrompt = `你是一名擅长中文情感表达的情书编辑。请润色用户提供的情书，让语言更自然、真诚、温柔，并有适度的画面感和情绪递进。必须保留原文中的事实、姓名、日期、称呼、人称、共同经历与核心情感，不得虚构故事、承诺或细节，也不要把表达改得油腻、夸张或模板化。保持原文语言和写信人的语气，可优化措辞、语序、段落与节奏。只输出可直接使用的情书正文，不添加标题、引号、说明、点评或 Markdown，全文不超过 1000 个字符。`
