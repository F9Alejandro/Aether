package search

import (
	"context"
	"fmt"
	"math"
	"sort"
	"aether/db"
	"aether/embedding"
	"aether/models"
)

// SearchEngine handles semantic search operations across tool components.
type SearchEngine struct {
	client        *db.SurrealClient
	embedder      embedding.Generator
	minConfidence float64
	maxResults    int
	Debug         bool
}

// NewSearchEngine initializes a new search engine instance.
func NewSearchEngine(client *db.SurrealClient, embedder embedding.Generator, minConf float64, maxRes int) *SearchEngine {
	return &SearchEngine{
		client:        client,
		embedder:      embedder,
		minConfidence: minConf,
		maxResults:    maxRes,
		Debug:         false,
	}
}

// Embedder returns the active embedding generator.
func (se *SearchEngine) Embedder() embedding.Generator {
	return se.embedder
}

// CosineSimilarity calculates the cosine similarity between two float64 vectors.
func CosineSimilarity(a, b []float64) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dotProduct, normA, normB float64
	for i := 0; i < len(a); i++ {
		dotProduct += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dotProduct / (math.Sqrt(normA) * math.Sqrt(normB))
}

// Search matches user natural queries to the best tools using multi-component semantic weighting.
func (se *SearchEngine) Search(ctx context.Context, query string) ([]models.SearchResult, error) {
	// 1. Exact String Cache Lookup (Bypasses embedding generator API completely)
	cachedIDs, hit, err := se.client.GetExactCache(ctx, query)
	if err == nil && hit {
		if se.Debug {
			fmt.Printf("🎯 [EXACT CACHE HIT] Bypassed prompt optimization, embedding generation, and database search!\n")
		} else {
			fmt.Printf("🎯 [EXACT CACHE HIT] Bypassed embedding generation and semantic search!\n")
		}
		cachedTools, err := se.client.FetchToolsByIDs(ctx, cachedIDs)
		if err == nil && len(cachedTools) > 0 {
			var results []models.SearchResult
			for _, t := range cachedTools {
				results = append(results, models.SearchResult{
					Tool:       t,
					FinalScore: 1.0,
					ComponentScores: map[string]float64{
						"description": 1.0,
						"parameters":  1.0,
						"examples":    1.0,
						"returns":     1.0,
					},
				})
			}
			return results, nil
		}
	}

	// 2. Generate query embedding
	qVector, err := se.embedder.Generate(ctx, query)
	if err != nil {
		return nil, err
	}

	// 3. Semantic Cache Lookup (Cosine similarity threshold >= 0.96)
	cachedIDs, hit, err = se.client.GetSemanticCache(ctx, qVector, 0.96)
	if err == nil && hit {
		if se.Debug {
			fmt.Printf("🎯 [SEMANTIC CACHE HIT] Cosine match >= 0.96. Bypassed database component scoring!\n")
		} else {
			fmt.Printf("🎯 [SEMANTIC CACHE HIT] Bypassed database component scoring calculations!\n")
		}
		cachedTools, err := se.client.FetchToolsByIDs(ctx, cachedIDs)
		if err == nil && len(cachedTools) > 0 {
			var results []models.SearchResult
			for _, t := range cachedTools {
				results = append(results, models.SearchResult{
					Tool:       t,
					FinalScore: 0.96,
					ComponentScores: map[string]float64{
						"description": 0.96,
						"parameters":  0.96,
						"examples":    0.96,
						"returns":     0.96,
					},
				})
			}
			return results, nil
		}
	}

	// 4. Query KNN on SurrealDB using HNSW index to fetch top candidate IDs.
	// We retrieve 15 candidates to ensure we cover enough tools.
	candidateIDs, err := se.client.FindCandidateToolIDs(ctx, qVector, 15)
	if err != nil {
		return nil, err
	}
	if se.Debug {
		fmt.Printf("🔍 [DEBUG] Candidate IDs returned by HNSW KNN: %v\n", candidateIDs)
	}
	if len(candidateIDs) == 0 {
		return nil, nil
	}

	// 5. Fetch all components for these candidates (to compute complete score weights)
	components, err := se.client.FetchComponentsForTools(ctx, candidateIDs)
	if err != nil {
		return nil, err
	}

	// Group components by tool ID
	componentsByTool := make(map[string][]models.ToolComponent)
	for _, comp := range components {
		componentsByTool[comp.ToolID] = append(componentsByTool[comp.ToolID], comp)
	}

	// Fetch full tool schemas for these candidates
	tools, err := se.client.FetchToolsByIDs(ctx, candidateIDs)
	if err != nil {
		return nil, err
	}

	toolsByID := make(map[string]models.Tool)
	for _, t := range tools {
		toolsByID[t.ID] = t
	}

	// 6. Calculate weighted score for each candidate tool
	var results []models.SearchResult
	for _, toolID := range candidateIDs {
		tool, ok := toolsByID[toolID]
		if !ok {
			continue
		}

		toolComps := componentsByTool[toolID]
		var descSim, maxParamSim, maxExampleSim, returnsSim float64

		for _, comp := range toolComps {
			sim := CosineSimilarity(qVector, comp.Embedding)
			// Avoid negative cosine similarities (clip at 0)
			if sim < 0 {
				sim = 0
			}

			switch comp.ComponentType {
			case "description":
				descSim = sim
			case "parameter":
				if sim > maxParamSim {
					maxParamSim = sim
				}
			case "example":
				if sim > maxExampleSim {
					maxExampleSim = sim
				}
			case "returns":
				returnsSim = sim
			}
		}

		// Calculate weighted sum
		// Description: 50%, Parameters: 25%, Examples: 15%, Return Type: 10%
		weightedScore := (descSim * 0.50) + (maxParamSim * 0.25) + (maxExampleSim * 0.15) + (returnsSim * 0.10)

		// Popularity Boost: log scale, caps at 0.05 for high usage (e.g. 100 uses)
		popularityBoost := 0.05 * (math.Log1p(float64(tool.UsageCount)) / math.Log1p(100.0))
		if popularityBoost > 0.05 {
			popularityBoost = 0.05
		}

		finalScore := weightedScore + popularityBoost
		if se.Debug {
			fmt.Printf("📊 [DEBUG] Tool: %-25s | Weighted: %.4f | PopBoost: %.4f | Final: %.4f | Scores: [Desc: %.2f, Param: %.2f, Ex: %.2f, Ret: %.2f]\n",
				tool.ID, weightedScore, popularityBoost, finalScore, descSim, maxParamSim, maxExampleSim, returnsSim)
		}

		// Apply thresholding filter
		if finalScore >= se.minConfidence {
			results = append(results, models.SearchResult{
				Tool:       tool,
				FinalScore: finalScore,
				ComponentScores: map[string]float64{
					"description": descSim,
					"parameters":  maxParamSim,
					"examples":    maxExampleSim,
					"returns":     returnsSim,
				},
				PopularityBoost: popularityBoost,
			})
		}
	}

	// 7. Sort by score descending
	sort.Slice(results, func(i, j int) bool {
		return results[i].FinalScore > results[j].FinalScore
	})

	// 8. Adaptive selection (Score drop-off thresholding)
	selectedCount := len(results)
	if selectedCount > se.maxResults {
		selectedCount = se.maxResults
	}

	for i := 1; i < len(results) && i < se.maxResults; i++ {
		prevScore := results[i-1].FinalScore
		currScore := results[i].FinalScore

		if prevScore > 0 {
			dropPercentage := (prevScore - currScore) / prevScore
			// If there's a > 30% drop, truncate the selection here
			if dropPercentage > 0.30 {
				selectedCount = i
				break
			}
		}
	}

	// Get final results list
	var finalResults []models.SearchResult
	if len(results) > selectedCount {
		finalResults = results[:selectedCount]
	} else {
		finalResults = results
	}

	// 9. Save final matched tools to the semantic cache (for future exact/semantic hits)
	if len(finalResults) > 0 {
		var matchedIDs []string
		for _, r := range finalResults {
			matchedIDs = append(matchedIDs, r.Tool.ID)
		}
		_ = se.client.SaveToCache(ctx, query, qVector, matchedIDs)
	}

	return finalResults, nil
}
