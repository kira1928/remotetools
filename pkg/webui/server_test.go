package webui

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

type deadlineRecorder struct {
	*httptest.ResponseRecorder
	deadlineSet chan time.Time
	once        sync.Once
}

func (r *deadlineRecorder) SetWriteDeadline(deadline time.Time) error {
	r.once.Do(func() { r.deadlineSet <- deadline })
	return nil
}

func TestSSEClearsWriteDeadline(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req := httptest.NewRequest(http.MethodGet, "/api/progress", nil).WithContext(ctx)
	recorder := &deadlineRecorder{
		ResponseRecorder: httptest.NewRecorder(),
		deadlineSet:      make(chan time.Time, 1),
	}

	done := make(chan struct{})
	go func() {
		handleSSE(recorder, req)
		close(done)
	}()

	select {
	case deadline := <-recorder.deadlineSet:
		if !deadline.IsZero() {
			t.Fatalf("SSE 写截止时间 = %s, want zero", deadline)
		}
	case <-time.After(time.Second):
		t.Fatal("SSE handler 未清除写截止时间")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("SSE handler 未在请求取消后退出")
	}
}

func TestSSEFlushesHeadersImmediately(t *testing.T) {
	server := NewWebUIServer()
	if err := server.Start(0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = server.Stop() })

	ctx, cancel := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://127.0.0.1:"+portString(server.GetPort())+"/api/progress", nil)
	if err != nil {
		t.Fatal(err)
	}

	client := &http.Client{Timeout: time.Second}
	resp, err := client.Do(req)
	if err != nil {
		cancel()
		t.Fatalf("SSE 响应头未及时返回: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		cancel()
		_ = resp.Body.Close()
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if got := resp.Header.Get("Content-Type"); got != "text/event-stream" {
		cancel()
		_ = resp.Body.Close()
		t.Fatalf("Content-Type = %q, want text/event-stream", got)
	}

	cancel()
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
}

func portString(port int) string {
	return fmt.Sprintf("%d", port)
}
