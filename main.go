package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"aether/config"
	"aether/db"
	"aether/embedding"
	"aether/models"
	"aether/search"
	"aether/tools"
)

func main() {
	// 1. Load config
	cfg := config.LoadConfig()

	ctx := context.Background()

	if len(os.Args) < 2 {
		runInteractive(ctx, cfg)
		return
	}

	subcommand := os.Args[1]
	switch subcommand {
	case "seed":
		seedCmd := flag.NewFlagSet("seed", flag.ExitOnError)
		fileFlag := seedCmd.String("file", "tools.json", "JSON file containing tools to seed (falls back to built-in registry if file not found)")
		seedCmd.Parse(os.Args[2:])

		embedder := setupEmbedder(ctx, cfg)
		defer embedder.Close()
		client := connectDB(ctx, cfg)
		defer client.Close(ctx)

		seedDatabase(ctx, client, embedder, *fileFlag)

	case "query":
		queryCmd := flag.NewFlagSet("query", flag.ExitOnError)
		optimizeFlag := queryCmd.Bool("optimize", false, "Optimize and compress wordy prompts before tool selection")
		sessionFlag := queryCmd.String("session", "default", "The session ID/context namespace (defaults to 'default')")
		debugFlag := queryCmd.Bool("debug", false, "Enable verbose scoring and evaluation logs")
		queryCmd.Parse(os.Args[2:])

		queryText := strings.Join(queryCmd.Args(), " ")
		if queryText == "" {
			fmt.Println("❌ Error: a query string is required. Usage: aether query [options] <query>")
			os.Exit(1)
		}

		embedder := setupEmbedder(ctx, cfg)
		defer embedder.Close()
		client := connectDB(ctx, cfg)
		defer client.Close(ctx)

		engine := search.NewSearchEngine(client, embedder, cfg.MinConfidence, cfg.MaxResults)
		engine.Debug = *debugFlag

		runQuerySearch(ctx, client, engine, embedder, queryText, *sessionFlag, *optimizeFlag)

	case "interactive":
		runInteractive(ctx, cfg)

	case "execute":
		execCmd := flag.NewFlagSet("execute", flag.ExitOnError)
		execCmd.Parse(os.Args[2:])
		toolID := execCmd.Arg(0)
		if toolID == "" {
			fmt.Println("❌ Error: tool ID is required. Usage: aether execute <tool_id>")
			os.Exit(1)
		}

		client := connectDB(ctx, cfg)
		defer client.Close(ctx)
		executeTool(ctx, client, toolID)

	case "manage":
		if len(os.Args) < 3 {
			fmt.Println("❌ Error: management action is required. Usage: aether manage <action> [options] (list, view, delete, create)")
			os.Exit(1)
		}
		action := os.Args[2]

		manageCmd := flag.NewFlagSet("manage", flag.ExitOnError)
		toolIDFlag := manageCmd.String("tool-id", "", "The target tool ID for management actions")
		toolDefFlag := manageCmd.String("tool-def", "", "JSON file containing a single tool schema to create/register")
		manageCmd.Parse(os.Args[3:])

		embedder := setupEmbedder(ctx, cfg)
		defer embedder.Close()
		client := connectDB(ctx, cfg)
		defer client.Close(ctx)

		handleToolManagement(ctx, client, embedder, action, *toolIDFlag, *toolDefFlag)

	case "db":
		if len(os.Args) < 3 {
			fmt.Println("❌ Error: database daemon action is required. Usage: aether db <action> [options] (start, stop, status, logs, install)")
			os.Exit(1)
		}
		action := os.Args[2]

		dbCmd := flag.NewFlagSet("db", flag.ExitOnError)
		logsCountFlag := dbCmd.Int("logs-count", 15, "Number of database log lines to print")
		dbCmd.Parse(os.Args[3:])

		handleDatabaseDaemon(ctx, action, *logsCountFlag)

	case "init":
		initCmd := flag.NewFlagSet("init", flag.ExitOnError)
		globalFlag := initCmd.Bool("global", false, "Initialize rule configuration globally (affects all workspaces)")
		nameFlag := initCmd.String("name", "", "The executable name or command path to run in the rule (defaults to absolute path of this binary)")
		initCmd.Parse(os.Args[2:])

		initializeAgentRules(ctx, *globalFlag, *nameFlag)

	case "memory":
		if len(os.Args) < 3 {
			fmt.Println("❌ Error: memory action is required. Usage: aether memory <action> [options] (add, query, list, clear)")
			os.Exit(1)
		}
		action := os.Args[2]

		memoryCmd := flag.NewFlagSet("memory", flag.ExitOnError)
		sessionFlag := memoryCmd.String("session", "default", "The session ID/context namespace (defaults to 'default')")
		categoryFlag := memoryCmd.String("category", "episodic", "Category of the memory (e.g. preference, episodic, task_log)")
		taskIDFlag := memoryCmd.String("task-id", "", "The specific background task ID to link with a task_log entry")
		memoryCmd.Parse(os.Args[3:])

		embedder := setupEmbedder(ctx, cfg)
		defer embedder.Close()
		client := connectDB(ctx, cfg)
		defer client.Close(ctx)

		switch action {
		case "add":
			content := strings.Join(memoryCmd.Args(), " ")
			if content == "" {
				fmt.Println("❌ Error: memory content is required. Usage: aether memory add [options] <content>")
				os.Exit(1)
			}
			handleMemoryAdd(ctx, client, embedder, content, *sessionFlag, *categoryFlag, *taskIDFlag)
		case "query":
			queryText := strings.Join(memoryCmd.Args(), " ")
			if queryText == "" {
				fmt.Println("❌ Error: memory query text is required. Usage: aether memory query [options] <query>")
				os.Exit(1)
			}
			handleMemoryQuery(ctx, client, embedder, queryText, *sessionFlag)
		case "list":
			handleMemoryList(ctx, client, *sessionFlag, *categoryFlag)
		case "clear":
			handleMemoryClear(ctx, client, *sessionFlag)
		default:
			fmt.Printf("❌ Error: unknown memory action '%s'. Supported: add, query, list, clear\n", action)
			os.Exit(1)
		}

	case "optimize":
		optimizeCmd := flag.NewFlagSet("optimize", flag.ExitOnError)
		optimizeCmd.Parse(os.Args[2:])

		textToOptimize := strings.Join(optimizeCmd.Args(), " ")
		if textToOptimize == "" {
			fmt.Println("❌ Error: text to optimize is required. Usage: aether optimize <text>")
			os.Exit(1)
		}

		embedder := setupEmbedder(ctx, cfg)
		defer embedder.Close()

		optimizer := search.NewPromptOptimizer(embedder)
		fmt.Println("⚡ Running prompt optimization pipeline...")
		optPrompt, intent, origWords, optWords, err := optimizer.Optimize(ctx, textToOptimize)
		if err != nil {
			log.Fatalf("❌ Prompt optimization failed: %v", err)
		}

		fmt.Printf("\n📉 PROMPT OPTIMIZATION REPORT:\n")
		fmt.Println("=====================================================================")
		fmt.Printf("  Original Word Count:  %d words\n", origWords)
		fmt.Printf("  Optimized Word Count: %d words\n", optWords)
		fmt.Printf("  Word/Token Reduction: %.2f%%\n", float64(origWords-optWords)/float64(origWords)*100.0)
		fmt.Printf("  Extracted Intent:     \"%s\"\n", intent)
		fmt.Println("=====================================================================")
		fmt.Printf("\n📝 OPTIMIZED CONTEXT:\n%s\n", optPrompt)

	case "help", "-help", "--help", "-h":
		printUsage()

	default:
		fmt.Printf("❌ Unknown subcommand '%s'\n\n", subcommand)
		printUsage()
	}
}

func connectDB(ctx context.Context, cfg *config.Config) *db.SurrealClient {
	fmt.Printf("🔌 Connecting to SurrealDB at %s...\n", cfg.SurrealURL)
	client, err := db.Connect(ctx, cfg.SurrealURL, cfg.SurrealUser, cfg.SurrealPass, cfg.SurrealNS, cfg.SurrealDB)
	if err != nil {
		log.Fatalf("❌ Failed to connect to SurrealDB: %v", err)
	}
	fmt.Println("✅ Connected successfully!")
	return client
}

func runInteractive(ctx context.Context, cfg *config.Config) {
	embedder := setupEmbedder(ctx, cfg)
	defer embedder.Close()
	client := connectDB(ctx, cfg)
	defer client.Close(ctx)

	engine := search.NewSearchEngine(client, embedder, cfg.MinConfidence, cfg.MaxResults)
	runInteractiveLoop(ctx, client, engine)
}

func printUsage() {
	fmt.Println("Usage: aether <subcommand> [options]")
	fmt.Println("\nSubcommands:")
	fmt.Println("  seed          Seed the SurrealDB database with tools and generate embeddings")
	fmt.Println("                Options: -file <path>")
	fmt.Println("  query         Run semantic tool selection for a natural language query")
	fmt.Println("                Options: -session <id> -optimize -debug")
	fmt.Println("  interactive   Run in interactive CLI mode (default if no subcommand provided)")
	fmt.Println("  execute       Simulate execution of a tool by ID")
	fmt.Println("  manage        Manage tools in database: list, view, delete, create")
	fmt.Println("                Options: -tool-id <id> -tool-def <path>")
	fmt.Println("  db            Manage database daemon: start, stop, status, logs, install")
	fmt.Println("                Options: -logs-count <num>")
	fmt.Println("  init          Initialize agent rule configuration file (AGENTS.md)")
	fmt.Println("                Options: -global -name <cli-name>")
	fmt.Println("  memory        Manage agent memory: add, query, list, clear")
	fmt.Println("                Options: -session <id> -category <cat> -task-id <id>")
	fmt.Println("  optimize      Compress and optimize a large text prompt/log directly")
	os.Exit(0)
}

// seedDatabase wipes the schema, creates indices, and indexes the tool library.
func seedDatabase(ctx context.Context, client *db.SurrealClient, embedder embedding.Generator, filePath string) {
	fmt.Println("\n🌱 Seeding database...")
	dim := embedder.Dimension()

	fmt.Printf("Creating schema and HNSW index (Dimension: %d)... ", dim)
	if err := client.InitializeSchema(ctx, dim); err != nil {
		log.Fatalf("❌ Schema initialization failed: %v", err)
	}
	fmt.Println("Done!")

	var library []models.Tool
	var err error

	// Try loading from file first
	if filePath != "" {
		if _, statErr := os.Stat(filePath); statErr == nil {
			fmt.Printf("Loading tools from JSON file: %s... ", filePath)
			library, err = loadToolsFromFile(filePath)
			if err != nil {
				log.Fatalf("❌ Error loading tools from file: %v", err)
			}
			fmt.Printf("Loaded %d tools.\n", len(library))
		} else {
			fmt.Printf("⚠️  Specified seed file '%s' not found. Falling back to built-in registry.\n", filePath)
			library = tools.GetLibrary()
		}
	} else {
		// Fallback to tools.json in current dir, then built-in registry
		if _, statErr := os.Stat("tools.json"); statErr == nil {
			fmt.Println("Found tools.json in current directory. Loading tools from tools.json... ")
			library, err = loadToolsFromFile("tools.json")
			if err != nil {
				log.Fatalf("❌ Error loading tools from tools.json: %v", err)
			}
			fmt.Printf("Loaded %d tools.\n", len(library))
		} else {
			fmt.Println("No tools.json found. Seeding with default built-in library.")
			library = tools.GetLibrary()
		}
	}

	fmt.Printf("Found %d tools in library. Embedding and storing tools...\n", len(library))

	for _, tool := range library {
		fmt.Printf("  Processing tool: %s... ", tool.ID)

		// 1. Gather all text components for batch embedding
		var texts []string
		var componentMapping []struct {
			cType string
			name  string
		}

		// Tool description
		texts = append(texts, tool.Description)
		componentMapping = append(componentMapping, struct{ cType, name string }{"description", ""})

		// Parameters
		for _, param := range tool.Parameters {
			texts = append(texts, fmt.Sprintf("%s: %s (%s)", param.Name, param.Description, param.Type))
			componentMapping = append(componentMapping, struct{ cType, name string }{"parameter", param.Name})
		}

		// Examples
		for _, ex := range tool.Examples {
			texts = append(texts, fmt.Sprintf("%s - %s", ex.Usage, ex.Description))
			componentMapping = append(componentMapping, struct{ cType, name string }{"example", ""})
		}

		// Returns
		texts = append(texts, tool.ReturnType.Description)
		componentMapping = append(componentMapping, struct{ cType, name string }{"returns", ""})

		// 2. Generate embeddings in batch
		start := time.Now()
		embeddings, err := embedder.GenerateBatch(ctx, texts)
		if err != nil {
			log.Fatalf("\n❌ Embedding generation failed for %s: %v", tool.ID, err)
		}
		duration := time.Since(start)

		// 3. Construct component structs
		var components []models.ToolComponent
		for i, emb := range embeddings {
			mapping := componentMapping[i]
			components = append(components, models.ToolComponent{
				ToolID:        tool.ID,
				ComponentType: mapping.cType,
				Name:          mapping.name,
				TextContent:   texts[i],
				Embedding:     emb,
				UsageCount:    tool.UsageCount,
			})
		}

		// 4. Save to SurrealDB
		if err := client.SaveTool(ctx, tool, components); err != nil {
			log.Fatalf("\n❌ Failed to save tool %s: %v", tool.ID, err)
		}
		fmt.Printf("Done (%d components, embedded in %v)\n", len(components), duration)
	}

	fmt.Println("\n🎉 Database seeding and embedding successfully completed!")
}

// executeTool simulates selecting a tool.
func executeTool(ctx context.Context, client *db.SurrealClient, toolID string) {
	fmt.Printf("\n🚀 Simulating execution of tool: %s...\n", toolID)
	err := client.IncrementToolUsage(ctx, toolID)
	if err != nil {
		fmt.Printf("❌ Failed to execute tool: %v\n", err)
		return
	}
	fmt.Println("✅ Execution complete! Popularity count incremented in SurrealDB.")
}

// runQuerySearch executes a single search query and displays token economics.
func runQuerySearch(ctx context.Context, client *db.SurrealClient, engine *search.SearchEngine, embedder embedding.Generator, query string, sessionID string, optimize bool) {
	var finalQuery = query
	if optimize {
		optimizer := search.NewPromptOptimizer(embedder)
		fmt.Println("⚡ Running prompt optimization pipeline (Cascading Chunking, Deduping & Intent Extraction)...")
		optPrompt, intent, origWords, optWords, err := optimizer.Optimize(ctx, query)
		if err != nil {
			fmt.Printf("⚠️  Prompt optimization failed: %v. Proceeding with raw query.\n", err)
		} else {
			savings := 100.0 * (1.0 - float64(optWords)/float64(origWords))
			fmt.Printf("\n📉 PROMPT OPTIMIZATION REPORT:\n")
			fmt.Println("=====================================================================")
			fmt.Printf("  Original Word Count:  %d words\n", origWords)
			fmt.Printf("  Optimized Word Count: %d words\n", optWords)
			fmt.Printf("  Word/Token Reduction: %.2f%%\n", savings)
			fmt.Printf("  Extracted Intent:     \"%s\"\n", intent)
			fmt.Println("=====================================================================")
			fmt.Printf("📝 OPTIMIZED PROMPT:\n%s\n", optPrompt)
			fmt.Println("=====================================================================")
			finalQuery = intent // Use the core intent for semantic tool search!
		}
	}

	fmt.Printf("\n🔍 Query: \"%s\"\n", finalQuery)

	// Fetch all tools to calculate full library token size
	allTools, err := client.GetAllTools(ctx)
	if err != nil {
		log.Fatalf("❌ Failed to fetch tools for metrics: %v", err)
	}
	if len(allTools) == 0 {
		fmt.Println("⚠️  Database is empty. Please seed the database first using 'seed' subcommand.")
		return
	}

	start := time.Now()
	results, err := engine.Search(ctx, finalQuery)
	duration := time.Since(start)
	if err != nil {
		log.Fatalf("❌ Search failed: %v", err)
	}

	printSearchResults(results, allTools, duration)

	// Semantically retrieve matching memories if sessionID is specified
	if sessionID != "" {
		vec, err := embedder.Generate(ctx, finalQuery)
		if err != nil {
			fmt.Printf("\n⚠️  Failed to generate query embedding for memory recall: %v\n", err)
			return
		}
		memories, err := client.QueryMemory(ctx, sessionID, vec, 5)
		if err != nil {
			fmt.Printf("\n⚠️  Memory search failed: %v\n", err)
			return
		}

		fmt.Printf("\n🧠 RECALLING ASSOCIATIVE MEMORIES (Session: %s):\n", sessionID)
		fmt.Println("=====================================================================")
		if len(memories) == 0 {
			fmt.Println("  No matching memories found in active session or global workspace namespace.")
		} else {
			for _, mem := range memories {
				fmt.Printf("  - [%s] [%s] %s (Time: %s)\n",
					mem.ID, mem.Category, mem.Content, mem.Timestamp.Format("2006-01-02 15:04:05"))
			}
		}
		fmt.Println("=====================================================================")
	}
}

// runInteractiveLoop runs a persistent CLI loop.
func runInteractiveLoop(ctx context.Context, client *db.SurrealClient, engine *search.SearchEngine) {
	reader := bufio.NewReader(os.Stdin)

	fmt.Println("\n=====================================================================")
	fmt.Println("   🤖 Aether: Context Router & Agent Memory Engine 🤖")
	fmt.Println("=====================================================================")
	fmt.Println("Commands:")
	fmt.Println("  :seed               Re-seed the database (generate embeddings)")
	fmt.Println("  :execute <tool_id>  Simulate execution (boost popularity)")
	fmt.Println("  :list               List all registered tools in the database")
	fmt.Println("  :view <tool_id>     View details/schema of a registered tool")
	fmt.Println("  :delete <tool_id>   Remove a tool from the database")
	fmt.Println("  :create <json_file> Register a new tool from a JSON definition file")
	fmt.Println("  :db <action>        Manage SurrealDB daemon: 'start', 'stop', 'status', 'logs', 'install'")
	fmt.Println("  :optimize <prompt>  Optimize and compress a wordy prompt")
	fmt.Println("  :mem-add <text>     Save a text fact/context in the active session memory")
	fmt.Println("  :mem-query <query>  Perform semantic vector recall across memory entries")
	fmt.Println("  :mem-list           Display chronological list of memory entries for this session")
	fmt.Println("  :mem-clear          Purge all memory entries in this session (separation)")
	fmt.Println("  :exit               Exit the application")
	fmt.Println("  [Or just type any natural language query below to find matching tools!]")
	fmt.Println("=====================================================================")

	for {
		fmt.Print("\n💬 Enter query or command > ")
		input, err := reader.ReadString('\n')
		if err != nil {
			break
		}
		input = strings.TrimSpace(input)
		if input == "" {
			continue
		}

		if input == ":exit" {
			fmt.Println("Goodbye!")
			break
		}

		if strings.HasPrefix(input, ":db ") {
			action := strings.TrimSpace(strings.TrimPrefix(input, ":db "))
			dm := db.NewDaemonManager()
			switch action {
			case "install":
				fmt.Println("🛠️  Starting local SurrealDB binary installation...")
				if err := dm.Install(ctx); err != nil {
					fmt.Printf("❌ Failed to install SurrealDB: %v\n", err)
				}
			case "start":
				if err := dm.Start(ctx); err != nil {
					fmt.Printf("❌ Failed to start SurrealDB daemon: %v\n", err)
				} else {
					fmt.Println("🚀 SurrealDB daemon started successfully in the background!")
				}
			case "stop":
				if err := dm.Stop(); err != nil {
					fmt.Printf("❌ Failed to stop SurrealDB daemon: %v\n", err)
				}
			case "status":
				fmt.Printf("ℹ️  SurrealDB Daemon Status: %s\n", dm.Status())
			case "logs":
				logs, err := dm.ShowLogs(15)
				if err != nil {
					fmt.Printf("❌ Failed to read daemon logs: %v\n", err)
				} else {
					fmt.Println("\n📋 SURREALDB DAEMON LOGS (Last 15 lines):")
					fmt.Println("---------------------------------------------------------------------")
					for _, line := range logs {
						fmt.Println(line)
					}
					fmt.Println("---------------------------------------------------------------------")
				}
			default:
				fmt.Printf("❌ Invalid database daemon action '%s'. Supported: start, stop, status, logs, install\n", action)
			}
			continue
		}

		if input == ":seed" {
			seedDatabase(ctx, client, engine.Embedder(), "tools.json")
			continue
		}

		if input == ":list" {
			toolsList, err := client.GetAllTools(ctx)
			if err != nil {
				fmt.Printf("❌ Failed to retrieve tools: %v\n", err)
				continue
			}
			fmt.Printf("\n📋 REGISTERED TOOLS (%d):\n", len(toolsList))
			fmt.Println("---------------------------------------------------------------------")
			for _, t := range toolsList {
				fmt.Printf("- %-25s | Pop: %-4d | %s\n", t.ID, t.UsageCount, t.Description)
			}
			fmt.Println("---------------------------------------------------------------------")
			continue
		}

		if strings.HasPrefix(input, ":view ") {
			toolID := strings.TrimSpace(strings.TrimPrefix(input, ":view "))
			toolsList, err := client.FetchToolsByIDs(ctx, []string{toolID})
			if err != nil || len(toolsList) == 0 {
				fmt.Printf("❌ Tool '%s' not found: %v\n", toolID, err)
				continue
			}
			data, _ := json.MarshalIndent(toolsList[0], "", "  ")
			fmt.Printf("\n🔍 Tool Schema for '%s':\n%s\n", toolID, string(data))
			continue
		}

		if strings.HasPrefix(input, ":delete ") {
			toolID := strings.TrimSpace(strings.TrimPrefix(input, ":delete "))
			err := client.DeleteTool(ctx, toolID)
			if err != nil {
				fmt.Printf("❌ Failed to delete tool '%s': %v\n", toolID, err)
				continue
			}
			fmt.Printf("🗑️  Successfully removed tool '%s' from SurrealDB!\n", toolID)
			continue
		}

		if strings.HasPrefix(input, ":mem-add ") {
			text := strings.TrimSpace(strings.TrimPrefix(input, ":mem-add "))
			vec, err := engine.Embedder().Generate(ctx, text)
			if err != nil {
				fmt.Printf("❌ Failed to generate embedding: %v\n", err)
				continue
			}
			entry := models.MemoryEntry{
				SessionID: "default",
				Content:   text,
				Category:  "episodic",
				Timestamp: time.Now(),
			}
			id, err := client.SaveMemory(ctx, entry, vec)
			if err != nil {
				fmt.Printf("❌ Failed to save memory: %v\n", err)
				continue
			}
			fmt.Printf("💾 Memory saved successfully under ID: %s (Session: default)\n", id)
			continue
		}

		if strings.HasPrefix(input, ":mem-query ") {
			queryText := strings.TrimSpace(strings.TrimPrefix(input, ":mem-query "))
			vec, err := engine.Embedder().Generate(ctx, queryText)
			if err != nil {
				fmt.Printf("❌ Failed to generate embedding: %v\n", err)
				continue
			}
			memories, err := client.QueryMemory(ctx, "default", vec, 5)
			if err != nil {
				fmt.Printf("❌ Memory query failed: %v\n", err)
				continue
			}
			fmt.Printf("\n🧠 RETRIEVED SEMANTIC MEMORIES (Session: default):\n")
			fmt.Println("---------------------------------------------------------------------")
			if len(memories) == 0 {
				fmt.Println("No matching memories found.")
			} else {
				for _, mem := range memories {
					fmt.Printf("- [%s] [%s] %s (Time: %s)\n", mem.ID, mem.Category, mem.Content, mem.Timestamp.Format("15:04:05"))
				}
			}
			fmt.Println("---------------------------------------------------------------------")
			continue
		}

		if input == ":mem-list" {
			memories, err := client.GetTimeSeriesMemories(ctx, "default", "", 20)
			if err != nil {
				fmt.Printf("❌ Failed to list memories: %v\n", err)
				continue
			}
			fmt.Printf("\n📋 CHRONOLOGICAL MEMORY LOGS (Session: default, Max 20):\n")
			fmt.Println("---------------------------------------------------------------------")
			if len(memories) == 0 {
				fmt.Println("No memories stored in this session.")
			} else {
				for _, mem := range memories {
					fmt.Printf("- [%s] [%s] %s (Time: %s)\n", mem.ID, mem.Category, mem.Content, mem.Timestamp.Format("2006-01-02 15:04:05"))
				}
			}
			fmt.Println("---------------------------------------------------------------------")
			continue
		}

		if input == ":mem-clear" {
			err := client.ClearSessionMemory(ctx, "default")
			if err != nil {
				fmt.Printf("❌ Failed to clear memory: %v\n", err)
				continue
			}
			fmt.Println("🗑️  Session memory cleared successfully (Session: default).")
			continue
		}

		if strings.HasPrefix(input, ":optimize ") {
			promptToOpt := strings.TrimSpace(strings.TrimPrefix(input, ":optimize "))
			optimizer := search.NewPromptOptimizer(engine.Embedder())
			fmt.Println("⚡ Optimizing prompt...")
			optPrompt, intent, origWords, optWords, err := optimizer.Optimize(ctx, promptToOpt)
			if err != nil {
				fmt.Printf("❌ Optimization failed: %v\n", err)
				continue
			}
			savings := 100.0 * (1.0 - float64(optWords)/float64(origWords))
			fmt.Printf("\n📉 PROMPT OPTIMIZATION REPORT:\n")
			fmt.Println("=====================================================================")
			fmt.Printf("  Original Word Count:  %d words\n", origWords)
			fmt.Printf("  Optimized Word Count: %d words\n", optWords)
			fmt.Printf("  Word/Token Reduction: %.2f%%\n", savings)
			fmt.Printf("  Extracted Intent:     \"%s\"\n", intent)
			fmt.Println("=====================================================================")
			fmt.Printf("📝 OPTIMIZED PROMPT:\n%s\n", optPrompt)
			fmt.Println("=====================================================================")

			// Auto search using the extracted intent!
			fmt.Println("\n🔍 Executing Tool Selection for Extracted Intent...")
			allTools, err := client.GetAllTools(ctx)
			if err != nil || len(allTools) == 0 {
				fmt.Println("⚠️  Cannot run tool selection: database empty or unreachable.")
				continue
			}
			start := time.Now()
			results, err := engine.Search(ctx, intent)
			duration := time.Since(start)
			if err != nil {
				fmt.Printf("❌ Search failed: %v\n", err)
				continue
			}
			printSearchResults(results, allTools, duration)
			continue
		}

		if strings.HasPrefix(input, ":create ") {
			defPath := strings.TrimSpace(strings.TrimPrefix(input, ":create "))
			data, err := os.ReadFile(defPath)
			if err != nil {
				fmt.Printf("❌ Failed to read file: %v\n", err)
				continue
			}
			var t models.Tool
			if err := json.Unmarshal(data, &t); err != nil {
				fmt.Printf("❌ Failed to parse JSON: %v\n", err)
				continue
			}
			if t.ID == "" {
				fmt.Println("❌ Error: tool 'id' field is required.")
				continue
			}

			var texts []string
			var componentMapping []struct {
				cType string
				name  string
			}
			texts = append(texts, t.Description)
			componentMapping = append(componentMapping, struct{ cType, name string }{"description", ""})

			for _, param := range t.Parameters {
				texts = append(texts, fmt.Sprintf("%s: %s (%s)", param.Name, param.Description, param.Type))
				componentMapping = append(componentMapping, struct{ cType, name string }{"parameter", param.Name})
			}
			for _, ex := range t.Examples {
				texts = append(texts, fmt.Sprintf("%s - %s", ex.Usage, ex.Description))
				componentMapping = append(componentMapping, struct{ cType, name string }{"example", ""})
			}
			texts = append(texts, t.ReturnType.Description)
			componentMapping = append(componentMapping, struct{ cType, name string }{"returns", ""})

			embeddings, err := engine.Embedder().GenerateBatch(ctx, texts)
			if err != nil {
				fmt.Printf("❌ Failed to generate embeddings: %v\n", err)
				continue
			}

			var components []models.ToolComponent
			for i, emb := range embeddings {
				mapping := componentMapping[i]
				components = append(components, models.ToolComponent{
					ToolID:        t.ID,
					ComponentType: mapping.cType,
					Name:          mapping.name,
					TextContent:   texts[i],
					Embedding:     emb,
					UsageCount:    t.UsageCount,
				})
			}

			if err := client.SaveTool(ctx, t, components); err != nil {
				fmt.Printf("❌ Failed to save tool: %v\n", err)
				continue
			}
			fmt.Printf("✅ Dynamically registered tool '%s' with %d components!\n", t.ID, len(components))
			continue
		}

		if strings.HasPrefix(input, ":execute ") {
			toolID := strings.TrimSpace(strings.TrimPrefix(input, ":execute "))
			executeTool(ctx, client, toolID)
			continue
		}

		// Check if seeded
		allTools, err := client.GetAllTools(ctx)
		if err != nil {
			fmt.Printf("❌ Failed to retrieve tools: %v\n", err)
			continue
		}
		if len(allTools) == 0 {
			fmt.Println("⚠️  Database is empty. Type ':seed' to load default tools first.")
			continue
		}

		// Treat input as a search query
		start := time.Now()
		results, err := engine.Search(ctx, input)
		duration := time.Since(start)
		if err != nil {
			fmt.Printf("❌ Search error: %v\n", err)
			continue
		}

		printSearchResults(results, allTools, duration)
	}
}

// ApproxTokens estimates token counts by dividing JSON serialized length by 4.
func ApproxTokens(v interface{}) int {
	data, err := json.Marshal(v)
	if err != nil {
		return 0
	}
	// 4 characters per token average
	return len(data) / 4
}

// printSearchResults displays matches and token cost reductions in ASCII tables.
func printSearchResults(results []models.SearchResult, allTools []models.Tool, duration time.Duration) {
	fmt.Printf("\n⏱️  Search latency: %v\n", duration)

	if len(results) == 0 {
		fmt.Println("❌ No matching tools found exceeding confidence threshold.")
		return
	}

	fmt.Println("\n🎯 MATCHED TOOLS:")
	fmt.Println("-----------------------------------------------------------------------------------------------------")
	fmt.Printf("| %-25s | %-12s | %-12s | %-40s |\n", "Tool ID", "Score", "Usage/Pop", "Top Component Match Scores")
	fmt.Println("-----------------------------------------------------------------------------------------------------")

	var selectedTools []models.Tool
	for _, res := range results {
		selectedTools = append(selectedTools, res.Tool)

		compBreakdown := fmt.Sprintf("Desc: %.2f, Param: %.2f, Ex: %.2f",
			res.ComponentScores["description"],
			res.ComponentScores["parameters"],
			res.ComponentScores["examples"])

		fmt.Printf("| %-25s | %-12.4f | %-12d | %-40s |\n",
			res.Tool.ID,
			res.FinalScore,
			res.Tool.UsageCount,
			compBreakdown)
	}
	fmt.Println("-----------------------------------------------------------------------------------------------------")

	// Calculate token economics
	allToolsTokens := 0
	for _, t := range allTools {
		allToolsTokens += ApproxTokens(t)
	}

	selectedToolsTokens := 0
	for _, t := range selectedTools {
		selectedToolsTokens += ApproxTokens(t)
	}

	savingsPercent := 0.0
	if allToolsTokens > 0 {
		savingsPercent = float64(allToolsTokens-selectedToolsTokens) / float64(allToolsTokens) * 100.0
	}

	fmt.Println("\n📈 TOKEN ECONOMICS:")
	fmt.Println("=====================================================================")
	fmt.Printf("  Total Tools in Database:     %d\n", len(allTools))
	fmt.Printf("  Total Library Context Size:  %d tokens (sending all tool definitions)\n", allToolsTokens)
	fmt.Println("---------------------------------------------------------------------")
	fmt.Printf("  Selected Tools for LLM:      %d\n", len(selectedTools))
	fmt.Printf("  Optimized Context Size:      %d tokens (sending filtered tools only)\n", selectedToolsTokens)
	fmt.Println("=====================================================================")
	fmt.Printf("  🎉 Context Space Saved:      %d tokens\n", allToolsTokens-selectedToolsTokens)
	fmt.Printf("  📉 LLM Input Token Savings:   %.2f%%\n", savingsPercent)
	fmt.Println("=====================================================================")
	writeRecommendedTools(results)
}

// setupEmbedder initializes the chosen embedding provider with a fallback hierarchy.
// Falls back to Gemini API (if key is set), then Mock Generator.
func setupEmbedder(ctx context.Context, cfg *config.Config) embedding.Generator {
	provider := cfg.EmbeddingProvider
	dim := cfg.EmbeddingDim

	getFallback := func(reason string) embedding.Generator {
		if cfg.GeminiAPIKey != "" {
			fmt.Printf("⚠️  %s. Falling back to Gemini API (Model: gemini-embedding-2, Dim: %d).\n", reason, dim)
			return embedding.NewGeminiGenerator(cfg.GeminiAPIKey, "gemini-embedding-2", dim)
		}
		fmt.Printf("⚠️  %s. No Gemini API key found. Falling back to Mock Generator (Dim: %d).\n", reason, dim)
		return embedding.NewMockGenerator(dim)
	}

	switch provider {
	case "onnx":
		fmt.Printf("🔍 Attempting to use local ONNX Go Embedder (Model: %s, Dim: %d)...\n", cfg.OnnxModelPath, dim)
		onnxGen := embedding.NewOnnxGenerator(cfg.OnnxModelPath, cfg.OnnxTokenizerPath, cfg.OnnxSharedLibPath, dim)
		if err := onnxGen.Init(); err != nil {
			return getFallback(fmt.Sprintf("ONNX initialization failed: %v", err))
		}
		// Try a quick test embedding to ensure runtime execution works
		if _, err := onnxGen.Generate(ctx, "test"); err != nil {
			onnxGen.Close()
			return getFallback(fmt.Sprintf("ONNX execution test failed: %v", err))
		}
		fmt.Println("✅ Local ONNX Go Embedder initialized successfully!")
		return onnxGen

	case "ollama":
		fmt.Printf("🦙 Attempting to use local Ollama Embedder (Model: %s at %s, Dim: %d)...\n", cfg.EmbeddingModel, cfg.OllamaURL, dim)
		ollamaGen := embedding.NewOllamaGenerator(cfg.OllamaURL, cfg.EmbeddingModel, dim)
		// Test Ollama server connectivity and model load
		testCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()
		if _, err := ollamaGen.Generate(testCtx, "test"); err != nil {
			return getFallback(fmt.Sprintf("Ollama connection or embedding failed: %v", err))
		}
		fmt.Println("✅ Local Ollama Embedder connected successfully!")
		return ollamaGen

	case "gemini":
		if cfg.GeminiAPIKey == "" {
			fmt.Printf("⚠️  Gemini provider selected but GEMINI_API_KEY is empty. Falling back to Mock Generator (Dim: %d).\n", dim)
			return embedding.NewMockGenerator(dim)
		}
		fmt.Printf("⚡ Using Gemini API Embedder (Model: %s, Dim: %d).\n", cfg.EmbeddingModel, dim)
		return embedding.NewGeminiGenerator(cfg.GeminiAPIKey, cfg.EmbeddingModel, dim)

	case "mock":
		fallthrough
	default:
		fmt.Printf("⚠️  Using deterministic keyword-aware Mock Embedder (Dim: %d).\n", dim)
		return embedding.NewMockGenerator(dim)
	}
}

// writeRecommendedTools writes the JSON definitions of the recommended tools to `.agents/recommended_tools.json`.
func writeRecommendedTools(results []models.SearchResult) {
	var toolsList []models.Tool
	for _, res := range results {
		toolsList = append(toolsList, res.Tool)
	}

	data, err := json.MarshalIndent(toolsList, "", "  ")
	if err != nil {
		fmt.Printf("⚠️  Failed to marshal recommended tools: %v\n", err)
		return
	}

	// Create .agents directory if it doesn't exist
	_ = os.MkdirAll(".agents", 0755)

	filePath := ".agents/recommended_tools.json"
	err = os.WriteFile(filePath, data, 0644)
	if err != nil {
		fmt.Printf("⚠️  Failed to write recommended tools to %s: %v\n", filePath, err)
		return
	}
	fmt.Printf("💾 Recommended tools exported to %s for agent usage!\n", filePath)
}

// loadToolsFromFile reads and parses a JSON tools library file.
func loadToolsFromFile(filePath string) ([]models.Tool, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	var toolsList []models.Tool
	if err := json.Unmarshal(data, &toolsList); err != nil {
		return nil, fmt.Errorf("failed to parse tools JSON: %w", err)
	}

	return toolsList, nil
}

// handleToolManagement processes tool registry operations: list, view, create, delete.
func handleToolManagement(ctx context.Context, client *db.SurrealClient, embedder embedding.Generator, action, toolID, toolDefPath string) {
	switch action {
	case "list":
		toolsList, err := client.GetAllTools(ctx)
		if err != nil {
			log.Fatalf("❌ Failed to list tools: %v", err)
		}
		fmt.Printf("\n📋 REGISTERED TOOLS (%d):\n", len(toolsList))
		fmt.Println("---------------------------------------------------------------------")
		for _, t := range toolsList {
			fmt.Printf("- %-25s | Pop: %-4d | %s\n", t.ID, t.UsageCount, t.Description)
		}
		fmt.Println("---------------------------------------------------------------------")

	case "view":
		if toolID == "" {
			log.Fatalf("❌ Error: -tool-id is required for view action.")
		}
		toolsList, err := client.FetchToolsByIDs(ctx, []string{toolID})
		if err != nil || len(toolsList) == 0 {
			log.Fatalf("❌ Tool '%s' not found: %v", toolID, err)
		}
		t := toolsList[0]
		data, _ := json.MarshalIndent(t, "", "  ")
		fmt.Printf("\n🔍 Tool Schema for '%s':\n%s\n", toolID, string(data))

	case "delete":
		if toolID == "" {
			log.Fatalf("❌ Error: -tool-id is required for delete action.")
		}
		err := client.DeleteTool(ctx, toolID)
		if err != nil {
			log.Fatalf("❌ Failed to delete tool '%s': %v", toolID, err)
		}
		fmt.Printf("🗑️  Successfully removed tool '%s' from SurrealDB!\n", toolID)

	case "create":
		if toolDefPath == "" {
			log.Fatalf("❌ Error: -tool-def file path is required for create action.")
		}
		data, err := os.ReadFile(toolDefPath)
		if err != nil {
			log.Fatalf("❌ Failed to read tool def file: %v", err)
		}
		var t models.Tool
		if err := json.Unmarshal(data, &t); err != nil {
			log.Fatalf("❌ Failed to parse tool definition JSON: %v", err)
		}
		if t.ID == "" {
			log.Fatalf("❌ Invalid tool definition: tool 'id' field is required.")
		}

		var texts []string
		var componentMapping []struct {
			cType string
			name  string
		}

		texts = append(texts, t.Description)
		componentMapping = append(componentMapping, struct{ cType, name string }{"description", ""})

		for _, param := range t.Parameters {
			texts = append(texts, fmt.Sprintf("%s: %s (%s)", param.Name, param.Description, param.Type))
			componentMapping = append(componentMapping, struct{ cType, name string }{"parameter", param.Name})
		}

		for _, ex := range t.Examples {
			texts = append(texts, fmt.Sprintf("%s - %s", ex.Usage, ex.Description))
			componentMapping = append(componentMapping, struct{ cType, name string }{"example", ""})
		}

		texts = append(texts, t.ReturnType.Description)
		componentMapping = append(componentMapping, struct{ cType, name string }{"returns", ""})

		embeddings, err := embedder.GenerateBatch(ctx, texts)
		if err != nil {
			log.Fatalf("❌ Embedding generation failed: %v", err)
		}

		var components []models.ToolComponent
		for i, emb := range embeddings {
			mapping := componentMapping[i]
			components = append(components, models.ToolComponent{
				ToolID:        t.ID,
				ComponentType: mapping.cType,
				Name:          mapping.name,
				TextContent:   texts[i],
				Embedding:     emb,
				UsageCount:    t.UsageCount,
			})
		}

		if err := client.SaveTool(ctx, t, components); err != nil {
			log.Fatalf("❌ Failed to save tool to db: %v", err)
		}
		fmt.Printf("✅ Dynamically registered tool '%s' with %d vector components!\n", t.ID, len(components))

	default:
		log.Fatalf("❌ Invalid action '%s'. Supported actions: list, view, create, delete", action)
	}
}

// handleDatabaseDaemon handles starting, stopping, showing status, or reading logs of the SurrealDB daemon process.
func handleDatabaseDaemon(ctx context.Context, action string, logCount int) {
	dm := db.NewDaemonManager()
	switch action {
	case "install":
		fmt.Println("🛠️  Starting local SurrealDB binary installation...")
		if err := dm.Install(ctx); err != nil {
			log.Fatalf("❌ Failed to install SurrealDB: %v", err)
		}
	case "start":
		if err := dm.Start(ctx); err != nil {
			log.Fatalf("❌ Failed to start SurrealDB daemon: %v", err)
		}
		fmt.Println("🚀 SurrealDB daemon started successfully in the background!")
	case "stop":
		if err := dm.Stop(); err != nil {
			log.Fatalf("❌ Failed to stop SurrealDB daemon: %v", err)
		}
	case "status":
		fmt.Printf("ℹ️  SurrealDB Daemon Status: %s\n", dm.Status())
	case "logs":
		logs, err := dm.ShowLogs(logCount)
		if err != nil {
			log.Fatalf("❌ Failed to retrieve daemon logs: %v", err)
		}
		fmt.Printf("\n📋 SURREALDB DAEMON LOGS (Last %d lines):\n", logCount)
		fmt.Println("---------------------------------------------------------------------")
		for _, line := range logs {
			fmt.Println(line)
		}
		fmt.Println("---------------------------------------------------------------------")
	default:
		log.Fatalf("❌ Invalid action '%s'. Supported actions: start, stop, status, logs, install", action)
	}
}

// initializeAgentRules writes the Semantic Tool Selection configuration rules (AGENTS.md)
// locally or globally to instruct the AI agent to always run prompt queries through the selector.
func initializeAgentRules(ctx context.Context, global bool, cliName string) {
	var targetPath string
	var ruleHeader string
	var locationName string

	// 1. Resolve target file paths and locations
	if global {
		home, err := os.UserHomeDir()
		if err != nil {
			log.Fatalf("❌ Failed to get user home directory: %v", err)
		}
		targetPath = filepath.Join(home, ".gemini", "config", "AGENTS.md")
		ruleHeader = "# Global Rules - Semantic Tool Selection (STS)"
		locationName = "globally in " + targetPath
	} else {
		_ = os.MkdirAll(".agents", 0755)
		targetPath = ".agents/AGENTS.md"
		ruleHeader = "# Workspace Rules - Semantic Tool Selection (STS)"
		locationName = "locally in workspace at " + targetPath
	}

	// 2. Resolve target CLI binary name/path
	resolvedCLI := cliName
	if resolvedCLI == "" {
		execPath, err := os.Executable()
		if err == nil {
			evalPath, err := filepath.EvalSymlinks(execPath)
			if err == nil {
				execPath = evalPath
			}
			// Don't use temporary debug builds path from go run
			if !strings.Contains(execPath, "/tmp/go-build") {
				resolvedCLI = execPath
			} else {
				resolvedCLI = "aether"
			}
		} else {
			resolvedCLI = "aether"
		}
	}

	// 3. Format rule content
	ruleContent := fmt.Sprintf(`

%s

This project implements **Aether**, an advanced agentic context routing and persistent memory engine. To optimize your context size (saving 75-90%% of tokens), enforce architectural standards, and maintain project-wide coherence, you **MUST STRICTLY** adhere to this pipeline. **DO NOT SKIP OR BYPASS THESE RULES.**

## 🚨 MANDATORY PIPELINE (Execute on Turn 1)

Before you select any tools, call files, or propose any implementation plans, you **MUST** run the semantic context query to retrieve active tools and persistent memories:

` + "```" + `bash
%s query -session "<session_id>" "<brief description of the user request>"
` + "```" + `
*(Note: Do not wander to other folders/projects looking for aether tool source. Use the exact binary path provided above. If it is not found, use '/root/.local/bin/aether' or the PATH command 'aether'.)*

### ⚠️ Self-Healing / Offline Database Startup
If the query command fails with a connection error or indicates SurrealDB is offline, you **MUST** start the database background daemon first and then retry the query:
` + "```" + `bash
%s db start
# Wait 1 second for the daemon to initialize, then run:
%s query -session "<session_id>" "<brief description of the user request>"
` + "```" + `

### 📋 Post-Query Actions (Mandatory)
1. **Immediately Read Recommended Tools**: You **MUST** read the file ` + "`.agents/recommended_tools.json`" + ` using your file viewing tool. This contains the exact schemas, parameters, and descriptions of the tools selected for this task.
2. **Restrict Your Tool Usage**: For the duration of this task, you are **ONLY** permitted to invoke the tools explicitly listed in ` + "`.agents/recommended_tools.json`" + `. Do not use default or general system-level tools for specialized tasks unless they are recommended.
3. **Adopt Persistent Context**: Adopt all guidelines, preferences, and architectural constraints returned in the stdout of the 'aether query' command.

## 🧠 persistence & Memory Logging (Turn N / Task Completion)

To keep context windows clean and persist design decisions across agent turns:
1. **Commit Architectural Choices / Feature States**: When you complete a task, resolve a bug, or make an architectural decision, save it in the persistent memory:
   ` + "```" + `bash
   %s memory add -session "<session_id>" -category "preference" "Decision: <concise summary of the architectural choice or feature state>"
   ` + "```" + `
2. **Recall Memories Independently**: If you need to search project memory without performing tool selection (e.g. to answer a question or check past work), query the memory store:
   ` + "```" + `bash
   %s memory query -session "<session_id>" "<task context or question>"
   ` + "```" + `
3. **Task and Execution Logs**: For long-running or background processes, log statuses periodically:
   ` + "```" + `bash
   %s memory add -session "<session_id>" -category "task_log" -task-id "<task_id>" "Log: <concise details>"
   ` + "```" + `

## 🛠️ Custom Tool Creation & Registration

To extend capabilities, you can dynamically register custom tools, helper scripts, or administration utilities:
1. Format your tool in JSON according to [TOOL_FORMAT.md](file:///root/STS-surreal/TOOL_FORMAT.md).
2. Register it into the vector database using:
   ` + "```" + `bash
   %s manage create -tool-def /path/to/new_tool.json
   ` + "```" + `
`, ruleHeader, resolvedCLI, resolvedCLI, resolvedCLI, resolvedCLI, resolvedCLI, resolvedCLI, resolvedCLI)

	// 4. Create or overwrite rules file
	_ = os.MkdirAll(filepath.Dir(targetPath), 0755)
	finalData := []byte(strings.TrimSpace(ruleContent) + "\n")

	// 5. Save rules file
	err := os.WriteFile(targetPath, finalData, 0644)
	if err != nil {
		log.Fatalf("❌ Failed to save agent rules: %v", err)
	}

	fmt.Printf("✅ Initialized agent rules configuration file %s\n", locationName)
}

// handleMemoryAdd saves a semantic memory entry inside the database.
func handleMemoryAdd(ctx context.Context, client *db.SurrealClient, embedder embedding.Generator, content, sessionID, category, taskID string) {
	fmt.Printf("⚡ Generating embedding for memory content... ")
	vec, err := embedder.Generate(ctx, content)
	if err != nil {
		log.Fatalf("❌ Failed to generate memory embedding: %v", err)
	}
	fmt.Println("Done!")

	entry := models.MemoryEntry{
		SessionID: sessionID,
		Content:   content,
		Category:  category,
		TaskID:    taskID,
		Timestamp: time.Now(),
	}

	id, err := client.SaveMemory(ctx, entry, vec)
	if err != nil {
		log.Fatalf("❌ Failed to save memory: %v", err)
	}

	fmt.Printf("✅ Saved memory successfully (ID: %s, Session: %s, Category: %s)\n", id, sessionID, category)
}

// handleMemoryQuery runs semantic search against active session memory.
func handleMemoryQuery(ctx context.Context, client *db.SurrealClient, embedder embedding.Generator, query, sessionID string) {
	fmt.Printf("⚡ Embedding memory query... ")
	vec, err := embedder.Generate(ctx, query)
	if err != nil {
		log.Fatalf("❌ Failed to generate query embedding: %v", err)
	}
	fmt.Println("Done!")

	memories, err := client.QueryMemory(ctx, sessionID, vec, 5)
	if err != nil {
		log.Fatalf("❌ Memory search failed: %v", err)
	}

	fmt.Printf("\n🧠 SEMANTIC MEMORY SEARCH RESULTS (Session: %s):\n", sessionID)
	fmt.Println("---------------------------------------------------------------------")
	if len(memories) == 0 {
		fmt.Println("No matching memories found.")
	} else {
		for _, mem := range memories {
			fmt.Printf("- [%s] [%s] %s (Time: %s)\n", mem.ID, mem.Category, mem.Content, mem.Timestamp.Format("2006-01-02 15:04:05"))
		}
	}
	fmt.Println("---------------------------------------------------------------------")
}

// handleMemoryList retrieves memory logs sequentially.
func handleMemoryList(ctx context.Context, client *db.SurrealClient, sessionID, category string) {
	memories, err := client.GetTimeSeriesMemories(ctx, sessionID, category, 50)
	if err != nil {
		log.Fatalf("❌ Failed to retrieve memories: %v", err)
	}

	fmt.Printf("\n📋 CHRONOLOGICAL MEMORY ENTRIES (Session: %s, Category Filter: '%s'):\n", sessionID, category)
	fmt.Println("---------------------------------------------------------------------")
	if len(memories) == 0 {
		fmt.Println("No memory records found.")
	} else {
		for _, mem := range memories {
			taskStr := ""
			if mem.TaskID != "" {
				taskStr = fmt.Sprintf(" | Task: %s", mem.TaskID)
			}
			fmt.Printf("[%s] %s | %s%s (%s)\n", mem.Timestamp.Format("2006-01-02 15:04:05"), mem.Category, mem.Content, taskStr, mem.ID)
		}
	}
	fmt.Println("---------------------------------------------------------------------")
}

// handleMemoryClear purges all memory logs in the session.
func handleMemoryClear(ctx context.Context, client *db.SurrealClient, sessionID string) {
	fmt.Printf("🗑️  Purging all memory logs under session ID: %s... ", sessionID)
	err := client.ClearSessionMemory(ctx, sessionID)
	if err != nil {
		log.Fatalf("❌ Failed to purge session memory: %v", err)
	}
	fmt.Println("Done!")
	fmt.Printf("✅ Memory cleared successfully for session: %s\n", sessionID)
}
