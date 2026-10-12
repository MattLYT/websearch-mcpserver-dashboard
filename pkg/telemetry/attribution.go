package telemetry

import (
	"context"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

type projectContextKey struct{}

// NormalizeProject accepts an opt-in label, never a filesystem path or URL.
func NormalizeProject(label string) string {
	label = strings.TrimSpace(label)
	if !utf8.ValidString(label) || utf8.RuneCountInString(label) > 64 {
		return ""
	}
	for _, r := range label {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && !strings.ContainsRune(" ._-", r) {
			return ""
		}
	}
	return label
}

func WithProject(ctx context.Context, project string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, projectContextKey{}, NormalizeProject(project))
}

func ProjectFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	project, _ := ctx.Value(projectContextKey{}).(string)
	return project
}

// Application groups known client aliases without rewriting historical rows.
func Application(client string) string {
	name := strings.ToLower(strings.TrimSpace(client))
	for _, app := range []string{"codex", "workbuddy", "claude", "cursor", "qwen", "gemini", "windsurf", "cherry", "cline", "roo", "trae", "zed", "vscode"} {
		if name == app || strings.HasPrefix(name, app+"-") || strings.HasPrefix(name, app+"_") || strings.HasPrefix(name, app+"/") || app == "cherry" && name == "cherrystudio" {
			return app
		}
	}
	return "unknown"
}

type AttributionUsage struct {
	Client      string `json:"client"`
	Application string `json:"application"`
	Project     string `json:"project"`
	Tool        string `json:"tool"`
	Requests    int64  `json:"requests"`
	Successes   int64  `json:"successes"`
	Failures    int64  `json:"failures"`
	DurationMS  int64  `json:"duration_ms"`
}

// AttributionUsageStats uses retained tool calls only; provider fan-out is not billed as extra client calls.
func (s *Store) AttributionUsageStats(days int) ([]AttributionUsage, int, error) {
	if days <= 0 {
		days = 7
	}
	days = min(days, s.retentionDays)
	cutoff := time.Now().AddDate(0, 0, -days).Unix()
	rows, err := s.db.Query(`SELECT client, project, tool, COUNT(*), SUM(success), SUM(duration_ms)
		FROM usage_events WHERE kind='tool' AND occurred_at>=?
		GROUP BY client, project, tool ORDER BY 4 DESC`, cutoff)
	if err != nil {
		return nil, days, err
	}
	defer rows.Close()
	out := make([]AttributionUsage, 0)
	for rows.Next() {
		var u AttributionUsage
		if err := rows.Scan(&u.Client, &u.Project, &u.Tool, &u.Requests, &u.Successes, &u.DurationMS); err != nil {
			return nil, days, err
		}
		u.Application = Application(u.Client)
		u.Failures = u.Requests - u.Successes
		out = append(out, u)
	}
	return out, days, rows.Err()
}
