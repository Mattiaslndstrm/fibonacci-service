package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	// patience is the longest a test waits for the service.
	patience = 10 * time.Second

	addrFlag = "-addr"
)

func TestRunServesUntilSignalled(t *testing.T) {
	for _, sig := range []syscall.Signal{syscall.SIGTERM, syscall.SIGINT} {
		t.Run(sig.String(), func(t *testing.T) {
			svc := start(t.Context(), t)

			ctx, cancel := context.WithTimeout(t.Context(), patience)
			defer cancel()

			req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+svc.addr+"/fibonacci?n=5", http.NoBody)
			require.NoError(t, err)

			resp, err := http.DefaultClient.Do(req)
			require.NoError(t, err)

			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			require.NoError(t, resp.Body.Close())
			assert.Equal(t, "[0,1,1,2,3]", string(body))
			assert.Equal(t, "request", svc.nextLine(t)["msg"])

			require.NoError(t, syscall.Kill(syscall.Getpid(), sig))

			assert.Equal(t, "shutting down", svc.nextLine(t)["msg"])
			assert.Equal(t, 0, svc.exitStatus(t))
		})
	}
}

func TestRunRejectsBadCommandLines(t *testing.T) {
	tests := []struct {
		name       string
		wantStderr string
		args       []string
		want       int
	}{
		{name: "an extra argument", args: []string{"foo"}, want: 2, wantStderr: "unexpected argument: foo"},
		{name: "an unknown flag", args: []string{"-nope"}, want: 2, wantStderr: "flag provided but not defined: -nope"},
		{name: "a request for help", args: []string{"-h"}, want: 0, wantStderr: `(default ":8080")`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer

			ctx, cancel := context.WithTimeout(t.Context(), patience)
			defer cancel()

			got := run(ctx, tt.args, &stdout, &stderr)

			assert.Equal(t, tt.want, got)
			assert.Contains(t, stderr.String(), tt.wantStderr)
			assert.Contains(t, stderr.String(), "Usage of fibonacci-service:")
			assert.Empty(t, stdout.String())
		})
	}
}

func TestRunFailsWhenTheAddressIsTaken(t *testing.T) {
	var listenConfig net.ListenConfig

	taken, err := listenConfig.Listen(t.Context(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)

	defer func() { assert.NoError(t, taken.Close()) }()

	var stdout bytes.Buffer

	ctx, cancel := context.WithTimeout(t.Context(), patience)
	defer cancel()

	got := run(ctx, []string{addrFlag, taken.Addr().String()}, &stdout, io.Discard)

	assert.Equal(t, 1, got)

	var line map[string]any
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &line))
	assert.Equal(t, "ERROR", line["level"])
	assert.Equal(t, "server failed", line["msg"])
	assert.NotEmpty(t, line["error"])
}

func TestRunExitsCleanlyWhenStoppedBeforeItListens(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	var stdout bytes.Buffer

	done := make(chan int, 1)

	go func() { done <- run(ctx, []string{addrFlag, "localhost:0"}, &stdout, io.Discard) }()

	assert.Equal(t, 0, service{done: done}.exitStatus(t))
	assert.Contains(t, stdout.String(), `"msg":"listening"`)
	assert.Contains(t, stdout.String(), `"msg":"shutting down"`)
}

func TestRequestFarAboveMaxHeaderBytesGets431(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	svc := start(ctx, t)

	var dialer net.Dialer

	conn, err := dialer.DialContext(ctx, "tcp", svc.addr)
	require.NoError(t, err)

	defer func() { assert.NoError(t, conn.Close()) }()

	go func() {
		request := "GET /fibonacci?n=" + strings.Repeat("9", 64<<10) + " HTTP/1.1\r\nHost: test\r\n\r\n"
		if _, writeErr := io.WriteString(conn, request); writeErr != nil {
			// The service may answer and close before it has read everything.
			return
		}
	}()

	require.NoError(t, conn.SetReadDeadline(time.Now().Add(patience)))

	statusLine, err := bufio.NewReader(conn).ReadString('\n')
	require.NoError(t, err)
	assert.Contains(t, statusLine, "431")

	cancel()
	assert.Equal(t, 0, svc.exitStatus(t))
}

type service struct {
	lines <-chan map[string]any
	done  <-chan int
	addr  string
}

func start(ctx context.Context, t *testing.T) service {
	t.Helper()

	reader, writer := io.Pipe()
	lines := make(chan map[string]any, 100)
	done := make(chan int, 1)

	go func() {
		done <- run(ctx, []string{addrFlag, "127.0.0.1:0"}, writer, io.Discard)

		assert.NoError(t, writer.Close())
	}()

	go func() {
		defer close(lines)

		scanner := bufio.NewScanner(reader)
		for scanner.Scan() {
			var line map[string]any
			if assert.NoError(t, json.Unmarshal(scanner.Bytes(), &line)) {
				lines <- line
			}
		}
	}()

	svc := service{lines: lines, done: done}
	listening := svc.nextLine(t)
	require.Equal(t, "listening", listening["msg"])

	addr, ok := listening["addr"].(string)
	require.True(t, ok, "addr is a string")

	svc.addr = addr

	return svc
}

func (svc service) nextLine(t *testing.T) map[string]any {
	t.Helper()

	select {
	case line := <-svc.lines:
		return line
	case <-time.After(patience):
		require.FailNow(t, "no log line")

		return nil
	}
}

func (svc service) exitStatus(t *testing.T) int {
	t.Helper()

	select {
	case status := <-svc.done:
		return status
	case <-time.After(patience):
		require.FailNow(t, "the service did not exit")

		return -1
	}
}
