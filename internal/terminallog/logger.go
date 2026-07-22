// Package terminallog provides terminal session logging for audit and replay.
// Logger writes raw terminal output to a file with session metadata headers,
// allowing full session replay. Manager coordinates multiple concurrent loggers.
package terminallog

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// Logger writes terminal session output to a file for audit and replay.
// The log file begins with a metadata header followed by raw terminal bytes
// exactly as emitted by the remote end, preserving the byte stream so the
// session can be replayed through a terminal emulator.
type Logger struct {
	file        *os.File
	path        string
	enabled     bool
	mu          sync.Mutex
	startTime   time.Time
	sessionName string
}

// NewLogger creates a new Logger for the named session.
// The log file is created under ~/.config/meatshell/logs/ with a filename of
// the form <timestamp>_<sessionname>.log. The directory is created with
// permissions 0755 if it does not exist. The returned Logger is enabled.
func NewLogger(sessionName string) (*Logger, error) {
	dir, err := logDir()
	if err != nil {
		return nil, fmt.Errorf("resolve log directory: %w", err)
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create log directory: %w", err)
	}

	safe := sanitizeFilename(sessionName)
	if safe == "" {
		safe = "session"
	}
	filename := fmt.Sprintf("%s_%s.log", time.Now().Format("20060102-150405"), safe)
	path := filepath.Join(dir, filename)

	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, fmt.Errorf("open log file: %w", err)
	}

	return &Logger{
		file:        file,
		path:        path,
		enabled:     true,
		startTime:   time.Now(),
		sessionName: sessionName,
	}, nil
}

// logDir returns the absolute path to the session log directory.
func logDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "meatshell", "logs"), nil
}

// sanitizeFilename replaces characters that are unsafe in filenames.
func sanitizeFilename(name string) string {
	replacer := strings.NewReplacer(
		"/", "_", "\\", "_", ":", "_", "*", "_",
		"?", "_", "\"", "_", "<", "_", ">", "_",
		"|", "_", " ", "_",
	)
	return replacer.Replace(name)
}

// HostInfo returns a string describing the local host suitable for log headers.
func HostInfo() string {
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}
	var username string
	if u, err := user.Current(); err == nil {
		username = u.Username
	}
	return fmt.Sprintf("%s@%s (%s/%s)", username, hostname, runtime.GOOS, runtime.GOARCH)
}

// Write writes raw terminal data to the log file.
// If the logger is disabled, the data is silently dropped.
func (l *Logger) Write(data []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.enabled || l.file == nil {
		return 0, nil
	}
	return l.file.Write(data)
}

// WriteString is a convenience wrapper around Write for string inputs.
func (l *Logger) WriteString(s string) {
	l.Write([]byte(s))
}

// WriteHeader writes a metadata header at the start of the log file.
// info is additional host information included in the header; if empty,
// HostInfo() is used automatically.
func (l *Logger) WriteHeader(info string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file == nil {
		return
	}
	if info == "" {
		info = HostInfo()
	}
	header := fmt.Sprintf(
		"=== GoShell Session Log ===\nSession: %s\nDate: %s\nHost: %s\n===\n\n",
		l.sessionName,
		l.startTime.Format("2006-01-02 15:04:05"),
		info,
	)
	l.file.WriteString(header)
}

// Close closes the underlying log file. After calling Close, further Write
// calls are silently dropped.
func (l *Logger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file == nil {
		return nil
	}
	err := l.file.Close()
	l.file = nil
	return err
}

// Path returns the log file path.
func (l *Logger) Path() string {
	return l.path
}

// Enabled returns whether logging is currently active.
func (l *Logger) Enabled() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.enabled
}

// SetEnabled enables or disables logging. When disabled, Write calls are
// silently dropped.
func (l *Logger) SetEnabled(enabled bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.enabled = enabled
}

// Manager coordinates multiple concurrent terminal loggers.
type Manager struct {
	mu      sync.Mutex
	loggers map[string]*Logger
}

// NewManager creates a new Manager.
func NewManager() *Manager {
	return &Manager{
		loggers: make(map[string]*Logger),
	}
}

// StartLog creates a new Logger for the named session and registers it.
// The session ID (equal to the log file path) can be passed to StopLog.
func (m *Manager) StartLog(sessionName string) (*Logger, error) {
	logger, err := NewLogger(sessionName)
	if err != nil {
		return nil, err
	}
	m.mu.Lock()
	m.loggers[logger.path] = logger
	m.mu.Unlock()
	return logger, nil
}

// StopLog closes the logger identified by sessionID and removes it from the
// manager. The sessionID is the Logger.Path() value returned by StartLog.
func (m *Manager) StopLog(sessionID string) {
	m.mu.Lock()
	logger, ok := m.loggers[sessionID]
	if ok {
		delete(m.loggers, sessionID)
	}
	m.mu.Unlock()
	if logger != nil {
		logger.Close()
	}
}

// ListLogs returns the paths of all .log files in the log directory, sorted
// newest first by filename (which starts with a timestamp).
func (m *Manager) ListLogs() []string {
	dir, err := logDir()
	if err != nil {
		return nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var paths []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !strings.HasSuffix(entry.Name(), ".log") {
			continue
		}
		paths = append(paths, filepath.Join(dir, entry.Name()))
	}
	// os.ReadDir returns entries sorted by name ascending. Since filenames
	// start with a timestamp, ascending order means oldest first; reverse so
	// the newest logs appear at the top.
	for i, j := 0, len(paths)-1; i < j; i, j = i+1, j-1 {
		paths[i], paths[j] = paths[j], paths[i]
	}
	return paths
}

// ClearLogs removes log files older than the specified duration.
// Loggers that are currently active (started via StartLog and not yet
// stopped) are preserved.
func (m *Manager) ClearLogs(olderThan time.Duration) {
	m.mu.Lock()
	activePaths := make(map[string]bool, len(m.loggers))
	for path := range m.loggers {
		activePaths[path] = true
	}
	m.mu.Unlock()

	dir, err := logDir()
	if err != nil {
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	cutoff := time.Now().Add(-olderThan)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		if activePaths[path] {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			os.Remove(path)
		}
	}
}
