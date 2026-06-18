package config

import (
	"encoding/json"
	"os"
	"strconv"
)

// Config holds all the configuration parameters for the application.
type Config struct {
	SurrealURL        string
	SurrealUser       string
	SurrealPass       string
	SurrealNS         string
	SurrealDB         string
	GeminiAPIKey      string
	EmbeddingProvider string // "gemini", "ollama", "onnx", "mock"
	EmbeddingModel    string // gemini or ollama model name
	EmbeddingDim      int
	MinConfidence     float64
	MaxResults        int
	UseMockEmbed      bool
	OllamaURL         string
	OllamaModel       string
	OnnxModelPath     string
	OnnxTokenizerPath string
	OnnxSharedLibPath string
}

// jsonConfig represents the JSON structure of config.json
type jsonConfig struct {
	SurrealURL        string   `json:"surreal_url"`
	SurrealUser       string   `json:"surreal_user"`
	SurrealPass       string   `json:"surreal_pass"`
	SurrealNS         string   `json:"surreal_ns"`
	SurrealDB         string   `json:"surreal_db"`
	GeminiAPIKey      string   `json:"gemini_api_key"`
	EmbeddingProvider string   `json:"embedding_provider"`
	EmbeddingModel    string   `json:"embedding_model"`
	EmbeddingDim      *int     `json:"embedding_dim"`
	MinConfidence     *float64 `json:"min_confidence"`
	MaxResults        *int     `json:"max_results"`
	OllamaURL         string   `json:"ollama_url"`
	OnnxModelPath     string   `json:"onnx_model_path"`
	OnnxTokenizerPath string   `json:"onnx_tokenizer_path"`
	OnnxSharedLibPath string   `json:"onnx_shared_lib_path"`
}

func getEnvOrFile(envKey string, fileVal string, defaultVal string) string {
	if val := os.Getenv(envKey); val != "" {
		return val
	}
	if fileVal != "" {
		return fileVal
	}
	return defaultVal
}

// LoadConfig reads configuration from a JSON file and environment variables, setting defaults.
func LoadConfig() *Config {
	configPath := os.Getenv("STS_CONFIG_FILE")
	if configPath == "" {
		configPath = "config.json"
	}

	var fileCfg jsonConfig
	if data, err := os.ReadFile(configPath); err == nil {
		_ = json.Unmarshal(data, &fileCfg)
	}

	// 1. Resolve GeminiAPIKey
	geminiKey := os.Getenv("GEMINI_API_KEY")
	if geminiKey == "" {
		geminiKey = os.Getenv("GOOGLE_API_KEY")
	}
	if geminiKey == "" {
		geminiKey = fileCfg.GeminiAPIKey
	}

	// 2. Resolve provider
	provider := getEnvOrFile("EMBEDDING_PROVIDER", fileCfg.EmbeddingProvider, "")
	if provider == "" {
		if geminiKey != "" {
			provider = "gemini"
		} else {
			provider = "mock"
		}
	}

	useMock := provider == "mock"

	// 3. Resolve model
	model := getEnvOrFile("EMBEDDING_MODEL", fileCfg.EmbeddingModel, "")
	if model == "" {
		if provider == "ollama" {
			model = "all-minilm"
		} else {
			model = "gemini-embedding-2"
		}
	}

	// 4. Resolve dim
	dim := 0
	if dimStr := os.Getenv("EMBEDDING_DIM"); dimStr != "" {
		if d, err := strconv.Atoi(dimStr); err == nil {
			dim = d
		}
	}
	if dim == 0 && fileCfg.EmbeddingDim != nil {
		dim = *fileCfg.EmbeddingDim
	}
	if dim == 0 {
		if provider == "ollama" && model == "all-minilm" {
			dim = 384
		} else if provider == "onnx" {
			dim = 384
		} else if model == "gemini-embedding-001" {
			dim = 3072
		} else {
			dim = 768
		}
	}

	// 5. Resolve min confidence
	minConf := 0.35
	if minConfStr := os.Getenv("MIN_CONFIDENCE"); minConfStr != "" {
		if mc, err := strconv.ParseFloat(minConfStr, 64); err == nil {
			minConf = mc
		}
	} else if fileCfg.MinConfidence != nil {
		minConf = *fileCfg.MinConfidence
	}

	// 6. Resolve max results
	maxResults := 5
	if maxResultsStr := os.Getenv("MAX_RESULTS"); maxResultsStr != "" {
		if mr, err := strconv.Atoi(maxResultsStr); err == nil {
			maxResults = mr
		}
	} else if fileCfg.MaxResults != nil {
		maxResults = *fileCfg.MaxResults
	}

	// 7. Resolve Surreal fields
	surrealURL := getEnvOrFile("SURREAL_URL", fileCfg.SurrealURL, "ws://localhost:8000")
	surrealUser := getEnvOrFile("SURREAL_USER", fileCfg.SurrealUser, "root")
	surrealPass := getEnvOrFile("SURREAL_PASS", fileCfg.SurrealPass, "root")
	surrealNS := getEnvOrFile("SURREAL_NS", fileCfg.SurrealNS, "sts")
	surrealDB := getEnvOrFile("SURREAL_DB", fileCfg.SurrealDB, "test")

	// 8. Resolve Ollama fields
	ollamaURL := getEnvOrFile("OLLAMA_URL", fileCfg.OllamaURL, "http://localhost:11434")

	// 9. Resolve ONNX fields
	onnxModelPath := getEnvOrFile("ONNX_MODEL_PATH", fileCfg.OnnxModelPath, "model.onnx")
	onnxTokenizerPath := getEnvOrFile("ONNX_TOKENIZER_PATH", fileCfg.OnnxTokenizerPath, "tokenizer.json")
	onnxSharedLibPath := getEnvOrFile("ONNX_SHARED_LIB_PATH", fileCfg.OnnxSharedLibPath, "libonnxruntime.so")

	return &Config{
		SurrealURL:        surrealURL,
		SurrealUser:       surrealUser,
		SurrealPass:       surrealPass,
		SurrealNS:         surrealNS,
		SurrealDB:         surrealDB,
		GeminiAPIKey:      geminiKey,
		EmbeddingProvider: provider,
		EmbeddingModel:    model,
		EmbeddingDim:      dim,
		MinConfidence:     minConf,
		MaxResults:        maxResults,
		UseMockEmbed:      useMock,
		OllamaURL:         ollamaURL,
		OllamaModel:       model,
		OnnxModelPath:     onnxModelPath,
		OnnxTokenizerPath: onnxTokenizerPath,
		OnnxSharedLibPath: onnxSharedLibPath,
	}
}
