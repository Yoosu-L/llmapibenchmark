package main

import (
	"crypto/tls"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/Yoosu-L/llmapibenchmark/internal/api"
	"github.com/Yoosu-L/llmapibenchmark/internal/utils"
	"github.com/sashabaranov/go-openai"
	"github.com/spf13/pflag"
)

const (
	defaultPrompt = "Write a long story, no less than 10,000 words, starting from a long, long time ago."
)

func main() {
	baseURL := pflag.StringP("base-url", "u", "", "Base URL of the OpenAI API")
	apiVersion := pflag.StringP("api-version", "v", "", "API version (api-version) query parameter value")
	apiKey := pflag.StringP("api-key", "k", "", "API key for authentication")
	model := pflag.StringP("model", "m", "", "Model to be used for the requests (optional)")
	prompt := pflag.StringP("prompt", "p", defaultPrompt, "Prompt to be used for generating responses")
	numWords := pflag.IntP("num-words", "n", 0, "If set to a value above 0 a random string with this length will be used as prompt")
	concurrencyStr := pflag.StringP("concurrency", "c", "1,2,4,8,16,32,64,128", "Comma-separated list of concurrency levels")
	maxTokens := pflag.IntP("max-tokens", "t", 512, "Maximum number of tokens to generate")
	format := pflag.StringP("format", "f", "", "Output format (optional)")
	help := pflag.BoolP("help", "h", false, "Show this help message")
	insecureSkipTLSVerify := pflag.Bool("insecure-skip-tls-verify", false, "Skip TLS certificate verification. Use with caution, this is insecure.")
	pflag.Parse()

	if *help {
		fmt.Printf("Usage of %s:\n", os.Args[0])
		pflag.PrintDefaults()
		os.Exit(0)
	}

	// Create benchmark
	benchmark := Benchmark{}
	benchmark.BaseURL = *baseURL
	benchmark.ApiVersion = *apiVersion
	benchmark.ApiKey = *apiKey
	benchmark.ModelName = *model
	benchmark.Prompt = *prompt
	benchmark.NumWords = *numWords
	benchmark.MaxTokens = *maxTokens

	// Parse concurrency levels
	concurrencyLevels, err := utils.ParseConcurrencyLevels(*concurrencyStr)
	if err != nil {
		log.Fatalf("Invalid concurrency levels: %v", err)
	}
	benchmark.ConcurrencyLevels = concurrencyLevels

	// Initialize OpenAI client
	if *baseURL == "" {
		log.Fatalf("--base-url is required")
	}
	config := openai.DefaultConfig(*apiKey)
	config.BaseURL = *baseURL
	config.APIVersion = *apiVersion

	if *insecureSkipTLSVerify {
		fmt.Fprintln(os.Stderr, "\n/!\\ WARNING: Skipping TLS certificate verification. This is insecure and should not be used in production. /!\\")

		// Clone the default Transport to preserve its settings
		defaultTransport, ok := http.DefaultTransport.(*http.Transport)
		if !ok {
			log.Fatalf("http.DefaultTransport is not an *http.Transport")
		}
		tr := defaultTransport.Clone()
		tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
		config.HTTPClient = &http.Client{Transport: tr}
	}

	client := openai.NewClientWithConfig(config)

	// Discover model name if not provided
	if *model == "" {
		discoveredModel, err := api.GetFirstAvailableModel(client)
		if err != nil {
			log.Printf("Error discovering model: %v", err)
			return
		}
		benchmark.ModelName = discoveredModel
	}

	// Determine input parameters and call benchmark function
	if *prompt != "Write a long story, no less than 10,000 words, starting from a long, long time ago." {
		benchmark.UseRandomInput = false
	} else if *numWords != 0 {
		benchmark.UseRandomInput = true
	} else {
		benchmark.UseRandomInput = false
	}

	// Get input tokens
	if benchmark.UseRandomInput {
		_, _, promptTokens, err := api.AskOpenAiRandomInput(client, benchmark.ModelName, *numWords/4, 4, nil)
		if err != nil {
			log.Fatalf("Error getting prompt tokens: %v", err)
		}
		benchmark.InputTokens = promptTokens
	} else {
		_, _, promptTokens, err := api.AskOpenAi(client, benchmark.ModelName, *prompt, 4, nil)
		if err != nil {
			log.Fatalf("Error getting prompt tokens: %v", err)
		}
		benchmark.InputTokens = promptTokens
	}

	if *format == "" {
		err := benchmark.runCli()
		if err != nil {
			log.Fatalf("Error running benchmark: %v", err)
		}
	} else {
		result, err := benchmark.run()
		if err != nil {
			log.Fatalf("Error running benchmark: %v", err)
		}

		var output string
		switch *format {
		case "json":
			output, err = result.Json()
		case "yaml":
			output, err = result.Yaml()
		default:
			log.Printf("Invalid format specified")
		}
		if err != nil {
			log.Fatalf("Error formatting benchmark result: %v", err)
		}
		fmt.Println(output)
	}
}
