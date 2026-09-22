package main

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/ocinsh/crwai"
)

func TestSeeFileCommandJSON(t *testing.T) {
	cmd := newRoot()
	var output bytes.Buffer
	cmd.SetOut(&output)
	cmd.SetArgs([]string{"--json", "see_file", filepath.Join("..", "..", "examples", "golang", "methods.go")})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var view crwai.FileView
	if err := json.Unmarshal(output.Bytes(), &view); err != nil {
		t.Fatalf("invalid JSON: %v: %s", err, output.String())
	}
	if len(view.Signatures) == 0 || !strings.Contains(view.Content, "use get_function") || strings.Contains(view.Content, "c.n++") {
		t.Fatalf("unexpected file preview: %#v", view)
	}
	var raw map[string]any
	if err := json.Unmarshal(output.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	if _, ok := raw["signatures"]; !ok {
		t.Fatal("missing top-level signatures")
	}
	if _, ok := raw["content"]; !ok {
		t.Fatal("missing top-level content")
	}
}

func TestSeeFileMCPResult(t *testing.T) {
	server := newMCPServer(crwai.New())
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(context.Background(), serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer serverSession.Close()
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "v0.0.1"}, nil)
	clientSession, err := client.Connect(context.Background(), clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer clientSession.Close()
	if got := clientSession.InitializeResult().Instructions; !strings.Contains(got, "Prefer crwai") || !strings.Contains(got, "see_file") {
		t.Fatalf("missing server guidance: %q", got)
	}
	result, err := clientSession.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "see_file",
		Arguments: map[string]any{"path": filepath.Join("..", "..", "examples", "golang", "methods.go")},
	})
	if err != nil || result.IsError {
		t.Fatalf("MCP call failed: %v, %#v", err, result)
	}
	encoded, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var output map[string]any
	if err := json.Unmarshal(encoded, &output); err != nil {
		t.Fatal(err)
	}
	if _, ok := output["signatures"]; !ok {
		t.Fatalf("missing top-level signatures: %s", encoded)
	}
	content, ok := output["content"].(string)
	if !ok || !strings.Contains(content, "use get_function") || strings.Contains(content, "c.n++") {
		t.Fatalf("unexpected MCP content: %s", encoded)
	}
}
