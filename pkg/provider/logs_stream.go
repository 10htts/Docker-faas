package provider

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/pkg/stdcopy"
)

// LogEntry is a single, timestamped log line from one function container
// instance. The gateway maps entries onto the pinned faas-provider log
// Message shape (github.com/openfaas/faas-provider v0.25.12 logs/logs.go:
// {name, namespace, instance, timestamp, text}).
type LogEntry struct {
	Name      string
	Instance  string
	Timestamp time.Time
	Text      string
}

// StreamFunctionLogs streams log lines from every container of a function,
// fanned into a single channel. The channel is closed when all container
// streams end or ctx is cancelled.
//
//   - since: only lines at/after this time (zero means from the beginning).
//   - tail:  max lines per container from the end (<= 0 means all).
//   - follow: keep the stream open and forward new lines as they appear.
//
// Instance is the container name, mirroring how the pinned providers report
// the log source. Docker multiplexed streams (non-TTY containers) are
// demultiplexed with stdcopy; timestamps are parsed from Docker's
// RFC3339Nano prefixes (Timestamps: true).
func (p *DockerProvider) StreamFunctionLogs(ctx context.Context, functionName string, since time.Time, tail int, follow bool) (<-chan LogEntry, error) {
	summaries, err := p.listFunctionContainers(ctx, functionName)
	if err != nil {
		return nil, fmt.Errorf("failed to list containers: %w", err)
	}

	options := container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Timestamps: true,
		Follow:     follow,
	}
	if tail > 0 {
		options.Tail = strconv.Itoa(tail)
	}
	if !since.IsZero() {
		options.Since = since.UTC().Format(time.RFC3339Nano)
	}

	out := make(chan LogEntry, 64)
	var wg sync.WaitGroup
	for _, summary := range summaries {
		instance := containerSummaryName(summary)

		tty := false
		if inspect, err := p.client.ContainerInspect(ctx, summary.ID); err == nil && inspect.Config != nil {
			tty = inspect.Config.Tty
		}

		reader, err := p.client.ContainerLogs(ctx, summary.ID, options)
		if err != nil {
			p.logger.Warnf("Failed to open log stream for container %s: %v", instance, err)
			continue
		}

		wg.Add(1)
		go func(reader io.ReadCloser, instance string, tty bool) {
			defer wg.Done()
			defer reader.Close()
			p.forwardContainerLogs(ctx, out, functionName, instance, reader, tty)
		}(reader, instance, tty)
	}

	go func() {
		wg.Wait()
		close(out)
	}()

	return out, nil
}

// forwardContainerLogs demultiplexes (when needed), splits, parses and
// forwards one container's log stream onto out until the stream ends or ctx
// is cancelled. Cancelling ctx also tears down the underlying HTTP stream, so
// follow-mode readers unblock.
func (p *DockerProvider) forwardContainerLogs(ctx context.Context, out chan<- LogEntry, functionName, instance string, stream io.Reader, tty bool) {
	lineReader := stream
	if !tty {
		// Docker multiplexes stdout/stderr frames for non-TTY containers;
		// demultiplex with stdcopy into a pipe we can scan line by line.
		pr, pw := io.Pipe()
		// Close the read end on EVERY exit path (normal end AND ctx cancel /
		// client disconnect). Without this, a StdCopy goroutine parked in
		// pw.Write — the steady state once `out` backpressures on a chatty
		// function — would block forever when the scan loop returns, leaking a
		// goroutine + pipe per disconnect (closing the Docker stream does not
		// unblock a pending pipe WRITE). Closing pr makes that write return
		// ErrClosedPipe so StdCopy exits.
		defer pr.Close()
		go func() {
			_, err := stdcopy.StdCopy(pw, pw, stream)
			pw.CloseWithError(err)
		}()
		lineReader = pr
	}

	scanner := bufio.NewScanner(lineReader)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		ts, text := ParseDockerLogLine(scanner.Text())
		select {
		case out <- LogEntry{Name: functionName, Instance: instance, Timestamp: ts, Text: text}:
		case <-ctx.Done():
			return
		}
	}
	if err := scanner.Err(); err != nil && ctx.Err() == nil {
		p.logger.Debugf("Log stream for container %s ended: %v", instance, err)
	}
}

// ParseDockerLogLine splits a Docker log line produced with Timestamps: true
// into its leading RFC3339Nano timestamp and the remaining text. Lines
// without a parsable timestamp are returned verbatim with a zero timestamp so
// callers can substitute a best-effort time.
func ParseDockerLogLine(line string) (time.Time, string) {
	if sep := strings.IndexByte(line, ' '); sep > 0 {
		if ts, err := time.Parse(time.RFC3339Nano, line[:sep]); err == nil {
			return ts, line[sep+1:]
		}
	}
	return time.Time{}, line
}
