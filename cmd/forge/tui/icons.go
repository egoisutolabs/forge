package tui

import "path/filepath"

// FileIcon returns a Nerd Font icon for the given filename based on its extension.
func FileIcon(filename string) string {
	ext := filepath.Ext(filename)
	switch ext {
	case ".go":
		return "\ue627" // Go
	case ".ts", ".tsx":
		return "\U000f06e6" // TypeScript
	case ".js", ".jsx":
		return "\ue781" // JavaScript
	case ".py":
		return "\ue73c" // Python
	case ".md":
		return "\ue73e" // Markdown
	case ".json":
		return "\ue60b" // JSON
	case ".yaml", ".yml":
		return "\ue6a8" // YAML
	case ".html":
		return "\ue736" // HTML
	case ".css":
		return "\ue749" // CSS
	case ".sh":
		return "\ue795" // Shell
	case ".rs":
		return "\ue7a8" // Rust
	case ".rb":
		return "\ue791" // Ruby
	case ".java":
		return "\ue738" // Java
	case ".c", ".h":
		return "\ue61e" // C
	case ".cpp", ".cc", ".cxx", ".hpp":
		return "\ue61d" // C++
	case ".toml":
		return "\ue6b2" // TOML
	case ".sql":
		return "\ue706" // SQL
	case ".dockerfile", ".Dockerfile":
		return "\ue7b0" // Docker
	case ".proto":
		return "\ue6b1" // Protobuf
	default:
		return "\uf15b" // Generic file
	}
}

// DirIcon returns a Nerd Font folder icon.
func DirIcon() string { return "\uf07b" }
