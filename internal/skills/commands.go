package skills

import "fmt"

// init registers the built-in slash command skills.
func init() {
	registerCostSkill()
	registerClearSkill()
	registerModelSkill()
	registerModelsSkill()
	registerCompactSkill()
	registerHelpSkill()
	registerHistorySkill()
	registerQuitSkill()
	registerConnectSkill()
	registerProvidersSkill()
}

func registerCostSkill() {
	RegisterBundledSkill(&Skill{
		Name:          "cost",
		Description:   "Display session token usage and cost breakdown",
		WhenToUse:     "When the user wants to see how many tokens have been used and how much the session costs",
		UserInvocable: true,
		Context:       ContextInline,
		Prompt: func(args string) string {
			return "Display the current session cost and token usage breakdown."
		},
	})
}

func registerClearSkill() {
	RegisterBundledSkill(&Skill{
		Name:          "clear",
		Description:   "Clear conversation display",
		WhenToUse:     "When the user wants to clear the visible conversation history",
		UserInvocable: true,
		Context:       ContextInline,
		Prompt: func(args string) string {
			return "Clear the conversation display."
		},
	})
}

func registerModelSkill() {
	RegisterBundledSkill(&Skill{
		Name:          "model",
		Description:   "Show or switch the current model",
		WhenToUse:     "When the user wants to see the current model or switch to a different one",
		UserInvocable: true,
		Context:       ContextInline,
		Prompt: func(args string) string {
			if args == "" {
				return "Show the current model name."
			}
			return fmt.Sprintf("Switch to model: %s", args)
		},
	})
}

func registerModelsSkill() {
	RegisterBundledSkill(&Skill{
		Name:          "models",
		Description:   "Open model picker or switch to a specific model",
		WhenToUse:     "When the user wants to browse available models or switch to a different model",
		UserInvocable: true,
		Context:       ContextInline,
		Prompt: func(args string) string {
			if args == "" {
				return "Open the interactive model picker to browse and select a model."
			}
			return fmt.Sprintf("Switch to model: %s", args)
		},
	})
}

func registerCompactSkill() {
	RegisterBundledSkill(&Skill{
		Name:          "compact",
		Description:   "Compact conversation history to save context",
		WhenToUse:     "When the conversation is getting long and the user wants to summarize older messages",
		UserInvocable: true,
		Context:       ContextInline,
		Prompt: func(args string) string {
			return "Compact the conversation history by summarizing older messages."
		},
	})
}

func registerHelpSkill() {
	RegisterBundledSkill(&Skill{
		Name:          "help",
		Description:   "Show available commands and keyboard shortcuts",
		WhenToUse:     "When the user needs help with available commands or keyboard shortcuts",
		UserInvocable: true,
		Context:       ContextInline,
		Prompt: func(args string) string {
			return "Show all available slash commands and keyboard shortcuts."
		},
	})
}

func registerHistorySkill() {
	RegisterBundledSkill(&Skill{
		Name:          "history",
		Description:   "Show input history",
		WhenToUse:     "When the user wants to see their previous inputs",
		UserInvocable: true,
		Context:       ContextInline,
		Prompt: func(args string) string {
			return "Show the input history for this session."
		},
	})
}

func registerQuitSkill() {
	RegisterBundledSkill(&Skill{
		Name:          "quit",
		Description:   "Exit Forge",
		WhenToUse:     "When the user wants to exit the application",
		UserInvocable: true,
		Context:       ContextInline,
		Prompt: func(args string) string {
			return "Exit the application."
		},
	})
}

func registerConnectSkill() {
	RegisterBundledSkill(&Skill{
		Name:          "connect",
		Description:   "Connect an API provider by entering your API key",
		WhenToUse:     "When the user wants to configure a new API provider or update an existing API key",
		UserInvocable: true,
		Context:       ContextInline,
		Prompt: func(args string) string {
			if args != "" {
				return fmt.Sprintf(`The user wants to connect the provider %q.

Steps:
1. Ask the user for their API key for %s (remind them it will be stored locally in ~/.forge/auth.json with secure permissions)
2. Once they provide the key, save it using the auth store
3. Confirm the key was saved successfully

Known providers: anthropic, openai, openrouter, groq, google, mistral, xai, deepinfra`, args, args)
			}
			return `The user wants to connect an API provider.

Steps:
1. Show the list of known providers: anthropic, openai, openrouter, groq, google, mistral, xai, deepinfra
2. Ask which provider they want to connect
3. Ask for their API key (remind them it will be stored locally in ~/.forge/auth.json with secure permissions)
4. Save the key using the auth store
5. Confirm the key was saved successfully`
		},
	})
}

func registerProvidersSkill() {
	RegisterBundledSkill(&Skill{
		Name:          "providers",
		Description:   "List configured API providers and their connection status",
		WhenToUse:     "When the user wants to see which API providers are configured and their status",
		UserInvocable: true,
		Context:       ContextInline,
		Prompt: func(args string) string {
			return `Show the status of all known API providers.

For each provider (anthropic, openai, openrouter, groq, google, mistral, xai, deepinfra), show:
- Connection status: connected or not configured
- Auth source: where the API key is coming from (auth.json, config.yaml, env var, or none)

Use the auth package to check each provider. Format as a clean table or list.`
		},
	})
}
