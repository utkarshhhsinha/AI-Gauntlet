package agent

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ── Gemini REST API types ─────────────────────────────────────────────────────

const geminiBaseURL = "https://generativelanguage.googleapis.com/v1beta"

type geminiContent struct {
	Role  string          `json:"role"`
	Parts []geminiPart    `json:"parts"`
}

type geminiPart struct {
	Text             string                  `json:"text,omitempty"`
	ThoughtSignature string                  `json:"thoughtSignature,omitempty"`
	FunctionCall     *geminiFunctionCall     `json:"functionCall,omitempty"`
	FunctionResponse *geminiFunctionResponse `json:"functionResponse,omitempty"`
}

type geminiFunctionCall struct {
	Name string         `json:"name"`
	Args map[string]any `json:"args"`
}

type geminiFunctionResponse struct {
	Name     string         `json:"name"`
	Response map[string]any `json:"response"`
}

type geminiTool struct {
	FunctionDeclarations []geminiFunctionDecl `json:"functionDeclarations"`
}

type geminiFunctionDecl struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters,omitempty"`
}

type geminiGenerateRequest struct {
	Contents          []geminiContent         `json:"contents"`
	Tools             []geminiTool            `json:"tools,omitempty"`
	SystemInstruction *geminiContent          `json:"systemInstruction,omitempty"`
	GenerationConfig  *geminiGenerationConfig `json:"generationConfig,omitempty"`
}

type geminiGenerationConfig struct {
	MaxOutputTokens int     `json:"maxOutputTokens,omitempty"`
	Temperature     float64 `json:"temperature,omitempty"`
}

type geminiStreamResponse struct {
	Candidates []struct {
		Content       geminiContent `json:"content"`
		FinishReason  string        `json:"finishReason"`
		FinishMessage string        `json:"finishMessage,omitempty"`
	} `json:"candidates"`
	UsageMetadata *struct {
		PromptTokenCount     int `json:"promptTokenCount"`
		CandidatesTokenCount int `json:"candidatesTokenCount"`
		TotalTokenCount      int `json:"totalTokenCount"`
	} `json:"usageMetadata"`
	Error *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Status  string `json:"status"`
	} `json:"error"`
}

// ── Model discovery ───────────────────────────────────────────────────────────

type geminiModelListResponse struct {
	Models []struct {
		Name                       string   `json:"name"`
		DisplayName                string   `json:"displayName"`
		SupportedGenerationMethods []string `json:"supportedGenerationMethods"`
	} `json:"models"`
}

// discoverGeminiModels fetches the live model list from the Gemini API using
// the provided API key. Only models that support generateContent (and thus
// tool-calling agentic flows) are returned.
func discoverGeminiModels(ctx context.Context, apiKey string) ([]Model, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("google: GOOGLE_API_KEY not set")
	}

	reqCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	url := fmt.Sprintf("%s/models?key=%s&pageSize=100", geminiBaseURL, apiKey)
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("google: build list-models request: %w", err)
	}
	req.Header.Set("User-Agent", "ai-gauntlet-daemon/1.0")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("google: list models: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("google: read list-models response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("google: list models HTTP %d: %s", resp.StatusCode, string(body))
	}

	var listing geminiModelListResponse
	if err := json.Unmarshal(body, &listing); err != nil {
		return nil, fmt.Errorf("google: parse model list: %w", err)
	}

	var models []Model
	for _, m := range listing.Models {
		supportsGenerate := false
		for _, method := range m.SupportedGenerationMethods {
			if method == "generateContent" {
				supportsGenerate = true
				break
			}
		}
		if !supportsGenerate {
			continue
		}
		// m.Name is "models/gemini-2.5-pro" — strip the prefix.
		id := strings.TrimPrefix(m.Name, "models/")
		if id == "" || strings.Contains(id, "embedding") || strings.Contains(id, "aqa") {
			continue
		}
		label := m.DisplayName
		if label == "" {
			label = id
		}
		models = append(models, Model{
			ID:       id,
			Label:    label,
			Provider: "google",
		})
	}

	// Sort models so newer versions (3.8 > 3.7 > 3.5 > 2.5) appear at the top of the UI picker
	sort.SliceStable(models, func(i, j int) bool {
		scoreI := googleModelRank(models[i].ID)
		scoreJ := googleModelRank(models[j].ID)
		if scoreI != scoreJ {
			return scoreI > scoreJ
		}
		return models[i].ID < models[j].ID
	})

	// Mark the latest flagship model as default
	for i := range models {
		if models[i].ID == "gemini-3.8-flash" || (models[i].ID == "gemini-3.7-flash" && models[0].ID != "gemini-3.8-flash") {
			models[i].Default = true
			break
		}
	}
	if len(models) > 0 && !hasDefault(models) {
		models[0].Default = true
	}

	return models, nil
}

var geminiVerRegex = regexp.MustCompile(`(?i)(?:^|[-_./:@ ])v?(\d+(?:\.\d+)?)(?:$|[-_./:@ ])`)

func googleModelRank(id string) float64 {
	lower := strings.ToLower(id)
	score := 0.0

	// Deprioritize non-text or specialized preview models
	if strings.Contains(lower, "tts") || strings.Contains(lower, "image") || strings.Contains(lower, "clip") ||
		strings.Contains(lower, "lyria") || strings.Contains(lower, "transcribe") || strings.Contains(lower, "robotics") {
		return -100.0
	}

	// Extract version
	clean := strings.ReplaceAll(lower, "04-2026", "")
	clean = strings.ReplaceAll(clean, "05-2026", "")
	clean = strings.ReplaceAll(clean, "10-2025", "")
	clean = strings.ReplaceAll(clean, "12-2025", "")
	matches := geminiVerRegex.FindAllStringSubmatch(clean, -1)
	ver := 0.0
	for _, m := range matches {
		if len(m) >= 2 {
			if v, err := strconv.ParseFloat(m[1], 64); err == nil && v < 20.0 {
				if v > ver {
					ver = v
				}
			}
		}
	}

	score += ver * 10.0
	if strings.Contains(lower, "flash") && !strings.Contains(lower, "lite") {
		score += 5.0
	} else if strings.Contains(lower, "pro") {
		score += 4.0
	} else if strings.Contains(lower, "lite") {
		score += 3.0
	}
	if strings.Contains(lower, "latest") {
		score += 1.0
	}
	return score
}

func hasDefault(models []Model) bool {
	for _, m := range models {
		if m.Default {
			return true
		}
	}
	return false
}

// googleAPIKey resolves the Gemini API key from cfg.Env first (set by daemon
// when GOOGLE_API_KEY is in the process environment), then falls back to the
// process environment directly.
func googleAPIKey(cfg Config) string {
	if k, ok := cfg.Env["GOOGLE_API_KEY"]; ok && k != "" {
		return k
	}
	if k, ok := cfg.Env["GEMINI_API_KEY"]; ok && k != "" {
		return k
	}
	if k := os.Getenv("GOOGLE_API_KEY"); k != "" {
		return k
	}
	if k := os.Getenv("GEMINI_API_KEY"); k != "" {
		return k
	}
	return readKeyFromEnvFile()
}

func readKeyFromEnvFile() string {
	dir, err := os.Getwd()
	if err != nil {
		return ""
	}
	for i := 0; i < 5; i++ {
		candidate := filepath.Join(dir, ".env")
		if data, err := os.ReadFile(candidate); err == nil {
			for _, line := range strings.Split(string(data), "\n") {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "GOOGLE_API_KEY=") {
					val := strings.Trim(strings.TrimPrefix(line, "GOOGLE_API_KEY="), `"' `)
					if val != "" {
						return val
					}
				}
				if strings.HasPrefix(line, "GEMINI_API_KEY=") {
					val := strings.Trim(strings.TrimPrefix(line, "GEMINI_API_KEY="), `"' `)
					if val != "" {
						return val
					}
				}
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return ""
}

func defaultGoogleTools() []geminiTool {
	return []geminiTool{
		{
			FunctionDeclarations: []geminiFunctionDecl{
				{
					Name:        "bash",
					Description: "Execute a bash shell command in the workspace directory. Use this to inspect issues, read or write files, run ai-gauntlet CLI commands, and execute shell scripts.",
					Parameters: map[string]any{
						"type": "OBJECT",
						"properties": map[string]any{
							"command": map[string]any{
								"type":        "STRING",
								"description": "The command line to execute.",
							},
						},
						"required": []string{"command"},
					},
				},
			},
		},
	}
}

// ── Backend ───────────────────────────────────────────────────────────────────

// googleBackend implements Backend by calling the Gemini REST API directly.
// It streams responses via the streamGenerateContent SSE endpoint and supports
// tool-call round-trips so agents can use AI Gauntlet's MCP tools.
type googleBackend struct {
	cfg Config
}

func (b *googleBackend) Execute(ctx context.Context, prompt string, opts ExecOptions) (*Session, error) {
	apiKey := googleAPIKey(b.cfg)
	if apiKey == "" {
		return nil, fmt.Errorf("google: GOOGLE_API_KEY is not set — add it to your .env file")
	}

	model := opts.Model
	if model == "" {
		model = "gemini-2.5-flash" // sensible default if no model is selected
	}

	msgs := make(chan Message, 64)
	results := make(chan Result, 1)

	session := &Session{
		Messages: msgs,
		Result:   results,
	}

	go func() {
		defer close(msgs)
		defer close(results)

		result := b.run(ctx, prompt, opts, model, apiKey, msgs)
		results <- result
	}()

	return session, nil
}

func (b *googleBackend) run(
	ctx context.Context,
	prompt string,
	opts ExecOptions,
	model string,
	apiKey string,
	msgs chan<- Message,
) Result {
	start := time.Now()

	// Build conversation history — one user turn to start.
	history := []geminiContent{
		{Role: "user", Parts: []geminiPart{{Text: prompt}}},
	}

	// Optional system instruction from opts.SystemPrompt.
	var sysInstruction *geminiContent
	if opts.SystemPrompt != "" {
		sysInstruction = &geminiContent{
			Parts: []geminiPart{{Text: opts.SystemPrompt}},
		}
	}

	var totalInput, totalOutput int64
	var finalOutput strings.Builder
	const maxTurns = 50

	for turn := 0; turn < maxTurns; turn++ {
		var tools []geminiTool
		if turn < maxTurns-1 {
			tools = defaultGoogleTools()
		}

		reqBody := geminiGenerateRequest{
			Contents:          history,
			Tools:             tools,
			SystemInstruction: sysInstruction,
		}

		respParts, _, inputTok, outputTok, err := b.streamOneTurn(ctx, model, apiKey, reqBody, msgs)
		totalInput += int64(inputTok)
		totalOutput += int64(outputTok)

		if err != nil {
			return Result{
				Status:     "failed",
				Error:      fmt.Sprintf("google: %v", err),
				DurationMs: time.Since(start).Milliseconds(),
			}
		}

		// Collect text output from this turn.
		for _, p := range respParts {
			if p.Text != "" {
				finalOutput.WriteString(p.Text)
			}
		}

		// Check if the model wants to call tools.
		var toolCalls []geminiFunctionCall
		for _, p := range respParts {
			if p.FunctionCall != nil {
				toolCalls = append(toolCalls, *p.FunctionCall)
			}
		}

		if len(toolCalls) == 0 {
			// No more tool calls — we're done.
			durationMs := time.Since(start).Milliseconds()
			usage := map[string]TokenUsage{}
			if totalInput > 0 || totalOutput > 0 {
				usage[model] = TokenUsage{
					InputTokens:  totalInput,
					OutputTokens: totalOutput,
				}
			}
			return Result{
				Status:     "completed",
				Output:     strings.TrimSpace(finalOutput.String()),
				DurationMs: durationMs,
				Usage:      usage,
			}
		}

		// Append the model's response to history, then execute tools.
		history = append(history, geminiContent{Role: "model", Parts: respParts})

		var toolResults []geminiPart
		for _, tc := range toolCalls {
			inputJSON, _ := json.Marshal(tc.Args)
			msgs <- Message{
				Type:   MessageToolUse,
				Tool:   tc.Name,
				CallID: tc.Name,
				Input:  tc.Args,
			}

			var toolResult map[string]any
			if tc.Name == "bash" {
				cmdStr, _ := tc.Args["command"].(string)
				cmd := exec.CommandContext(ctx, "/bin/bash", "-c", cmdStr)
				if opts.Cwd != "" {
					cmd.Dir = opts.Cwd
				}
				cmd.Env = os.Environ()
				for k, v := range b.cfg.Env {
					cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
				}
				out, err := cmd.CombinedOutput()
				outputStr := string(out)
				if err != nil && outputStr == "" {
					outputStr = err.Error()
				}
				if len(outputStr) > 30000 {
					outputStr = outputStr[:30000] + "\n[output truncated]"
				}
				toolResult = map[string]any{
					"output": outputStr,
				}
			} else {
				toolResult = map[string]any{
					"result": fmt.Sprintf("Tool %q invoked with: %s", tc.Name, string(inputJSON)),
				}
			}

			outputBytes, _ := json.Marshal(toolResult)
			msgs <- Message{
				Type:   MessageToolResult,
				Tool:   tc.Name,
				CallID: tc.Name,
				Output: string(outputBytes),
			}
			toolResults = append(toolResults, geminiPart{
				FunctionResponse: &geminiFunctionResponse{
					Name:     tc.Name,
					Response: toolResult,
				},
			})
		}
		history = append(history, geminiContent{Role: "user", Parts: toolResults})
		if turn == maxTurns-2 {
			// Ask the model to summarize its work on the final turn
			history = append(history, geminiContent{
				Role: "user",
				Parts: []geminiPart{
					{Text: "You have reached the maximum allowed tool turns. Please provide your final answer, summary of work done, and conclusions now."},
				},
			})
		}
	}

	durationMs := time.Since(start).Milliseconds()
	out := strings.TrimSpace(finalOutput.String())
	if out != "" {
		return Result{
			Status:     "completed",
			Output:     out,
			DurationMs: durationMs,
		}
	}

	return Result{
		Status:     "failed",
		Error:      "google: exceeded maximum tool-call turns",
		DurationMs: durationMs,
	}
}

// streamOneTurn sends one generateContent request and reads the SSE stream,
// returning the assembled response parts, the finish reason, and token counts.
func (b *googleBackend) streamOneTurn(
	ctx context.Context,
	model, apiKey string,
	reqBody geminiGenerateRequest,
	msgs chan<- Message,
) (parts []geminiPart, finishReason string, inputTokens, outputTokens int, err error) {
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, "", 0, 0, fmt.Errorf("marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/models/%s:streamGenerateContent?alt=sse&key=%s",
		geminiBaseURL, model, apiKey)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, "", 0, 0, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "ai-gauntlet-daemon/1.0")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, "", 0, 0, fmt.Errorf("POST streamGenerateContent: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		// Surface the raw error for Cerebra quota detection.
		errMsg := fmt.Sprintf("HTTP %d: %s", resp.StatusCode, string(body))
		return nil, "", 0, 0, fmt.Errorf("%s", errMsg)
	}

	// Read SSE stream: each event is "data: <json>\n\n".
	scanner := bufio.NewScanner(resp.Body)
	var textBuf strings.Builder

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			break
		}

		var chunk geminiStreamResponse
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}

		// Handle API-level errors embedded in the stream.
		if chunk.Error != nil {
			return nil, "", 0, 0, fmt.Errorf("API error %d (%s): %s",
				chunk.Error.Code, chunk.Error.Status, chunk.Error.Message)
		}

		if chunk.UsageMetadata != nil {
			inputTokens = chunk.UsageMetadata.PromptTokenCount
			outputTokens = chunk.UsageMetadata.CandidatesTokenCount
		}

		for _, cand := range chunk.Candidates {
			finishReason = cand.FinishReason
			for _, p := range cand.Content.Parts {
				if p.FunctionCall != nil {
					parts = append(parts, p)
				} else if p.Text != "" {
					textBuf.WriteString(p.Text)
					msgs <- Message{Type: MessageText, Content: p.Text}
				}
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, finishReason, inputTokens, outputTokens, fmt.Errorf("read stream: %w", err)
	}

	// Assemble the accumulated text as a single part alongside any tool calls.
	if textBuf.Len() > 0 {
		parts = append([]geminiPart{{Text: textBuf.String()}}, parts...)
	}
	return parts, finishReason, inputTokens, outputTokens, nil
}
