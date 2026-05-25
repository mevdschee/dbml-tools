package lsp

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"sync"
)

// Run starts an LSP server that reads JSON-RPC messages from in and writes
// responses + notifications to out. Logs go to logger (may be nil).
func Run(in io.Reader, out io.Writer, logger *log.Logger) error {
	if logger == nil {
		logger = log.New(io.Discard, "", 0)
	}
	s := &server{
		t:      newTransport(in, out),
		log:    logger,
		docs:   map[string]*document{},
		closed: make(chan struct{}),
	}
	return s.loop()
}

// RunStdio is a convenience entry point: stdin → stdout, optional log path.
func RunStdio(logPath string) error {
	var logger *log.Logger
	if logPath != "" {
		f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			return err
		}
		defer f.Close()
		logger = log.New(f, "[dbml-lsp] ", log.LstdFlags)
	}
	return Run(os.Stdin, os.Stdout, logger)
}

// ---------------------------------------------------------------------------
// Server core
// ---------------------------------------------------------------------------

type server struct {
	t   *transport
	log *log.Logger

	mu   sync.RWMutex
	docs map[string]*document

	closed chan struct{}
}

func (s *server) loop() error {
	for {
		select {
		case <-s.closed:
			return nil
		default:
		}
		payload, err := s.t.readMessage()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			s.log.Printf("read error: %v", err)
			return err
		}
		var req rpcRequest
		if err := json.Unmarshal(payload, &req); err != nil {
			s.log.Printf("parse error: %v", err)
			continue
		}
		s.handle(&req)
	}
}

func (s *server) handle(req *rpcRequest) {
	s.log.Printf("→ %s", req.Method)
	isNotif := len(req.ID) == 0

	dispatch := map[string]func(json.RawMessage) (interface{}, *rpcError){
		"initialize":              s.onInitialize,
		"shutdown":                s.onShutdown,
		"textDocument/hover":      s.onHover,
		"textDocument/completion": s.onCompletion,
		"textDocument/definition": s.onDefinition,
		"textDocument/references": s.onReferences,
		"textDocument/rename":     s.onRename,
		"textDocument/prepareRename": s.onPrepareRename,
		"textDocument/documentSymbol": s.onDocumentSymbol,
	}
	notif := map[string]func(json.RawMessage){
		"initialized":             func(json.RawMessage) {},
		"exit":                    s.onExit,
		"textDocument/didOpen":    s.onDidOpen,
		"textDocument/didChange":  s.onDidChange,
		"textDocument/didClose":   s.onDidClose,
		"textDocument/didSave":    func(json.RawMessage) {},
		"$/cancelRequest":         func(json.RawMessage) {},
		"workspace/didChangeConfiguration": func(json.RawMessage) {},
	}

	if isNotif {
		if h, ok := notif[req.Method]; ok {
			h(req.Params)
		}
		return
	}

	h, ok := dispatch[req.Method]
	if !ok {
		s.replyError(req.ID, errMethodNotFound, "method not found: "+req.Method)
		return
	}
	res, rerr := h(req.Params)
	if rerr != nil {
		s.replyError(req.ID, rerr.Code, rerr.Message)
		return
	}
	s.reply(req.ID, res)
}

// ---------------------------------------------------------------------------
// Reply helpers
// ---------------------------------------------------------------------------

func (s *server) reply(id json.RawMessage, result interface{}) {
	resp := rpcResponse{JSONRPC: "2.0", ID: id, Result: result}
	if err := s.t.writeMessage(&resp); err != nil {
		s.log.Printf("write reply: %v", err)
	}
}

func (s *server) replyError(id json.RawMessage, code int, msg string) {
	resp := rpcResponse{JSONRPC: "2.0", ID: id, Error: &rpcError{Code: code, Message: msg}}
	if err := s.t.writeMessage(&resp); err != nil {
		s.log.Printf("write error reply: %v", err)
	}
}

func (s *server) notify(method string, params interface{}) {
	n := rpcNotification{JSONRPC: "2.0", Method: method, Params: params}
	if err := s.t.writeMessage(&n); err != nil {
		s.log.Printf("write notify: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Document store helpers
// ---------------------------------------------------------------------------

func (s *server) getDoc(uri string) *document {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.docs[uri]
}

func (s *server) setDoc(d *document) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.docs[d.uri] = d
}

func (s *server) deleteDoc(uri string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.docs, uri)
}

func paramsErr(err error) *rpcError {
	return &rpcError{Code: errInvalidParams, Message: fmt.Sprintf("invalid params: %v", err)}
}
