package embedding

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/sugarme/tokenizer"
	"github.com/sugarme/tokenizer/pretrained"
	ort "github.com/yalue/onnxruntime_go"
)

// Generator defines the interface for generating embeddings.
type Generator interface {
	Generate(ctx context.Context, text string) ([]float64, error)
	GenerateBatch(ctx context.Context, texts []string) ([][]float64, error)
	Dimension() int
	Close() error
}

// MockGenerator generates deterministic keyword-aware embeddings.
type MockGenerator struct {
	dim int
}

func NewMockGenerator(dim int) *MockGenerator {
	return &MockGenerator{dim: dim}
}

func (m *MockGenerator) Dimension() int {
	return m.dim
}

func (m *MockGenerator) Close() error {
	return nil
}

// categoryKeywords maps semantic categories to lists of keywords.
var categoryKeywords = [][]string{
	// Category 0: Notification / Messaging
	{"slack", "notify", "message", "channel", "alert", "ping", "chat", "send_slack"},
	// Category 1: Database operations
	{"database", "sql", "query", "backup", "db", "postgres", "mysql", "table", "schema", "db_tool", "backup_db"},
	// Category 2: Cloud / Infrastructure Deployment
	{"deploy", "cloud", "aws", "docker", "kubernetes", "ec2", "instance", "server", "vm", "container", "deploy_app"},
	// Category 3: Email operations
	{"email", "mail", "sendgrid", "smtp", "inbox", "send_email"},
	// Category 4: Monitoring / Logging
	{"log", "metric", "monitor", "grafana", "prometheus", "dashboard", "alerting", "elk"},
}

func (m *MockGenerator) Generate(ctx context.Context, text string) ([]float64, error) {
	textLower := strings.ToLower(text)
	vec := make([]float64, m.dim)

	// 1. Generate base deterministic noise using SHA-256
	var sumSq float64
	for i := 0; i < m.dim; i++ {
		h := sha256.New()
		h.Write([]byte(fmt.Sprintf("%s-%d", text, i)))
		hashBytes := h.Sum(nil)
		// Convert first 4 bytes to float between -0.1 and 0.1
		val := (float64(int32(binary.BigEndian.Uint32(hashBytes[:4]))) / 2147483647.0) * 0.1
		vec[i] = val
	}

	// 2. Add keyword signals to specific blocks of the vector
	blockSize := m.dim / (len(categoryKeywords) + 1)
	if blockSize <= 0 {
		blockSize = 10
	}

	for catIdx, keywords := range categoryKeywords {
		matched := false
		for _, kw := range keywords {
			if strings.Contains(textLower, kw) {
				matched = true
				break
			}
		}

		if matched {
			// Boost the dimensions corresponding to this category block
			start := catIdx * blockSize
			end := (catIdx + 1) * blockSize
			if end > m.dim {
				end = m.dim
			}

			// Add a strong signal to this block
			for j := start; j < end; j++ {
				vec[j] += 2.5
			}
		}
	}

	// 3. Normalize the vector so its L2 norm is 1.0 (unit vector)
	for i := 0; i < m.dim; i++ {
		sumSq += vec[i] * vec[i]
	}
	mag := math.Sqrt(sumSq)
	if mag > 0 {
		for i := 0; i < m.dim; i++ {
			vec[i] /= mag
		}
	}

	return vec, nil
}

func (m *MockGenerator) GenerateBatch(ctx context.Context, texts []string) ([][]float64, error) {
	results := make([][]float64, len(texts))
	for i, text := range texts {
		vec, err := m.Generate(ctx, text)
		if err != nil {
			return nil, err
		}
		results[i] = vec
	}
	return results, nil
}

// GeminiGenerator generates real embeddings using the Google Gemini API.
type GeminiGenerator struct {
	apiKey string
	model  string
	dim    int
	client *http.Client
}

func NewGeminiGenerator(apiKey, model string, dim int) *GeminiGenerator {
	return &GeminiGenerator{
		apiKey: apiKey,
		model:  model,
		dim:    dim,
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

func (g *GeminiGenerator) Dimension() int {
	return g.dim
}

func (g *GeminiGenerator) Close() error {
	return nil
}

type geminiPart struct {
	Text string `json:"text"`
}

type geminiContent struct {
	Parts []geminiPart `json:"parts"`
}

type geminiEmbedRequest struct {
	Model                string        `json:"model"`
	Content              geminiContent `json:"content"`
	OutputDimensionality int           `json:"outputDimensionality,omitempty"`
}

type geminiEmbedResponse struct {
	Embedding struct {
		Values []float64 `json:"values"`
	} `json:"embedding"`
}

type geminiBatchRequest struct {
	Requests []geminiEmbedRequest `json:"requests"`
}

type geminiBatchResponse struct {
	Embeddings []struct {
		Values []float64 `json:"values"`
	} `json:"embeddings"`
}

func (g *GeminiGenerator) Generate(ctx context.Context, text string) ([]float64, error) {
	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:embedContent?key=%s", g.model, g.apiKey)

	reqBody := geminiEmbedRequest{
		Model: "models/" + g.model,
		Content: geminiContent{
			Parts: []geminiPart{{Text: text}},
		},
	}
	// Truncate using Matryoshka if output dimension is customized
	if (g.model == "gemini-embedding-001" || g.model == "gemini-embedding-2") && g.dim != 3072 {
		reqBody.OutputDimensionality = g.dim
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := g.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request to gemini: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errData map[string]interface{}
		_ = json.NewDecoder(resp.Body).Decode(&errData)
		return nil, fmt.Errorf("gemini api returned status %d: %v", resp.StatusCode, errData)
	}

	var respBody geminiEmbedResponse
	if err := json.NewDecoder(resp.Body).Decode(&respBody); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return respBody.Embedding.Values, nil
}

func (g *GeminiGenerator) GenerateBatch(ctx context.Context, texts []string) ([][]float64, error) {
	// Gemini batch API allows batching requests
	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:batchEmbedContents?key=%s", g.model, g.apiKey)

	requests := make([]geminiEmbedRequest, len(texts))
	for i, text := range texts {
		requests[i] = geminiEmbedRequest{
			Model: "models/" + g.model,
			Content: geminiContent{
				Parts: []geminiPart{{Text: text}},
			},
		}
		if (g.model == "gemini-embedding-001" || g.model == "gemini-embedding-2") && g.dim != 3072 {
			requests[i].OutputDimensionality = g.dim
		}
	}

	reqBody := geminiBatchRequest{Requests: requests}
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal batch request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create batch request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := g.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send batch request to gemini: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errData map[string]interface{}
		_ = json.NewDecoder(resp.Body).Decode(&errData)
		return nil, fmt.Errorf("gemini batch api returned status %d: %v", resp.StatusCode, errData)
	}

	var respBody geminiBatchResponse
	if err := json.NewDecoder(resp.Body).Decode(&respBody); err != nil {
		return nil, fmt.Errorf("failed to decode batch response: %w", err)
	}

	embeddings := make([][]float64, len(respBody.Embeddings))
	for i, emb := range respBody.Embeddings {
		embeddings[i] = emb.Values
	}

	return embeddings, nil
}

// OllamaGenerator generates embeddings using a local Ollama service.
type OllamaGenerator struct {
	url    string
	model  string
	dim    int
	client *http.Client
}

func NewOllamaGenerator(url, model string, dim int) *OllamaGenerator {
	return &OllamaGenerator{
		url:    url,
		model:  model,
		dim:    dim,
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

func (o *OllamaGenerator) Dimension() int {
	return o.dim
}

func (o *OllamaGenerator) Close() error {
	return nil
}

type ollamaEmbedRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

type ollamaEmbedResponse struct {
	Model      string      `json:"model"`
	Embeddings [][]float64 `json:"embeddings"`
}

func (o *OllamaGenerator) Generate(ctx context.Context, text string) ([]float64, error) {
	vecs, err := o.GenerateBatch(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	if len(vecs) == 0 {
		return nil, fmt.Errorf("no embedding returned from ollama")
	}
	return vecs[0], nil
}

func (o *OllamaGenerator) GenerateBatch(ctx context.Context, texts []string) ([][]float64, error) {
	url := fmt.Sprintf("%s/api/embed", o.url)

	reqBody := ollamaEmbedRequest{
		Model: o.model,
		Input: texts,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal ollama request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create ollama request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := o.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request to ollama: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errData map[string]interface{}
		_ = json.NewDecoder(resp.Body).Decode(&errData)
		return nil, fmt.Errorf("ollama returned status %d: %v", resp.StatusCode, errData)
	}

	var respBody ollamaEmbedResponse
	if err := json.NewDecoder(resp.Body).Decode(&respBody); err != nil {
		return nil, fmt.Errorf("failed to decode ollama response: %w", err)
	}

	return respBody.Embeddings, nil
}

// OnnxGenerator generates embeddings locally using ONNX Runtime.
type OnnxGenerator struct {
	modelPath     string
	tokenizerPath string
	sharedLibPath string
	dim           int
	session       *ort.DynamicAdvancedSession
	tokenizer     *tokenizer.Tokenizer
}

func NewOnnxGenerator(modelPath, tokenizerPath, sharedLibPath string, dim int) *OnnxGenerator {
	return &OnnxGenerator{
		modelPath:     modelPath,
		tokenizerPath: tokenizerPath,
		sharedLibPath: sharedLibPath,
		dim:           dim,
	}
}

func (g *OnnxGenerator) Dimension() int {
	return g.dim
}

func (g *OnnxGenerator) Close() error {
	var errs []string
	if g.session != nil {
		if err := g.session.Destroy(); err != nil {
			errs = append(errs, err.Error())
		}
		g.session = nil
	}
	if ort.IsInitialized() {
		if err := ort.DestroyEnvironment(); err != nil {
			errs = append(errs, err.Error())
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("errors closing onnx: %s", strings.Join(errs, "; "))
	}
	return nil
}

// Init loads the shared library and initializes the environment/session/tokenizer.
func (g *OnnxGenerator) Init() error {
	// 0. Verify local ONNX model files are present
	if _, err := os.Stat(g.sharedLibPath); os.IsNotExist(err) {
		return fmt.Errorf("shared library path '%s' does not exist", g.sharedLibPath)
	}
	if _, err := os.Stat(g.modelPath); os.IsNotExist(err) {
		return fmt.Errorf("ONNX model file '%s' does not exist", g.modelPath)
	}
	if _, err := os.Stat(g.tokenizerPath); os.IsNotExist(err) {
		return fmt.Errorf("tokenizer JSON file '%s' does not exist", g.tokenizerPath)
	}

	// 1. Initialize ONNX Runtime shared library
	ort.SetSharedLibraryPath(g.sharedLibPath)
	err := ort.InitializeEnvironment()
	if err != nil {
		return fmt.Errorf("failed to initialize onnx environment: %w", err)
	}

	// 2. Load the model session (Dynamic so it handles dynamic seqLen inputs)
	session, err := ort.NewDynamicAdvancedSession(
		g.modelPath,
		[]string{"input_ids", "attention_mask", "token_type_ids"},
		[]string{"last_hidden_state"},
		nil,
	)
	if err != nil {
		ort.DestroyEnvironment()
		return fmt.Errorf("failed to create dynamic onnx session: %w", err)
	}
	g.session = session

	// 3. Load tokenizer config directly from file once
	tk, err := pretrained.FromFile(g.tokenizerPath)
	if err != nil {
		session.Destroy()
		ort.DestroyEnvironment()
		return fmt.Errorf("failed to load tokenizer configuration: %w", err)
	}
	g.tokenizer = tk

	return nil
}

func (g *OnnxGenerator) Generate(ctx context.Context, text string) ([]float64, error) {
	if g.session == nil || g.tokenizer == nil {
		return nil, fmt.Errorf("onnx generator is not initialized")
	}

	// Tokenize text using the pretrained HuggingFace config
	encoding, err := g.tokenizer.EncodeSingle(text)
	if err != nil {
		return nil, fmt.Errorf("failed to tokenize input text: %w", err)
	}

	seqLen := len(encoding.Ids)
	if seqLen == 0 {
		return make([]float64, g.dim), nil
	}

	// Convert tokens to int64 dimensions
	ids := make([]int64, seqLen)
	typeIds := make([]int64, seqLen)
	attnMask := make([]int64, seqLen)

	for i := 0; i < seqLen; i++ {
		ids[i] = int64(encoding.Ids[i])
		typeIds[i] = int64(encoding.TypeIds[i])
		attnMask[i] = int64(encoding.AttentionMask[i])
	}

	// Prepare shapes: [1, seqLen]
	inputShape := ort.NewShape(1, int64(seqLen))

	// Create inputs
	inputIdsTensor, err := ort.NewTensor(inputShape, ids)
	if err != nil {
		return nil, fmt.Errorf("failed to create input_ids tensor: %w", err)
	}
	defer inputIdsTensor.Destroy()

	attnMaskTensor, err := ort.NewTensor(inputShape, attnMask)
	if err != nil {
		return nil, fmt.Errorf("failed to create attention_mask tensor: %w", err)
	}
	defer attnMaskTensor.Destroy()

	tokenTypeTensor, err := ort.NewTensor(inputShape, typeIds)
	if err != nil {
		return nil, fmt.Errorf("failed to create token_type_ids tensor: %w", err)
	}
	defer tokenTypeTensor.Destroy()

	// Pre-allocate dynamic output tensor shape: [1, seqLen, g.dim]
	outputShape := ort.NewShape(1, int64(seqLen), int64(g.dim))
	outputTensor, err := ort.NewEmptyTensor[float32](outputShape)
	if err != nil {
		return nil, fmt.Errorf("failed to preallocate output tensor: %w", err)
	}
	defer outputTensor.Destroy()

	// Run session
	err = g.session.Run(
		[]ort.Value{inputIdsTensor, attnMaskTensor, tokenTypeTensor},
		[]ort.Value{outputTensor},
	)
	if err != nil {
		return nil, fmt.Errorf("onnx session execution failed: %w", err)
	}

	// Get output hidden states
	results := outputTensor.GetData()

	// Mean Pooling: average the states where attention mask is active
	embedding := make([]float64, g.dim)
	var validTokens int
	for s := 0; s < seqLen; s++ {
		if attnMask[s] == 1 {
			validTokens++
			for d := 0; d < g.dim; d++ {
				embedding[d] += float64(results[s*g.dim+d])
			}
		}
	}

	if validTokens > 0 {
		for d := 0; d < g.dim; d++ {
			embedding[d] /= float64(validTokens)
		}
	}

	// Normalize vector L2 norm
	var sumSq float64
	for d := 0; d < g.dim; d++ {
		sumSq += embedding[d] * embedding[d]
	}
	mag := math.Sqrt(sumSq)
	if mag > 0 {
		for d := 0; d < g.dim; d++ {
			embedding[d] /= mag
		}
	}

	return embedding, nil
}

func (g *OnnxGenerator) GenerateBatch(ctx context.Context, texts []string) ([][]float64, error) {
	results := make([][]float64, len(texts))
	for i, text := range texts {
		vec, err := g.Generate(ctx, text)
		if err != nil {
			return nil, err
		}
		results[i] = vec
	}
	return results, nil
}

// GenerateText implementation for OnnxGenerator (uses heuristic fallback)
func (g *OnnxGenerator) GenerateText(ctx context.Context, prompt string) (string, error) {
	return HeuristicExtractIntent(prompt), nil
}

// GenerateText implementation for MockGenerator (uses heuristic)
func (m *MockGenerator) GenerateText(ctx context.Context, prompt string) (string, error) {
	return HeuristicExtractIntent(prompt), nil
}

// GenerateText implementation for GeminiGenerator (uses gemini-1.5-flash API)
type geminiTextRequest struct {
	Contents []geminiContent `json:"contents"`
}

type geminiTextResponse struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
}

func (g *GeminiGenerator) GenerateText(ctx context.Context, prompt string) (string, error) {
	if g.apiKey == "" {
		return HeuristicExtractIntent(prompt), nil
	}
	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/gemini-1.5-flash:generateContent?key=%s", g.apiKey)

	reqBody := geminiTextRequest{
		Contents: []geminiContent{
			{
				Parts: []geminiPart{
					{Text: "Extract the core system administration, programming, audit, or security instruction from this user prompt. Respond with only a single, concise sentence: " + prompt},
				},
			},
		},
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal text request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := g.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send request to gemini: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return HeuristicExtractIntent(prompt), nil
	}

	var respBody geminiTextResponse
	if err := json.NewDecoder(resp.Body).Decode(&respBody); err != nil {
		return "", fmt.Errorf("failed to decode text response: %w", err)
	}

	if len(respBody.Candidates) > 0 && len(respBody.Candidates[0].Content.Parts) > 0 {
		return strings.TrimSpace(respBody.Candidates[0].Content.Parts[0].Text), nil
	}

	return HeuristicExtractIntent(prompt), nil
}

// GenerateText implementation for OllamaGenerator
type ollamaGenerateRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	Stream bool   `json:"stream"`
}

type ollamaGenerateResponse struct {
	Response string `json:"response"`
}

func (o *OllamaGenerator) GenerateText(ctx context.Context, prompt string) (string, error) {
	url := fmt.Sprintf("%s/api/generate", o.url)

	reqBody := ollamaGenerateRequest{
		Model:  o.model,
		Prompt: "Extract the core system administration, programming, audit, or security instruction from this user prompt. Respond with only a single, concise sentence: " + prompt,
		Stream: false,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal ollama text request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("failed to create ollama request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := o.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to connect to ollama: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return HeuristicExtractIntent(prompt), nil
	}

	var respBody ollamaGenerateResponse
	if err := json.NewDecoder(resp.Body).Decode(&respBody); err != nil {
		return "", fmt.Errorf("failed to decode ollama response: %w", err)
	}

	return strings.TrimSpace(respBody.Response), nil
}

// HeuristicExtractIntent extracts core lines from a prompt locally as a fallback
func HeuristicExtractIntent(text string) string {
	lines := strings.Split(text, "\n")
	var selected []string
	keywords := []string{"slack", "notify", "message", "database", "sql", "query", "backup", "deploy", "docker", "kubernetes", "email", "smtp", "log", "metric", "monitor", "grep", "test", "linter", "audit", "scan", "service"}

	for _, line := range lines {
		lineClean := strings.TrimSpace(line)
		if lineClean == "" {
			continue
		}

		lower := strings.ToLower(lineClean)
		isQuestion := strings.HasSuffix(lower, "?") || strings.Contains(lower, "how to") || strings.Contains(lower, "what is") || strings.Contains(lower, "can you") || strings.Contains(lower, "should") || strings.Contains(lower, "is there")

		hasKeyword := false
		for _, kw := range keywords {
			if strings.Contains(lower, kw) {
				hasKeyword = true
				break
			}
		}

		if isQuestion || hasKeyword || len(lineClean) < 100 {
			selected = append(selected, lineClean)
		}
	}

	if len(selected) == 0 {
		return text
	}

	if len(selected) > 3 {
		selected = selected[:3]
	}
	return strings.Join(selected, " ")
}
