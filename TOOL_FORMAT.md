# Tool Creation and Formatting Guide

This guide documents the required JSON schema format and registration process for adding new semantic tools to the **Semantic Tool Selection (STS)** registry.

---

## 🏗️ How Tool Selection Works

Instead of embedding a tool definition as a single text block, this project splits every tool into **four distinct components** for indexing and scoring. This multi-component decomposition allows the vector search to score matching sub-elements independently:

1. **Tool Description** (Weight: **50%**) - The high-level capability.
2. **Parameters** (Weight: **25%**) - Inputs details.
3. **Usage Examples** (Weight: **15%**) - Invocation syntax.
4. **Return Types** (Weight: **10%**) - Output schemas.

Your descriptions and examples must be precise and descriptive to maximize semantic similarity cosine match scores.

---

## 📋 JSON Schema Blueprint

Every new tool must be defined in a JSON file matching the following structure. A template file is available at [tools/template_tool.json](file:///root/STS-surreal/tools/template_tool.json).

```json
{
  "id": "tool_name_identifier",
  "description": "Detailed description of the tool's core actions, systems affected, and domain category.",
  "parameters": [
    {
      "name": "param_name",
      "type": "string",
      "description": "Parameter details: formats, validation bounds, or default fallbacks.",
      "required": true
    }
  ],
  "examples": [
    {
      "usage": "tool_name_identifier(param_name='value')",
      "description": "Explanation of what this execution context represents."
    }
  ],
  "return_type": {
    "type": "object",
    "description": "Details of the output fields, arrays, or status codes returned by the execution."
  },
  "usage_count": 1
}
```

### Constraints:
* **`id`**: Must be lower_snake_case and unique in the database.
* **`parameters[].type`**: Supported types are `string`, `int`, `bool`, `array`, `object`.
* **`usage_count`**: Seed with `1`. This populates the popularity metric (popularity provides a small logarithmic tie-breaker boost of up to `0.05` on scores).

---

## 🚀 How to Register a Tool

Once your JSON definition is ready (e.g. `new_tool.json`), you can register it dynamically into the running SurrealDB registry in one of two ways:

### Method A: Via CLI Management Flags
Run this command from your terminal:
```bash
rtk ./aether manage create -tool-def /path/to/new_tool.json
```

### Method B: Via Interactive Console
Launch the interactive shell:
```bash
rtk ./aether
```
And execute the `:create` command inside the console:
```
💬 Enter query or command > :create /path/to/new_tool.json
```

---

## 🛠️ Design Best Practices for Agent Developers

When writing tool definitions to be matched by LLMs:
1. **Keyword Abundance**: Include relevant synonyms inside the tool description (e.g. if the tool is `backup_database`, mention "archive", "export", "dump", "compress").
2. **Usage Examples**: Write realistic examples reflecting how the agent would invoke it (e.g., matching the syntax of the system shell or scripts).
3. **Avoid Jargon**: Use clear, descriptive prose so that natural language queries match successfully.
