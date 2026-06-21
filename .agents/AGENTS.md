# Workspace Rules - Semantic Tool Selection (STS)

This project implements **Aether**, an advanced agentic context routing and persistent memory engine. To optimize your context size (saving 75-90% of tokens), enforce architectural standards, and maintain project-wide coherence, you **MUST STRICTLY** adhere to this pipeline. **DO NOT SKIP OR BYPASS THESE RULES.**

## 🚨 MANDATORY PIPELINE (Execute on Turn 1)

Before you select any tools, call files, or propose any implementation plans, you **MUST** run the semantic context query to retrieve active tools and persistent memories:

```bash
/root/STS-surreal/aether query -session "<session_id>" "<brief description of the user request>"
```
*(Note: Do not wander to other folders/projects looking for aether tool source. Use the exact binary path provided above. If it is not found, use '/root/.local/bin/aether' or the PATH command 'aether'.)*

### ⚠️ Self-Healing / Offline Database Startup
If the query command fails with a connection error or indicates SurrealDB is offline, you **MUST** start the database background daemon first and then retry the query:
```bash
/root/STS-surreal/aether db start
# Wait 1 second for the daemon to initialize, then run:
/root/STS-surreal/aether query -session "<session_id>" "<brief description of the user request>"
```

### 📋 Post-Query Actions (Mandatory)
1. **Immediately Read Recommended Tools**: You **MUST** read the file `.agents/recommended_tools.json` using your file viewing tool. This contains the exact schemas, parameters, and descriptions of the tools selected for this task.
2. **Restrict Your Tool Usage**: For the duration of this task, you are **ONLY** permitted to invoke the tools explicitly listed in `.agents/recommended_tools.json`. Do not use default or general system-level tools for specialized tasks unless they are recommended.
3. **Adopt Persistent Context**: Adopt all guidelines, preferences, and architectural constraints returned in the stdout of the 'aether query' command.

## 🧠 persistence & Memory Logging (Turn N / Task Completion)

To keep context windows clean and persist design decisions across agent turns:
1. **Commit Architectural Choices / Feature States**: When you complete a task, resolve a bug, or make an architectural decision, save it in the persistent memory:
   ```bash
   /root/STS-surreal/aether memory add -session "<session_id>" -category "preference" "Decision: <concise summary of the architectural choice or feature state>"
   ```
2. **Recall Memories Independently**: If you need to search project memory without performing tool selection (e.g. to answer a question or check past work), query the memory store:
   ```bash
   /root/STS-surreal/aether memory query -session "<session_id>" "<task context or question>"
   ```
3. **Task and Execution Logs**: For long-running or background processes, log statuses periodically:
   ```bash
   /root/STS-surreal/aether memory add -session "<session_id>" -category "task_log" -task-id "<task_id>" "Log: <concise details>"
   ```

## 🛠️ Custom Tool Creation & Registration

To extend capabilities, you can dynamically register custom tools, helper scripts, or administration utilities:
1. Format your tool in JSON according to [TOOL_FORMAT.md](file:///root/STS-surreal/TOOL_FORMAT.md).
2. Register it into the vector database using:
   ```bash
   /root/STS-surreal/aether manage create -tool-def /path/to/new_tool.json
   ```
