package api

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/schollz/progressbar/v3"
)

// contentFields lists the streaming delta keys that carry model output.
// OpenAI never standardized a reasoning field, so servers invented their own:
// content (regular message), reasoning (vLLM, RFC #27755) and
// reasoning_content (DeepSeek / GLM / DashScope, older vLLM).
var contentFields = []string{
	"content",
	"reasoning",
	"reasoning_content",
}

func isContentField(name string) bool {
	for _, field := range contentFields {
		if field == name {
			return true
		}
	}
	return false
}

// ignorableDeltaFields are delta keys that legitimately never carry model output.
// Any other key arriving as a non-empty string is treated as possible protocol
// drift and reported once per process.
var ignorableDeltaFields = map[string]bool{
	"role":             true,
	"refusal":          true,
	"tool_calls":       true,
	"function_call":    true,
	"logprobs":         true,
	"finish_reason":    true,
	"audio":            true,
	"index":            true,
	"token_ids":        true,
	"stop_reason":      true,
	"prompt_token_ids": true,
	"prompt_text":      true,
	"obfuscation":      true,
}

// How a TTFT was obtained. Only TtftSourceContent reflects prompt processing.
const (
	TtftSourceContent  = "content"
	TtftSourceFallback = "fallback"
	TtftSourceNone     = "none"
)

var (
	unknownFieldMu     sync.Mutex
	unknownFieldWarned = map[string]bool{}
)

// ChatResult is the outcome of a single streaming chat completion.
type ChatResult struct {
	TTFT float64
	// TTFTSource is a category: TtftSourceContent, TtftSourceFallback or TtftSourceNone.
	TTFTSource string
	// TTFTField is the delta key that produced the first token (for example
	// "reasoning" or "content"). Only set when TTFTSource is TtftSourceContent.
	TTFTField        string
	CompletionTokens int
	PromptTokens     int
}

// NewClient creates a client for an OpenAI-compatible endpoint.
// Retries are disabled: a silent retry would fold into the measured duration
// while the request still reports success.
func NewClient(baseURL, apiVersion, apiKey string, opts ...option.RequestOption) openai.Client {
	requestOptions := []option.RequestOption{
		option.WithAPIKey(apiKey),
		option.WithBaseURL(baseURL),
		option.WithMaxRetries(0),
	}
	if apiVersion != "" {
		requestOptions = append(requestOptions, option.WithQuery("api-version", apiVersion))
	}
	requestOptions = append(requestOptions, opts...)

	return openai.NewClient(requestOptions...)
}

// AskOpenAi sends a prompt to the OpenAI API, processes the response stream and returns stats on it.
func AskOpenAi(client openai.Client, model string, prompt string, maxTokens int, bar *progressbar.ProgressBar) (ChatResult, error) {
	result := ChatResult{TTFTSource: TtftSourceNone}

	start := time.Now()

	stream := client.Chat.Completions.NewStreaming(
		context.Background(),
		openai.ChatCompletionNewParams{
			Model:    model,
			Messages: []openai.ChatCompletionMessageParamUnion{openai.UserMessage(prompt)},
			// Send both the deprecated `max_tokens` and `max_completion_tokens`
			// so that older self-hosted servers are still capped correctly.
			MaxTokens:           openai.Int(int64(maxTokens)),
			MaxCompletionTokens: openai.Int(int64(maxTokens)),
			Temperature:         openai.Float(1),
			StreamOptions: openai.ChatCompletionStreamOptionsParam{
				// Without this most servers omit `usage` from the stream entirely.
				IncludeUsage: openai.Bool(true),
			},
		},
	)

	var (
		firstContentAt  float64 // when the first recognised content field arrived
		contentSource   string  // which field it was
		firstFinishAt   float64 // when finish_reason arrived
		estimatedTokens int     // real-time token estimation for the progress bar
		usage           *openai.CompletionUsage
	)

	for stream.Next() {
		chunk := stream.Current()

		// `usage` is only present on the final chunk when stream_options.include_usage is set.
		if chunk.Usage.TotalTokens != 0 {
			finalUsage := chunk.Usage
			usage = &finalUsage
		}

		if len(chunk.Choices) == 0 {
			continue
		}
		choice := chunk.Choices[0]

		text, source, unknown := parseDelta(choice.Delta.RawJSON())
		for _, name := range unknown {
			warnUnknownDeltaField(name)
		}

		if text != "" {
			// Reasoning content counts towards the progress bar just like content does.
			newTokens := estimateTokens(text)
			estimatedTokens += newTokens

			if firstContentAt == 0 {
				firstContentAt = time.Since(start).Seconds()
				contentSource = source
			}
			if bar != nil {
				bar.Add(newTokens)
			}
		}

		if firstFinishAt == 0 && choice.FinishReason != "" {
			firstFinishAt = time.Since(start).Seconds()
		}
	}

	if err := stream.Err(); err != nil {
		return ChatResult{}, fmt.Errorf("stream error: %w", err)
	}

	switch {
	case firstContentAt > 0:
		result.TTFT = firstContentAt
		result.TTFTSource = TtftSourceContent
		result.TTFTField = contentSource
	case firstFinishAt > 0:
		// No content field was recognized; the only timestamp is the end of the
		// stream. Keep it for diagnostics, flagged as not a real TTFT.
		result.TTFT = firstFinishAt
		result.TTFTSource = TtftSourceFallback
	}

	if usage != nil {
		result.PromptTokens = int(usage.PromptTokens)
		result.CompletionTokens = int(usage.CompletionTokens)

		// Final adjustment: if we have actual completion tokens, adjust the progress bar.
		if bar != nil && result.CompletionTokens > 0 {
			if diff := result.CompletionTokens - estimatedTokens; diff != 0 {
				bar.Add(diff) // Could be positive or negative
			}
		}
	} else {
		// If no usage info, use our estimated tokens as completion tokens.
		result.CompletionTokens = estimatedTokens
	}

	return result, nil
}

func AskOpenAiRandomInput(client openai.Client, model string, numWords int, maxTokens int, bar *progressbar.ProgressBar) (ChatResult, error) {
	prompt := generateRandomPhrase(numWords)
	return AskOpenAi(client, model, prompt, maxTokens, bar)
}

// parseDelta extracts model output from a raw streaming delta. Raw JSON is used
// because the SDK's typed accessors silently drop vendor fields such as
// `reasoning`. Returns the text, the source field, and unrecognized non-empty
// string fields for drift warnings.
func parseDelta(raw string) (text string, source string, unknown []string) {
	if raw == "" {
		return "", "", nil
	}

	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &fields); err != nil {
		return "", "", nil
	}

	var builder strings.Builder
	for _, name := range contentFields {
		rawValue, ok := fields[name]
		if !ok {
			continue
		}
		var value string
		if err := json.Unmarshal(rawValue, &value); err != nil || value == "" {
			continue
		}
		builder.WriteString(value)
		if source == "" {
			source = name
		}
	}

	for name, rawValue := range fields {
		if isContentField(name) || ignorableDeltaFields[name] {
			continue
		}
		var value string
		if err := json.Unmarshal(rawValue, &value); err == nil && value != "" {
			unknown = append(unknown, name)
		}
	}
	sort.Strings(unknown)

	return builder.String(), source, unknown
}

// warnUnknownDeltaField reports an unrecognized delta field once per process,
// guarding against silently miscounted TTFT when a server renames its fields.
func warnUnknownDeltaField(name string) {
	unknownFieldMu.Lock()
	alreadyWarned := unknownFieldWarned[name]
	unknownFieldWarned[name] = true
	unknownFieldMu.Unlock()

	if alreadyWarned {
		return
	}

	fmt.Fprintf(os.Stderr,
		"\n\033[33m[warn]\033[0m unrecognized delta field %q (non-empty string).\n"+
			"       If it carries model output, TTFT is underestimated. Add %q to contentFields in internal/api/api_client.go.\n",
		name, name)
}

func estimateTokens(content string) int {
	if content == "" {
		return 0
	}

	content = strings.TrimSpace(content)
	if len(content) == 0 {
		return 0
	}

	words := strings.Fields(content)
	wordCount := len(words)

	// Different strategies based on content type
	if wordCount > 0 {
		// For text with clear word boundaries: ~1.3 tokens per word on average
		// This accounts for subword tokenization in modern models
		return max(1, int(float64(wordCount)*1.3))
	}
	// For content without clear word boundaries (like punctuation, single chars)
	// Use character-based estimation: ~3-4 characters per token
	charCount := len(content)
	return max(1, int(float64(charCount)/3.0))
}

// GetFirstAvailableModel retrieves the first available model from the OpenAI API.
func GetFirstAvailableModel(client openai.Client) (string, error) {
	page, err := client.Models.List(context.Background())
	if err != nil {
		return "", fmt.Errorf("failed to list models: %w", err)
	}

	if len(page.Data) == 0 {
		return "", fmt.Errorf("no models available")
	}

	return page.Data[0].ID, nil
}
