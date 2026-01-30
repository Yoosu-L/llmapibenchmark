package utils

import (
	"math"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Yoosu-L/llmapibenchmark/internal/api"

	"github.com/sashabaranov/go-openai"
	"github.com/schollz/progressbar/v3"
)

type SpeedMeasurement struct {
	BaseUrl        string
	ApiVersion     string
	ApiKey         string
	ModelName      string
	Prompt         string
	UseRandomInput bool
	NumWords       int
	MaxTokens      int
	Latency        float64
	Concurrency    int
}

type SpeedResult struct {
	Concurrency      int     `json:"concurrency" yaml:"concurrency"`
	GenerationSpeed  float64 `json:"generation_speed" yaml:"generation-speed"`
	PromptThroughput float64 `json:"prompt_throughput" yaml:"prompt-throughput"`
	MaxTtft          float64 `json:"max_ttft" yaml:"max-ttft"`
	MinTtft          float64 `json:"min_ttft" yaml:"min-ttft"`
	SuccessRate      float64 `json:"success_rate" yaml:"success-rate"`
}

func roundToTwoDecimals(f float64) float64 {
	return math.Round(f*100) / 100
}

// Run measures API generation throughput and TTFT.
func (setup *SpeedMeasurement) Run(bar *progressbar.ProgressBar) (SpeedResult, error) {
	config := openai.DefaultConfig(setup.ApiKey)
	config.BaseURL = setup.BaseUrl
	config.APIVersion = setup.ApiVersion
	client := openai.NewClientWithConfig(config)

	var wg sync.WaitGroup
	var responseTokens sync.Map
	var promptTokens sync.Map
	var ttfts sync.Map
	var successfulRequests atomic.Int32
	var failedRequests atomic.Int32

	start := time.Now()

	// Send requests concurrently (restored from debugging version)
	for i := 0; i < setup.Concurrency; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			var ttft float64
			var completionTokens, inputTokens int
			var err error
			if setup.UseRandomInput {
				ttft, completionTokens, inputTokens, err = api.AskOpenAiRandomInput(client, setup.ModelName, setup.NumWords, setup.MaxTokens, bar)
			} else {
				ttft, completionTokens, inputTokens, err = api.AskOpenAi(client, setup.ModelName, setup.Prompt, setup.MaxTokens, bar)
			}
			if err != nil {
				failedRequests.Add(1)
				return
			}
			successfulRequests.Add(1)
			ttfts.Store(index, ttft)
			responseTokens.Store(index, completionTokens)
			promptTokens.Store(index, inputTokens)
		}(i)
	}

	wg.Wait()
	duration := time.Since(start)

	// Calculate total tokens
	totalResponseTokens := 0
	responseTokens.Range(func(_, value interface{}) bool {
		totalResponseTokens += value.(int)
		return true
	})

	totalPromptTokens := 0
	promptTokens.Range(func(_, value interface{}) bool {
		totalPromptTokens += value.(int)
		return true
	})

	measurement := SpeedResult{}
	measurement.Concurrency = setup.Concurrency

	// Calculate success rate
	totalRequests := setup.Concurrency
	if totalRequests > 0 {
		measurement.SuccessRate = float64(successfulRequests.Load()) / float64(totalRequests)
	}

	// Calculate max and min TTFT
	measurement.MaxTtft = 0.0
	measurement.MinTtft = math.Inf(1)
	ttfts.Range(func(_, value interface{}) bool {
		ttft := value.(float64)
		if ttft > measurement.MaxTtft {
			measurement.MaxTtft = ttft
		}
		if ttft < measurement.MinTtft {
			measurement.MinTtft = ttft
		}
		return true
	})
	measurement.MaxTtft = roundToTwoDecimals(measurement.MaxTtft)
	measurement.MinTtft = roundToTwoDecimals(measurement.MinTtft)

	// Calculate speed (tokens/second)
	measurement.GenerationSpeed = roundToTwoDecimals(float64(totalResponseTokens) / (duration.Seconds() - setup.Latency/1000))

	// Calculate Prompt Throughput
	measurement.PromptThroughput = roundToTwoDecimals(float64(totalPromptTokens) / (measurement.MaxTtft - setup.Latency/1000))

	return measurement, nil
}
