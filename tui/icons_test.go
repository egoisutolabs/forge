package tui

import "testing"

func TestFileIcon_KnownExtensions(t *testing.T) {
	cases := []struct {
		filename string
		want     string
	}{
		{"main.go", "\ue627"},
		{"app.ts", "\U000f06e6"},
		{"app.tsx", "\U000f06e6"},
		{"index.js", "\ue781"},
		{"index.jsx", "\ue781"},
		{"script.py", "\ue73c"},
		{"README.md", "\ue73e"},
		{"config.json", "\ue60b"},
		{"config.yaml", "\ue6a8"},
		{"config.yml", "\ue6a8"},
		{"index.html", "\ue736"},
		{"style.css", "\ue749"},
		{"run.sh", "\ue795"},
		{"lib.rs", "\ue7a8"},
		{"app.rb", "\ue791"},
		{"Main.java", "\ue738"},
		{"main.c", "\ue61e"},
		{"main.cpp", "\ue61d"},
		{"Cargo.toml", "\ue6b2"},
		{"query.sql", "\ue706"},
	}
	for _, c := range cases {
		got := FileIcon(c.filename)
		if got != c.want {
			t.Errorf("FileIcon(%q) = %q, want %q", c.filename, got, c.want)
		}
	}
}

func TestFileIcon_UnknownExtension(t *testing.T) {
	got := FileIcon("data.xyz")
	want := "\uf15b"
	if got != want {
		t.Errorf("FileIcon(unknown) = %q, want %q", got, want)
	}
}

func TestFileIcon_NoExtension(t *testing.T) {
	got := FileIcon("Makefile")
	want := "\uf15b"
	if got != want {
		t.Errorf("FileIcon(no ext) = %q, want %q", got, want)
	}
}

func TestDirIcon(t *testing.T) {
	got := DirIcon()
	want := "\uf07b"
	if got != want {
		t.Errorf("DirIcon() = %q, want %q", got, want)
	}
}
