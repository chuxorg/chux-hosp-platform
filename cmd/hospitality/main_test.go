package main

import (
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestVersionWithoutConfiguration(t *testing.T) {
	var out bytes.Buffer
	if err := run(context.Background(), []string{"--version"}, func(string) string { return "" }, &out); err != nil {
		t.Fatal(err)
	}
	if out.String() != "hospitality dev commit=unknown built=unknown\n" {
		t.Fatalf("unexpected version %q", out.String())
	}
}

func TestInvalidCommandsAndRollbackOptIn(t *testing.T) {
	for _, args := range [][]string{{"unknown"}, {"--version", "extra"}, {"migrate"}, {"migrate", "down"}, {"migrate", "up", "extra"}} {
		if err := run(context.Background(), args, func(string) string { return "" }, io.Discard); err == nil || !strings.Contains(err.Error(), "usage") {
			t.Fatalf("invalid command accepted: %v", args)
		}
	}
}

func TestMissingConfiguration(t *testing.T) {
	if err := run(context.Background(), nil, func(string) string { return "" }, io.Discard); err == nil {
		t.Fatal("started without DATABASE_URL")
	}
}

func TestGracefulShutdownDrainsRequest(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	finish := func() { once.Do(func() { close(release) }) }
	defer finish()
	served := make(chan error, 1)
	go func() {
		served <- serve(ctx, listener, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			close(entered)
			<-release
			_, _ = w.Write([]byte("finished"))
		}))
	}()
	request := make(chan error, 1)
	go func() {
		client := &http.Client{Timeout: 5 * time.Second}
		defer client.CloseIdleConnections()
		response, err := client.Get("http://" + listener.Addr().String())
		if err != nil {
			request <- err
			return
		}
		defer response.Body.Close()
		data, err := io.ReadAll(response.Body)
		if err == nil && string(data) != "finished" {
			err = io.ErrUnexpectedEOF
		}
		request <- err
	}()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("request never started")
	}
	cancel()
	finish()
	select {
	case err := <-request:
		if err != nil {
			t.Fatalf("in-flight request interrupted: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("request did not complete")
	}
	select {
	case err := <-served:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("shutdown did not complete")
	}
	connection, err := net.DialTimeout("tcp", listener.Addr().String(), 100*time.Millisecond)
	if err == nil {
		connection.Close()
		t.Fatal("listener remains open after shutdown")
	}
}
