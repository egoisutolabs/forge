package skills

// globalRegistry is the package-level registry of bundled skills.
// Populated by RegisterBundledSkill() at init time.
var globalRegistry = NewRegistry()

// RegisterBundledSkill adds a skill to the global bundled-skills registry.
// Call this from init() functions in bundled skill packages.
func RegisterBundledSkill(s *Skill) {
	s.Source = "bundled"
	globalRegistry.Register(s)
}

// BundledRegistry returns the global registry that holds all built-in skills.
func BundledRegistry() *SkillRegistry {
	return globalRegistry
}

// init registers the built-in skills that ship with Forge.
func init() {
	registerCommitSkill()
	registerReviewSkill()
}

func registerCommitSkill() {
	RegisterBundledSkill(&Skill{
		Name:          "commit",
		Description:   "Create a git commit with a generated commit message",
		WhenToUse:     "When you want to stage and commit changes with an auto-generated message",
		UserInvocable: true,
		Context:       ContextInline,
		Prompt: func(args string) string {
			return "Review the staged git changes (or all changes if nothing is staged) and create a well-formatted commit.\n\n" +
				"Steps:\n" +
				"1. Run `git status` to see what is staged/unstaged\n" +
				"2. Run `git diff --staged` (or `git diff` if nothing staged) to see the changes\n" +
				"3. Write a concise commit message following the conventional commits format: <type>(<scope>): <description>\n" +
				"4. Run `git commit -m \"<message>\"`\n\n" +
				"Commit types: feat, fix, docs, style, refactor, test, chore\n" +
				"Keep the subject line under 72 characters. Add a body only if the change is non-obvious."
		},
	})
}

func registerReviewSkill() {
	RegisterBundledSkill(&Skill{
		Name:          "review",
		Description:   "Review code changes and provide feedback",
		WhenToUse:     "When you want a code review of the current changes or a specific file",
		UserInvocable: true,
		Context:       ContextInline,
		Prompt: func(args string) string {
			subject := args
			if subject == "" {
				subject = "the current staged changes"
			}
			return `Review ` + subject + ` for:
1. Correctness — logic errors, edge cases, off-by-one
2. Security — injection, unsafe deserialization, secrets in code
3. Performance — unnecessary allocations, N+1 queries, blocking calls
4. Readability — naming, documentation, code structure
5. Test coverage — missing cases, brittle assertions

Provide specific, actionable feedback. For each issue: quote the relevant code, explain the problem, and suggest a fix.`
		},
	})
}
