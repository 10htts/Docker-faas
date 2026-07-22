package provider

import (
	"context"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/docker/docker/pkg/stdcopy"
	"github.com/sirupsen/logrus"
)

func quietProvider() *DockerProvider {
	l := logrus.New()
	l.SetOutput(io.Discard)
	return &DockerProvider{logger: l, network: "testnet"}
}

// TestForwardContainerLogsDemuxesAndParses exercises the REAL stdcopy demux +
// timestamp-parse path (the logs handler tests use a fake streamer and never hit
// this). It builds a multiplexed Docker log stream and asserts the parsed
// LogEntry timestamps and text.
func TestForwardContainerLogsDemuxesAndParses(t *testing.T) {
	pr, pw := io.Pipe()
	// Write two stdout frames with RFC3339Nano timestamps (Timestamps: true).
	stdout := stdcopy.NewStdWriter(pw, stdcopy.Stdout)
	go func() {
		_, _ = stdout.Write([]byte("2026-07-21T17:00:00.5Z hello world\n"))
		_, _ = stdout.Write([]byte("2026-07-21T17:00:01Z second line\n"))
		pw.Close()
	}()

	out := make(chan LogEntry, 8)
	p := quietProvider()
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		p.forwardContainerLogs(context.Background(), out, "fn", "fn-0", pr, false)
		close(out)
	}()

	var got []LogEntry
	for e := range out {
		got = append(got, e)
	}
	wg.Wait()

	if len(got) != 2 {
		t.Fatalf("expected 2 log entries, got %d: %+v", len(got), got)
	}
	if got[0].Text != "hello world" || got[1].Text != "second line" {
		t.Fatalf("text mis-parsed: %q / %q", got[0].Text, got[1].Text)
	}
	if got[0].Instance != "fn-0" || got[0].Name != "fn" {
		t.Fatalf("name/instance not stamped: %+v", got[0])
	}
	want0 := time.Date(2026, 7, 21, 17, 0, 0, 500_000_000, time.UTC)
	if !got[0].Timestamp.Equal(want0) {
		t.Fatalf("timestamp mis-parsed: got %s want %s", got[0].Timestamp, want0)
	}
}

// TestForwardContainerLogsReturnsOnCancel proves the RT (log-leak) fix: with the
// out channel never drained (a disconnected client) and the source still open,
// cancelling ctx must make forwardContainerLogs return promptly rather than
// block forever. (The deferred pr.Close() additionally unblocks the internal
// stdcopy writer; here we assert the observable return.)
func TestForwardContainerLogsReturnsOnCancel(t *testing.T) {
	pr, pw := io.Pipe()
	stdout := stdcopy.NewStdWriter(pw, stdcopy.Stdout)
	// Keep producing so the scanner has a line to hand to the (undrained) out.
	go func() {
		for i := 0; i < 1000; i++ {
			if _, err := stdout.Write([]byte("2026-07-21T17:00:00Z spam\n")); err != nil {
				return
			}
		}
	}()

	out := make(chan LogEntry) // unbuffered, never read → backpressure
	p := quietProvider()
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		p.forwardContainerLogs(ctx, out, "fn", "fn-0", pr, false)
		close(done)
	}()

	cancel()
	select {
	case <-done:
		// returned promptly on cancel
	case <-time.After(5 * time.Second):
		t.Fatal("forwardContainerLogs did not return after ctx cancel (blocked forever)")
	}
	_ = pw // keep source open to prove the return is driven by ctx, not EOF
}
