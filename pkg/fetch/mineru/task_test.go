package mineru

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func taskTestZIP(t *testing.T) []byte {
	t.Helper()
	var buffer bytes.Buffer
	w := zip.NewWriter(&buffer)
	f, err := w.Create("full.md")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write([]byte("# Original selected pages")); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func TestResumeTaskUsesOnlyGETAndPreservesResult(t *testing.T) {
	for _, kind := range []string{"standard", "batch", "agent"} {
		t.Run(kind, func(t *testing.T) {
			archive := taskTestZIP(t)
			var server *httptest.Server
			queries := 0
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet {
					t.Errorf("resume submitted %s", r.Method)
				}
				if r.URL.Path == "/result" {
					if kind == "agent" {
						_, _ = w.Write([]byte("# Original selected pages"))
					} else {
						_, _ = w.Write(archive)
					}
					return
				}
				queries++
				route := "/api/v4/extract/task/job-1"
				if kind == "batch" {
					route = "/api/v4/extract-results/batch/job-1"
				}
				if kind == "agent" {
					route = "/api/v1/agent/parse/job-1"
				}
				if r.URL.Path != route {
					t.Errorf("wrong route: %s", r.URL.Path)
				}
				if kind != "agent" && r.Header.Get("Authorization") != "Bearer test-token" {
					t.Error("missing authorization")
				}
				if kind == "agent" && r.Header.Get("Authorization") != "" {
					t.Error("token sent to Agent API")
				}
				w.Header().Set("Content-Type", "application/json")
				data := fmt.Sprintf("{\"state\":\"done\",\"full_zip_url\":%q,\"markdown_url\":%q}", server.URL+"/result", server.URL+"/result")
				if kind == "batch" {
					data = "{\"extract_result\":[" + data + "]}"
				}
				fmt.Fprintf(w, "{\"code\":0,\"data\":%s}", data)
			}))
			defer server.Close()
			client := NewFromConfig("test-token", "", "", false, true, true, "", 0)
			client.endpoint = server.URL
			md, err := client.ResumeTask(context.Background(), kind+":job-1")
			if err != nil {
				t.Fatal(err)
			}
			if queries != 1 || !strings.Contains(md, "Original selected pages") {
				t.Fatalf("queries=%d md=%s", queries, md)
			}
		})
	}
}

func TestTaskPendingRetainsRefAndProgress(t *testing.T) {
	var queries atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		queries.Add(1)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, "{\"code\":0,\"data\":{\"state\":\"running\",\"extract_progress\":{\"extracted_pages\":2,\"total_pages\":8}}}")
	}))
	defer server.Close()
	client := NewFromConfig("test-token", "", "", false, true, true, "", 0)
	client.endpoint = server.URL
	for _, wait := range []time.Duration{0, 25 * time.Millisecond} {
		queries.Store(0)
		progress := ""
		ctx := WithTaskProgress(WithTaskWait(context.Background(), wait), func(s string) { progress = s })
		_, err := client.ResumeTask(ctx, "standard:job-1")
		var pending *TaskPendingError
		if !errors.As(err, &pending) || pending.TaskRef != "standard:job-1" ||
			!strings.Contains(pending.Status, "2/8") || !strings.Contains(progress, "2/8") {
			t.Fatalf("wait=%s error=%v progress=%s", wait, err, progress)
		}
		if queries.Load() != 1 {
			t.Fatalf("queries=%d", queries.Load())
		}
	}
}

func TestTaskCancellationInterruptsPollingSleep(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, "{\"code\":0,\"data\":{\"state\":\"pending\"}}")
	}))
	defer server.Close()
	client := NewFromConfig("test-token", "", "", false, true, true, "", 0)
	client.endpoint = server.URL
	ctx = WithTaskProgress(ctx, func(string) { time.AfterFunc(20*time.Millisecond, cancel) })
	start := time.Now()
	_, err := client.pollStandardTask(ctx, "job-1")
	if !errors.Is(err, context.Canceled) || time.Since(start) > time.Second {
		t.Fatalf("cancel latency=%s err=%v", time.Since(start), err)
	}
}

func TestNewTaskCanResumeWithoutResubmission(t *testing.T) {
	archive := taskTestZIP(t)
	var submits atomic.Int32
	var done atomic.Bool
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v4/extract/task":
			submits.Add(1)
			fmt.Fprint(w, "{\"code\":0,\"data\":{\"task_id\":\"job-1\"}}")
		case "/api/v4/extract/task/job-1":
			if done.Load() {
				fmt.Fprintf(w, "{\"code\":0,\"data\":{\"state\":\"done\",\"full_zip_url\":%q}}", server.URL+"/result")
			} else {
				fmt.Fprint(w, "{\"code\":0,\"data\":{\"state\":\"preprocessing\"}}")
			}
		case "/result":
			_, _ = w.Write(archive)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()
	client := NewFromConfig("test-token", "", "", false, true, true, "", 0)
	client.endpoint = server.URL
	_, err := client.ParseURLWithPages(WithTaskWait(context.Background(), 0), "https://example.com/file.pdf", []int{1, 2}, 0)
	var pending *TaskPendingError
	if !errors.As(err, &pending) || !strings.Contains(pending.Status, "preprocessing") {
		t.Fatalf("pending=%v", err)
	}
	done.Store(true)
	md, err := client.ResumeTask(context.Background(), pending.TaskRef)
	if err != nil || submits.Load() != 1 || !strings.Contains(md, "Original selected pages") {
		t.Fatalf("submits=%d md=%s err=%v", submits.Load(), md, err)
	}
}

func TestResumeWaitPollsOriginalTaskUntilDone(t *testing.T) {
	archive := taskTestZIP(t)
	var queries atomic.Int32
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("unexpected submit: %s", r.Method)
		}
		if r.URL.Path == "/result" {
			_, _ = w.Write(archive)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if queries.Add(1) == 1 {
			fmt.Fprint(w, "{\"code\":0,\"data\":{\"state\":\"pending\"}}")
			return
		}
		fmt.Fprintf(w, "{\"code\":0,\"data\":{\"state\":\"done\",\"full_zip_url\":%q}}", server.URL+"/result")
	}))
	defer server.Close()
	client := NewFromConfig("test-token", "", "", false, true, true, "", 0)
	client.endpoint = server.URL
	_, err := client.ResumeTask(WithTaskWait(context.Background(), 4*time.Second), "standard:job-1")
	if err != nil || queries.Load() != 2 {
		t.Fatalf("queries=%d err=%v", queries.Load(), err)
	}
}

func TestResumeTaskRejectsInvalidReferencesBeforeNetwork(t *testing.T) {
	client := NewFromConfig("test-token", "", "", false, true, true, "", 0)
	client.endpoint = "http://127.0.0.1:1"
	for _, ref := range []string{"job-1", "other:job", "standard:../secrets", "batch:job?x=1", "agent:", "standard:https://example.com"} {
		if _, err := client.ResumeTask(context.Background(), ref); err == nil || !strings.Contains(err.Error(), "格式无效") {
			t.Fatalf("%q: %v", ref, err)
		}
	}
	client.token = ""
	if _, err := client.ResumeTask(context.Background(), "batch:job-1"); err == nil || !strings.Contains(err.Error(), "Token") {
		t.Fatalf("missing token: %v", err)
	}
}

func TestTaskFailuresAreNotPending(t *testing.T) {
	for _, tc := range []struct {
		name, body, want string
		status           int
	}{
		{"failed", "{\"code\":0,\"data\":{\"state\":\"failed\",\"err_code\":-60018,\"err_msg\":\"quota\"}}", "每日解析额度", 200},
		{"unauthorized", "{}", "HTTP 401", 401},
		{"missing-state", "{\"code\":0,\"data\":{}}", "未返回任务状态", 200},
		{"message-only", "{\"code\":0,\"data\":{\"state\":\"failed\",\"err_msg\":\"upstream download failed\"}}", "upstream download failed", 200},
		{"string-code", "{\"code\":\"0\",\"data\":{\"state\":\"failed\",\"err_code\":\"-60018\"}}", "每日解析额度", 200},
		{"missing-result", "{\"code\":0,\"data\":{\"state\":\"done\"}}", "未返回结果链接", 200},
		{"rejected-url", "{\"code\":-60023,\"msg\":\"url rejected\"}", "远程 PDF URL", 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				fmt.Fprint(w, tc.body)
			}))
			defer server.Close()
			client := NewFromConfig("test-token", "", "", false, true, true, "", 0)
			client.endpoint = server.URL
			_, err := client.ResumeTask(context.Background(), "standard:job-1")
			var pending *TaskPendingError
			if err == nil || errors.As(err, &pending) || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err=%v", err)
			}
			if tc.name == "rejected-url" && !errors.Is(err, ErrRemoteURLRejected) {
				t.Fatalf("sentinel lost: %v", err)
			}
		})
	}
}
