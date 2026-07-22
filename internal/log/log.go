package log

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
)

var logger *slog.Logger

func Init(debug bool) {
	configDir, _ := os.UserConfigDir()
	logDir := filepath.Join(configDir, "meatshell")
	os.MkdirAll(logDir, 0755)

	logFile, err := os.OpenFile(filepath.Join(logDir, "meatshell.log"),
		os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		slog.Error("failed to open log file, using stderr only", "err", err)
		logger = slog.New(slog.NewTextHandler(os.Stderr, nil))
		return
	}

	var level slog.Level
	if debug {
		level = slog.LevelDebug
	} else {
		level = slog.LevelInfo
	}

	multi := io.MultiWriter(logFile, os.Stderr)
	logger = slog.New(slog.NewTextHandler(multi, &slog.HandlerOptions{Level: level}))
	slog.SetDefault(logger)
}

func Debug(msg string, args ...any) { slog.Debug(msg, args...) }
func Info(msg string, args ...any)  { slog.Info(msg, args...) }
func Warn(msg string, args ...any)  { slog.Warn(msg, args...) }
func Error(msg string, args ...any) { slog.Error(msg, args...) }
