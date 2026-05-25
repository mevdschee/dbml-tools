package lsp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
)

// fakePipe lets the test drive the server over a goroutine, treating one
// bytes.Buffer as input and capturing the other for output.
type fakePipe struct {
	mu sync.Mutex
	buf bytes.Buffer
	closed bool
}

func (p *fakePipe) Read(b []byte) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.buf.Len() == 0 {
		if p.closed {
			return 0, io.EOF
		}
		return 0, io.EOF
	}
	return p.buf.Read(b)
}

func (p *fakePipe) Write(b []byte) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.buf.Write(b)
}

func (p *fakePipe) Close() { p.mu.Lock(); p.closed = true; p.mu.Unlock() }

// frame writes a single LSP framed message to b.
func frame(method, id string, params interface{}) string {
	var msg map[string]interface{}
	if id == "" {
		msg = map[string]interface{}{"jsonrpc": "2.0", "method": method, "params": params}
	} else {
		idVal, _ := json.Marshal(json.RawMessage(id))
		msg = map[string]interface{}{"jsonrpc": "2.0", "id": json.RawMessage(idVal), "method": method, "params": params}
	}
	payload, _ := json.Marshal(msg)
	return fmt.Sprintf("Content-Length: %d\r\n\r\n%s", len(payload), payload)
}

func runServerWith(t *testing.T, msgs []string) []byte {
	t.Helper()
	in := bytes.NewBufferString(strings.Join(msgs, ""))
	var out bytes.Buffer
	if err := Run(in, &out, nil); err != nil {
		t.Fatalf("server: %v", err)
	}
	return out.Bytes()
}

// parseMessages splits framed output into raw payloads.
func parseMessages(t *testing.T, data []byte) []map[string]interface{} {
	t.Helper()
	var out []map[string]interface{}
	for len(data) > 0 {
		idx := bytes.Index(data, []byte("\r\n\r\n"))
		if idx < 0 {
			break
		}
		header := string(data[:idx])
		data = data[idx+4:]
		var clen int
		for _, line := range strings.Split(header, "\r\n") {
			if strings.HasPrefix(strings.ToLower(line), "content-length:") {
				fmt.Sscanf(line, "Content-Length: %d", &clen)
			}
		}
		if clen == 0 || clen > len(data) {
			break
		}
		var m map[string]interface{}
		if err := json.Unmarshal(data[:clen], &m); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		out = append(out, m)
		data = data[clen:]
	}
	return out
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestServer_Initialize(t *testing.T) {
	out := runServerWith(t, []string{
		frame("initialize", `1`, map[string]interface{}{}),
		frame("exit", "", nil),
	})
	msgs := parseMessages(t, out)
	if len(msgs) == 0 {
		t.Fatal("no response")
	}
	res, ok := msgs[0]["result"].(map[string]interface{})
	if !ok {
		t.Fatalf("no result: %+v", msgs[0])
	}
	caps := res["capabilities"].(map[string]interface{})
	if caps["hoverProvider"] != true {
		t.Errorf("expected hoverProvider")
	}
	if caps["definitionProvider"] != true {
		t.Errorf("expected definitionProvider")
	}
}

func TestServer_DidOpenPublishesDiagnostics(t *testing.T) {
	// Source has a parse error: missing body
	openParams := map[string]interface{}{
		"textDocument": map[string]interface{}{
			"uri": "file:///a.dbml", "languageId": "dbml", "version": 1,
			"text": "Table",
		},
	}
	out := runServerWith(t, []string{
		frame("initialize", `1`, map[string]interface{}{}),
		frame("textDocument/didOpen", "", openParams),
		frame("exit", "", nil),
	})
	msgs := parseMessages(t, out)
	var found bool
	for _, m := range msgs {
		if m["method"] == "textDocument/publishDiagnostics" {
			p := m["params"].(map[string]interface{})
			d := p["diagnostics"].([]interface{})
			if len(d) > 0 {
				found = true
			}
		}
	}
	if !found {
		t.Errorf("expected diagnostics for broken source")
	}
}

func TestServer_Hover(t *testing.T) {
	src := "Table users {\n  id int\n}\n"
	openParams := map[string]interface{}{
		"textDocument": map[string]interface{}{
			"uri": "file:///a.dbml", "languageId": "dbml", "version": 1, "text": src,
		},
	}
	// Cursor on 'users' (column 6 on line 0)
	hoverParams := map[string]interface{}{
		"textDocument": map[string]interface{}{"uri": "file:///a.dbml"},
		"position":     map[string]interface{}{"line": 0, "character": 8},
	}
	out := runServerWith(t, []string{
		frame("initialize", `1`, map[string]interface{}{}),
		frame("textDocument/didOpen", "", openParams),
		frame("textDocument/hover", `2`, hoverParams),
		frame("exit", "", nil),
	})
	msgs := parseMessages(t, out)
	var got map[string]interface{}
	for _, m := range msgs {
		if id, ok := m["id"]; ok {
			if jn, _ := id.(float64); jn == 2 {
				got = m
			}
		}
	}
	if got == nil {
		t.Fatalf("no hover response: %+v", msgs)
	}
	res, ok := got["result"].(map[string]interface{})
	if !ok {
		t.Fatalf("no result")
	}
	contents := res["contents"].(map[string]interface{})
	if v, _ := contents["value"].(string); !strings.Contains(v, "users") {
		t.Errorf("missing 'users' in hover: %v", v)
	}
}

func TestServer_Completion(t *testing.T) {
	src := ""
	openParams := map[string]interface{}{
		"textDocument": map[string]interface{}{
			"uri": "file:///a.dbml", "languageId": "dbml", "version": 1, "text": src,
		},
	}
	out := runServerWith(t, []string{
		frame("initialize", `1`, map[string]interface{}{}),
		frame("textDocument/didOpen", "", openParams),
		frame("textDocument/completion", `2`, map[string]interface{}{
			"textDocument": map[string]interface{}{"uri": "file:///a.dbml"},
			"position":     map[string]interface{}{"line": 0, "character": 0},
		}),
		frame("exit", "", nil),
	})
	msgs := parseMessages(t, out)
	var got map[string]interface{}
	for _, m := range msgs {
		if id, ok := m["id"]; ok {
			if jn, _ := id.(float64); jn == 2 {
				got = m
			}
		}
	}
	if got == nil {
		t.Fatal("no completion response")
	}
	res := got["result"].(map[string]interface{})
	items := res["items"].([]interface{})
	if len(items) == 0 {
		t.Fatal("no items")
	}
	var labels []string
	for _, it := range items {
		labels = append(labels, it.(map[string]interface{})["label"].(string))
	}
	want := []string{"Table", "Enum", "Ref"}
	for _, w := range want {
		found := false
		for _, l := range labels {
			if l == w {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("missing %q in: %v", w, labels)
		}
	}
}

// Hovering over a position with no symbol must still return a well-formed
// JSON-RPC response with an explicit `result: null` — otherwise clients like
// vscode-languageclient reject the message as invalid.
func TestServer_HoverEmptyHasResultField(t *testing.T) {
	openParams := map[string]interface{}{
		"textDocument": map[string]interface{}{
			"uri": "file:///a.dbml", "languageId": "dbml", "version": 1, "text": "",
		},
	}
	hoverParams := map[string]interface{}{
		"textDocument": map[string]interface{}{"uri": "file:///a.dbml"},
		"position":     map[string]interface{}{"line": 0, "character": 0},
	}
	out := runServerWith(t, []string{
		frame("initialize", `1`, map[string]interface{}{}),
		frame("textDocument/didOpen", "", openParams),
		frame("textDocument/hover", `2`, hoverParams),
		frame("exit", "", nil),
	})
	msgs := parseMessages(t, out)
	for _, m := range msgs {
		id, ok := m["id"]
		if !ok {
			continue
		}
		jn, _ := id.(float64)
		if jn != 2 {
			continue
		}
		_, hasResult := m["result"]
		_, hasError := m["error"]
		if !hasResult && !hasError {
			t.Fatalf("hover response missing both result and error: %+v", m)
		}
		return
	}
	t.Fatal("no hover response")
}

func TestServer_Rename(t *testing.T) {
	src := "Table users {\n  id int\n}\nRef: users.id > users.id"
	openParams := map[string]interface{}{
		"textDocument": map[string]interface{}{
			"uri": "file:///a.dbml", "languageId": "dbml", "version": 1, "text": src,
		},
	}
	renameParams := map[string]interface{}{
		"textDocument": map[string]interface{}{"uri": "file:///a.dbml"},
		"position":     map[string]interface{}{"line": 0, "character": 8},
		"newName":      "people",
	}
	out := runServerWith(t, []string{
		frame("initialize", `1`, map[string]interface{}{}),
		frame("textDocument/didOpen", "", openParams),
		frame("textDocument/rename", `2`, renameParams),
		frame("exit", "", nil),
	})
	msgs := parseMessages(t, out)
	var got map[string]interface{}
	for _, m := range msgs {
		if id, ok := m["id"]; ok {
			if jn, _ := id.(float64); jn == 2 {
				got = m
			}
		}
	}
	if got == nil {
		t.Fatal("no rename response")
	}
	res := got["result"].(map[string]interface{})
	changes := res["changes"].(map[string]interface{})
	edits, ok := changes["file:///a.dbml"].([]interface{})
	if !ok || len(edits) < 3 {
		t.Fatalf("expected >=3 edits, got %d", len(edits))
	}
	for _, e := range edits {
		if e.(map[string]interface{})["newText"] != "people" {
			t.Errorf("unexpected newText: %v", e)
		}
	}
}
