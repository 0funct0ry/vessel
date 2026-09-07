package dockerapi

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"io"
	"testing"
)

// frame builds one multiplexed-stream frame: 1 byte stream type, 3 bytes
// padding, 4 bytes big-endian length, then the payload.
func frame(streamType byte, payload string) []byte {
	buf := make([]byte, frameHeaderSize+len(payload))
	buf[0] = streamType
	binary.BigEndian.PutUint32(buf[4:8], uint32(len(payload)))
	copy(buf[8:], payload)
	return buf
}

func newFramedReader(data []byte) *LogReader {
	return &LogReader{
		body: io.NopCloser(bytes.NewReader(data)),
		br:   bufio.NewReader(bytes.NewReader(data)),
		tty:  false,
	}
}

func TestLogReader_FramedStream(t *testing.T) {
	var buf bytes.Buffer
	buf.Write(frame(1, "hello stdout\n"))
	buf.Write(frame(2, "oops stderr\n"))

	r := newFramedReader(buf.Bytes())

	line, err := r.Next()
	if err != nil {
		t.Fatalf("first Next: %v", err)
	}
	if line.Stream != StreamStdout || line.Text != "hello stdout" {
		t.Fatalf("first line = %+v, want stdout %q", line, "hello stdout")
	}

	line, err = r.Next()
	if err != nil {
		t.Fatalf("second Next: %v", err)
	}
	if line.Stream != StreamStderr || line.Text != "oops stderr" {
		t.Fatalf("second line = %+v, want stderr %q", line, "oops stderr")
	}

	if _, err := r.Next(); err != io.EOF {
		t.Fatalf("third Next: got %v, want io.EOF", err)
	}
}

// splitReader dribbles out data n bytes at a time per Read call, so a frame
// header or payload can straddle two Read calls the way a real socket would.
type splitReader struct {
	data  []byte
	chunk int
}

func (s *splitReader) Read(p []byte) (int, error) {
	if len(s.data) == 0 {
		return 0, io.EOF
	}
	n := s.chunk
	if n > len(p) {
		n = len(p)
	}
	if n > len(s.data) {
		n = len(s.data)
	}
	copy(p, s.data[:n])
	s.data = s.data[n:]
	return n, nil
}

func TestLogReader_FrameSplitAcrossReads(t *testing.T) {
	payload := "line split across reads\n"
	data := frame(1, payload)

	// 3-byte chunks guarantee the 8-byte header itself is split across
	// multiple Read calls, and so is the payload.
	sr := &splitReader{data: data, chunk: 3}
	r := &LogReader{
		body: io.NopCloser(sr),
		br:   bufio.NewReader(sr),
	}

	line, err := r.Next()
	if err != nil {
		t.Fatalf("Next: %v", err)
	}
	if line.Stream != StreamStdout || line.Text != "line split across reads" {
		t.Fatalf("line = %+v, want stdout %q", line, "line split across reads")
	}

	if _, err := r.Next(); err != io.EOF {
		t.Fatalf("second Next: got %v, want io.EOF", err)
	}
}

func TestLogReader_TTYStream(t *testing.T) {
	data := []byte("raw line one\nraw line two\n")
	r := &LogReader{
		body: io.NopCloser(bytes.NewReader(data)),
		br:   bufio.NewReader(bytes.NewReader(data)),
		tty:  true,
	}

	line, err := r.Next()
	if err != nil {
		t.Fatalf("first Next: %v", err)
	}
	if line.Stream != StreamStdout || line.Text != "raw line one" {
		t.Fatalf("first line = %+v", line)
	}

	line, err = r.Next()
	if err != nil {
		t.Fatalf("second Next: %v", err)
	}
	if line.Text != "raw line two" {
		t.Fatalf("second line = %+v", line)
	}

	if _, err := r.Next(); err != io.EOF {
		t.Fatalf("third Next: got %v, want io.EOF", err)
	}
}
