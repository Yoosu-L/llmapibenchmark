package utils

import (
	"math"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Yoosu-L/llmapibenchmark/internal/api"

	"github.com/openai/openai-go/v3"
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

	// Client is shared with the rest of the benchmark so transport settings
	// (retries, TLS) apply consistently.
	Client openai.Client
}

type SpeedResult struct {
	Concurrency      int     `json:"concurrency" yaml:"concurrency"`
	GenerationSpeed  float64 `json:"generation_speed" yaml:"generation-speed"`
	PromptThroughput float64 `json:"prompt_throughput" yaml:"prompt-throughput"`
	MaxTtft          float64 `json:"max_ttft" yaml:"max-ttft"`
	MinTtft          float64 `json:"min_ttft" yaml:"min-ttft"`
	SuccessRate      float64 `json:"success_rate" yaml:"success-rate"`
	Duration         float64 `json:"duration" yaml:"duration"`

	// PromptTtftSource records how TTFT was determined. Prompt TPS and TTFT
	// columns are only meaningful for api.TtftSourceContent; otherwise they
	// describe the end of the stream and are displayed as "n/a".
	PromptTtftSource string `json:"prompt_ttft_source" yaml:"prompt-ttft-source"`
	// PromptTtftField is the delta key that produced the first token, e.g.
	// "reasoning" or "content". "mixed" when requests disagreed.
	PromptTtftField string `json:"prompt_ttft_field" yaml:"prompt-ttft-field"`
	// TtftFallbackRequests counts the requests whose TTFT fell back to finish_reason.
	TtftFallbackRequests int `json:"ttft_fallback_requests" yaml:"ttft-fallback-requests"`
}

// TtftAvailable reports whether the TTFT-derived columns carry a real measurement.
func (measurement SpeedResult) TtftAvailable() bool {
	return measurement.PromptTtftSource == api.TtftSourceContent
}

func roundToTwoDecimals(f float64) float64 {
	return math.Round(f*100) / 100
}

// Run measures API generation throughput and TTFT.
func (setup *SpeedMeasurement) Run(bar *progressbar.ProgressBar) (SpeedResult, error) {
	client := setup.Client

	var wg sync.WaitGroup
	var responseTokens sync.Map
	var promptTokens sync.Map
	var ttfts sync.Map
	var ttftSources sync.Map
	var ttftFields sync.Map
	var successfulRequests atomic.Int32
	var failedRequests atomic.Int32

	start := time.Now()

	// Send requests concurrently (restored from debugging version)
	for i := 0; i < setup.Concurrency; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			var chat api.ChatResult
			var err error
			if setup.UseRandomInput {
				chat, err = api.AskOpenAiRandomInput(client, setup.ModelName, setup.NumWords, setup.MaxTokens, bar)
			} else {
				chat, err = api.AskOpenAi(client, setup.ModelName, setup.Prompt, setup.MaxTokens, bar)
			}
			if err != nil {
				failedRequests.Add(1)
				return
			}
			successfulRequests.Add(1)
			ttfts.Store(index, chat.TTFT)
			ttftSources.Store(index, chat.TTFTSource)
			ttftFields.Store(index, chat.TTFTField)
			responseTokens.Store(index, chat.CompletionTokens)
			promptTokens.Store(index, chat.PromptTokens)
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

	// Determine whether TTFT reflects prompt processing for every request.
	// One fallback request is enough to distrust the row: Prompt TPS uses the max TTFT.
	contentRequests, fallbackRequests := 0, 0
	ttftSources.Range(func(_, value interface{}) bool {
		switch value.(string) {
		case api.TtftSourceContent:
			contentRequests++
		case api.TtftSourceFallback:
			fallbackRequests++
		}
		return true
	})
	measurement.TtftFallbackRequests = fallbackRequests
	switch {
	case contentRequests > 0 && fallbackRequests == 0:
		measurement.PromptTtftSource = api.TtftSourceContent
	case contentRequests > 0 || fallbackRequests > 0:
		measurement.PromptTtftSource = api.TtftSourceFallback
	default:
		measurement.PromptTtftSource = api.TtftSourceNone
	}

	// Record which delta key produced the first token.
	fieldCounts := map[string]int{}
	ttftFields.Range(func(_, value interface{}) bool {
		if field, ok := value.(string); ok && field != "" {
			fieldCounts[field]++
		}
		return true
	})
	switch len(fieldCounts) {
	case 0:
		measurement.PromptTtftField = ""
	case 1:
		for field := range fieldCounts {
			measurement.PromptTtftField = field
		}
	default:
		measurement.PromptTtftField = "mixed"
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
	measurement.Duration = roundToTwoDecimals(float64(duration.Seconds()))

	// Calculate speed (tokens/second)
	// Ensure we don't divide by zero or negative values
	genDuration := duration.Seconds() - setup.Latency/1000
	if genDuration <= 0 {
		genDuration = duration.Seconds() // Fallback to total duration if latency is weird
	}
	measurement.GenerationSpeed = roundToTwoDecimals(float64(totalResponseTokens) / genDuration)

	// Calculate Prompt Throughput
	// Prompt TPS = Total Prompt Tokens / Max TTFT
	// We subtract network latency to get a better estimate of the model's prompt processing speed
	promptDuration := measurement.MaxTtft - setup.Latency/1000
	if promptDuration <= 0 {
		// If TTFT is very low (e.g. cached or local), use MaxTtft directly
		if measurement.MaxTtft > 0 {
			promptDuration = measurement.MaxTtft
		} else {
			promptDuration = duration.Seconds() // Fallback
		}
	}
	measurement.PromptThroughput = roundToTwoDecimals(float64(totalPromptTokens) / promptDuration)

	return measurement, nil
}
