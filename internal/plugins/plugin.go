// Package plugins manages forge plugins — discoverable units that bundle
// skills, agents, and hooks together and can be enabled or disabled at runtime.
//
// Plugin directory layout:
//
//	my-plugin/
//	├── plugin.json          # manifest (name, version, description, skills, hooks, agents)
//	├── skills/              # skill markdown files (path listed under "skills" key)
//	├── agents/              # agent markdown files (path listed under "agents" key)
//	└── hooks/hooks.json     # optional hook config (path given by "hooks" key)
package plugins

import "github.com/egoisutolabs/forge/internal/hooks"

// PluginManifest is the parsed content of a plugin.json file.
// It describes the plugin's metadata and declares which relative paths
// contain skills, agents, and hooks.
type PluginManifest struct {
	// Required fields
	Name    string `json:"name"`
	Version string `json:"version"`

	// Optional metadata
	Description string `json:"description,omitempty"`

	// SkillPaths is a list of paths (relative to the plugin dir) that contain
	// skill markdown files. Corresponds to the "skills" key in plugin.json.
	SkillPaths []string `json:"skills,omitempty"`

	// HooksFile is a path (relative to the plugin dir) pointing to a
	// hooks.json file. Corresponds to the "hooks" key in plugin.json.
	HooksFile string `json:"hooks,omitempty"`

	// AgentPaths is a list of paths (relative to the plugin dir) that contain
	// agent markdown files. Corresponds to the "agents" key in plugin.json.
	AgentPaths []string `json:"agents,omitempty"`
}

// Plugin is a fully-loaded plugin: the manifest fields are resolved into
// absolute paths and the hook configuration has been parsed and merged.
type Plugin struct {
	// Name is the plugin's unique identifier (from the manifest).
	Name string

	// Version is the plugin's version string.
	Version string

	// Description is an optional human-readable summary.
	Description string

	// Dir is the absolute path to the plugin's directory on disk.
	Dir string

	// SkillPaths contains absolute paths to the plugin's skill directories
	// or individual skill files, resolved from the manifest's "skills" list.
	SkillPaths []string

	// AgentPaths contains absolute paths to the plugin's agent directories
	// or individual agent files, resolved from the manifest's "agents" list.
	AgentPaths []string

	// HookConfigs holds the parsed hook configuration loaded from the
	// plugin's hooks file (if any).
	HookConfigs hooks.HooksSettings

	// Enabled indicates whether this plugin is currently active.
	// Disabled plugins contribute no skills, agents, or hooks.
	Enabled bool

	// IsBuiltin marks plugins that ship bundled with the application.
	// Builtin plugins are registered programmatically rather than discovered
	// from the filesystem.
	IsBuiltin bool
}
