package search

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"aether/embedding"
)

// PromptOptimizer manages context size reduction pipelines including chunking, deduplication, and cascading intent extraction.
type PromptOptimizer struct {
	embedder embedding.Generator
}

// NewPromptOptimizer initializes a new prompt optimizer.
func NewPromptOptimizer(embedder embedding.Generator) *PromptOptimizer {
	return &PromptOptimizer{embedder: embedder}
}

// ChunkText splits a long text into chunks by paragraphs (or lines if no paragraphs exist).
func (po *PromptOptimizer) ChunkText(text string, maxWords int) []string {
	paragraphs := strings.Split(text, "\n\n")

	// If only 1 paragraph was found, try splitting by single newlines
	if len(paragraphs) <= 1 {
		paragraphs = strings.Split(text, "\n")
	}

	var chunks []string

	for _, p := range paragraphs {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		words := strings.Fields(p)
		if len(words) == 0 {
			continue
		}

		// If a single paragraph/line is extremely long, split it
		if len(words) > maxWords {
			for idx := 0; idx < len(words); idx += maxWords {
				end := idx + maxWords
				if end > len(words) {
					end = len(words)
				}
				chunks = append(chunks, strings.Join(words[idx:end], " "))
			}
		} else {
			chunks = append(chunks, p)
		}
	}

	return chunks
}

// DeduplicateChunks filters out chunks that are semantically redundant (similarity > threshold).
func (po *PromptOptimizer) DeduplicateChunks(ctx context.Context, chunks []string, threshold float64) ([]string, error) {
	if len(chunks) <= 1 {
		return chunks, nil
	}

	embeddings, err := po.embedder.GenerateBatch(ctx, chunks)
	if err != nil {
		return nil, err
	}

	var uniqueChunks []string
	var uniqueEmbeddings [][]float64

	for i, chunk := range chunks {
		isDuplicate := false
		emb := embeddings[i]

		for _, uEmb := range uniqueEmbeddings {
			sim := CosineSimilarity(emb, uEmb)
			if sim > threshold {
				isDuplicate = true
				break
			}
		}

		if !isDuplicate {
			uniqueChunks = append(uniqueChunks, chunk)
			uniqueEmbeddings = append(uniqueEmbeddings, emb)
		}
	}

	return uniqueChunks, nil
}

// ExtractIntent calls the underlying generator to summarize the core instructions from the text.
func (po *PromptOptimizer) ExtractIntent(ctx context.Context, prompt string) (string, error) {
	if tg, ok := po.embedder.(interface {
		GenerateText(ctx context.Context, prompt string) (string, error)
	}); ok {
		return tg.GenerateText(ctx, prompt)
	}
	return embedding.HeuristicExtractIntent(prompt), nil
}

// SelectRelevantChunks sorts and filters chunks based on their semantic relevance to the extracted intent.
func (po *PromptOptimizer) SelectRelevantChunks(ctx context.Context, intent string, chunks []string, minConfidence float64, maxResults int) ([]string, error) {
	if len(chunks) <= 1 {
		return chunks, nil
	}

	intentEmb, err := po.embedder.Generate(ctx, intent)
	if err != nil {
		return nil, err
	}

	chunkEmbs, err := po.embedder.GenerateBatch(ctx, chunks)
	if err != nil {
		return nil, err
	}

	type chunkScore struct {
		chunk string
		score float64
	}

	var scored []chunkScore
	for i, chunk := range chunks {
		sim := CosineSimilarity(intentEmb, chunkEmbs[i])
		if sim >= minConfidence {
			scored = append(scored, chunkScore{chunk: chunk, score: sim})
		}
	}

	// Sort by score descending
	sort.Slice(scored, func(i, j int) bool {
		return scored[i].score > scored[j].score
	})

	var result []string
	for i := 0; i < len(scored) && i < maxResults; i++ {
		result = append(result, scored[i].chunk)
	}

	return result, nil
}

// Optimize runs the full cascading token-saving pipeline on a raw, wordy prompt.
func (po *PromptOptimizer) Optimize(ctx context.Context, rawPrompt string) (string, string, int, int, error) {
	originalWordCount := len(strings.Fields(rawPrompt))

	// 1. Extract the core intent/instruction first
	intent, err := po.ExtractIntent(ctx, rawPrompt)
	if err != nil {
		return "", "", 0, 0, fmt.Errorf("failed to extract core intent: %w", err)
	}

	// 2. Chunk the raw wordy prompt
	chunks := po.ChunkText(rawPrompt, 150) // Max 150 words per chunk

	// 3. Deduplicate redundant chunks semantically (threshold: 0.90 similarity)
	deduped, err := po.DeduplicateChunks(ctx, chunks, 0.90)
	if err != nil {
		return "", "", 0, 0, fmt.Errorf("failed to deduplicate chunks: %w", err)
	}

	// 4. Select the most relevant chunks matching the core intent (threshold: 0.40 similarity, max 3 chunks)
	relevant, err := po.SelectRelevantChunks(ctx, intent, deduped, 0.40, 3)
	if err != nil {
		return "", "", 0, 0, fmt.Errorf("failed to filter relevant chunks: %w", err)
	}

	// 5. Construct the final optimized prompt context
	var finalPrompt string
	if len(relevant) > 0 {
		finalPrompt = fmt.Sprintf("CORE INTENT: %s\n\nRELEVANT CONTEXT CHUNKS:\n%s", intent, strings.Join(relevant, "\n---\n"))
	} else {
		finalPrompt = fmt.Sprintf("CORE INTENT: %s", intent)
	}

	optimizedWordCount := len(strings.Fields(finalPrompt))
	return finalPrompt, intent, originalWordCount, optimizedWordCount, nil
}
