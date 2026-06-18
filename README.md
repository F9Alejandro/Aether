# Semantic Tool Selection with Go, SurrealDB, and Gemini

This project implements a token-optimized **Semantic Tool Selection (STS)** system in **Go** using **SurrealDB** (replacing Redis) and **Gemini Embeddings** (replacing OpenAI).

By matching user natural queries semantically to specific tool sub-components rather than sending all tools to the LLM on every query, this architecture reduces context sizes by **75–90%**, significantly lowering token costs and reducing latency.

---

## 🏗️ Architecture Comparison

| Feature | Original (Article) | Our Implementation |
| :--- | :--- | :--- |
| **Language** | Node.js / Python | Go (Golang) |
| **Vector Store** | Redis Stack (HNSW) | **SurrealDB** (HNSW Vector Indexing) |
| **Embeddings** | OpenAI `text-embedding-3-small` | **Gemini `gemini-embedding-2`** (or `gemini-embedding-001`) |
| **DB Query Syntax** | Redis FT.SEARCH KNN | **SurrealQL `<\|k, COSINE\|>`** operator |

---

## 📂 Project Structure

- `main.go`: The CLI entry point. Coordinates database seeding, interactive querying, execution simulations, and token metrics.
- `config/config.go`: Config loader reading database credentials, namespace/db configuration, and Gemini API keys with default fallbacks.
- `models/models.go`: Structures representing `Tool` definitions, `ToolComponent` (embeddings), and `SearchResult`.
- `db/surreal.go`: Database client managing connections, `SCHEMALESS` definitions, `HNSW` indexes, and SurrealQL queries.
- `embedding/embedding.go`: Interface for embeddings, containing a real `Gemini` REST client and a normalized, deterministic, keyword-aware `Mock` generator for zero-dependency testing.
- `search/search.go`: Core search service managing HNSW retrieval, multi-component aggregation, popularity boosting, and adaptive thresholding.
- `tools/tools.go`: Registry of mock enterprise tools (Slack notifications, PostgreSQL/MySQL backups, Kubernetes scaling, AWS S3 directories, log readers).

---

## 🛢️ SurrealDB Schema & Indexing

The schema is configured dynamically based on the embedding dimension in Go:

```sql
-- Schema setup
DEFINE TABLE tool SCHEMALESS;
DEFINE TABLE tool_component SCHEMALESS;

-- Enforce float array validation on the vector field
DEFINE FIELD embedding ON TABLE tool_component TYPE array<float>;

-- Create the HNSW index on the vector field
DEFINE INDEX hnsw_idx ON TABLE tool_component FIELDS embedding HNSW DIMENSION 768 DIST COSINE;
```

---

## 🔍 Semantic Scoring Mechanics

Instead of embedding the entire tool definition as a single text block, each tool is decomposed into four semantic elements and queried using specialized weights:

1. **Tool Description** (Weight: **50%**) - The high-level tool capability.
2. **Parameters** (Weight: **25%**) - Description of inputs the tool accepts.
3. **Usage Examples** (Weight: **15%**) - Scenarios of how the tool is invoked.
4. **Return Types** (Weight: **10%**) - Description of what the tool outputs.

### Scoring Formula
$$\text{Relevance}(T, Q) = 0.50 \cdot \text{Sim}(Q, T_{\text{desc}}) + 0.25 \cdot \max_{p \in T_{\text{params}}}(\text{Sim}(Q, p)) + 0.15 \cdot \max_{e \in T_{\text{examples}}}(\text{Sim}(Q, e)) + 0.10 \cdot \text{Sim}(Q, T_{\text{returns}})$$

Popularity is boosted using a logarithmic scale based on usage frequency (adding up to `0.05` as a tie-breaker).

### Adaptive Selection
Tools are sorted descending by score. The list is sliced dynamically by checking for a significant drop-off:
- If score drops by **> 30%** between two consecutive matches, the list is truncated.
- A confidence threshold filters out poorly matching tools (default: `0.35`).
- The selection is capped at a maximum count (default: `5`).

---

## 🚀 How to Run the Program

### 1. Prerequisite: SurrealDB Database Instance
SurrealDB must be running. If not already up, you can start it via Docker:
```bash
docker run --name surrealdb -d -p 8000:8000 surrealdb/surrealdb:latest start --user root --pass root
```

### 2. Configuration & Fallbacks (JSON or Environment Variables)

The application reads configuration parameters from `config.json` in the current working directory (or the path specified by the `STS_CONFIG_FILE` env variable). Values in the configuration file can be overridden by environment variables.

Supported providers are `gemini`, `ollama`, `onnx`, and `mock`.

If a local provider (`onnx` or `ollama`) is selected but fails to initialize or connect at runtime, the application will automatically:
1. Fall back to the **Gemini API** if `GEMINI_API_KEY` is present in the environment or config.
2. Fall back to the deterministic, keyword-aware **Mock Generator** if no key is present.

Example `config.json`:
```json
{
  "embedding_provider": "gemini",
  "embedding_model": "gemini-embedding-2",
  "embedding_dim": 768,
  "gemini_api_key": ""
}
```

### 3. Build the Application
```bash
go build -o sts-surreal
```

### 4. Seed the Database (Required initially)
This wipes previous structures, creates tables, defines the HNSW index, generates component embeddings, and seeds the tools. By default, it loads from `tools.json` in the current working directory, falling back to the built-in Go registry if the file is not found:
```bash
# Seed from tools.json (or built-in fallback)
./sts-surreal -seed

# Seed from a custom JSON file
./sts-surreal -seed -seed-file custom_tools.json
```

### 5. Run Search Queries
Query semantic tool selection with a natural language string:
```bash
./sts-surreal -query "I need to notify the team on slack about the deployment status"
```
To run query with scoring breakdown logs:
```bash
./sts-surreal -query "I need to notify the team on slack" -debug
```

### 6. Interactive Mode
Run the application in interactive console mode:
```bash
./sts-surreal
```
Within the interactive console:
- Just type any search query (e.g. `run database backup`)
- Type `:execute <tool_id>` (e.g. `:execute backup_database`) to increment popularity
- Type `:list` to list all tools in the database
- Type `:view <tool_id>` to view details/schema of a registered tool
- Type `:delete <tool_id>` to remove a tool from the database
- Type `:create <json_file>` to register a new tool from JSON
- Type `:db <action>` to manage the SurrealDB background process ('start', 'stop', 'status', 'logs')
- Type `:seed` to re-index tools
- Type `:exit` to close the shell

### 7. Manage the Tool Registry
Manage tools dynamically in the database without recompiling:

**Via CLI Flags:**
```bash
# List registered tools
./sts-surreal -manage list

# View tool details
./sts-surreal -manage view -tool-id execute_hex_code

# Remove a tool
./sts-surreal -manage delete -tool-id obsolete_tool

# Register a tool from a JSON file
./sts-surreal -manage create -tool-def new_tool.json
```

### 8. Manage the Database Daemon
Manage the lifecycle of a self-contained SurrealDB background process (daemon) through the CLI:

**Via CLI Flags:**
```bash
# Start the background daemon process
./sts-surreal -db start

# Display the running status and PID of the daemon
./sts-surreal -db status

# View the last 15 lines of database output logs
./sts-surreal -db logs --db-logs-count 15

# Terminate the background daemon
./sts-surreal -db stop
```

### 9. Prompt Optimization Pipeline (Token-Saving Filter)
Compress and optimize wordy, log-heavy, or repetitive queries before executing tool matching:

**Via CLI Flags:**
```bash
# Optimize query and perform tool selection on the compressed core intent
./sts-surreal -query "Here are my server logs... I need to query the database..." -optimize
```

**Via Interactive Console:**
```
💬 Enter query or command > :optimize Here are my server logs... I need to query the database...
```

The pipeline dynamically runs:
1. **Cascading Intent Extraction**: Calls the active LLM provider (or a keyword-aware heuristic fallback) to distill the core user instruction down to a single concise sentence.
2. **Line-Preserving Chunking**: Chunks the input text by paragraphs and newlines.
3. **Semantic Deduplication**: Checks cosine similarity between chunks (threshold: `0.90`) and removes duplicates (e.g. repeated logs or stack traces).
4. **Relevance Selection**: Retains only the chunks highly similar to the core intent (threshold: `0.40`), filtering out noise.

### 10. Initialize Agent Rules Configuration
Automatically generate or append Semantic Tool Selection rules to the agent rule configuration files (`AGENTS.md`) locally or globally:

**Via CLI Flags:**
```bash
# Initialize local workspace rules (.agents/AGENTS.md)
./sts-surreal -init

# Initialize local workspace rules with a custom CLI command/name
./sts-surreal -init -init-name "my-custom-sts"

# Initialize global rules to apply rules across all workspaces
./sts-surreal -init -init-global
```

### 11. Agent Memory Manager
Maintain long-term semantic context, task logs, and user preferences to reduce token footprint on conversations and support session isolation.

**Session Separation (Isolation):**
Specify a `-session` namespace (such as a directory path, active branch name, or UUID). Global instructions and configuration preferences can be written under the `-session "global"` namespace, which are always returned during queries regardless of the active workspace session.

**Chronological (Time-Series) Querying:**
Chronological logging allows logging events, outputs, and status checks over time (e.g. using the `-category "task_log"` and `-task-id "<id>"` parameters). They can be listed sequentially using the `-memory-list` operation.

**CLI Memory Commands:**
```bash
# Add a workspace-specific preference memory
./sts-surreal -memory-add "The user's code files must always be stored in '/root/project'." -session "my-workspace" -category "preference"

# Add a global user instruction/preference (persistent across all sessions)
./sts-surreal -memory-add "The user prefers Go over Python." -session "global" -category "preference"

# Semantically query session memories (returns both session-specific and global matches)
./sts-surreal -memory-query "Where should code files be stored?" -session "my-workspace"

# View chronological memory logs
./sts-surreal -memory-list -session "my-workspace" -category "preference"

# Clear memory for a specific session namespace (starts with a clean slate)
./sts-surreal -memory-clear -session "my-workspace"
```

**Interactive Console Commands:**
*   `:mem-add <text>` — Save memory entry to the active `default` session.
*   `:mem-query <query>` — Recall matched semantic memories.
*   `:mem-list` — Print chronological memories list.
*   `:mem-clear` — Clear the active session memory.

---

## 📊 Sample Search Output (Interactive Analytics)

```
🔍 Query: "send a message to the team on slack about the outage"

⏱️  Search latency: 16.70ms

🎯 MATCHED TOOLS:
-----------------------------------------------------------------------------------------------------
| Tool ID                   | Score        | Usage/Pop    | Top Component Match Scores               |
-----------------------------------------------------------------------------------------------------
| send_slack_message        | 0.9386       | 45           | Desc: 1.00, Param: 1.00, Ex: 1.00        |
| send_email_report         | 0.8401       | 20           | Desc: 0.70, Param: 1.00, Ex: 0.71        |
-----------------------------------------------------------------------------------------------------

📈 TOKEN ECONOMICS:
=====================================================================
  Total Tools in Database:     10
  Total Library Context Size:  2539 tokens (sending all tool definitions)
---------------------------------------------------------------------
  Selected Tools for LLM:      2
  Optimized Context Size:      546 tokens (sending filtered tools only)
=====================================================================
  🎉 Context Space Saved:      1993 tokens
  📉 LLM Input Token Savings:   78.50%
=====================================================================
```
