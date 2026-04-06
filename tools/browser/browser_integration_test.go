package browser

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"testing"
)

func TestBrowserTool_WithRealAgentBrowser(t *testing.T) {
	if _, err := exec.LookPath("agent-browser"); err != nil {
		t.Skip("agent-browser not found in PATH")
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<!doctype html>
<html>
  <head><title>Forge Browser Test</title></head>
  <body>
    <h1>Welcome</h1>
    <a id="go" href="/next">Next page</a>
    <label for="name">Name</label>
    <input id="name" />
  </body>
</html>`)
	})
	mux.HandleFunc("/next", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<!doctype html>
<html>
  <head><title>Next Page</title></head>
  <body>
    <h1>Destination</h1>
  </body>
</html>`)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	session := strings.NewReplacer("/", "-", " ", "-").Replace(strings.ToLower(t.Name()))
	tool := &Tool{Command: "agent-browser"}

	t.Cleanup(func() {
		_, _ = tool.Execute(context.Background(), mustJSON(t, map[string]any{
			"action":  "close",
			"session": session,
		}), nil)
	})

	openResult, err := tool.Execute(context.Background(), mustJSON(t, map[string]any{
		"action":  "open",
		"url":     server.URL,
		"session": session,
	}), nil)
	if err != nil {
		t.Fatalf("open: unexpected error: %v", err)
	}
	if openResult.IsError {
		t.Fatalf("open failed: %s", openResult.Content)
	}

	snapshotResult, err := tool.Execute(context.Background(), mustJSON(t, map[string]any{
		"action":      "snapshot",
		"session":     session,
		"interactive": true,
		"compact":     true,
		"depth":       5,
	}), nil)
	if err != nil {
		t.Fatalf("snapshot: unexpected error: %v", err)
	}
	if snapshotResult.IsError {
		t.Fatalf("snapshot failed: %s", snapshotResult.Content)
	}
	if !strings.Contains(snapshotResult.Content, "Next page") {
		t.Fatalf("snapshot missing expected content: %s", snapshotResult.Content)
	}

	clickResult, err := tool.Execute(context.Background(), mustJSON(t, map[string]any{
		"action":   "click",
		"selector": "#go",
		"session":  session,
	}), nil)
	if err != nil {
		t.Fatalf("click: unexpected error: %v", err)
	}
	if clickResult.IsError {
		t.Fatalf("click failed: %s", clickResult.Content)
	}

	urlResult, err := tool.Execute(context.Background(), mustJSON(t, map[string]any{
		"action":  "get",
		"get":     "url",
		"session": session,
	}), nil)
	if err != nil {
		t.Fatalf("get url: unexpected error: %v", err)
	}
	if urlResult.IsError {
		t.Fatalf("get url failed: %s", urlResult.Content)
	}
	if !strings.Contains(urlResult.Content, "/next") {
		t.Fatalf("expected navigation to /next, got: %s", urlResult.Content)
	}

	closeResult, err := tool.Execute(context.Background(), mustJSON(t, map[string]any{
		"action":  "close",
		"session": session,
	}), nil)
	if err != nil {
		t.Fatalf("close: unexpected error: %v", err)
	}
	if closeResult.IsError {
		t.Fatalf("close failed: %s", closeResult.Content)
	}
}

func TestBrowserTool_FindAndTabsWithRealAgentBrowser(t *testing.T) {
	if _, err := exec.LookPath("agent-browser"); err != nil {
		t.Skip("agent-browser not found in PATH")
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<!doctype html>
<html>
  <head><title>Forge Browser Find Test</title></head>
  <body>
    <h1>Welcome</h1>
    <a href="/next">Next page</a>
  </body>
</html>`)
	})
	mux.HandleFunc("/next", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<!doctype html>
<html>
  <head><title>Next Page</title></head>
  <body>
    <h1>Destination</h1>
  </body>
</html>`)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	session := strings.NewReplacer("/", "-", " ", "-").Replace(strings.ToLower(t.Name()))
	tool := &Tool{Command: "agent-browser"}

	t.Cleanup(func() {
		_, _ = tool.Execute(context.Background(), mustJSON(t, map[string]any{
			"action":  "close",
			"session": session,
		}), nil)
	})

	openResult, err := tool.Execute(context.Background(), mustJSON(t, map[string]any{
		"action":  "open",
		"url":     server.URL,
		"session": session,
	}), nil)
	if err != nil {
		t.Fatalf("open: unexpected error: %v", err)
	}
	if openResult.IsError {
		t.Fatalf("open failed: %s", openResult.Content)
	}

	findResult, err := tool.Execute(context.Background(), mustJSON(t, map[string]any{
		"action":      "find",
		"locator":     "text",
		"value":       "Next page",
		"find_action": "click",
		"session":     session,
	}), nil)
	if err != nil {
		t.Fatalf("find: unexpected error: %v", err)
	}
	if findResult.IsError {
		t.Fatalf("find failed: %s", findResult.Content)
	}

	urlResult, err := tool.Execute(context.Background(), mustJSON(t, map[string]any{
		"action":  "get",
		"get":     "url",
		"session": session,
	}), nil)
	if err != nil {
		t.Fatalf("get url after find: unexpected error: %v", err)
	}
	if urlResult.IsError {
		t.Fatalf("get url after find failed: %s", urlResult.Content)
	}
	if !strings.Contains(urlResult.Content, "/next") {
		t.Fatalf("expected navigation to /next, got: %s", urlResult.Content)
	}

	newTabResult, err := tool.Execute(context.Background(), mustJSON(t, map[string]any{
		"action":     "tab",
		"tab_action": "new",
		"session":    session,
	}), nil)
	if err != nil {
		t.Fatalf("tab new: unexpected error: %v", err)
	}
	if newTabResult.IsError {
		t.Fatalf("tab new failed: %s", newTabResult.Content)
	}

	listResult, err := tool.Execute(context.Background(), mustJSON(t, map[string]any{
		"action":     "tab",
		"tab_action": "list",
		"session":    session,
	}), nil)
	if err != nil {
		t.Fatalf("tab list: unexpected error: %v", err)
	}
	if listResult.IsError {
		t.Fatalf("tab list failed: %s", listResult.Content)
	}
	if strings.TrimSpace(listResult.Content) == "" {
		t.Fatal("tab list returned empty output")
	}

	tabIndex := 0
	switchResult, err := tool.Execute(context.Background(), mustJSON(t, map[string]any{
		"action":     "tab",
		"tab_action": "switch",
		"tab_index":  tabIndex,
		"session":    session,
	}), nil)
	if err != nil {
		t.Fatalf("tab switch: unexpected error: %v", err)
	}
	if switchResult.IsError {
		t.Fatalf("tab switch failed: %s", switchResult.Content)
	}

	switchURLResult, err := tool.Execute(context.Background(), mustJSON(t, map[string]any{
		"action":  "get",
		"get":     "url",
		"session": session,
	}), nil)
	if err != nil {
		t.Fatalf("get url after tab switch: unexpected error: %v", err)
	}
	if switchURLResult.IsError {
		t.Fatalf("get url after tab switch failed: %s", switchURLResult.Content)
	}
	if !strings.Contains(switchURLResult.Content, "/next") {
		t.Fatalf("expected active tab to remain on /next, got: %s", switchURLResult.Content)
	}
}

func mustJSON(t *testing.T, v map[string]any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal JSON: %v", err)
	}
	return b
}
