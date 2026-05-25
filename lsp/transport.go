package lsp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
)

// transport reads and writes LSP JSON-RPC messages framed by Content-Length.
type transport struct {
	r *bufio.Reader
	w io.Writer
	m sync.Mutex // serializes writes
}

func newTransport(r io.Reader, w io.Writer) *transport {
	return &transport{r: bufio.NewReader(r), w: w}
}

// readMessage reads a single JSON-RPC message from the input. Returns the
// raw payload bytes or io.EOF when the stream ends.
func (t *transport) readMessage() ([]byte, error) {
	var contentLen int
	var sawHeader bool
	for {
		line, err := t.r.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			if !sawHeader {
				continue // tolerate stray blank lines before any header
			}
			break // end of headers
		}
		sawHeader = true
		if strings.HasPrefix(strings.ToLower(line), "content-length:") {
			v := strings.TrimSpace(line[len("Content-Length:"):])
			n, err := strconv.Atoi(v)
			if err != nil {
				return nil, fmt.Errorf("invalid Content-Length: %v", err)
			}
			contentLen = n
		}
	}
	if contentLen == 0 {
		return nil, fmt.Errorf("missing Content-Length header")
	}
	buf := make([]byte, contentLen)
	if _, err := io.ReadFull(t.r, buf); err != nil {
		return nil, err
	}
	return buf, nil
}

// writeMessage marshals v and writes it with the LSP framing.
func (t *transport) writeMessage(v interface{}) error {
	t.m.Lock()
	defer t.m.Unlock()
	payload, err := json.Marshal(v)
	if err != nil {
		return err
	}
	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(payload))
	if _, err := t.w.Write([]byte(header)); err != nil {
		return err
	}
	_, err = t.w.Write(payload)
	return err
}
