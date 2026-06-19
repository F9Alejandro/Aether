package db

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// DaemonManager handles spawning and controlling the SurrealDB background process.
type DaemonManager struct {
	cmd      *exec.Cmd
	pid      int
	logPath  string
	isDocker bool
}

// getDaemonDir returns the absolute path to the directory containing daemon files,
// located in the 'db_daemon' subdirectory of the program executable.
func getDaemonDir() string {
	execPath, err := os.Executable()
	if err != nil {
		return "db_daemon"
	}
	evalPath, err := filepath.EvalSymlinks(execPath)
	if err == nil {
		execPath = evalPath
	}
	return filepath.Join(filepath.Dir(execPath), "db_daemon")
}

// NewDaemonManager initializes a new daemon manager.
func NewDaemonManager() *DaemonManager {
	daemonDir := getDaemonDir()
	_ = os.MkdirAll(daemonDir, 0755)
	return &DaemonManager{
		logPath: filepath.Join(daemonDir, "surrealdb.log"),
	}
}

// Install downloads and extracts the official static SurrealDB binary for the host OS and architecture.
func (dm *DaemonManager) Install(ctx context.Context) error {
	daemonDir := getDaemonDir()
	binPath := filepath.Join(daemonDir, "surreal")
	if _, err := os.Stat(binPath); err == nil {
		fmt.Printf("✅ 'surreal' static binary is already installed locally in %s/\n", daemonDir)
		return nil
	}

	var url string
	switch runtime.GOOS {
	case "linux":
		switch runtime.GOARCH {
		case "amd64":
			url = "https://releases.surrealdb.com/stable/surreal-stable.linux-amd64.tgz"
		case "arm64":
			url = "https://releases.surrealdb.com/stable/surreal-stable.linux-arm64.tgz"
		default:
			return fmt.Errorf("unsupported Linux architecture: %s", runtime.GOARCH)
		}
	default:
		return fmt.Errorf("unsupported operating system: %s. Please install 'surreal' manually", runtime.GOOS)
	}

	fmt.Printf("📥 Downloading static SurrealDB release from %s...\n", url)

	// Create request
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create download request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to download release: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server returned status %d", resp.StatusCode)
	}

	// Extract the .tgz archive in-memory
	fmt.Println("📦 Extracting tar.gz archive...")
	gzipReader, err := gzip.NewReader(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to initialize gzip reader: %w", err)
	}
	defer gzipReader.Close()

	tarReader := tar.NewReader(gzipReader)
	found := false

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break // End of archive
		}
		if err != nil {
			return fmt.Errorf("failed to read tar header: %w", err)
		}

		// Look for the "surreal" binary file inside the archive
		if header.Typeflag == tar.TypeReg && (header.Name == "surreal" || strings.HasSuffix(header.Name, "/surreal")) {
			// Create local binary
			outFile, err := os.OpenFile(binPath, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0755)
			if err != nil {
				return fmt.Errorf("failed to create destination binary: %w", err)
			}
			defer outFile.Close()

			if _, err := io.Copy(outFile, tarReader); err != nil {
				return fmt.Errorf("failed to write binary file: %w", err)
			}
			found = true
			break
		}
	}

	if !found {
		return fmt.Errorf("could not find 'surreal' binary in the downloaded archive")
	}

	fmt.Printf("✅ Static SurrealDB binary installed successfully in %s!\n", binPath)
	return nil
}

// Start spawns the SurrealDB daemon in the background.
// Start spawns the SurrealDB daemon in the background.
func (dm *DaemonManager) Start(ctx context.Context) error {
	// Check if already running
	status := dm.Status()
	if strings.HasPrefix(status, "Running") {
		return fmt.Errorf("SurrealDB daemon is already running (%s)", status)
	}

	daemonDir := getDaemonDir()
	dataPath := filepath.Join(daemonDir, "data")
	_ = os.MkdirAll(dataPath, 0755)

	// 1. Determine execution method
	var execCmd *exec.Cmd
	dm.isDocker = false

	localBin := filepath.Join(daemonDir, "surreal")
	if _, err := os.Stat(localBin); err == nil {
		// Local static binary in daemonDir exists
		fmt.Printf("🚀 Found local static binary at %s. Spawning detached database daemon...\n", localBin)
		execCmd = exec.Command(localBin, "start", "--user", "root", "--pass", "root", "--bind", "127.0.0.1:8000", "surrealkv://"+dataPath)
		execCmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	} else if _, err := exec.LookPath("surreal"); err == nil {
		// Global surreal binary found
		fmt.Println("🚀 Found global 'surreal' binary in PATH. Spawning detached database daemon...")
		execCmd = exec.Command("surreal", "start", "--user", "root", "--pass", "root", "--bind", "127.0.0.1:8000", "surrealkv://"+dataPath)
		execCmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	} else if _, err := exec.LookPath("docker"); err == nil {
		// Fallback to Docker
		fmt.Println("🐳 Local 'surreal' binary not found. Spawning via Docker container 'aetherdb' in daemon mode...")
		dm.isDocker = true
		// Make sure any previous container is removed
		_ = exec.Command("docker", "rm", "-f", "aetherdb").Run()
		execCmd = exec.Command("docker", "run", "-d", "--name", "aetherdb", "-p", "8000:8000", "-v", dataPath+":/data", "surrealdb/surrealdb:latest", "start", "--user", "root", "--pass", "root", "surrealkv://data")
	} else {
		return fmt.Errorf("no database daemon found. Run ':db install' (interactive) or '-db install' (CLI) to automatically install the static SurrealDB binary")
	}

	// 2. Setup stdout/stderr for local execution logs
	var logFile *os.File
	var err error
	if !dm.isDocker {
		logFile, err = os.OpenFile(dm.logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
		if err != nil {
			return fmt.Errorf("failed to open daemon log file: %w", err)
		}
		execCmd.Stdout = logFile
		execCmd.Stderr = logFile
	}

	// 3. Start process
	if err := execCmd.Start(); err != nil {
		if logFile != nil {
			logFile.Close()
		}
		return fmt.Errorf("failed to start process: %w", err)
	}

	dm.cmd = execCmd
	dm.pid = execCmd.Process.Pid

	pidPath := filepath.Join(daemonDir, "surrealdb.pid")
	dockerPath := filepath.Join(daemonDir, "surrealdb.docker")

	// Save PID and daemon mode info for recovery
	_ = os.WriteFile(pidPath, []byte(strconv.Itoa(dm.pid)), 0644)
	if dm.isDocker {
		_ = os.WriteFile(dockerPath, []byte("true"), 0644)
		// Wait for docker client command to exit
		_ = execCmd.Wait()
		
		// Wait another second for container networking to set up
		time.Sleep(1 * time.Second)
		
		// Confirm container is running
		inspectOut, err := exec.Command("docker", "inspect", "-f", "{{.State.Running}}", "aetherdb").Output()
		if err != nil || strings.TrimSpace(string(inspectOut)) != "true" {
			// Container exited or failed
			dockerLogs, _ := exec.Command("docker", "logs", "aetherdb").CombinedOutput()
			return fmt.Errorf("Docker container failed to start: %s", string(dockerLogs))
		}
	} else {
		_ = os.Remove(dockerPath)
		// Wait a bit to ensure local process doesn't crash instantly
		time.Sleep(1 * time.Second)
		if logFile != nil {
			logFile.Close()
		}

		// Check if process is still running
		if err := execCmd.Process.Signal(syscall.Signal(0)); err != nil {
			dm.cmd = nil
			dm.pid = 0
			return fmt.Errorf("daemon exited immediately after startup. Check logs at %s", dm.logPath)
		}
	}

	return nil
}

// Stop terminates the SurrealDB daemon process.
func (dm *DaemonManager) Stop() error {
	daemonDir := getDaemonDir()
	dockerPath := filepath.Join(daemonDir, "surrealdb.docker")
	pidPath := filepath.Join(daemonDir, "surrealdb.pid")

	if _, err := os.Stat(dockerPath); err == nil {
		dm.isDocker = true
	}

	if dm.isDocker {
		fmt.Println("Stopping Docker container 'aetherdb'...")
		_ = exec.Command("docker", "stop", "aetherdb").Run()
		_ = exec.Command("docker", "rm", "aetherdb").Run()
		_ = os.Remove(dockerPath)
		_ = os.Remove(pidPath)
		fmt.Println("🛑 SurrealDB daemon container stopped and removed.")
		return nil
	}

	// Local binary stop
	if dm.pid == 0 {
		if data, err := os.ReadFile(pidPath); err == nil {
			if p, err := strconv.Atoi(strings.TrimSpace(string(data))); err == nil {
				dm.pid = p
			}
		}
	}

	if dm.pid == 0 {
		return fmt.Errorf("daemon process is not running or no PID tracked")
	}

	proc, err := os.FindProcess(dm.pid)
	if err != nil {
		return fmt.Errorf("failed to find process PID %d: %w", dm.pid, err)
	}

	_ = proc.Signal(syscall.SIGTERM)
	time.Sleep(500 * time.Millisecond)
	_ = proc.Kill()

	dm.cmd = nil
	dm.pid = 0
	_ = os.Remove(pidPath)

	fmt.Println("🛑 Local SurrealDB daemon process terminated.")
	return nil
}

// Status returns a string describing the daemon status.
func (dm *DaemonManager) Status() string {
	daemonDir := getDaemonDir()
	dockerPath := filepath.Join(daemonDir, "surrealdb.docker")
	pidPath := filepath.Join(daemonDir, "surrealdb.pid")

	if _, err := os.Stat(dockerPath); err == nil {
		dm.isDocker = true
	}

	if dm.isDocker {
		out, err := exec.Command("docker", "inspect", "-f", "{{.State.Running}}", "aetherdb").Output()
		if err == nil && strings.TrimSpace(string(out)) == "true" {
			return "Running (via Docker container 'aetherdb')"
		}
		return "Stopped"
	}

	// Local binary check
	if dm.pid == 0 {
		if data, err := os.ReadFile(pidPath); err == nil {
			if p, err := strconv.Atoi(strings.TrimSpace(string(data))); err == nil {
				dm.pid = p
			}
		}
	}

	if dm.pid == 0 {
		return "Stopped"
	}

	proc, err := os.FindProcess(dm.pid)
	if err != nil {
		return "Stopped"
	}

	// Check if running (signal 0 check)
	if err := proc.Signal(syscall.Signal(0)); err != nil {
		return "Stopped (Stale PID)"
	}

	return fmt.Sprintf("Running (PID: %d, local binary)", dm.pid)
}

// ShowLogs returns the last N lines of the log file or Docker container logs.
func (dm *DaemonManager) ShowLogs(n int) ([]string, error) {
	daemonDir := getDaemonDir()
	dockerPath := filepath.Join(daemonDir, "surrealdb.docker")

	if _, err := os.Stat(dockerPath); err == nil {
		dm.isDocker = true
	}

	if dm.isDocker {
		out, err := exec.Command("docker", "logs", "--tail", strconv.Itoa(n), "aetherdb").CombinedOutput()
		if err != nil {
			return nil, fmt.Errorf("failed to get docker logs: %w", err)
		}
		lines := strings.Split(string(out), "\n")
		// Clean up trailing empty line
		if len(lines) > 0 && lines[len(lines)-1] == "" {
			lines = lines[:len(lines)-1]
		}
		return lines, nil
	}

	// Local file
	file, err := os.Open(dm.logPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	if len(lines) > n {
		return lines[len(lines)-n:], nil
	}
	return lines, nil
}
