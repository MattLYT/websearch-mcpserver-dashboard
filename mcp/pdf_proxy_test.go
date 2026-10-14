package mcpserver

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
	"websearch/pkg/config"
	"websearch/pkg/fetch/webfetch"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func usePDFTestProxy(t *testing.T, endpoint string) {
	t.Helper()
	restoreGlobals(t)
	webfetchLazyCfg = &config.Config{Proxy: config.ProxyConfig{Enabled: true, Endpoint: endpoint}}
}

func syntheticProxyPDF() []byte {
	return syntheticProxyPDFPages(1)
}

func syntheticProxyPDFPages(pages int) []byte {
	stream := "BT /F1 12 Tf (Proxy PDF text) Tj ET"
	objects := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 4 0 R >> >> /Contents 5 0 R >>",
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
		fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(stream), stream),
	}
	kids := []string{"3 0 R"}
	for page := 1; page < pages; page++ {
		kids = append(kids, fmt.Sprintf("%d 0 R", len(objects)+1))
		objects = append(objects, objects[2])
	}
	objects[1] = fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", strings.Join(kids, " "), pages)
	var b bytes.Buffer
	b.WriteString("%PDF-1.4\n")
	offsets := []int{0}
	for i, object := range objects {
		offsets = append(offsets, b.Len())
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", i+1, object)
	}
	xref := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n0000000000 65535 f \n", len(offsets))
	for _, offset := range offsets[1:] {
		fmt.Fprintf(&b, "%010d 00000 n \n", offset)
	}
	fmt.Fprintf(&b, "trailer\n<< /Root 1 0 R /Size %d >>\nstartxref\n%d\n%%%%EOF\n", len(offsets), xref)
	return b.Bytes()
}

func TestPDFHandlerUsesConfiguredProxy(t *testing.T) {
	initTestLogger()
	var methods []string
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		methods = append(methods, r.Method+" "+r.URL.Path)
		if r.URL.Host != "93.184.216.34:80" {
			t.Errorf("unpinned proxy request: url=%s host=%s", r.URL, r.Host)
		}
		if r.URL.Path == "/start.pdf" {
			http.Redirect(w, r, "/final.pdf", http.StatusFound)
			return
		}
		w.Header().Set("Content-Type", "application/pdf")
		w.Write(syntheticProxyPDF())
	}))
	defer proxy.Close()
	usePDFTestProxy(t, proxy.URL)
	conf := config.CleanFetchConfig{FileOutputDir: t.TempDir(), MaxInlineLines: 100}
	fetcher, err := webfetch.NewFromConfig(conf, config.PDFParserConfig{}, proxy.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer fetcher.Close()
	webfetchInst = fetcher
	result, _, err := PDFParserHandler(context.Background(), nil, &PDFParserParams{Path: "http://93.184.216.34/start.pdf", Pages: "1"})
	if err != nil {
		t.Fatal(err)
	}
	text := result.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(text, "Proxy PDF text") || !strings.Contains(text, "pdf-lib") {
		t.Fatalf("proxy result=%s", text)
	}
	if strings.Join(methods, ",") != "HEAD /start.pdf,HEAD /final.pdf,GET /start.pdf,GET /final.pdf" {
		t.Fatalf("proxy did not handle preflight/download/relative redirects: %v", methods)
	}
}

func TestPDFForwardProxyPinsWireURIAndRetainsHost(t *testing.T) {
	for _, path := range []string{"", "/a%2Fb%20c.pdf?q=a%2Fb&x=1"} {
		t.Run(path, func(t *testing.T) {
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			wire := make(chan string, 1)
			go func() {
				conn, err := listener.Accept()
				if err != nil {
					return
				}
				defer conn.Close()
				conn.SetDeadline(time.Now().Add(5 * time.Second))
				reader := bufio.NewReader(conn)
				var request strings.Builder
				for {
					line, err := reader.ReadString('\n')
					if err != nil {
						wire <- err.Error()
						return
					}
					request.WriteString(line)
					if line == "\r\n" {
						break
					}
				}
				wire <- request.String()
				io.WriteString(conn, "HTTP/1.1 200 OK\r\nContent-Length: 2\r\nConnection: close\r\n\r\nok")
			}()
			proxyURL, _ := url.Parse("http://" + listener.Addr().String())
			client := &http.Client{Timeout: 5 * time.Second, Transport: pdfDownloadTransport{proxy: http.ProxyURL(proxyURL)}}
			req, _ := http.NewRequest(http.MethodGet, "http://93.184.216.34"+path, nil)
			req.Host = "documents.example"
			response, err := client.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			response.Body.Close()
			got := <-wire
			if path == "" {
				path = "/"
			}
			if !strings.HasPrefix(got, "GET http://93.184.216.34:80"+path+" HTTP/1.1\r\n") || !strings.Contains(got, "\r\nHost: documents.example\r\n") {
				t.Fatalf("proxy request must pin authority and retain origin Host: %q", got)
			}
		})
	}
}

func TestPDFProxyDownloadSafetyAndLimits(t *testing.T) {
	for _, test := range []struct {
		name, redirect string
		body           string
		wantError      bool
	}{
		{"limit", "", "123456", true},
		{"exact limit", "", "12345", false},
		{"private redirect", "http://127.0.0.1/private.pdf", "", true},
		{"metadata redirect", "http://169.254.169.254/latest/", "", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if test.redirect != "" {
					http.Redirect(w, r, test.redirect, http.StatusFound)
					return
				}
				w.Header().Set("Transfer-Encoding", "chunked")
				io.WriteString(w, test.body)
			}))
			defer proxy.Close()
			usePDFTestProxy(t, proxy.URL)
			path, cleanup, err := downloadPDFWithLimit(context.Background(), "http://93.184.216.34/test.pdf", 5)
			if test.wantError {
				if err == nil || path != "" || cleanup != nil {
					t.Fatalf("unsafe/oversized download accepted: path=%s err=%v", path, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			got, err := os.ReadFile(path)
			cleanup()
			if err != nil || string(got) != test.body {
				t.Fatalf("body=%q err=%v", got, err)
			}
		})
	}
	for _, target := range []string{"http://127.0.0.1/test.pdf", "http://[::1]/test.pdf"} {
		proxy := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Error("private target reached proxy") }))
		usePDFTestProxy(t, proxy.URL)
		proxyURL, _ := url.Parse(proxy.URL)
		req, _ := http.NewRequest(http.MethodGet, target, nil)
		_, err := (pdfDownloadTransport{proxy: http.ProxyURL(proxyURL)}).RoundTrip(req)
		proxy.Close()
		if err == nil {
			t.Fatal("private target accepted by proxy transport")
		}
	}
}

func TestPDFProxyCONNECTPinsTargetAndChecksCertificate(t *testing.T) {
	tlsTarget := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("untrusted TLS certificate accepted") }))
	defer tlsTarget.Close()
	connects := make(chan string, 1)
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodConnect {
			t.Errorf("expected CONNECT, got %s", r.Method)
			return
		}
		connects <- r.Host
		upstream, err := net.Dial("tcp", tlsTarget.Listener.Addr().String())
		if err != nil {
			t.Error(err)
			return
		}
		conn, _, err := w.(http.Hijacker).Hijack()
		if err != nil {
			upstream.Close()
			t.Error(err)
			return
		}
		defer conn.Close()
		defer upstream.Close()
		io.WriteString(conn, "HTTP/1.1 200 Connection Established\r\n\r\n")
		go io.Copy(upstream, conn)
		io.Copy(conn, upstream)
	}))
	defer proxy.Close()
	usePDFTestProxy(t, proxy.URL)
	_, _, err := downloadPDFWithLimit(context.Background(), "https://93.184.216.34/test.pdf", 100)
	if err == nil || !strings.Contains(err.Error(), "certificate") {
		t.Fatalf("certificate must be checked: %v", err)
	}
	select {
	case host := <-connects:
		if host != "93.184.216.34:443" {
			t.Fatalf("CONNECT target=%s", host)
		}
	case <-time.After(time.Second):
		t.Fatal("HTTPS bypassed configured proxy")
	}
}

func TestPDFDownloadEnvironmentProxy(t *testing.T) {
	if os.Getenv("WS_PDF_PROXY_CHILD") == "1" {
		initTestLogger()
		webfetchLazyCfg = &config.Config{CleanFetch: config.CleanFetchConfig{UseSystemProxy: true}}
		path, cleanup, err := downloadPDFWithLimit(context.Background(), "http://93.184.216.34/env.pdf", 10)
		if err != nil {
			t.Fatal(err)
		}
		defer cleanup()
		body, _ := os.ReadFile(path)
		if string(body) != "proxy" {
			t.Fatalf("body=%q", body)
		}
		return
	}
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "proxy") }))
	defer proxy.Close()
	cmd := exec.Command(os.Args[0], "-test.run=^TestPDFDownloadEnvironmentProxy$")
	for _, env := range os.Environ() {
		key := strings.ToUpper(strings.SplitN(env, "=", 2)[0])
		if key != "HTTP_PROXY" && key != "HTTPS_PROXY" && key != "ALL_PROXY" && key != "NO_PROXY" && key != "WS_PDF_PROXY_CHILD" {
			cmd.Env = append(cmd.Env, env)
		}
	}
	cmd.Env = append(cmd.Env, "WS_PDF_PROXY_CHILD=1", "HTTP_PROXY="+proxy.URL, "HTTPS_PROXY="+proxy.URL, "NO_PROXY=")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("environment proxy failed: %v\n%s", err, out)
	}
}

func TestNormalizePDFDownloadURL(t *testing.T) {
	for _, test := range []struct{ in, want string }{
		{"https://github.com/mozilla/pdf.js/blob/master/test/pdfs/tracemonkey.pdf", "https://raw.githubusercontent.com/mozilla/pdf.js/master/test/pdfs/tracemonkey.pdf"},
		{"https://github.com/u/r/blob/main/a%20b.pdf?download=1#page=2", "https://raw.githubusercontent.com/u/r/main/a%20b.pdf?download=1#page=2"},
		{"https://github.com/u/r/blob/feature%2Fbranch/a.pdf", "https://raw.githubusercontent.com/u/r/feature%2Fbranch/a.pdf"},
		{"https://github.com.evil.example/u/r/blob/main/a.pdf", ""},
		{"https://github.com@evil.example/u/r/blob/main/a.pdf", ""},
		{"https://github.com/u/r/blob/main", ""},
		{"https://example.com/github.com/u/r/blob/main/a.pdf", ""},
	} {
		if test.want == "" {
			test.want = test.in
		}
		if got, remote := resolvePDFPath(test.in); got != test.want || !remote {
			t.Errorf("resolvePDFPath(%q)=%s,%v want %s", test.in, got, remote, test.want)
		}
	}
}
