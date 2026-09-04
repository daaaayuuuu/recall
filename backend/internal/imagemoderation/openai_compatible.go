package imagemoderation

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"net/http"
	"strings"

	"gamegen/backend/internal/platform/config"

	"golang.org/x/image/draw"
)

const (
	chatCompletionsPath       = "/chat/completions"
	moderationPreviewMaxEdge  = 768
	moderationPreviewQuality  = 82
	moderationResponseMaxSize = 64 << 10
	moderationMaxPixels       = 100_000_000
)

var allowedCategories = map[string]struct{}{
	"sexual_explicit":  {},
	"minor_safety":     {},
	"graphic_violence": {},
	"self_harm":        {},
	"hate_extremism":   {},
	"illegal_activity": {},
	"privacy_document": {},
	"other_unsafe":     {},
}

type OpenAICompatibleReviewer struct {
	config     config.AIImageModerationConfig
	httpClient *http.Client
}

func NewOpenAICompatibleReviewer(cfg config.AIImageModerationConfig) *OpenAICompatibleReviewer {
	return &OpenAICompatibleReviewer{
		config:     cfg,
		httpClient: &http.Client{Timeout: cfg.Timeout},
	}
}

func New(cfg config.AIImageModerationConfig) Reviewer {
	if strings.TrimSpace(cfg.APIKey) == "" {
		return UnconfiguredReviewer{}
	}
	switch strings.TrimSpace(cfg.Provider) {
	case "openai-compatible":
		return NewOpenAICompatibleReviewer(cfg)
	default:
		return UnconfiguredReviewer{}
	}
}

func (reviewer *OpenAICompatibleReviewer) Configured() bool {
	return strings.TrimSpace(reviewer.config.BaseURL) != "" &&
		strings.TrimSpace(reviewer.config.APIKey) != "" &&
		strings.TrimSpace(reviewer.config.Model) != "" &&
		reviewer.config.Timeout > 0 && reviewer.config.MaxOutputTokens > 0
}

type moderationChatRequest struct {
	Model          string                   `json:"model"`
	Messages       []moderationMessage      `json:"messages"`
	ResponseFormat moderationResponseFormat `json:"response_format"`
	Temperature    float64                  `json:"temperature"`
	MaxTokens      int                      `json:"max_tokens"`
	Stream         bool                     `json:"stream"`
}

type moderationMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

type moderationContentPart struct {
	Type     string              `json:"type"`
	Text     string              `json:"text,omitempty"`
	ImageURL *moderationImageURL `json:"image_url,omitempty"`
}

type moderationImageURL struct {
	URL    string `json:"url"`
	Detail string `json:"detail"`
}

type moderationResponseFormat struct {
	Type string `json:"type"`
}

type moderationChatResponse struct {
	ID      string `json:"id"`
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

type providerDecision struct {
	Approved   *bool    `json:"approved"`
	Categories []string `json:"categories"`
	Reason     string   `json:"reason"`
}

func (reviewer *OpenAICompatibleReviewer) Review(ctx context.Context, input Input) (Decision, error) {
	if !reviewer.Configured() {
		return Decision{}, ErrNotConfigured
	}
	if input.Image == nil {
		return Decision{}, fmt.Errorf("%w: image is required", ErrInvalidResponse)
	}
	preview, err := encodeModerationPreview(input.Image)
	if err != nil {
		return Decision{}, fmt.Errorf("prepare moderation preview: %w", err)
	}
	imageURL := "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(preview)
	payload := moderationChatRequest{
		Model: reviewer.config.Model,
		Messages: []moderationMessage{
			{Role: "system", Content: moderationSystemPrompt},
			{Role: "user", Content: []moderationContentPart{
				{Type: "text", Text: "Review this image for the controlled product purpose: " + string(input.Purpose) + ". Return JSON only."},
				{Type: "image_url", ImageURL: &moderationImageURL{URL: imageURL, Detail: "low"}},
			}},
		},
		ResponseFormat: moderationResponseFormat{Type: "json_object"},
		Temperature:    0,
		MaxTokens:      reviewer.config.MaxOutputTokens,
		Stream:         false,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return Decision{}, fmt.Errorf("encode image moderation request: %w", err)
	}
	endpoint := strings.TrimRight(reviewer.config.BaseURL, "/") + chatCompletionsPath
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return Decision{}, fmt.Errorf("create image moderation request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+reviewer.config.APIKey)
	request.Header.Set("Content-Type", "application/json")

	response, err := reviewer.httpClient.Do(request)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return Decision{}, ErrTimeout
		}
		return Decision{}, fmt.Errorf("%w: request failed", ErrUnavailable)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, moderationResponseMaxSize))
		return Decision{}, fmt.Errorf("%w: HTTP %d", ErrUnavailable, response.StatusCode)
	}

	var result moderationChatResponse
	decoder := json.NewDecoder(io.LimitReader(response.Body, moderationResponseMaxSize))
	if err := decoder.Decode(&result); err != nil || len(result.Choices) == 0 {
		return Decision{}, fmt.Errorf("%w: decode response", ErrInvalidResponse)
	}
	decision, err := parseProviderDecision(result.Choices[0].Message.Content)
	if err != nil {
		return Decision{}, err
	}
	decision.ProviderRequestID = strings.TrimSpace(result.ID)
	return decision, nil
}

func parseProviderDecision(content string) (Decision, error) {
	content = strings.TrimSpace(content)
	if strings.HasPrefix(content, "```") {
		content = strings.TrimPrefix(content, "```json")
		content = strings.TrimPrefix(content, "```")
		content = strings.TrimSuffix(strings.TrimSpace(content), "```")
	}
	var raw providerDecision
	decoder := json.NewDecoder(strings.NewReader(content))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&raw); err != nil || raw.Approved == nil {
		return Decision{}, fmt.Errorf("%w: decision is not valid JSON", ErrInvalidResponse)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return Decision{}, fmt.Errorf("%w: decision contains trailing content", ErrInvalidResponse)
	}
	if len(raw.Categories) > len(allowedCategories) {
		return Decision{}, fmt.Errorf("%w: too many categories", ErrInvalidResponse)
	}
	seen := make(map[string]struct{}, len(raw.Categories))
	for _, category := range raw.Categories {
		if _, ok := allowedCategories[category]; !ok {
			return Decision{}, fmt.Errorf("%w: unknown category", ErrInvalidResponse)
		}
		if _, duplicate := seen[category]; duplicate {
			return Decision{}, fmt.Errorf("%w: duplicate category", ErrInvalidResponse)
		}
		seen[category] = struct{}{}
	}
	reason := strings.TrimSpace(raw.Reason)
	if reason == "" || len([]rune(reason)) > 240 || (*raw.Approved && len(raw.Categories) != 0) || (!*raw.Approved && len(raw.Categories) == 0) {
		return Decision{}, fmt.Errorf("%w: inconsistent decision", ErrInvalidResponse)
	}
	return Decision{Approved: *raw.Approved, Categories: raw.Categories, Reason: reason}, nil
}

func encodeModerationPreview(source io.ReadSeeker) ([]byte, error) {
	if _, err := source.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	configuration, _, err := image.DecodeConfig(source)
	if err != nil || configuration.Width <= 0 || configuration.Height <= 0 ||
		uint64(configuration.Width)*uint64(configuration.Height) > moderationMaxPixels {
		return nil, errors.New("invalid image dimensions")
	}
	if _, err := source.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	decoded, _, err := image.Decode(source)
	if err != nil {
		return nil, err
	}
	width, height := configuration.Width, configuration.Height
	if width > moderationPreviewMaxEdge || height > moderationPreviewMaxEdge {
		ratio := min(float64(moderationPreviewMaxEdge)/float64(width), float64(moderationPreviewMaxEdge)/float64(height))
		width = max(1, int(float64(width)*ratio))
		height = max(1, int(float64(height)*ratio))
		destination := image.NewRGBA(image.Rect(0, 0, width, height))
		draw.CatmullRom.Scale(destination, destination.Bounds(), decoded, decoded.Bounds(), draw.Over, nil)
		decoded = destination
	}
	var preview bytes.Buffer
	if err := jpeg.Encode(&preview, decoded, &jpeg.Options{Quality: moderationPreviewQuality}); err != nil {
		return nil, err
	}
	return preview.Bytes(), nil
}

const moderationSystemPrompt = `You are a safety reviewer for user-uploaded images in a personalized relationship game. Evaluate only the visible image. Ordinary portraits, couples, affection, travel, food, swimwear, art, and everyday scenes are allowed. Reject only when the image clearly contains one or more of these categories: sexual_explicit (explicit sexual activity or clearly exposed genitals), minor_safety (sexualized or exploitative content involving a minor), graphic_violence (gore or severe visible injury), self_harm (active or graphic self-harm), hate_extremism (praise or recruitment for hateful or extremist activity), illegal_activity (clear depiction or instruction of serious illegal activity), privacy_document (readable identity, financial, medical, or authentication documents), other_unsafe (a comparably severe safety risk not covered above). When uncertain, approve benign content; do not reject merely because faces, children, alcohol, tattoos, or text are present. Return exactly one JSON object with keys approved (boolean), categories (array of the listed strings; empty when approved), and reason (brief English explanation without personal identification). Do not return Markdown or additional keys.`
