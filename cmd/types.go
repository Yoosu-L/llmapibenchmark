package main

import (
	"github.com/Yoosu-L/llmapibenchmark/internal/utils"
	"github.com/openai/openai-go/v3"
)

type Benchmark struct {
	BaseURL           string
	ApiVersion        string
	ApiKey            string
	ModelName         string
	Prompt            string
	InputTokens       int
	MaxTokens         int
	ConcurrencyLevels []int
	UseRandomInput    bool
	NumWords          int

	// Client is shared by the latency probe, model discovery and the measurements.
	Client openai.Client
}

type BenchmarkResult struct {
	ModelName   string              `json:"model_name" yaml:"model-name"`
	InputTokens int                 `json:"input_tokens" yaml:"input-tokens"`
	MaxTokens   int                 `json:"output_tokens" yaml:"output-tokens"` // Historically been called Output Tokens
	Latency     float64             `json:"latency" yaml:"latency"`
	Results     []utils.SpeedResult `json:"results" yaml:"results"`
}
