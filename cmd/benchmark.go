package main

import (
	"fmt"
	"os"
	"strings"
	"sync/atomic"

	"github.com/Yoosu-L/llmapibenchmark/internal/utils"
	"github.com/schollz/progressbar/v3"
)

func (benchmark *Benchmark) runCli() error {
	// Test latency
	latency, err := utils.MeasureLatency(benchmark.BaseURL, 5)
	if err != nil {
		return fmt.Errorf("latency test error: %v", err)
	}

	bold := "\033[1m"
	green := "\033[32m"
	dim := "\033[2m"
	reset := "\033[0m"
	// Drop ANSI colors on redirect too; agents parsing stdout only want text.
	stdoutIsTTY := utils.IsTerminal(os.Stdout)
	if !stdoutIsTTY {
		bold, green, dim, reset = "", "", "", ""
	}
	printTableHeader := func() {
		fmt.Printf("%s%s| Conc | Decode TPS | Prompt TPS | Min TTFT(s) | Max TTFT(s) | Success | Total(s) |%s\n", green, bold, reset)
		fmt.Printf("%s|:----:|:----------:|:----------:|:-----------:|:-----------:|:-------:|:--------:|%s\n", green, reset)
	}

	// The real input token count only arrives in the final usage chunk of the
	// first benchmark request, so the header is printed after the first round
	// instead of paying for a separate probe request up front.
	var inputTokens atomic.Int64

	// Test each concurrency level and print results
	var results []utils.SpeedResult
	var notices []string
	headerPrinted := false
	for _, concurrency := range benchmark.ConcurrencyLevels {
		result, err := benchmark.measureSpeed(latency, concurrency, true, &inputTokens)
		if err != nil {
			return fmt.Errorf("concurrency %d: %v", concurrency, err)
		}

		if !headerPrinted {
			benchmark.InputTokens = int(inputTokens.Load())
			utils.PrintBenchmarkHeader(benchmark.ModelName, benchmark.InputTokens, benchmark.MaxTokens, latency)
			printTableHeader()
			headerPrinted = true
		}

		// Print current results
		fmt.Printf("%s%s%s\n", green, utils.FormatRow(result), reset)

		// Save results for later
		results = append(results, result)
		if notice := utils.TtftNotice(result); notice != "" {
			notices = append(notices, notice)
		}
	}

	if !headerPrinted {
		// Nothing was measured (e.g. empty concurrency list); still print the
		// header so the closing separator is not orphaned.
		utils.PrintBenchmarkHeader(benchmark.ModelName, benchmark.InputTokens, benchmark.MaxTokens, latency)
		printTableHeader()
	}

	fmt.Printf("%s|:----:|:----------:|:----------:|:-----------:|:-----------:|:-------:|:--------:|%s\n", green, reset)
	for _, notice := range notices {
		fmt.Printf("%s→ %s%s\n", dim, notice, reset)
	}
	fmt.Println("\n" + green + strings.Repeat("=", 80) + reset)

	// Save results to Markdown only when --output was given.
	if benchmark.OutputPath != "" {
		utils.SaveResultsToMD(benchmark.OutputPath, results, benchmark.ModelName, benchmark.InputTokens, benchmark.MaxTokens, latency)
	}

	return nil
}

func (benchmark *Benchmark) run() (BenchmarkResult, error) {
	result := BenchmarkResult{}
	result.ModelName = benchmark.ModelName
	result.InputTokens = benchmark.InputTokens
	result.MaxTokens = benchmark.MaxTokens

	// Test latency
	latency, err := utils.MeasureLatency(benchmark.BaseURL, 5)
	if err != nil {
		return result, fmt.Errorf("error testing latency: %v", err)
	}
	result.Latency = latency

	var inputTokens atomic.Int64
	for _, concurrency := range benchmark.ConcurrencyLevels {
		measurement, err := benchmark.measureSpeed(latency, concurrency, false, &inputTokens)
		if err != nil {
			return result, fmt.Errorf("concurrency %d: %v", concurrency, err)
		}

		// Backfill from the first successful request's usage chunk.
		if result.InputTokens == 0 {
			result.InputTokens = int(inputTokens.Load())
		}

		result.Results = append(result.Results, measurement)
	}

	return result, nil
}

func (benchmark *Benchmark) measureSpeed(latency float64, concurrency int, clearProgress bool, inputTokens *atomic.Int64) (utils.SpeedResult, error) {

	// Progress bar and terminal escapes only make sense on an interactive
	// stderr; with a pipe/redirect they would flood the captured output.
	interactive := utils.IsTerminal(os.Stderr)

	var bar *progressbar.ProgressBar
	if interactive {
		// Disable terminal auto-wrap (DECAWM) to prevent the progress bar from breaking into multiple new lines
		fmt.Fprint(os.Stderr, "\x1b[?7l")
		// Re-enable terminal auto-wrap when the function returns
		defer fmt.Fprint(os.Stderr, "\x1b[?7h")

		// Create a progress bar for this specific concurrency level
		expectedTokens := concurrency * benchmark.MaxTokens
		// Pad description to a fixed length for consistent alignment
		description := fmt.Sprintf("Conc %-2d", concurrency)
		barWidth := 20

		bar = progressbar.NewOptions(expectedTokens,
			progressbar.OptionSetWriter(os.Stderr),
			progressbar.OptionSetDescription(description),
			progressbar.OptionSetWidth(barWidth),
			progressbar.OptionUseANSICodes(true),
			progressbar.OptionShowCount(),
			progressbar.OptionShowIts(),
			progressbar.OptionSetItsString("tokens"),
			progressbar.OptionSpinnerType(14),
			progressbar.OptionSetRenderBlankState(true),
		)
	}

	speedMeasurement := utils.SpeedMeasurement{
		BaseUrl:     benchmark.BaseURL,
		ApiVersion:  benchmark.ApiVersion,
		ApiKey:      benchmark.ApiKey,
		ModelName:   benchmark.ModelName,
		Prompt:      benchmark.Prompt,
		NumWords:    benchmark.NumWords,
		MaxTokens:   benchmark.MaxTokens,
		Latency:     latency,
		Concurrency: concurrency,
		Client:      benchmark.Client,
	}
	if benchmark.UseRandomInput {
		speedMeasurement.UseRandomInput = true
	}
	speedMeasurement.PromptTokensSink = inputTokens

	result, err := speedMeasurement.Run(bar)
	if err != nil {
		return result, fmt.Errorf("measurement error: %v", err)
	}

	if bar != nil {
		bar.Finish()
		if clearProgress {
			bar.Clear()
		} else {
			fmt.Fprintf(os.Stderr, "\n")
		}
		bar.Close()
	}

	return result, nil
}
