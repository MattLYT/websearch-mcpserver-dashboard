package mcpserver

import (
	"bufio"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

func TestPDFSOCKSProxyPinsProtocolTarget(t *testing.T) {
	for _, scheme := range []string{"socks5", "socks5h"} {
		t.Run(scheme, func(t *testing.T) {
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			observed := make(chan error, 1)
			go func() {
				observed <- inspectPDFSOCKSRequest(listener)
			}()
			proxyURL, _ := url.Parse(scheme + "://" + listener.Addr().String())
			client := &http.Client{Timeout: 5 * time.Second, Transport: pdfDownloadTransport{proxy: http.ProxyURL(proxyURL)}}
			req, _ := http.NewRequest(http.MethodGet, "http://93.184.216.34/a%2Fb.pdf?q=x%2Fy", nil)
			req.Host = "documents.example"
			response, err := client.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			response.Body.Close()
			if err := <-observed; err != nil {
				t.Fatal(err)
			}
		})
	}
}

func inspectPDFSOCKSRequest(listener net.Listener) error {
	conn, err := listener.Accept()
	if err != nil {
		return err
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(5 * time.Second))
	var greeting [2]byte
	if _, err := io.ReadFull(conn, greeting[:]); err != nil {
		return err
	}
	if greeting[0] != 5 || greeting[1] == 0 {
		return fmt.Errorf("invalid SOCKS greeting: %v", greeting)
	}
	methods := make([]byte, int(greeting[1]))
	if _, err := io.ReadFull(conn, methods); err != nil {
		return err
	}
	if _, err := conn.Write([]byte{5, 0}); err != nil {
		return err
	}
	var command [10]byte
	if _, err := io.ReadFull(conn, command[:]); err != nil {
		return err
	}
	want := [10]byte{5, 1, 0, 1, 93, 184, 216, 34, 0, 80}
	if command != want {
		return fmt.Errorf("SOCKS target must be checked IPv4, not remotely resolved hostname: got %v want %v", command, want)
	}
	if _, err := conn.Write([]byte{5, 0, 0, 1, 127, 0, 0, 1, 0, 80}); err != nil {
		return err
	}
	req, err := http.ReadRequest(bufio.NewReader(conn))
	if err != nil {
		return err
	}
	req.Body.Close()
	if req.Host != "documents.example" || req.RequestURI != "/a%2Fb.pdf?q=x%2Fy" {
		return fmt.Errorf("SOCKS changed origin Host or encoded path: host=%q uri=%q", req.Host, req.RequestURI)
	}
	_, err = io.WriteString(conn, "HTTP/1.1 200 OK\r\nContent-Length: 2\r\nConnection: close\r\n\r\nok")
	return err
}

func TestPDFHTTPSProxyRejectsUntrustedCertificate(t *testing.T) {
	proxy := httptest.NewUnstartedServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("request reached an HTTPS proxy with an untrusted certificate")
	}))
	proxy.Config.ErrorLog = log.New(io.Discard, "", 0)
	proxy.StartTLS()
	defer proxy.Close()
	proxyURL, _ := url.Parse(proxy.URL)
	client := &http.Client{Timeout: 5 * time.Second, Transport: pdfDownloadTransport{proxy: http.ProxyURL(proxyURL)}}
	_, err := client.Get("http://93.184.216.34/test.pdf")
	var untrusted x509.UnknownAuthorityError
	if !errors.As(err, &untrusted) {
		t.Fatalf("HTTPS proxy certificate must be verified: %v", err)
	}
}
