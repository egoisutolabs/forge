package tui

import "testing"

func TestParseDiffFiles_SingleFile(t *testing.T) {
	diff := `diff --git a/foo.go b/foo.go
--- a/foo.go
+++ b/foo.go
@@ -1,3 +1,4 @@
 package main
+import "fmt"
 func main() {
-    println("hello")
+    fmt.Println("hello")
 }
`
	files := parseDiffFiles(diff)
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}
	if files[0].Path != "foo.go" {
		t.Fatalf("expected path %q, got %q", "foo.go", files[0].Path)
	}
	if files[0].Added != 2 {
		t.Fatalf("expected 2 added lines, got %d", files[0].Added)
	}
	if files[0].Removed != 1 {
		t.Fatalf("expected 1 removed line, got %d", files[0].Removed)
	}
}

func TestParseDiffFiles_MultipleFiles(t *testing.T) {
	diff := `diff --git a/a.go b/a.go
--- a/a.go
+++ b/a.go
@@ -1 +1 @@
-old
+new
diff --git a/b.go b/b.go
--- a/b.go
+++ b/b.go
@@ -1 +1,2 @@
 existing
+added
`
	files := parseDiffFiles(diff)
	if len(files) != 2 {
		t.Fatalf("expected 2 files, got %d", len(files))
	}
	if files[0].Path != "a.go" {
		t.Errorf("expected first file %q, got %q", "a.go", files[0].Path)
	}
	if files[1].Path != "b.go" {
		t.Errorf("expected second file %q, got %q", "b.go", files[1].Path)
	}
}

func TestDiffDialog_FileListNavigation(t *testing.T) {
	theme := InitTheme()
	messages := []DisplayMessage{
		{
			Role:     "tool",
			ToolName: "Edit",
			Content: `diff --git a/a.go b/a.go
--- a/a.go
+++ b/a.go
@@ -1 +1 @@
-old
+new
diff --git a/b.go b/b.go
--- a/b.go
+++ b/b.go
@@ -1 +1 @@
-old
+new
`,
		},
	}

	d := NewDiffDialog(messages, 80, 40, theme)
	if d == nil {
		t.Fatal("expected diff dialog to be created")
	}
	if d.FileCount() != 2 {
		t.Fatalf("expected 2 files, got %d", d.FileCount())
	}
	if d.Cursor() != 0 {
		t.Fatalf("expected cursor at 0, got %d", d.Cursor())
	}

	// Navigate down
	d.HandleKey("down")
	if d.Cursor() != 1 {
		t.Fatalf("expected cursor at 1 after down, got %d", d.Cursor())
	}

	// Navigate up
	d.HandleKey("up")
	if d.Cursor() != 0 {
		t.Fatalf("expected cursor at 0 after up, got %d", d.Cursor())
	}

	// Don't go past 0
	d.HandleKey("up")
	if d.Cursor() != 0 {
		t.Fatalf("expected cursor clamped at 0, got %d", d.Cursor())
	}
}

func TestDiffDialog_DetailView(t *testing.T) {
	theme := InitTheme()
	messages := []DisplayMessage{
		{
			Role:     "tool",
			ToolName: "Edit",
			Content: `diff --git a/a.go b/a.go
--- a/a.go
+++ b/a.go
@@ -1 +1 @@
-old
+new
`,
		},
	}

	d := NewDiffDialog(messages, 80, 40, theme)
	if d == nil {
		t.Fatal("expected diff dialog to be created")
	}

	// Enter detail view
	d.HandleKey("enter")
	if !d.InDetail() {
		t.Fatal("expected to be in detail view after enter")
	}
	if !d.files[0].Viewed {
		t.Fatal("expected file to be marked as viewed")
	}

	// Navigate detail
	d.HandleKey("down")
	d.HandleKey("up")

	// Back to list
	d.HandleKey("esc")
	if d.InDetail() {
		t.Fatal("expected to exit detail view after esc")
	}
}

func TestDiffDialog_CloseFromList(t *testing.T) {
	theme := InitTheme()
	messages := []DisplayMessage{
		{
			Role:     "tool",
			ToolName: "Edit",
			Content: `diff --git a/a.go b/a.go
--- a/a.go
+++ b/a.go
@@ -1 +1 @@
-old
+new
`,
		},
	}

	d := NewDiffDialog(messages, 80, 40, theme)
	if d == nil {
		t.Fatal("expected diff dialog to be created")
	}

	closed := d.HandleKey("esc")
	if !closed {
		t.Fatal("expected dialog to close on esc from list view")
	}
}

func TestDiffDialog_NoDiffs(t *testing.T) {
	theme := InitTheme()
	messages := []DisplayMessage{
		{Role: "tool", ToolName: "Read", Content: "some text"},
	}

	d := NewDiffDialog(messages, 80, 40, theme)
	if d != nil {
		t.Fatal("expected nil dialog when no diff content")
	}
}

func TestDiffDialog_Render(t *testing.T) {
	theme := InitTheme()
	messages := []DisplayMessage{
		{
			Role:     "tool",
			ToolName: "Edit",
			Content: `diff --git a/a.go b/a.go
--- a/a.go
+++ b/a.go
@@ -1 +1 @@
-old
+new
`,
		},
	}

	d := NewDiffDialog(messages, 80, 40, theme)
	if d == nil {
		t.Fatal("expected diff dialog to be created")
	}

	// Should not panic
	listView := d.Render()
	if listView == "" {
		t.Fatal("expected non-empty list view render")
	}

	// Detail view
	d.HandleKey("enter")
	detailView := d.Render()
	if detailView == "" {
		t.Fatal("expected non-empty detail view render")
	}
}

func TestDiffDialog_BackspaceExitsDetail(t *testing.T) {
	theme := InitTheme()
	messages := []DisplayMessage{
		{
			Role:     "tool",
			ToolName: "Write",
			Content: `diff --git a/a.go b/a.go
--- a/a.go
+++ b/a.go
@@ -1 +1 @@
-old
+new
`,
		},
	}

	d := NewDiffDialog(messages, 80, 40, theme)
	if d == nil {
		t.Fatal("expected diff dialog")
	}

	d.HandleKey("enter")
	if !d.InDetail() {
		t.Fatal("should be in detail")
	}

	d.HandleKey("backspace")
	if d.InDetail() {
		t.Fatal("backspace should exit detail view")
	}
}
