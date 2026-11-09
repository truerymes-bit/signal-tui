package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

func TestEventStreamSingleConnection(t *testing.T) {
	var conns int32
	done := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&conns, 1)
		w.Header().Set("Content-Type", "text/event-stream")
		fl, _ := w.(http.Flusher)
		for i := 0; i < 3; i++ {
			fmt.Fprintf(w, "data: {\"envelope\":{\"source\":\"s\",\"timestamp\":%d,\"dataMessage\":{\"message\":\"m%d\"}}}\n\n", i+1, i+1)
			if fl != nil {
				fl.Flush()
			}
		}
		<-done
	}))
	defer srv.Close()
	defer func() { close(done) }()

	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil {
		t.Fatal(err)
	}

	ch := make(chan tea.Msg, 10)
	eventDeliver = func(m tea.Msg) { ch <- m }
	defer func() { eventDeliver = nil }()

	cmd := listenEvents(port, "acct")
	go func() { _ = cmd() }()

	var msgs []tea.Msg
	for i := 0; i < 3; i++ {
		select {
		case m := <-ch:
			msgs = append(msgs, m)
		case <-time.After(2 * time.Second):
			t.Fatal("timeout waiting for event")
		}
	}

	if len(msgs) != 3 {
		t.Fatalf("got %d events, want 3", len(msgs))
	}
	for i, m := range msgs {
		mr, ok := m.(messageReceivedMsg)
		if !ok {
			t.Fatalf("event %d not messageReceivedMsg", i)
		}
		if mr.msg.Text != fmt.Sprintf("m%d", i+1) {
			t.Errorf("event %d text = %q, want %q", i, mr.msg.Text, fmt.Sprintf("m%d", i+1))
		}
	}
	if got := atomic.LoadInt32(&conns); got != 1 {
		t.Errorf("connections = %d, want 1", got)
	}
}
