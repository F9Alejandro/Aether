package models

import "time"

// Parameter represents a parameter description for a tool.
type Parameter struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
}

// Example represents a usage example for a tool.
type Example struct {
	Usage       string `json:"usage"`
	Description string `json:"description"`
}

// ReturnType represents the output schema of a tool.
type ReturnType struct {
	Type        string `json:"type"`
	Description string `json:"description"`
}

// Tool represents a complete technical definition of a tool.
type Tool struct {
	ID          string      `json:"id"`
	Description string      `json:"description"`
	Parameters  []Parameter `json:"parameters"`
	Examples    []Example   `json:"examples"`
	ReturnType  ReturnType  `json:"return_type"`
	UsageCount  int         `json:"usage_count"`
}

// ToolComponent represents a semantic sub-component of a tool to be embedded.
type ToolComponent struct {
	ID            string    `json:"id,omitempty"`
	ToolID        string    `json:"tool_id"`
	ComponentType string    `json:"component_type"` // "description", "parameter", "example", "returns"
	Name          string    `json:"name,omitempty"`           // Name of the parameter if ComponentType is "parameter"
	TextContent   string    `json:"text_content"`
	Embedding     []float64 `json:"embedding"`
	UsageCount    int       `json:"usage_count"`
}

// SearchResult represents a tool match with its associated scores and final relevance.
type SearchResult struct {
	Tool             Tool               `json:"tool"`
	FinalScore       float64            `json:"final_score"`
	ComponentScores  map[string]float64 `json:"component_scores"`
	PopularityBoost  float64            `json:"popularity_boost"`
}

// MemoryEntry represents a single memory record or log entry.
type MemoryEntry struct {
	ID        string    `json:"id,omitempty"`
	SessionID string    `json:"session_id"` // Separates sessions or workspaces. Global preferences use "global".
	Content   string    `json:"content"`
	Category  string    `json:"category"`   // e.g. "preference", "episodic", "task_log"
	TaskID    string    `json:"task_id,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

// MemoryComponent holds the vector embedding and reference to a MemoryEntry.
type MemoryComponent struct {
	ID        string    `json:"id,omitempty"`
	MemoryID  string    `json:"memory_id"`
	Embedding []float64 `json:"embedding"`
	Text      string    `json:"text"`
}

// MemorySearchResult holds a retrieved memory and its cosine relevance score.
type MemorySearchResult struct {
	Memory    MemoryEntry `json:"memory"`
	Score     float64     `json:"score"`
}

// CacheEntry represents a cached tool search result.
type CacheEntry struct {
	ID        string    `json:"id,omitempty"`
	Query     string    `json:"query"`
	Tools     []string  `json:"tools"` // Matched Tool IDs
	Timestamp time.Time `json:"timestamp"`
	Embedding []float64 `json:"embedding,omitempty"`
}
