# Workspace Rules - Semantic Tool Selection (STS)

This project implements a semantic tool selection filter and associative agent memory database to optimize context size. All agent sessions must adhere to the following rules to maximize token savings.

## 🛠️ Semantic Tool Filtering & Memory Recall Constraint (Unified Context Pipeline)

Before proposing or executing any complex task requiring multiple tool calls or design considerations, you **MUST** run the user prompt through the semantic selector under your active session to identify both matching tools and recalled memory preferences in a single step:

```bash
rtk /root/STS-surreal/sts-surreal -query "<brief description of the user request>" -session "<session_id>"
```

- Running this command automatically exports the complete JSON schema definitions of the matched tools to the workspace file `.agents/recommended_tools.json`.
- It also performs semantic memory retrieval across the database, returning relevant workspace guidelines and global preferences in standard output.
- **You MUST immediately read `.agents/recommended_tools.json`** to obtain the precise parameter and usage schemas of the recommended tools.
- Do not inject all default tool schemas into your thinking or output context. For the duration of the current task, only refer to or use the schemas of the filtered tools found in `.agents/recommended_tools.json`.
- Adopt the retrieved memory constraints, preferences, or past design decisions returned in stdout.

## 🧠 Agent Memory & Logging Operations

To keep context windows clean and persist design decisions across turns:
1. **Isolate Sessions**: Run all memory operations under a specific session namespace using the `-session "<session_id>"` flag (e.g. matching your active workspace name or active branch).
2. **Recall Memories Independently**: If you need to search memories without selecting tools, run a semantic memory query using:
   ```bash
   rtk /root/STS-surreal/sts-surreal -memory-query "<task context or question>" -session "<session_id>"
   ```
3. **Commit New Memories**: After completing a feature, resolving a bug, or making a critical architectural decision, save it in the memory database:
   ```bash
   rtk /root/STS-surreal/sts-surreal -memory-add "Decision: <concise summary of the architectural choice or feature state>" -session "<session_id>" -category "preference"
   ```
4. **Log Task Progress**: For background tasks, log key outputs or execution logs to the database for future lookback:
   ```bash
   rtk /root/STS-surreal/sts-surreal -memory-add "Log: <concise log details>" -session "<session_id>" -category "task_log" -task-id "<task_id>"
   ```

## 🛠️ Custom Tool Creation & Registration

To extend capabilities, you can dynamically register custom tools, helper scripts, or administration utilities:
1. Format your tool in JSON according to [TOOL_FORMAT.md](file:///root/STS-surreal/TOOL_FORMAT.md).
2. Register it into the vector database using:
   ```bash
   rtk /root/STS-surreal/sts-surreal -manage create -tool-def /path/to/new_tool.json
   ```

## 📉 Token Optimization for Commands & Logs

To keep context windows small and avoid bloating history logs:
- **Always prefix shell commands with `rtk`** (e.g., `rtk go test`, `rtk docker ps`, `rtk git status`).
- When reading logs or verifying files, use `rtk`'s built-in log/stderr filtering capabilities to only display warnings or failures, stripping out boilerplates and successful execution logs.
