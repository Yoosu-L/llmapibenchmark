package utils

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
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

// IsTerminal reports whether f is attached to an interactive terminal.
// Pipes and redirected files are not: progress bars and per-token redraws
// would flood agent/CI output, so callers disable them.
func IsTerminal(f *os.File) bool {
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

// PrintBenchmarkHeader prints the benchmark header with details about the test.
func PrintBenchmarkHeader(modelName string, inputTokens int, maxTokens int, latency float64) {
	width := 80
	title := "LLM API Throughput Benchmark"
	url := "https://github.com/Yoosu-L/llmapibenchmark"
	timeStr := fmt.Sprintf("Time: %s", time.Now().UTC().Format("2006-01-02 15:04:05 UTC+0"))

	// ANSI Colors; plain text when stdout is redirected.
	cyan, green, dim, bold, reset := "\033[36m", "\033[32m", "\033[2m", "\033[1m", "\033[0m"
	if !IsTerminal(os.Stdout) {
		cyan, green, dim, bold, reset = "", "", "", "", ""
	}

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

// SaveResultsToMD prepends the benchmark results to the Markdown file at path.
// Existing content is kept below, so the newest run is always at the top.
// Returns the resolved file path, or "" on failure.
func SaveResultsToMD(path string, results []SpeedResult, modelName string, inputTokens int, maxTokens int, latency float64) string {
	// Resolve the target file: bare --output or a directory gets the default name.
	if !strings.HasSuffix(strings.ToLower(path), ".md") {
		path = filepath.Join(path, "llm-api-benchmark.md")
	}
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			log.Printf("Error creating output directory: %v", err)
			return ""
		}
	}

	var block strings.Builder
	block.WriteString(fmt.Sprintf("## %s\n\n", time.Now().Format("2006-01-02 15:04:05")))
	block.WriteString(fmt.Sprintf("```\nModel: %s\nLatency: %.2f ms\nInput: %d tokens / Output: %d tokens\n```\n\n", modelName, latency, inputTokens, maxTokens))
	block.WriteString("| Conc | Decode TPS | Prompt TPS | Min TTFT(s) | Max TTFT(s) | Success | Total(s) |\n")
	block.WriteString("|:----:|:----------:|:----------:|:-----------:|:-----------:|:-------:|:--------:|\n")

	var notices []string
	for _, measurement := range results {
		block.WriteString(FormatRow(measurement) + "\n")
		if notice := TtftNotice(measurement); notice != "" {
			notices = append(notices, notice)
		}
	}

	for _, notice := range notices {
		block.WriteString("\n> " + notice + "\n")
	}

	// Prepend: newest results first, previous history below.
	existing, err := os.ReadFile(path)
	if err == nil && len(existing) > 0 {
		block.WriteString("\n---\n\n" + string(existing))
	}
	if err := os.WriteFile(path, []byte(block.String()), 0o644); err != nil {
		log.Printf("Error writing file: %v", err)
		return ""
	}

	fmt.Printf("Results saved to: %s\n\n", path)
	return path
}
