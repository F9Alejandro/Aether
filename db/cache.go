package db

import (
	"context"
	"fmt"
	"strings"
	"time"

	surrealdb "github.com/surrealdb/surrealdb.go"
	"aether/models"
)

// ExactCacheResult holds query result for literal exact matches.
type ExactCacheResult struct {
	Tools []string `json:"tools"`
}

// SemanticCacheKNNResult holds KNN matched cache query and similarity score.
type SemanticCacheKNNResult struct {
	Tools      []string `json:"tools"`
	Similarity float64  `json:"similarity"`
}

// GetExactCache performs a fast exact string lookup in the cache, bypassing embedding generation.
func (sc *SurrealClient) GetExactCache(ctx context.Context, query string) ([]string, bool, error) {
	q := `
		SELECT tools 
		FROM semantic_cache 
		WHERE string::lowercase(query) = string::lowercase($query) 
		LIMIT 1;
	`
	res, err := surrealdb.Query[[]ExactCacheResult](ctx, sc.db, q, map[string]any{
		"query": strings.TrimSpace(query),
	})
	if err != nil {
		return nil, false, fmt.Errorf("exact cache query failed: %w", err)
	}

	if len(*res) > 0 && len((*res)[0].Result) > 0 {
		return (*res)[0].Result[0].Tools, true, nil
	}

	return nil, false, nil
}

// GetSemanticCache queries the cache semantically using cosine vector similarity.
func (sc *SurrealClient) GetSemanticCache(ctx context.Context, queryVector []float64, threshold float64) ([]string, bool, error) {
	q := `
		SELECT tools, vector::similarity::cosine(embedding, $q_vector) AS similarity 
		FROM semantic_cache 
		WHERE embedding <|1, COSINE|> $q_vector;
	`
	res, err := surrealdb.Query[[]SemanticCacheKNNResult](ctx, sc.db, q, map[string]any{
		"q_vector": queryVector,
	})
	if err != nil {
		return nil, false, fmt.Errorf("semantic cache KNN search failed: %w", err)
	}

	if len(*res) > 0 && len((*res)[0].Result) > 0 {
		match := (*res)[0].Result[0]
		// SurrealDB cosine similarity is usually returned where 1.0 is exact match
		if match.Similarity >= threshold {
			return match.Tools, true, nil
		}
	}

	return nil, false, nil
}

// SaveToCache saves a query and its matched tools to the semantic cache.
func (sc *SurrealClient) SaveToCache(ctx context.Context, query string, queryVector []float64, tools []string) error {
	entry := models.CacheEntry{
		Query:     strings.TrimSpace(query),
		Tools:     tools,
		Timestamp: time.Now(),
		Embedding: queryVector,
	}

	// We insert without custom RecordID to let SurrealDB auto-generate the ID
	_, err := surrealdb.Create[models.CacheEntry](ctx, sc.db, "semantic_cache", entry)
	if err != nil {
		return fmt.Errorf("failed to save query results to cache: %w", err)
	}

	return nil
}

// ClearCache clears all cached query entries.
func (sc *SurrealClient) ClearCache(ctx context.Context) error {
	_, err := surrealdb.Query[any](ctx, sc.db, "DELETE semantic_cache;", nil)
	if err != nil {
		return fmt.Errorf("failed to clear semantic cache: %w", err)
	}
	return nil
}
