package platform

import (
	"log/slog"
	"os"
	"strings"
)

// ConfigureLogging installs a JSON slog handler that writes to stdout, satisfying
// 12-factor XI (logs as an event stream the platform captures). The level is
// driven by cfg.LogLevel (LOG_LEVEL); unknown or empty values fall back to info.
func ConfigureLogging(cfg Config) {
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: parseLogLevel(cfg.LogLevel)})
	slog.SetDefault(slog.New(handler))
}

func parseLogLevel(value string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
