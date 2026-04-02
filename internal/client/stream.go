package client

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
)

func drainNDJSON(body io.Reader) error {
	logger := slog.Default()
	scanner := bufio.NewScanner(body)
	for scanner.Scan() {
		var line map[string]any
		if err := json.Unmarshal(scanner.Bytes(), &line); err != nil {
			continue
		}

		if done, _ := line["done"].(bool); done {
			if errMsg, ok := line["error"].(string); ok {
				return fmt.Errorf("%s", errMsg)
			}
			return nil
		}

		msg, _ := line["msg"].(string)
		level, _ := line["level"].(string)

		var attrs []any
		for k, v := range line {
			if k == "time" || k == "level" || k == "msg" {
				continue
			}
			attrs = append(attrs, k, v)
		}

		logger.Log(context.Background(), parseLogLevel(level), msg, attrs...)
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	return fmt.Errorf("ored: stream ended without completion signal")
}

func parseLogLevel(s string) slog.Level {
	switch s {
	case "DEBUG":
		return slog.LevelDebug
	case "WARN":
		return slog.LevelWarn
	case "ERROR":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
