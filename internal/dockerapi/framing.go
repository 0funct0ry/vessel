package dockerapi

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// StreamType identifies which container stream a LogLine came from.
type StreamType int

const (
	// StreamStdout marks a line read from the container's stdout.
	StreamStdout StreamType = 1
	// StreamStderr marks a line read from the container's stderr.
	StreamStderr StreamType = 2
)

// LogLine is one line of container output, demultiplexed and timestamped.
type LogLine struct {
	Time   time.Time
	Stream StreamType
	Text   string
}

// LogsOptions controls GET /containers/{id}/logs.
type LogsOptions struct {
	Follow     bool
	Tail       string // "all" or a count; empty means the daemon default
	Since      string
	Until      string
	Timestamps bool
	Stdout     bool
	Stderr     bool
}

// LogReader streams demultiplexed log lines from a container. Call Next in a
// loop until it returns io.EOF (or another error); Close releases the
// underlying HTTP response.
type LogReader struct {
	body io.ReadCloser
	br   *bufio.Reader
	tty  bool
}

// Close releases the underlying HTTP connection.
func (r *LogReader) Close() error {
	return r.body.Close()
}

// frameHeaderSize is Docker's multiplexed stream frame header: 1 byte stream
// type, 3 bytes padding, 4 bytes big-endian payload length.
const frameHeaderSize = 8

// Next reads and returns the next log line. For a multiplexed (non-TTY)
// stream it demultiplexes Docker's 8-byte-header framing; for a TTY stream
// it reads raw newline-delimited text as stdout. It returns io.EOF when the
// stream ends.
func (r *LogReader) Next() (LogLine, error) {
	if r.tty {
		return r.nextTTYLine()
	}
	return r.nextFramedLine()
}

func (r *LogReader) nextTTYLine() (LogLine, error) {
	line, err := r.br.ReadString('\n')
	if err != nil && line == "" {
		return LogLine{}, err
	}
	return LogLine{Stream: StreamStdout, Text: trimNewline(line)}, nil
}

func (r *LogReader) nextFramedLine() (LogLine, error) {
	var header [frameHeaderSize]byte
	if _, err := io.ReadFull(r.br, header[:]); err != nil {
		return LogLine{}, err
	}

	streamType := StreamType(header[0])
	size := binary.BigEndian.Uint32(header[4:8])

	payload := make([]byte, size)
	if _, err := io.ReadFull(r.br, payload); err != nil {
		return LogLine{}, fmt.Errorf("dockerapi: reading log frame payload: %w", err)
	}

	return LogLine{Stream: streamType, Text: trimNewline(string(payload))}, nil
}

func trimNewline(s string) string {
	return string(bytes.TrimRight([]byte(s), "\n"))
}

// LogStream calls GET /containers/{id}/logs and returns a LogReader over the
// response body. The caller must call Close when done (or on context
// cancellation, which aborts the underlying HTTP request).
func (c *Client) LogStream(ctx context.Context, id string, opts LogsOptions) (*LogReader, error) {
	q := url.Values{}
	if opts.Follow {
		q.Set("follow", "1")
	}
	if opts.Tail != "" {
		q.Set("tail", opts.Tail)
	}
	if opts.Since != "" {
		q.Set("since", opts.Since)
	}
	if opts.Until != "" {
		q.Set("until", opts.Until)
	}
	if opts.Timestamps {
		q.Set("timestamps", "1")
	}
	if opts.Stdout {
		q.Set("stdout", "1")
	}
	if opts.Stderr {
		q.Set("stderr", "1")
	}

	path := "/containers/" + url.PathEscape(id) + "/logs?" + q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url(path), nil)
	if err != nil {
		return nil, fmt.Errorf("dockerapi: building logs request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnreachable, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, mapError(resp)
	}

	// Docker signals a TTY-attached container's raw (non-multiplexed) stream
	// via Content-Type: application/vnd.docker.raw-stream; anything else
	// (application/vnd.docker.multiplexed-stream, or absent) is framed.
	tty := resp.Header.Get("Content-Type") == "application/vnd.docker.raw-stream"

	return &LogReader{body: resp.Body, br: bufio.NewReader(resp.Body), tty: tty}, nil
}
