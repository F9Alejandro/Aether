package db

import (
	"context"
	"fmt"
	"strings"

	"github.com/surrealdb/surrealdb.go"
	dbmodels "github.com/surrealdb/surrealdb.go/pkg/models"
	"aether/models"
)

// SurrealClient manages connections and database operations for SurrealDB.
type SurrealClient struct {
	db  *surrealdb.DB
	url string
	ns  string
	sdb string
}

// Connect initializes the connection to SurrealDB.
func Connect(ctx context.Context, url, user, pass, ns, sdb string) (*SurrealClient, error) {
	db, err := surrealdb.FromEndpointURLString(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to surrealdb: %w", err)
	}

	// Sign in
	if _, err = db.SignIn(ctx, surrealdb.Auth{
		Username: user,
		Password: pass,
	}); err != nil {
		db.Close(ctx)
		return nil, fmt.Errorf("failed to sign in: %w", err)
	}

	// Create namespace and database if they don't exist
	setupCmd := fmt.Sprintf("DEFINE NAMESPACE IF NOT EXISTS %s; USE NAMESPACE %s; DEFINE DATABASE IF NOT EXISTS %s;", ns, ns, sdb)
	if _, err = surrealdb.Query[any](ctx, db, setupCmd, nil); err != nil {
		db.Close(ctx)
		return nil, fmt.Errorf("failed to setup namespace/database: %w", err)
	}

	// Select NS/DB
	if err := db.Use(ctx, ns, sdb); err != nil {
		db.Close(ctx)
		return nil, fmt.Errorf("failed to use ns/db: %w", err)
	}

	return &SurrealClient{
		db:  db,
		url: url,
		ns:  ns,
		sdb: sdb,
	}, nil
}

// Close closes the connection to the database.
func (sc *SurrealClient) Close(ctx context.Context) error {
	return sc.db.Close(ctx)
}

// InitializeSchema sets up tables and the HNSW vector index.
func (sc *SurrealClient) InitializeSchema(ctx context.Context, dim int) error {
	// Drop existing tables
	cleanupQuery := `
		REMOVE TABLE tool;
		REMOVE TABLE tool_component;
		REMOVE TABLE agent_memory;
		REMOVE TABLE agent_memory_component;
		REMOVE TABLE semantic_cache;
	`
	_, _ = surrealdb.Query[any](ctx, sc.db, cleanupQuery, nil)

	// Define schema
	schemaQuery := fmt.Sprintf(`
		DEFINE TABLE tool SCHEMALESS;
		DEFINE TABLE tool_component SCHEMALESS;
		DEFINE FIELD embedding ON TABLE tool_component TYPE array<float>;
		DEFINE INDEX hnsw_idx ON TABLE tool_component FIELDS embedding HNSW DIMENSION %d DIST COSINE;

		DEFINE TABLE agent_memory SCHEMALESS;
		DEFINE TABLE agent_memory_component SCHEMALESS;
		DEFINE FIELD embedding ON TABLE agent_memory_component TYPE array<float>;
		DEFINE INDEX memory_hnsw_idx ON TABLE agent_memory_component FIELDS embedding HNSW DIMENSION %d DIST COSINE;

		DEFINE TABLE semantic_cache SCHEMALESS;
		DEFINE FIELD embedding ON TABLE semantic_cache TYPE array<float>;
		DEFINE INDEX hnsw_cache_idx ON TABLE semantic_cache FIELDS embedding HNSW DIMENSION %d DIST COSINE;
	`, dim, dim, dim)

	_, err := surrealdb.Query[any](ctx, sc.db, schemaQuery, nil)
	if err != nil {
		return fmt.Errorf("failed to define schemas and index: %w", err)
	}

	return nil
}

// SaveTool stores a tool's definition and its component embeddings in the database.
func (sc *SurrealClient) SaveTool(ctx context.Context, tool models.Tool, components []models.ToolComponent) error {
	// Use custom RecordID to store under the correct table and ID
	toolRecordID := dbmodels.NewRecordID("tool", tool.ID)
	_, err := surrealdb.Create[models.Tool](ctx, sc.db, toolRecordID, tool)
	if err != nil {
		return fmt.Errorf("failed to save tool definition: %w", err)
	}

	// Save components
	for _, comp := range components {
		_, err := surrealdb.Create[models.ToolComponent](ctx, sc.db, "tool_component", comp)
		if err != nil {
			return fmt.Errorf("failed to save tool component %s: %w", comp.ComponentType, err)
		}
	}

	return nil
}

// IncrementToolUsage bumps the popularity/usage_count of a tool.
func (sc *SurrealClient) IncrementToolUsage(ctx context.Context, toolID string) error {
	q := `
		UPDATE $id SET usage_count += 1;
		UPDATE tool_component SET usage_count += 1 WHERE tool_id = $tool_id;
	`
	recordID := dbmodels.NewRecordID("tool", toolID)
	_, err := surrealdb.Query[any](ctx, sc.db, q, map[string]any{
		"id":      recordID,
		"tool_id": toolID,
	})
	return err
}

// GetAllTools retrieves all registered tools.
func (sc *SurrealClient) GetAllTools(ctx context.Context) ([]models.Tool, error) {
	q := "SELECT meta::id(id) AS id, * FROM tool;"
	res, err := surrealdb.Query[[]models.Tool](ctx, sc.db, q, nil)
	if err != nil {
		return nil, err
	}
	if len(*res) == 0 {
		return nil, nil
	}

	// SurrealDB returns list of results for each query statement in the query block
	return (*res)[0].Result, nil
}

// KNNResult holds the output from the KNN query.
type KNNResult struct {
	ToolID string `json:"tool_id"`
}

// FindCandidateToolIDs finds candidate tool IDs using the HNSW index.
func (sc *SurrealClient) FindCandidateToolIDs(ctx context.Context, queryVector []float64, k int) ([]string, error) {
	// Search in HNSW index using KNN query syntax.
	// Note: SurrealDB requires the limit k to be a literal integer in the query string.
	q := fmt.Sprintf(`
		SELECT tool_id 
		FROM tool_component 
		WHERE embedding <|%d, COSINE|> $q_vector;
	`, k)
	vars := map[string]any{
		"q_vector": queryVector,
	}

	res, err := surrealdb.Query[[]KNNResult](ctx, sc.db, q, vars)
	if err != nil {
		return nil, fmt.Errorf("KNN search failed: %w", err)
	}
	if len(*res) == 0 {
		return nil, nil
	}

	// Extract unique tool IDs
	seen := make(map[string]bool)
	var candidateIDs []string
	for _, doc := range (*res)[0].Result {
		if !seen[doc.ToolID] {
			seen[doc.ToolID] = true
			candidateIDs = append(candidateIDs, doc.ToolID)
		}
	}

	return candidateIDs, nil
}

// FetchComponentsForTools retrieves all component records for a specific list of tool IDs.
func (sc *SurrealClient) FetchComponentsForTools(ctx context.Context, toolIDs []string) ([]models.ToolComponent, error) {
	if len(toolIDs) == 0 {
		return nil, nil
	}

	// Prepare dynamic SQL for IN check
	placeholders := make([]string, len(toolIDs))
	vars := make(map[string]any)
	for i, id := range toolIDs {
		key := fmt.Sprintf("id%d", i)
		placeholders[i] = "$" + key
		vars[key] = id
	}

	q := fmt.Sprintf("SELECT * FROM tool_component WHERE tool_id IN [%s];", strings.Join(placeholders, ", "))
	res, err := surrealdb.Query[[]models.ToolComponent](ctx, sc.db, q, vars)
	if err != nil {
		return nil, err
	}
	if len(*res) == 0 {
		return nil, nil
	}

	return (*res)[0].Result, nil
}

// FetchToolsByIDs retrieves complete Tool definitions for a list of tool IDs.
func (sc *SurrealClient) FetchToolsByIDs(ctx context.Context, toolIDs []string) ([]models.Tool, error) {
	if len(toolIDs) == 0 {
		return nil, nil
	}

	ids := make([]dbmodels.RecordID, len(toolIDs))
	for i, id := range toolIDs {
		ids[i] = dbmodels.NewRecordID("tool", id)
	}

	q := "SELECT meta::id(id) AS id, * FROM tool WHERE id IN $ids;"
	res, err := surrealdb.Query[[]models.Tool](ctx, sc.db, q, map[string]any{"ids": ids})
	if err != nil {
		return nil, err
	}
	if len(*res) == 0 {
		return nil, nil
	}

	return (*res)[0].Result, nil
}

// DeleteTool removes a tool's definition and its component embeddings from the database.
func (sc *SurrealClient) DeleteTool(ctx context.Context, toolID string) error {
	q := `
		DELETE $id;
		DELETE tool_component WHERE tool_id = $tool_id;
	`
	recordID := dbmodels.NewRecordID("tool", toolID)
	_, err := surrealdb.Query[any](ctx, sc.db, q, map[string]any{
		"id":      recordID,
		"tool_id": toolID,
	})
	return err
}

// SaveMemory stores an agent memory record and its vector embedding in the database.
func (sc *SurrealClient) SaveMemory(ctx context.Context, entry models.MemoryEntry, embedding []float64) (string, error) {
	var q string
	var vars map[string]any
	if entry.ID != "" {
		q = "SELECT meta::id(id) AS id FROM (CREATE $id CONTENT $entry);"
		vars = map[string]any{
			"id":    dbmodels.NewRecordID("agent_memory", entry.ID),
			"entry": entry,
		}
	} else {
		q = "SELECT meta::id(id) AS id FROM (CREATE agent_memory CONTENT $entry);"
		vars = map[string]any{
			"entry": entry,
		}
	}

	res, err := surrealdb.Query[[]struct {
		ID string `json:"id"`
	}](ctx, sc.db, q, vars)
	if err != nil {
		return "", fmt.Errorf("failed to save memory entry: %w", err)
	}

	if len(*res) == 0 || len((*res)[0].Result) == 0 {
		return "", fmt.Errorf("failed to save memory entry: empty result returned")
	}

	memoryID := (*res)[0].Result[0].ID

	// Create memory vector component
	comp := models.MemoryComponent{
		MemoryID:  memoryID,
		Embedding: embedding,
		Text:      entry.Content,
	}
	_, err = surrealdb.Create[models.MemoryComponent](ctx, sc.db, "agent_memory_component", comp)
	if err != nil {
		return "", fmt.Errorf("failed to save memory vector component: %w", err)
	}

	return memoryID, nil
}

// MemoryKNNResult holds target memory ID from KNN queries.
type MemoryKNNResult struct {
	MemoryID string `json:"memory_id"`
}

// QueryMemory queries agent memory semantically using HNSW index and filters by session.
func (sc *SurrealClient) QueryMemory(ctx context.Context, sessionID string, queryVector []float64, k int) ([]models.MemoryEntry, error) {
	q := fmt.Sprintf(`
		SELECT memory_id 
		FROM agent_memory_component 
		WHERE embedding <|%d, COSINE|> $q_vector;
	`, k)

	res, err := surrealdb.Query[[]MemoryKNNResult](ctx, sc.db, q, map[string]any{
		"q_vector": queryVector,
	})
	if err != nil {
		return nil, fmt.Errorf("memory KNN search failed: %w", err)
	}
	if len(*res) == 0 {
		return nil, nil
	}

	var memoryIDs []string
	for _, doc := range (*res)[0].Result {
		memoryIDs = append(memoryIDs, doc.MemoryID)
	}
	if len(memoryIDs) == 0 {
		return nil, nil
	}

	// Fetch full memory records matching session or global
	qFetch := `
		SELECT meta::id(id) AS id, * 
		FROM agent_memory 
		WHERE id IN $ids AND (session_id = $session_id OR session_id = 'global')
		ORDER BY timestamp DESC;
	`

	recIDs := make([]dbmodels.RecordID, len(memoryIDs))
	for i, idStr := range memoryIDs {
		parts := strings.Split(idStr, ":")
		var tbl, val string
		if len(parts) == 2 {
			tbl = parts[0]
			val = parts[1]
		} else {
			tbl = "agent_memory"
			val = idStr
		}
		recIDs[i] = dbmodels.NewRecordID(tbl, val)
	}

	resMem, err := surrealdb.Query[[]models.MemoryEntry](ctx, sc.db, qFetch, map[string]any{
		"ids":        recIDs,
		"session_id": sessionID,
	})
	if err != nil {
		return nil, err
	}
	if len(*resMem) == 0 {
		return nil, nil
	}

	return (*resMem)[0].Result, nil
}

// GetTimeSeriesMemories retrieves memories sequentially/chronologically filtered by category.
func (sc *SurrealClient) GetTimeSeriesMemories(ctx context.Context, sessionID string, category string, limit int) ([]models.MemoryEntry, error) {
	q := `
		SELECT meta::id(id) AS id, * 
		FROM agent_memory 
		WHERE session_id = $session_id AND ($category = '' OR category = $category) 
		ORDER BY timestamp DESC 
		LIMIT $limit;
	`
	res, err := surrealdb.Query[[]models.MemoryEntry](ctx, sc.db, q, map[string]any{
		"session_id": sessionID,
		"category":   category,
		"limit":      limit,
	})
	if err != nil {
		return nil, err
	}
	if len(*res) == 0 {
		return nil, nil
	}
	return (*res)[0].Result, nil
}

// ClearSessionMemory purges all memory entries and components associated with a session.
func (sc *SurrealClient) ClearSessionMemory(ctx context.Context, sessionID string) error {
	q := `
		DELETE agent_memory_component WHERE memory_id.session_id = $session_id;
		DELETE agent_memory WHERE session_id = $session_id;
	`
	_, err := surrealdb.Query[any](ctx, sc.db, q, map[string]any{
		"session_id": sessionID,
	})
	return err
}
