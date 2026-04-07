package skills

import (
	"reflect"
	"testing"
)

func TestParseFrontmatter_NoPreamble(t *testing.T) {
	doc := "# Just content\nNo frontmatter here."
	fm, body := ParseFrontmatter(doc)
	if body != doc {
		t.Errorf("body = %q, want original doc", body)
	}
	// Defaults
	if !fm.UserInvocable {
		t.Error("UserInvocable should default to true")
	}
	if fm.Context != ContextInline {
		t.Errorf("Context should default to inline, got %q", fm.Context)
	}
}

func TestParseFrontmatter_EmptyFrontmatter(t *testing.T) {
	doc := "---\n---\n# Body"
	_, body := ParseFrontmatter(doc)
	if body != "# Body" {
		t.Errorf("body = %q, want %q", body, "# Body")
	}
}

func TestParseFrontmatter_Description(t *testing.T) {
	doc := "---\ndescription: My skill description\n---\nbody"
	fm, _ := ParseFrontmatter(doc)
	if fm.Description != "My skill description" {
		t.Errorf("Description = %q, want %q", fm.Description, "My skill description")
	}
}

func TestParseFrontmatter_DescriptionQuoted(t *testing.T) {
	doc := "---\ndescription: \"quoted description\"\n---\n"
	fm, _ := ParseFrontmatter(doc)
	if fm.Description != "quoted description" {
		t.Errorf("Description = %q, want %q", fm.Description, "quoted description")
	}
}

func TestParseFrontmatter_WhenToUse(t *testing.T) {
	doc := "---\nwhen-to-use: When you need to commit\n---\n"
	fm, _ := ParseFrontmatter(doc)
	if fm.WhenToUse != "When you need to commit" {
		t.Errorf("WhenToUse = %q", fm.WhenToUse)
	}
}

func TestParseFrontmatter_AllowedToolsInline(t *testing.T) {
	doc := "---\nallowed-tools: [Bash, Read, Glob]\n---\n"
	fm, _ := ParseFrontmatter(doc)
	want := []string{"Bash", "Read", "Glob"}
	if !reflect.DeepEqual(fm.AllowedTools, want) {
		t.Errorf("AllowedTools = %v, want %v", fm.AllowedTools, want)
	}
}

func TestParseFrontmatter_AllowedToolsMultiLine(t *testing.T) {
	doc := "---\nallowed-tools:\n  - Bash\n  - Read\n---\n"
	fm, _ := ParseFrontmatter(doc)
	want := []string{"Bash", "Read"}
	if !reflect.DeepEqual(fm.AllowedTools, want) {
		t.Errorf("AllowedTools = %v, want %v", fm.AllowedTools, want)
	}
}

func TestParseFrontmatter_UserInvocableFalse(t *testing.T) {
	doc := "---\nuser-invocable: false\n---\n"
	fm, _ := ParseFrontmatter(doc)
	if fm.UserInvocable {
		t.Error("UserInvocable should be false")
	}
}

func TestParseFrontmatter_UserInvocableDefault(t *testing.T) {
	doc := "---\ndescription: no user-invocable key\n---\n"
	fm, _ := ParseFrontmatter(doc)
	if !fm.UserInvocable {
		t.Error("UserInvocable should default to true")
	}
}

func TestParseFrontmatter_ContextFork(t *testing.T) {
	doc := "---\ncontext: fork\n---\n"
	fm, _ := ParseFrontmatter(doc)
	if fm.Context != ContextFork {
		t.Errorf("Context = %q, want %q", fm.Context, ContextFork)
	}
}

func TestParseFrontmatter_ContextInlineDefault(t *testing.T) {
	doc := "---\ndescription: no context key\n---\n"
	fm, _ := ParseFrontmatter(doc)
	if fm.Context != ContextInline {
		t.Errorf("Context = %q, want %q", fm.Context, ContextInline)
	}
}

func TestParseFrontmatter_BodyPreserved(t *testing.T) {
	doc := "---\ndescription: test\n---\n# Heading\n\nParagraph text.\n"
	_, body := ParseFrontmatter(doc)
	want := "# Heading\n\nParagraph text.\n"
	if body != want {
		t.Errorf("body = %q, want %q", body, want)
	}
}

func TestParseFrontmatter_FullExample(t *testing.T) {
	doc := "---\ndescription: Commit changes\nwhen-to-use: When committing\nallowed-tools: [Bash, Read]\nuser-invocable: true\ncontext: inline\n---\n# Prompt body"
	fm, body := ParseFrontmatter(doc)

	if fm.Description != "Commit changes" {
		t.Errorf("Description = %q", fm.Description)
	}
	if fm.WhenToUse != "When committing" {
		t.Errorf("WhenToUse = %q", fm.WhenToUse)
	}
	if !reflect.DeepEqual(fm.AllowedTools, []string{"Bash", "Read"}) {
		t.Errorf("AllowedTools = %v", fm.AllowedTools)
	}
	if !fm.UserInvocable {
		t.Error("UserInvocable should be true")
	}
	if fm.Context != ContextInline {
		t.Errorf("Context = %q", fm.Context)
	}
	if body != "# Prompt body" {
		t.Errorf("body = %q", body)
	}
}

func TestParseInlineList_Empty(t *testing.T) {
	if got := parseInlineList("[]"); len(got) != 0 {
		t.Errorf("expected empty, got %v", got)
	}
}

func TestParseInlineList_SingleItem(t *testing.T) {
	got := parseInlineList("[Bash]")
	if len(got) != 1 || got[0] != "Bash" {
		t.Errorf("got %v", got)
	}
}

func TestParseBool(t *testing.T) {
	tests := []struct {
		in   string
		def  bool
		want bool
	}{
		{"true", false, true},
		{"True", false, true},
		{"yes", false, true},
		{"false", true, false},
		{"False", true, false},
		{"no", true, false},
		{"", true, true},
		{"garbage", false, false},
	}
	for _, tc := range tests {
		if got := parseBool(tc.in, tc.def); got != tc.want {
			t.Errorf("parseBool(%q, %v) = %v, want %v", tc.in, tc.def, got, tc.want)
		}
	}
}
