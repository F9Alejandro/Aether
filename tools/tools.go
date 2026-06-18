package tools

import "sts-surreal/models"

// GetLibrary returns a slice of standard enterprise tools for database seeding.
func GetLibrary() []models.Tool {
	return []models.Tool{
		{
			ID:          "send_slack_message",
			Description: "Send instant notifications, status updates, or alerts to a designated Slack channel using webhooks.",
			Parameters: []models.Parameter{
				{Name: "channel_id", Type: "string", Description: "Target Slack channel name or identifier (e.g., #engineering, C12345)", Required: true},
				{Name: "message", Type: "string", Description: "The actual text content to publish to the channel.", Required: true},
				{Name: "username", Type: "string", Description: "Custom display name for the bot sender.", Required: false},
				{Name: "webhook_url", Type: "string", Description: "Optional direct webhook endpoint to bypass default routing.", Required: false},
			},
			Examples: []models.Example{
				{Usage: "send_slack_message(channel_id='#engineering', message='Build pipeline succeeded!')", Description: "Notify developers about successful build"},
				{Usage: "send_slack_message(channel_id='#ops-alerts', message='CRITICAL: CPU threshold exceeded 90%')", Description: "Send a critical system warning to ops"},
			},
			ReturnType: models.ReturnType{
				Type:        "object",
				Description: "Confirmation detailing request status and timestamp of delivery.",
			},
			UsageCount: 45,
		},
		{
			ID:          "backup_database",
			Description: "Create a compressed backup archive of the specified PostgreSQL or MySQL database and upload it to secure storage.",
			Parameters: []models.Parameter{
				{Name: "db_name", Type: "string", Description: "Name of the target database schema.", Required: true},
				{Name: "storage_bucket", Type: "string", Description: "S3 or Cloud Storage bucket name to save the backup zip.", Required: true},
				{Name: "compression_level", Type: "int", Description: "Level of compression (1-9), defaults to 6.", Required: false},
			},
			Examples: []models.Example{
				{Usage: "backup_database(db_name='users_prod', storage_bucket='company-db-backups')", Description: "Create user database backup"},
				{Usage: "backup_database(db_name='inventory_db', storage_bucket='cold-backups', compression_level=9)", Description: "Create highly compressed archive of inventory database"},
			},
			ReturnType: models.ReturnType{
				Type:        "object",
				Description: "Object specifying backup filename, file size, bucket location, and SHA-256 hash.",
			},
			UsageCount: 12,
		},
		{
			ID:          "query_database",
			Description: "Execute a read-only SQL SELECT query against the active database cluster to fetch data records.",
			Parameters: []models.Parameter{
				{Name: "sql_query", Type: "string", Description: "The SQL query string to run. Must be read-only (SELECT).", Required: true},
				{Name: "db_cluster", Type: "string", Description: "Target cluster name (primary, replica-us, replica-eu).", Required: false},
				{Name: "row_limit", Type: "int", Description: "Max number of rows to return, defaults to 100.", Required: false},
			},
			Examples: []models.Example{
				{Usage: "query_database(sql_query='SELECT * FROM users WHERE status = \"active\" LIMIT 10')", Description: "Query active user records"},
				{Usage: "query_database(sql_query='SELECT COUNT(*) FROM orders', db_cluster='replica-eu')", Description: "Count total orders in EU replica"},
			},
			ReturnType: models.ReturnType{
				Type:        "array",
				Description: "Array of JSON objects matching rows returned by the database server.",
			},
			UsageCount: 89,
		},
		{
			ID:          "deploy_docker_container",
			Description: "Deploy a specified Docker image tag to the cloud environment (ECS, Kubernetes, or App Runner).",
			Parameters: []models.Parameter{
				{Name: "image_uri", Type: "string", Description: "Docker image registry path (e.g. gcr.io/my-project/api:v2.1.0)", Required: true},
				{Name: "environment", Type: "string", Description: "Target deployment tier (staging, production, sandbox)", Required: true},
				{Name: "port", Type: "int", Description: "Application listening port inside container.", Required: true},
				{Name: "replicas", Type: "int", Description: "Target number of container instances, defaults to 2.", Required: false},
			},
			Examples: []models.Example{
				{Usage: "deploy_docker_container(image_uri='docker.io/library/nginx:latest', environment='staging', port=80)", Description: "Deploy nginx to staging"},
				{Usage: "deploy_docker_container(image_uri='gcr.io/proj/api:v1', environment='production', port=8080, replicas=5)", Description: "Deploy custom API to production with 5 instances"},
			},
			ReturnType: models.ReturnType{
				Type:        "object",
				Description: "Deployment status, endpoint URLs, and deployment revision ID.",
			},
			UsageCount: 33,
		},
		{
			ID:          "send_email_report",
			Description: "Send HTML/Text email reports, alerts, or newsletters through secure SMTP or SendGrid provider.",
			Parameters: []models.Parameter{
				{Name: "recipient", Type: "string", Description: "Target email address of the receiver.", Required: true},
				{Name: "subject", Type: "string", Description: "Subject line of the email.", Required: true},
				{Name: "body_content", Type: "string", Description: "Text or HTML body content of the message.", Required: true},
				{Name: "attachments", Type: "array", Description: "List of file paths or S3 URIs to attach.", Required: false},
			},
			Examples: []models.Example{
				{Usage: "send_email_report(recipient='ceo@company.com', subject='Q2 Performance Report', body_content='Please find Q2 metrics attached')", Description: "Email performance report to management"},
				{Usage: "send_email_report(recipient='devs@company.com', subject='Alert: System Outage', body_content='API is down in prod')", Description: "Send high-priority email alert"},
			},
			ReturnType: models.ReturnType{
				Type:        "object",
				Description: "Delivery message ID and transmission status code.",
			},
			UsageCount: 20,
		},
		{
			ID:          "fetch_application_logs",
			Description: "Query and retrieve recent application logs from centralized log management systems (CloudWatch, ELK, Datadog) based on filters.",
			Parameters: []models.Parameter{
				{Name: "service_name", Type: "string", Description: "Name of the microservice (e.g. auth-service, gateway).", Required: true},
				{Name: "log_level", Type: "string", Description: "Log level filter (ERROR, WARN, INFO, DEBUG).", Required: false},
				{Name: "duration_minutes", Type: "int", Description: "Time window in minutes to look back, defaults to 15.", Required: false},
				{Name: "search_pattern", Type: "string", Description: "Keyword or regex to search inside logs.", Required: false},
			},
			Examples: []models.Example{
				{Usage: "fetch_application_logs(service_name='payment-api', log_level='ERROR', duration_minutes=30)", Description: "Get recent errors in payment service"},
				{Usage: "fetch_application_logs(service_name='gateway', search_pattern='TimeoutException')", Description: "Search gateway logs for timeouts"},
			},
			ReturnType: models.ReturnType{
				Type:        "array",
				Description: "Array of log entries with timestamp, log level, and raw log message.",
			},
			UsageCount: 74,
		},
		{
			ID:          "scale_kubernetes_deployment",
			Description: "Scale the replication count for a specific Kubernetes Deployment resource.",
			Parameters: []models.Parameter{
				{Name: "deployment_name", Type: "string", Description: "Name of the target deployment resource.", Required: true},
				{Name: "namespace", Type: "string", Description: "Kubernetes namespace (default, production, staging).", Required: false},
				{Name: "replicas", Type: "int", Description: "The desired number of running replicas.", Required: true},
			},
			Examples: []models.Example{
				{Usage: "scale_kubernetes_deployment(deployment_name='auth-server', replicas=4)", Description: "Scale auth deployment replicas to 4"},
				{Usage: "scale_kubernetes_deployment(deployment_name='web-front', namespace='production', replicas=10)", Description: "Scale production front-end to 10 instances"},
			},
			ReturnType: models.ReturnType{
				Type:        "object",
				Description: "Result summary with target replicas, active replicas, and namespace context.",
			},
			UsageCount: 15,
		},
		{
			ID:          "list_aws_s3_buckets",
			Description: "List files, directories, and metadata stored inside an Amazon Web Services S3 bucket.",
			Parameters: []models.Parameter{
				{Name: "bucket_name", Type: "string", Description: "The name of the target AWS S3 bucket.", Required: true},
				{Name: "prefix", Type: "string", Description: "Directory path prefix to filter objects.", Required: false},
				{Name: "max_keys", Type: "int", Description: "Max keys to list in a single batch, defaults to 1000.", Required: false},
			},
			Examples: []models.Example{
				{Usage: "list_aws_s3_buckets(bucket_name='company-reports', prefix='2026/')", Description: "List reports from the 2026 directory"},
				{Usage: "list_aws_s3_buckets(bucket_name='media-assets', max_keys=10)", Description: "List first 10 items in media bucket"},
			},
			ReturnType: models.ReturnType{
				Type:        "array",
				Description: "List of files with keys, size in bytes, and last modified dates.",
			},
			UsageCount: 28,
		},
		{
			ID:          "get_server_metrics",
			Description: "Fetch real-time host hardware metrics (CPU load, memory load, disk IO) from telemetry targets (Prometheus).",
			Parameters: []models.Parameter{
				{Name: "node_id", Type: "string", Description: "Target host machine name or IP address.", Required: true},
				{Name: "metric_type", Type: "string", Description: "Metric class to pull (cpu, memory, disk, network), defaults to cpu.", Required: false},
				{Name: "interval", Type: "string", Description: "Time interval step (e.g. 5m, 1h).", Required: false},
			},
			Examples: []models.Example{
				{Usage: "get_server_metrics(node_id='prod-db-01', metric_type='memory')", Description: "Retrieve memory usage from prod-db-01"},
				{Usage: "get_server_metrics(node_id='k8s-worker-12', interval='1h')", Description: "Retrieve hourly CPU usage from worker"},
			},
			ReturnType: models.ReturnType{
				Type:        "array",
				Description: "Time series data containing timestamps and float values.",
			},
			UsageCount: 61,
		},
		{
			ID:          "create_git_pr",
			Description: "Create a Git Pull Request or Merge Request automatically to merge code changes from development branch to release branch.",
			Parameters: []models.Parameter{
				{Name: "repository", Type: "string", Description: "Repository namespace/name (e.g., github/workspace/api-service).", Required: true},
				{Name: "source_branch", Type: "string", Description: "The feature branch containing commits.", Required: true},
				{Name: "target_branch", Type: "string", Description: "The branch to merge into (usually main or develop).", Required: true},
				{Name: "pr_title", Type: "string", Description: "Title of the Pull Request.", Required: true},
				{Name: "pr_description", Type: "string", Description: "Body description/release notes for reviewer.", Required: false},
			},
			Examples: []models.Example{
				{Usage: "create_git_pr(repository='org/api-service', source_branch='feature-auth', target_branch='develop', pr_title='Add oauth support')", Description: "Open pull request for OAuth branch"},
			},
			ReturnType: models.ReturnType{
				Type:        "object",
				Description: "Pull request details including PR URL, state, ID, and merge status.",
			},
			UsageCount: 19,
		},
		{
			ID:          "send_discord_message",
			Description: "Send real-time alerts, status updates, or notifications to a designated Discord channel using official Webhook APIs.",
			Parameters: []models.Parameter{
				{Name: "webhook_url", Type: "string", Description: "The unique Discord webhook endpoint URL.", Required: true},
				{Name: "content", Type: "string", Description: "The message text content (supports Discord markdown formatting).", Required: true},
				{Name: "username", Type: "string", Description: "Overrides the webhook bot's default username.", Required: false},
				{Name: "avatar_url", Type: "string", Description: "Overrides the webhook bot's default profile image link.", Required: false},
			},
			Examples: []models.Example{
				{Usage: "send_discord_message(webhook_url='https://discord.com/api/webhooks/...', content='**Alert**: Production DB backup failed!')", Description: "Send failed backup alert to discord"},
				{Usage: "send_discord_message(webhook_url='https://discord.com/api/webhooks/...', content='New release deployed to staging.', username='Staging Bot')", Description: "Send release success confirmation"},
			},
			ReturnType: models.ReturnType{
				Type:        "object",
				Description: "Confirmation details including HTTP response code and transmission status.",
			},
			UsageCount: 22,
		},
		{
			ID:          "execute_hex_code",
			Description: "Compile and execute a binary program directly from raw hex instruction bytes or machine language to run operations with maximum compression and speed, cutting down text representation size.",
			Parameters: []models.Parameter{
				{Name: "hex_instructions", Type: "string", Description: "Raw hexadecimal encoded string of compile target/machine instructions.", Required: true},
				{Name: "architecture", Type: "string", Description: "Target runtime architecture (x86_64, arm64, wasm).", Required: true},
				{Name: "memory_limit_mb", Type: "int", Description: "Max memory allocation limit in MB, defaults to 64.", Required: false},
			},
			Examples: []models.Example{
				{Usage: "execute_hex_code(hex_instructions='48c7c00100000048c7c701000000488d350a00000048c7c20c0000000f05b83c00000031ff0f0548656c6c6f20576f726c640a', architecture='x86_64')", Description: "Execute a raw x86_64 'Hello World' hex instruction assembly"},
			},
			ReturnType: models.ReturnType{
				Type:        "object",
				Description: "Execution report specifying exit code, stdout string, stderr string, execution time, and CPU cycle consumption.",
			},
			UsageCount: 5,
		},
		{
			ID:          "spawn_background_worker",
			Description: "Spawn and run an asynchronous background process or system script autonomously. The process runs decoupled from the active agent thread, allowing continuous execution, telemetry collection, or task queue monitoring without blocking the session or waiting for interactive user response.",
			Parameters: []models.Parameter{
				{Name: "command", Type: "string", Description: "The binary, script, or command to execute in the background.", Required: true},
				{Name: "working_dir", Type: "string", Description: "Working directory where the process is spawned.", Required: false},
				{Name: "log_file", Type: "string", Description: "File path to redirect standard output and stderr streams to capture logs.", Required: false},
				{Name: "auto_restart", Type: "bool", Description: "Restart the process automatically if it crashes or terminates with a non-zero exit code, defaults to false.", Required: false},
			},
			Examples: []models.Example{
				{Usage: "spawn_background_worker(command='go run worker.go', log_file='logs/worker.log', auto_restart=true)", Description: "Spawn worker.go daemon with logging and auto-restart policy"},
			},
			ReturnType: models.ReturnType{
				Type:        "object",
				Description: "Details containing the process ID (PID), log target, status (running), and timestamp.",
			},
			UsageCount: 15,
		},
		{
			ID:          "add_agent_memory",
			Description: "Save a text fact, configuration preference, architectural decision, or task log in the persistent agent memory database. This allows persisting context across CLI sessions and agent runs.",
			Parameters: []models.Parameter{
				{Name: "content", Type: "string", Description: "The actual factual statement, preference description, or execution log details to save.", Required: true},
				{Name: "session", Type: "string", Description: "The isolated session namespace/context (e.g., project name, active git branch, or 'global' for cross-session preferences).", Required: true},
				{Name: "category", Type: "string", Description: "Type of memory: 'preference' (guidelines), 'episodic' (general facts), or 'task_log' (execution logs). Defaults to 'episodic'.", Required: false},
				{Name: "task_id", Type: "string", Description: "Optional background task identifier to associate with log entries.", Required: false},
			},
			Examples: []models.Example{
				{Usage: "rtk ./sts-surreal -memory-add \"User prefers Docker deployments on port 8080\" -session \"web-app\" -category \"preference\"", Description: "Save a project-scoped deployment preference"},
				{Usage: "rtk ./sts-surreal -memory-add \"Build failed: compiler error in main.go:45\" -session \"web-app\" -category \"task_log\" -task-id \"task_101\"", Description: "Record an execution log entry for a specific task"},
			},
			ReturnType: models.ReturnType{
				Type:        "string",
				Description: "Confirmation message containing the saved record ID.",
			},
			UsageCount: 15,
		},
		{
			ID:          "query_agent_memory",
			Description: "Retrieve relevant context, decisions, and configuration preferences from the agent memory database using semantic (KNN) vector similarity. Returns both session-specific and global matches.",
			Parameters: []models.Parameter{
				{Name: "query", Type: "string", Description: "The search query or task context to find matching memories for.", Required: true},
				{Name: "session", Type: "string", Description: "The active session namespace/context (e.g., project name, git branch). Will automatically include global memories.", Required: true},
			},
			Examples: []models.Example{
				{Usage: "rtk ./sts-surreal -memory-query \"deployment port preferences\" -session \"web-app\"", Description: "Retrieve configuration preferences related to deployment port"},
			},
			ReturnType: models.ReturnType{
				Type:        "array",
				Description: "List of matching memory entries with their ID, category, content, and timestamp.",
			},
			UsageCount: 20,
		},
		{
			ID:          "list_agent_memories",
			Description: "List episodic context entries, preferences, or task progress logs in chronological (time-series) order for a specific session namespace.",
			Parameters: []models.Parameter{
				{Name: "session", Type: "string", Description: "The target session namespace to list.", Required: true},
				{Name: "category", Type: "string", Description: "Optional filter to restrict results to a specific category (e.g., 'task_log', 'preference').", Required: false},
			},
			Examples: []models.Example{
				{Usage: "rtk ./sts-surreal -memory-list -session \"web-app\" -category \"task_log\"", Description: "View the execution log history for the web-app session"},
			},
			ReturnType: models.ReturnType{
				Type:        "array",
				Description: "Chronological list of memory entries matching the filters.",
			},
			UsageCount: 10,
		},
		{
			ID:          "clear_agent_session",
			Description: "Purge all memory entries associated with a specific session namespace to ensure session isolation and start with a clean slate. Does not delete global memories.",
			Parameters: []models.Parameter{
				{Name: "session", Type: "string", Description: "The session namespace to clear.", Required: true},
			},
			Examples: []models.Example{
				{Usage: "rtk ./sts-surreal -memory-clear -session \"web-app\"", Description: "Wipe all memories for the web-app session"},
			},
			ReturnType: models.ReturnType{
				Type:        "string",
				Description: "Confirmation status indicating successful cleanup.",
			},
			UsageCount: 5,
		},
		{
			ID:          "optimize_prompt_context",
			Description: "Optimize and compress wordy text prompts, application log dumps, stack traces, or large file contents to extract the core instruction and remove redundant chunks before routing to the main LLM. This significantly reduces token usage.",
			Parameters: []models.Parameter{
				{Name: "text_content", Type: "string", Description: "The raw wordy text, console output, stack trace, or file contents to optimize.", Required: true},
			},
			Examples: []models.Example{
				{Usage: "rtk ./sts-surreal -optimize-text \"[ERROR] 2026-06-18T14:55:14 ... compiler error in main.go:45 ... (repeated 50 times)\"", Description: "Optimize and compress a redundant stack trace error log dump"},
			},
			ReturnType: models.ReturnType{
				Type:        "string",
				Description: "A formatted optimized text block containing the extracted core intent and relevant deduplicated chunks.",
			},
			UsageCount: 15,
		},
	}
}
