package api

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/sashabaranov/go-openai"
)

// AskOpenAiStream sends a prompt to the OpenAI API, processes the response stream and returns stats on it.
func AskOpenAiStream(client *openai.Client, model string, prompt string, maxTokens int) (float64, int, int, error) {
	start := time.Now()

	var (
		timeToFirstToken float64
		firstTokenSeen   bool
		lastUsage        *openai.Usage
	)

	stream, err := client.CreateChatCompletionStream(
		context.Background(),
		openai.ChatCompletionRequest{
			Model: model,
			Messages: []openai.ChatCompletionMessage{
				{
					Role:    openai.ChatMessageRoleUser,
					Content: prompt,
				},
			},
			// Add the deprecated `MaxTokens` for backward compatibility with some older API servers.
			MaxTokens:           maxTokens,
			MaxCompletionTokens: maxTokens,
			Temperature:         1,
			Stream:              true,
			StreamOptions: &openai.StreamOptions{
				IncludeUsage: true,
			},
		},
	)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("OpenAI API request failed: %w", err)
	}
	defer stream.Close()

	chunksReceived := 0
	for {
		resp, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return 0, 0, 0, fmt.Errorf("stream error: %w", err)
		}

		if !firstTokenSeen && len(resp.Choices) > 0 {
			content := resp.Choices[0].Delta.Content
			if strings.TrimSpace(content) != "" {
				timeToFirstToken = time.Since(start).Seconds()
				firstTokenSeen = true
			}
		}

		if resp.Usage != nil {
			lastUsage = resp.Usage
		}

		chunksReceived++
		if os.Getenv("DEBUG") == "true" {
			fmt.Fprintf(os.Stderr, "Chunks received: %d       \r", chunksReceived)
		}
	}
	if os.Getenv("DEBUG") == "true" {
		fmt.Fprintf(os.Stderr, "\n")
	}

	var promptTokens, completionTokens int
	if lastUsage != nil {
		promptTokens = lastUsage.PromptTokens
		completionTokens = lastUsage.CompletionTokens
	}

	return timeToFirstToken, completionTokens, promptTokens, nil
}

func AskOpenAiStreamWithRandomInput(client *openai.Client, model string, numWords int, maxTokens int) (float64, int, int, error) {
	prompt := generateRandomPhrase(numWords)
	return AskOpenAiStream(client, model, prompt, maxTokens)
}

// AskOpenAi sends a prompt to the OpenAI API and returns the response, not using streaming.
func AskOpenAi(client *openai.Client, model, prompt string, maxTokens int) (*openai.ChatCompletionResponse, error) {
	resp, err := client.CreateChatCompletion(
		context.Background(),
		openai.ChatCompletionRequest{
			Model: model,
			Messages: []openai.ChatCompletionMessage{
				{
					Role:    openai.ChatMessageRoleSystem,
					Content: "You are a helpful assistant.",
				},
				{
					Role:    openai.ChatMessageRoleUser,
					Content: prompt,
				},
			},
			// Add the deprecated `MaxTokens` for backward compatibility with some older API servers.
			MaxTokens:           maxTokens,
			MaxCompletionTokens: maxTokens,
			Temperature:         1,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("OpenAI API request failed: %w", err)
	}
	return &resp, nil
}

func AskOpenAiWithRandomInput(client *openai.Client, model string, numWords int, maxTokens int) (*openai.ChatCompletionResponse, error) {
	prompt := generateRandomPhrase(numWords)
	return AskOpenAi(client, model, prompt, maxTokens)
}

// GetFirstAvailableModel retrieves the first available model from the OpenAI API.
func GetFirstAvailableModel(client *openai.Client) (string, error) {
	modelList, err := client.ListModels(context.Background())
	if err != nil {
		return "", fmt.Errorf("failed to list models: %w", err)
	}

	if len(modelList.Models) == 0 {
		return "", fmt.Errorf("no models available")
	}

	return modelList.Models[0].ID, nil
}
