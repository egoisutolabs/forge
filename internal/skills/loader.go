package skills

import (
	"os"
	"path/filepath"
	"strings"
)

// LoadSkillsDir scans dir for .md files and registers each as a skill in
// registry. The skill name is the path relative to dir with the .md extension
// stripped and directory separators replaced by "/" (already the case on
// Unix). Skills that fail to parse are silently skipped.
//
// source is stored on the Skill to indicate where it was loaded from
// (e.g. "user", "project").
func LoadSkillsDir(dir string, source string, registry *SkillRegistry) error {
	info, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // silently ignore missing directories
		}
		return err
	}
	if !info.IsDir() {
		return nil
	}

	return filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // skip unreadable entries
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".md") {
			return nil
		}

		s, err := loadSkillFile(path, dir, source)
		if err != nil {
			return nil // skip malformed files
		}
		registry.Register(s)
		return nil
	})
}

// loadSkillFile reads one markdown skill file and returns a Skill.
func loadSkillFile(path, baseDir, source string) (*Skill, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	// Derive skill name from relative path: strip base dir and .md extension.
	rel, err := filepath.Rel(baseDir, path)
	if err != nil {
		rel = filepath.Base(path)
	}
	name := strings.TrimSuffix(rel, ".md")
	// Normalise to forward slashes (on all platforms).
	name = filepath.ToSlash(name)

	fm, body := ParseFrontmatter(string(data))

	// Use filename as description fallback.
	desc := fm.Description
	if desc == "" {
		desc = name
	}

	prompt := body
	s := &Skill{
		Name:          name,
		Description:   desc,
		WhenToUse:     fm.WhenToUse,
		AllowedTools:  fm.AllowedTools,
		Context:       fm.Context,
		UserInvocable: fm.UserInvocable,
		Source:        source,
		Prompt:        func(_ string) string { return prompt },
	}
	return s, nil
}

// DefaultSearchPaths returns the standard skill directories in priority order
// (lowest to highest): user-global, then project-local.
//
// cwd is the current working directory used to locate the project-local dir.
func DefaultSearchPaths(cwd string) []struct {
	Dir    string
	Source string
} {
	home, _ := os.UserHomeDir()
	paths := []struct {
		Dir    string
		Source string
	}{
		{Dir: filepath.Join(home, ".forge", "skills"), Source: "user"},
		{Dir: filepath.Join(cwd, ".forge", "skills"), Source: "project"},
	}
	return paths
}

// LoadDefaultSkills loads skills from the default search paths into registry.
// Later sources (higher priority) overwrite earlier ones with the same name.
func LoadDefaultSkills(cwd string, registry *SkillRegistry) error {
	for _, p := range DefaultSearchPaths(cwd) {
		if err := LoadSkillsDir(p.Dir, p.Source, registry); err != nil {
			return err
		}
	}
	return nil
}
