# LLM API Benchmark Tool

## Overview

The LLM API Benchmark Tool is a flexible Go-based utility designed to measure and analyze the performance of OpenAI-compatible API endpoints across different concurrency levels. This tool provides in-depth insights into API throughput, generation speed, and token processing capabilities.

## Key Features

- 🚀 Dynamic Concurrency Testing
- 📊 Comprehensive Performance Metrics
- 🔍 Flexible Configuration
- 📝 Markdown Result Reporting
- 🌐 Compatible with Any OpenAI-Like API
- 📏 Arbitrary Length Dynamic Input Prompt

## Performance Metrics Measured

1. **Decode Throughput (Decode TPS)**
   - Measures pure decoding speed: completion tokens divided by decode time only
   - Decode time excludes prefill: per request it is the total time after the first token
   - Calculates across multiple concurrency levels

2. **Prompt Throughput**
   - Analyzes input token processing speed
   - Helps understand API's prompt handling efficiency

3. **Time to First Token (TTFT)**
   - Measures initial response latency
   - Provides both minimum and maximum TTFT
   - Critical for understanding real-time responsiveness

## Example Output
```
Input Tokens: 45
Output Tokens: 512
Test Model: Qwen2.5-7B-Instruct-AWQ
Latency: 2.20 ms
```

| Concurrency | Decode Throughput (tokens/s) |  Prompt Throughput (tokens/s) | Min TTFT (s) | Max TTFT (s) |
|-------------|----------------------------------|-------------------------------|--------------|--------------|
|           1 |                            58.49 |                        846.81 |         0.05 |         0.05 |
|           2 |                           114.09 |                        989.94 |         0.08 |         0.09 |
|           4 |                           222.62 |                       1193.99 |         0.11 |         0.15 |
|           8 |                           414.35 |                       1479.76 |         0.11 |         0.24 |
|          16 |                           752.26 |                       1543.29 |         0.13 |         0.47 |
|          32 |                           653.94 |                       1625.07 |         0.14 |         0.89 |


## Usage
### [Quick Start Guide](https://pikoo.de/posts/llm_api_performance_evaluation_tool_guide/)

### Minimal Configuration

**Linux:**
```bash
./llmapibenchmark_linux_amd64 --base-url https://your-api-endpoint.com/v1
```

**Windows:**
```cmd
llmapibenchmark_windows_amd64.exe --base-url https://your-api-endpoint.com/v1
```

### Full Configuration

**Linux:**
```bash
./llmapibenchmark_linux_amd64 \
  --base-url https://your-api-endpoint.com/v1 \
  --api-key YOUR_API_KEY \
  --model gpt-3.5-turbo \
  --concurrency 1,2,4,8,16 \
  --max-tokens 512 \
  --num-words 513 \
  --prompt "Your custom prompt here" \
  --format json
```

**Windows:**
```cmd
llmapibenchmark_windows_amd64.exe ^
  --base-url https://your-api-endpoint.com/v1 ^
  --api-key YOUR_API_KEY ^
  --model gpt-3.5-turbo ^
  --concurrency 1,2,4,8,16 ^
  --max-tokens 512 ^
  --num-words 513 ^
  --prompt "Your custom prompt here" ^
  --format json
```

## Command-Line Parameters

| Parameter | Short | Description | Default | Required |
|---|---|---|---|---|
| `--base-url` | `-u` | Base URL for LLM API endpoint | Empty (MUST be specified) | Yes |
| `--api-key` | `-k` | API authentication key | None | No |
| `--model` | `-m` | Specific AI model to test | Automatically discovers first available model | No |
| `--concurrency` | `-c` | Comma-separated concurrency levels to test | `1,2,4,8,16,32,64,128` | No |
| `--max-tokens` | `-t` | Maximum tokens to generate per request | `512` | No |
| `--num-words` | `-n` | Number of words for random input prompt | `0` | No |
| `--prompt` | `-p` | Text prompt for generating responses | A long story | No |
| `--format` | `-f` | Output format (json, yaml) | `""` | No |
| `--help` | `-h` | Show help message | `false` | No |

## Output

The tool provides output in multiple formats, controlled by the `--format` flag.

### Default (CLI Table and Markdown File)

If no format is specified, the tool generates:
1.  **Real-time console results**: A table is displayed in the terminal with live updates.
2.  **Markdown file**: A detailed report is saved to `API_Throughput_{ModelName}.md`.

**Markdown File Columns:**
- **Concurrency**: Number of concurrent requests
- **Decode Throughput**: Tokens generated per second of pure decoding time (prefill excluded)
- **Prompt Throughput**: Input token processing speed
- **Min TTFT**: Minimum time to first token
- **Max TTFT**: Maximum time to first token

### JSON Output (`--format json`)

When using the `--format json` flag, the results are printed to the console in JSON format.

### YAML Output (`--format yaml`)

When using the `--format yaml` flag, the results are printed to the console in YAML format.

## Best Practices

- Test with various prompt lengths and complexities
- Compare different models
- Monitor for consistent performance
- Be mindful of API rate limits
- Use `-numWords` to control input length

## Limitations

- Requires active API connection
- Results may vary based on network conditions
- Does not simulate real-world complex scenarios

## Disclaimer

This tool is for performance analysis and should be used responsibly in compliance with API provider's usage policies.