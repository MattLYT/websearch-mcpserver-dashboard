// Package telemetry stores privacy-preserving usage and passive health data for
// the local dashboard. Raw queries and URLs are never persisted.
package telemetry

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	_ "modernc.org/sqlite"
)

const defaultRetentionDays = 30

var (
	defaultMu    sync.RWMutex
	defaultStore *Store
)

// Event is a single tool or provider observation. Query is summarized before
// persistence and is never written verbatim.
type Event struct {
	Kind        string
	Tool        string
	Provider    string
	Query       string
	Success     bool
	Duration    time.Duration
	CacheHit    bool
	ResultCount int
	Error       error
	// Detail carries small, non-sensitive context such as which parser handled
	// a PDF. Raw queries and URLs are still never stored in this field.
	Detail string
	// ErrorKind is a stable failure category derived from Error at record time.
	ErrorKind string
	// RequestID correlates one tool call with the provider events it caused.
	RequestID string
	// AttemptChain summarizes which providers were attempted and who returned.
	AttemptChain string
}

type StoredEvent struct {
	ID            int64  `json:"id"`
	OccurredAt    string `json:"occurred_at"`
	Kind          string `json:"kind"`
	Tool          string `json:"tool"`
	Provider      string `json:"provider"`
	Success       bool   `json:"success"`
	DurationMS    int64  `json:"duration_ms"`
	CacheHit      bool   `json:"cache_hit"`
	ResultCount   int    `json:"result_count"`
	ErrorSummary  string `json:"error_summary,omitempty"`
	ErrorKind     string `json:"error_kind,omitempty"`
	Detail        string `json:"detail,omitempty"`
	RequestID     string `json:"request_id,omitempty"`
	AttemptChain  string `json:"attempt_chain,omitempty"`
	QueryHash     string `json:"query_hash,omitempty"`
	QueryChars    int    `json:"query_chars,omitempty"`
	QueryLanguage string `json:"query_language,omitempty"`
	QueryTopic    string `json:"query_topic,omitempty"`
	QueryKeywords string `json:"query_keywords,omitempty"`
}

type DailyUsage struct {
	Day         string `json:"day"`
	Kind        string `json:"kind"`
	Tool        string `json:"tool"`
	Provider    string `json:"provider"`
	Requests    int64  `json:"requests"`
	Successes   int64  `json:"successes"`
	Failures    int64  `json:"failures"`
	CacheHits   int64  `json:"cache_hits"`
	DurationMS  int64  `json:"duration_ms"`
	ResultCount int64  `json:"result_count"`
}

type Health struct {
	Name                string           `json:"name"`
	Kind                string           `json:"kind"`
	Status              string           `json:"status"`
	LastSuccessAt       string           `json:"last_success_at,omitempty"`
	LastFailureAt       string           `json:"last_failure_at,omitempty"`
	LastSeenAt          string           `json:"last_seen_at,omitempty"`
	FailureRate         float64          `json:"failure_rate"`
	SampleSize          int              `json:"sample_size"`
	ConsecutiveFailures int              `json:"consecutive_failures"`
	RecentOutcomes      []bool           `json:"recent_outcomes"`
	Today               DailyUsage       `json:"today"`
	LastError           string           `json:"last_error,omitempty"`
	State               string           `json:"state,omitempty"`
	Confidence          string           `json:"confidence,omitempty"`
	SuspendedUntil      string           `json:"suspended_until,omitempty"`
	SuspendReason       string           `json:"suspend_reason,omitempty"`
	SuspendCountdown    int64            `json:"suspend_countdown_sec,omitempty"`
	P95DurationMS       int64            `json:"p95_duration_ms"`
	P95MS               int64            `json:"-"`
	ErrorKinds          []ErrorKindCount `json:"error_kinds,omitempty"`
}

// ErrorKindCount is the failure composition of one source over the health
// window, ordered by count descending.
type ErrorKindCount struct {
	Kind  string `json:"kind"`
	Count int    `json:"count"`
}

// EventFilter selects privacy-preserving event metadata. Source is matched
// against provider for provider events and tool for tool events.
type EventFilter struct {
	Kind      string
	Status    string
	Source    string
	ErrorKind string
	RequestID string
	Limit     int
}

type Overview struct {
	GeneratedAt string       `json:"generated_at"`
	Today       DailyUsage   `json:"today"`
	Providers   []Health     `json:"providers"`
	Tools       []Health     `json:"tools"`
	Trend       []DailyUsage `json:"trend"`
}

type Store struct {
	db            *sql.DB
	location      *time.Location
	retentionDays int
}

func Open(path string, retentionDays int) (*Store, error) {
	if retentionDays <= 0 {
		retentionDays = defaultRetentionDays
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create telemetry directory: %w", err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open telemetry database: %w", err)
	}
	db.SetMaxOpenConns(1)
	if _, err = db.Exec(`
		PRAGMA journal_mode=WAL;
		PRAGMA busy_timeout=5000;
		CREATE TABLE IF NOT EXISTS usage_events (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			occurred_at INTEGER NOT NULL,
			day TEXT NOT NULL,
			kind TEXT NOT NULL,
			tool TEXT NOT NULL DEFAULT '',
			provider TEXT NOT NULL DEFAULT '',
			success INTEGER NOT NULL,
			duration_ms INTEGER NOT NULL DEFAULT 0,
			cache_hit INTEGER NOT NULL DEFAULT 0,
			result_count INTEGER NOT NULL DEFAULT 0,
			error_summary TEXT NOT NULL DEFAULT '',
			query_hash TEXT NOT NULL DEFAULT '',
			query_chars INTEGER NOT NULL DEFAULT 0,
			query_language TEXT NOT NULL DEFAULT '',
			query_topic TEXT NOT NULL DEFAULT '',
			query_keywords TEXT NOT NULL DEFAULT '',
			detail TEXT NOT NULL DEFAULT '',
			error_kind TEXT NOT NULL DEFAULT '',
			request_id TEXT NOT NULL DEFAULT '',
			attempt_chain TEXT NOT NULL DEFAULT ''
		);
		CREATE INDEX IF NOT EXISTS idx_usage_events_time ON usage_events(occurred_at DESC);
		CREATE INDEX IF NOT EXISTS idx_usage_events_provider ON usage_events(kind, provider, occurred_at DESC);
		CREATE TABLE IF NOT EXISTS daily_usage (
			day TEXT NOT NULL,
			kind TEXT NOT NULL,
			tool TEXT NOT NULL DEFAULT '',
			provider TEXT NOT NULL DEFAULT '',
			requests INTEGER NOT NULL DEFAULT 0,
			successes INTEGER NOT NULL DEFAULT 0,
			failures INTEGER NOT NULL DEFAULT 0,
			cache_hits INTEGER NOT NULL DEFAULT 0,
			duration_ms INTEGER NOT NULL DEFAULT 0,
			result_count INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY(day, kind, tool, provider)
		);`); err != nil {
		db.Close()
		return nil, fmt.Errorf("initialize telemetry database: %w", err)
	}
	// Existing databases created before these columns were added still open.
	// A duplicate-column error means this migration already ran.
	_, _ = db.Exec(`ALTER TABLE usage_events ADD COLUMN detail TEXT NOT NULL DEFAULT ''`)
	_, _ = db.Exec(`ALTER TABLE usage_events ADD COLUMN error_kind TEXT NOT NULL DEFAULT ''`)
	_, _ = db.Exec(`ALTER TABLE usage_events ADD COLUMN request_id TEXT NOT NULL DEFAULT ''`)
	_, _ = db.Exec(`ALTER TABLE usage_events ADD COLUMN attempt_chain TEXT NOT NULL DEFAULT ''`)
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		loc = time.FixedZone("CST", 8*60*60)
	}
	return &Store{db: db, location: loc, retentionDays: retentionDays}, nil
}

func SetDefault(s *Store) {
	defaultMu.Lock()
	defaultStore = s
	defaultMu.Unlock()
}

func Default() *Store {
	defaultMu.RLock()
	defer defaultMu.RUnlock()
	return defaultStore
}

func Record(e Event) {
	if s := Default(); s != nil {
		_ = s.Record(e)
	}
}

func (s *Store) Close() error {
	defaultMu.Lock()
	if defaultStore == s {
		defaultStore = nil
	}
	defaultMu.Unlock()
	return s.db.Close()
}

func (s *Store) Record(e Event) error {
	now := time.Now()
	if e.Kind == "" {
		e.Kind = "tool"
	}
	hash, chars, lang, topic, keywords := summarizeQuery(e.Query)
	errSummary := summarizeError(e.Error)
	errorKind := e.ErrorKind
	if errorKind == "" {
		errorKind = ClassifyErrorKind(e.Error)
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	success := boolInt(e.Success)
	cacheHit := boolInt(e.CacheHit)
	day := now.In(s.location).Format("2006-01-02")
	if _, err = tx.Exec(`INSERT INTO usage_events
		(occurred_at,day,kind,tool,provider,success,duration_ms,cache_hit,result_count,error_summary,query_hash,query_chars,query_language,query_topic,query_keywords,detail,error_kind,request_id,attempt_chain)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, now.Unix(), day, e.Kind, e.Tool, e.Provider, success,
		e.Duration.Milliseconds(), cacheHit, e.ResultCount, errSummary, hash, chars, lang, topic, keywords, e.Detail, errorKind, e.RequestID, e.AttemptChain); err != nil {
		return err
	}
	if _, err = tx.Exec(`INSERT INTO daily_usage(day,kind,tool,provider,requests,successes,failures,cache_hits,duration_ms,result_count)
		VALUES(?,?,?,?,1,?,?,?,?,?)
		ON CONFLICT(day,kind,tool,provider) DO UPDATE SET
		requests=requests+1, successes=successes+excluded.successes, failures=failures+excluded.failures,
		cache_hits=cache_hits+excluded.cache_hits, duration_ms=duration_ms+excluded.duration_ms,
		result_count=result_count+excluded.result_count`, day, e.Kind, e.Tool, e.Provider, success, 1-success, cacheHit,
		e.Duration.Milliseconds(), e.ResultCount); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) Cleanup() error {
	cutoff := time.Now().AddDate(0, 0, -s.retentionDays).Unix()
	_, err := s.db.Exec(`DELETE FROM usage_events WHERE occurred_at < ?`, cutoff)
	return err
}

func (s *Store) Recent(limit int) ([]StoredEvent, error) {
	return s.RecentFiltered(EventFilter{Limit: limit})
}

func (s *Store) RecentFiltered(filter EventFilter) ([]StoredEvent, error) {
	limit := filter.Limit
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	where := []string{"1=1"}
	args := make([]any, 0, 4)
	if filter.Kind == "provider" || filter.Kind == "tool" {
		where = append(where, "kind=?")
		args = append(args, filter.Kind)
	}
	if filter.Status == "success" {
		where = append(where, "success=1")
	} else if filter.Status == "failure" {
		where = append(where, "success=0")
	}
	if filter.Source != "" {
		if filter.Kind == "provider" {
			where = append(where, "provider=?")
			args = append(args, filter.Source)
		} else if filter.Kind == "tool" {
			where = append(where, "tool=?")
			args = append(args, filter.Source)
		} else {
			where = append(where, "(provider=? OR tool=?)")
			args = append(args, filter.Source, filter.Source)
		}
	}
	if filter.ErrorKind != "" {
		where = append(where, "error_kind=?")
		args = append(args, filter.ErrorKind)
	}
	if filter.RequestID != "" {
		where = append(where, "request_id=?")
		args = append(args, filter.RequestID)
	}
	args = append(args, limit)
	query := `SELECT id,occurred_at,kind,tool,provider,success,duration_ms,cache_hit,result_count,
		error_summary,query_hash,query_chars,query_language,query_topic,query_keywords,detail,error_kind,request_id,attempt_chain
		FROM usage_events WHERE ` + strings.Join(where, " AND ") + ` ORDER BY occurred_at DESC,id DESC LIMIT ?`
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]StoredEvent, 0, limit)
	for rows.Next() {
		var e StoredEvent
		var ts int64
		var ok, hit int
		if err := rows.Scan(&e.ID, &ts, &e.Kind, &e.Tool, &e.Provider, &ok, &e.DurationMS, &hit,
			&e.ResultCount, &e.ErrorSummary, &e.QueryHash, &e.QueryChars, &e.QueryLanguage, &e.QueryTopic, &e.QueryKeywords, &e.Detail, &e.ErrorKind, &e.RequestID, &e.AttemptChain); err != nil {
			return nil, err
		}
		e.Success, e.CacheHit = ok == 1, hit == 1
		e.OccurredAt = time.Unix(ts, 0).In(s.location).Format(time.RFC3339)
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s *Store) Overview() (Overview, error) {
	now := time.Now()
	out := Overview{GeneratedAt: now.In(s.location).Format(time.RFC3339)}
	day := now.In(s.location).Format("2006-01-02")
	_ = s.db.QueryRow(`SELECT COALESCE(SUM(requests),0),COALESCE(SUM(successes),0),COALESCE(SUM(failures),0),
		COALESCE(SUM(cache_hits),0),COALESCE(SUM(duration_ms),0),COALESCE(SUM(result_count),0)
		FROM daily_usage WHERE day=? AND kind='tool'`, day).Scan(&out.Today.Requests, &out.Today.Successes,
		&out.Today.Failures, &out.Today.CacheHits, &out.Today.DurationMS, &out.Today.ResultCount)
	out.Today.Day, out.Today.Kind = day, "tool"
	var err error
	out.Providers, err = s.health("provider", now)
	if err != nil {
		return out, err
	}
	out.Tools, err = s.health("tool", now)
	if err != nil {
		return out, err
	}
	out.Trend, err = s.trend(14)
	return out, err
}

func (s *Store) trend(days int) ([]DailyUsage, error) {
	start := time.Now().In(s.location).AddDate(0, 0, -(days - 1)).Format("2006-01-02")
	rows, err := s.db.Query(`SELECT day,COALESCE(SUM(requests),0),COALESCE(SUM(successes),0),COALESCE(SUM(failures),0),
		COALESCE(SUM(cache_hits),0),COALESCE(SUM(duration_ms),0),COALESCE(SUM(result_count),0)
		FROM daily_usage WHERE day>=? AND kind='tool' GROUP BY day ORDER BY day`, start)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []DailyUsage
	for rows.Next() {
		var d DailyUsage
		if err := rows.Scan(&d.Day, &d.Requests, &d.Successes, &d.Failures, &d.CacheHits, &d.DurationMS, &d.ResultCount); err != nil {
			return nil, err
		}
		d.Kind = "tool"
		out = append(out, d)
	}
	return out, rows.Err()
}

func (s *Store) health(kind string, now time.Time) ([]Health, error) {
	field := "provider"
	if kind == "tool" {
		field = "tool"
	}
	rows, err := s.db.Query(fmt.Sprintf(`SELECT %s FROM usage_events WHERE kind=? AND %s<>'' GROUP BY %s`, field, field, field), kind)
	if err != nil {
		return nil, err
	}
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			rows.Close()
			return nil, err
		}
		names = append(names, name)
	}
	rows.Close()
	var out []Health
	today, err := s.todayByName(kind, now)
	if err != nil {
		return nil, err
	}
	for _, name := range names {
		query := fmt.Sprintf(`SELECT occurred_at,success,error_summary,duration_ms,error_kind FROM usage_events WHERE kind=? AND %s=? ORDER BY occurred_at DESC,id DESC LIMIT 20`, field)
		r, err := s.db.Query(query, kind, name)
		if err != nil {
			return nil, err
		}
		h := Health{Name: name, Kind: kind, Status: "unknown", Today: today[name]}
		var latestSuccess bool
		var lastTS int64
		consecutiveFailures := 0
		countingConsecutive := true
		var newestFirst []bool
		var durations []int64
		errorKinds := map[string]int{}
		var lastFailureTS int64
		lastFailureKind := ""
		for r.Next() {
			var ts int64
			var ok int
			var msg, kind string
			var durationMS int64
			if err := r.Scan(&ts, &ok, &msg, &durationMS, &kind); err != nil {
				r.Close()
				return nil, err
			}
			h.SampleSize++
			durations = append(durations, durationMS)
			if ok == 0 && kind != "" {
				errorKinds[kind]++
			}
			newestFirst = append(newestFirst, ok == 1)
			if h.SampleSize == 1 {
				lastTS, latestSuccess, h.LastError = ts, ok == 1, msg
				h.LastSeenAt = time.Unix(ts, 0).In(s.location).Format(time.RFC3339)
			}
			if ok == 1 && h.LastSuccessAt == "" {
				h.LastSuccessAt = time.Unix(ts, 0).In(s.location).Format(time.RFC3339)
				countingConsecutive = false
			}
			if ok == 0 {
				if lastFailureKind == "" {
					lastFailureTS, lastFailureKind = ts, kind
				}
				if countingConsecutive {
					consecutiveFailures++
				}
				h.FailureRate++
				if h.LastFailureAt == "" {
					h.LastFailureAt = time.Unix(ts, 0).In(s.location).Format(time.RFC3339)
				}
			}
		}
		r.Close()
		h.ConsecutiveFailures = consecutiveFailures
		h.P95MS = calculateP95(durations)
		for kind, count := range errorKinds {
			h.ErrorKinds = append(h.ErrorKinds, ErrorKindCount{Kind: kind, Count: count})
		}
		sort.Slice(h.ErrorKinds, func(i, j int) bool {
			if h.ErrorKinds[i].Count != h.ErrorKinds[j].Count {
				return h.ErrorKinds[i].Count > h.ErrorKinds[j].Count
			}
			return h.ErrorKinds[i].Kind < h.ErrorKinds[j].Kind
		})
		for i := len(newestFirst) - 1; i >= 0; i-- {
			h.RecentOutcomes = append(h.RecentOutcomes, newestFirst[i])
		}
		if h.SampleSize > 0 {
			h.FailureRate /= float64(h.SampleSize)
			age := now.Sub(time.Unix(lastTS, 0))
			suspension := EvaluateSuspension(CurrentSuspensionPolicy(), consecutiveFailures, lastFailureKind, time.Unix(lastFailureTS, 0), now)
			if suspension.Suspended {
				h.State = "suspended"
				h.SuspendedUntil = suspension.Until.In(s.location).Format(time.RFC3339)
				h.SuspendReason = suspension.ErrorKind
				h.SuspendCountdown = int64(time.Until(suspension.Until).Seconds())
			}
			// Confidence tracks evidence strength so a single failure is never
			// read as a broken source; it is exposed separately from status.
			if h.SampleSize < minSampleSize {
				h.Confidence = "insufficient"
			} else {
				h.Confidence = "ok"
			}
			switch {
			case age > 24*time.Hour:
				h.Status = "unknown"
				h.State = "stale"
			case consecutiveFailures >= 3:
				// Repeated failures are meaningful even in a small window.
				h.Status = "down"
				if h.State == "" {
					h.State = "suspended"
				}
			case !latestSuccess || h.FailureRate > 0.20:
				h.Status = "degraded"
				h.State = "degraded"
			default:
				h.Status = "healthy"
				h.State = "healthy"
			}
		}
		out = append(out, h)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// minSampleSize keeps one-off failures from being read as a broken source.
const minSampleSize = 5

// calculateP95 returns the nearest-rank 95th percentile of durations in
// milliseconds. Fewer than 20 observations fall back to the maximum.
func calculateP95(values []int64) int64 {
	if len(values) == 0 {
		return 0
	}
	sorted := append([]int64(nil), values...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	idx := (len(sorted)*95 + 99) / 100
	if idx < 1 {
		idx = 1
	}
	if idx > len(sorted) {
		idx = len(sorted)
	}
	return sorted[idx-1]
}

func (s *Store) todayByName(kind string, now time.Time) (map[string]DailyUsage, error) {
	field := "provider"
	if kind == "tool" {
		field = "tool"
	}
	day := now.In(s.location).Format("2006-01-02")
	query := fmt.Sprintf(`SELECT %s,requests,successes,failures,cache_hits,duration_ms,result_count
		FROM daily_usage WHERE day=? AND kind=? AND %s<>''`, field, field)
	rows, err := s.db.Query(query, day, kind)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]DailyUsage{}
	for rows.Next() {
		var name string
		var d DailyUsage
		if err := rows.Scan(&name, &d.Requests, &d.Successes, &d.Failures, &d.CacheHits, &d.DurationMS, &d.ResultCount); err != nil {
			return nil, err
		}
		d.Day, d.Kind = day, kind
		if kind == "provider" {
			d.Provider = name
		} else {
			d.Tool = name
		}
		out[name] = d
	}
	return out, rows.Err()
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

var (
	reURL    = regexp.MustCompile(`(?i)https?://\S+`)
	reEmail  = regexp.MustCompile(`(?i)\b[\w.+-]+@[\w.-]+\.[a-z]{2,}\b`)
	reIP     = regexp.MustCompile(`\b(?:\d{1,3}\.){3}\d{1,3}\b`)
	reLong   = regexp.MustCompile(`\b\d{5,}\b`)
	reSecret = regexp.MustCompile(`(?i)\b(?:sk|pk|api|key|token|bearer)[-_]?[a-z0-9_-]{8,}\b`)
)

func summarizeQuery(q string) (string, int, string, string, string) {
	q = strings.TrimSpace(q)
	if q == "" {
		return "", 0, "", "", ""
	}
	sum := sha256.Sum256([]byte(q))
	hash := hex.EncodeToString(sum[:8])
	chars := len([]rune(q))
	clean := reURL.ReplaceAllString(q, "[url]")
	clean = reEmail.ReplaceAllString(clean, "[email]")
	clean = reIP.ReplaceAllString(clean, "[ip]")
	clean = reLong.ReplaceAllString(clean, "[number]")
	clean = reSecret.ReplaceAllString(clean, "[secret]")
	lang := detectLanguage(clean)
	topic := classifyTopic(strings.ToLower(clean))
	keywords := safeKeywords(clean)
	return hash, chars, lang, topic, keywords
}

func detectLanguage(s string) string {
	var han, latin int
	for _, r := range s {
		if unicode.Is(unicode.Han, r) {
			han++
		}
		if unicode.Is(unicode.Latin, r) {
			latin++
		}
	}
	switch {
	case han > 0 && latin > 0:
		return "mixed"
	case han > 0:
		return "zh"
	case latin > 0:
		return "en"
	default:
		return "other"
	}
}

func classifyTopic(s string) string {
	topics := []struct {
		name  string
		words []string
	}{
		{"软件与开发", []string{"api", "github", "代码", "编程", "docker", "go ", "python", "javascript"}},
		{"学术研究", []string{"论文", "研究", "paper", "doi", "arxiv", "journal"}},
		{"财经商业", []string{"股票", "公司", "市场", "价格", "finance", "revenue", "stock"}},
		{"新闻时事", []string{"新闻", "最新", "今日", "news", "latest"}},
		{"产品与采购", []string{"购买", "推荐", "产品", "评测", "price", "review"}},
	}
	for _, t := range topics {
		for _, w := range t.words {
			if strings.Contains(s, w) {
				return t.name
			}
		}
	}
	return "通用检索"
}

func safeKeywords(s string) string {
	fields := strings.FieldsFunc(s, func(r rune) bool { return !(unicode.IsLetter(r) || unicode.IsNumber(r) || unicode.Is(unicode.Han, r)) })
	stop := map[string]bool{"the": true, "and": true, "for": true, "with": true, "what": true, "how": true, "一个": true, "怎么": true, "什么": true, "是否": true}
	var out []string
	seen := map[string]bool{}
	for _, f := range fields {
		f = strings.TrimSpace(f)
		if len([]rune(f)) < 2 || len([]rune(f)) > 18 || stop[strings.ToLower(f)] || strings.Contains(f, "[") || seen[f] {
			continue
		}
		seen[f] = true
		out = append(out, f)
		if len(out) == 3 {
			break
		}
	}
	return strings.Join(out, " · ")
}

func summarizeError(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	msg = reURL.ReplaceAllString(msg, "[url]")
	msg = reEmail.ReplaceAllString(msg, "[email]")
	msg = reSecret.ReplaceAllString(msg, "[secret]")
	msg = strings.TrimSpace(msg)
	if len([]rune(msg)) > 220 {
		msg = string([]rune(msg)[:220]) + "…"
	}
	return msg
}

func IsUnavailable(err error) bool { return errors.Is(err, sql.ErrConnDone) }
