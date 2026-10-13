package utils

import (
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/Yoosu-L/llmapibenchmark/internal/api"
)

// TtftNotAvailable is printed when a TTFT-derived metric is not a real measurement.
const TtftNotAvailable = "n/a"

// PromptTpsCell returns the Prompt TPS column value, or "n/a" when TTFT never
// fired on a content field (the number would describe the whole stream).
func PromptTpsCell(measurement SpeedResult) interface{} {
	if !measurement.TtftAvailable() {
		return TtftNotAvailable
	}
	return measurement.PromptThroughput
}

// MinTtftCell returns the value to print for the Min TTFT column.
func MinTtftCell(measurement SpeedResult) interface{} {
	if !measurement.TtftAvailable() {
		return TtftNotAvailable
	}
	return measurement.MinTtft
}

// MaxTtftCell returns the value to print for the Max TTFT column.
func MaxTtftCell(measurement SpeedResult) interface{} {
	if !measurement.TtftAvailable() {
		return TtftNotAvailable
	}
	return measurement.MaxTtft
}

// FormatCell renders a value with the printf widths the tables use.
func FormatCell(value interface{}, width int, decimals int) string {
	switch typed := value.(type) {
	case string:
		return fmt.Sprintf("%*s", width, typed)
	case int:
		return fmt.Sprintf("%*d", width, typed)
	case float64:
		return fmt.Sprintf("%*.*f", width, decimals, typed)
	default:
		return fmt.Sprintf("%*v", width, value)
	}
}

// FormatRow renders one result row, shared by the console table and the Markdown report.
func FormatRow(measurement SpeedResult) string {
	return fmt.Sprintf("| %s | %s | %s | %s | %s | %s | %s |",
		FormatCell(measurement.Concurrency, 4, 0),
		FormatCell(measurement.DecodeTps, 10, 2),
		FormatCell(PromptTpsCell(measurement), 10, 2),
		FormatCell(MinTtftCell(measurement), 11, 2),
		FormatCell(MaxTtftCell(measurement), 11, 2),
		FormatCell(measurement.SuccessRate*100, 6, 2)+"%",
		FormatCell(measurement.Duration, 8, 2),
	)
}

// TtftNotice explains why a row shows "n/a", or an empty string when it does not.
func TtftNotice(measurement SpeedResult) string {
	switch measurement.PromptTtftSource {
	case api.TtftSourceFallback:
		return fmt.Sprintf(
			"concurrency %d: TTFT never fired on a content field (%d/%d requests fell back to finish_reason); Prompt TPS and TTFT shown as n/a, Decode TPS falls back to total duration.",
			measurement.Concurrency, measurement.TtftFallbackRequests, measurement.Concurrency)
	case api.TtftSourceNone:
		return fmt.Sprintf("concurrency %d: no valid TTFT.", measurement.Concurrency)
	}
	return ""
}

// PrintBenchmarkHeader prints the benchmark header with details about the test.
func PrintBenchmarkHeader(modelName string, inputTokens int, maxTokens int, latency float64) {
	width := 80
	title := "LLM API Throughput Benchmark"
	url := "https://github.com/Yoosu-L/llmapibenchmark"
	timeStr := fmt.Sprintf("Time: %s", time.Now().UTC().Format("2006-01-02 15:04:05 UTC+0"))

	// ANSI Colors
	cyan := "\033[36m"
	green := "\033[32m"
	dim := "\033[2m"
	bold := "\033[1m"
	reset := "\033[0m"

	border := strings.Repeat("#", width)

	center := func(s string, w int) string {
		if len(s) >= w {
			return s
		}
		left := (w - len(s)) / 2
		right := w - len(s) - left
		return strings.Repeat(" ", left) + s + strings.Repeat(" ", right)
	}

	fmt.Println(cyan + border + reset)
	fmt.Println(bold + center(title, width) + reset)
	fmt.Println(dim + center(url, width) + reset)
	fmt.Println(dim + center(timeStr, width) + reset)
	fmt.Println(cyan + border + reset)

	fmt.Printf("%s%sModel:%s %-25s | %sLatency:%s %.2f ms%s\n", green, bold, reset+green, modelName, bold, reset+green, latency, reset)
	fmt.Printf("%s%sInput:%s %-25d | %sOutput:%s  %d tokens%s\n\n", green, bold, reset+green, inputTokens, bold, reset+green, maxTokens, reset)
}

// SaveResultsToMD saves the benchmark results to a Markdown file.
func SaveResultsToMD(results []SpeedResult, modelName string, inputTokens int, maxTokens int, latency float64) {
	// sanitize modelName to create a safe filename (replace path separators)
	safeModelName := strings.ReplaceAll(modelName, "/", "_")
	safeModelName = strings.ReplaceAll(safeModelName, "\\", "_")
	safeModelName = strings.TrimSpace(safeModelName)
	if safeModelName == "" {
		safeModelName = "model"
	}
	filename := fmt.Sprintf("API_Throughput_%s.md", safeModelName)
	file, err := os.Create(filename)
	if err != nil {
		log.Printf("Error creating file: %v", err)
		return
	}
	defer file.Close()

	file.WriteString(fmt.Sprintf("```\nModel: %s\nLatency: %.2f ms\nInput: %d tokens / Output: %d tokens\n```\n\n", modelName, latency, inputTokens, maxTokens))
	file.WriteString("| Conc | Decode TPS | Prompt TPS | Min TTFT(s) | Max TTFT(s) | Success | Total(s) |\n")
	file.WriteString("|:----:|:----------:|:----------:|:-----------:|:-----------:|:-------:|:--------:|\n")

	var notices []string
	for _, measurement := range results {
		file.WriteString(FormatRow(measurement) + "\n")
		if notice := TtftNotice(measurement); notice != "" {
			notices = append(notices, notice)
		}
	}

	for _, notice := range notices {
		file.WriteString("\n> " + notice + "\n")
	}

	fmt.Printf("Results saved to: %s\n\n", filename)
}
