package services

import (
	"net"
	"testing"
	"time"
)

func TestProxyStatusClassification(t *testing.T) {
	cases := []struct {
		status int
		ok     bool
	}{
		{status: 200, ok: true},
		{status: 204, ok: true},
		{status: 302, ok: true},
		{status: 400, ok: false},
		{status: 403, ok: false},
		{status: 500, ok: false},
	}

	for _, tc := range cases {
		got := tc.status >= 200 && tc.status < 400
		if got != tc.ok {
			t.Fatalf("status %d => %v, want %v", tc.status, got, tc.ok)
		}
	}
}

func TestProxyUsesProvidedTimeout(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		time.Sleep(5 * time.Second)
	}()

	started := time.Now()
	result := TestProxy("http://"+ln.Addr().String(), time.Second)
	elapsed := time.Since(started)

	<-done

	if result.OK {
		t.Fatalf("expected timeout failure, got success: %+v", result)
	}
	if result.Status != 0 {
		t.Fatalf("status = %d, want 0", result.Status)
	}
	if result.Error == nil {
		t.Fatalf("expected timeout error, got nil")
	}
	if elapsed >= 5*time.Second {
		t.Fatalf("elapsed = %v, want less than 5s", elapsed)
	}
}
