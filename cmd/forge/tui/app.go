// Package tui implements the Bubbletea TUI for Forge CLI.
// It is the Go equivalent of Claude Code's Ink/React REPL (screens/REPL.tsx).
package tui

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/harmonica"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"

	"github.com/egoisutolabs/forge/internal/config"
	log "github.com/egoisutolabs/forge/internal/logger"
	"github.com/egoisutolabs/forge/internal/provider"
)

// AppModel is the root Bubbletea model for the Forge REPL.
type AppModel struct {
	// Conversation state
	messages []DisplayMessage

	// Bubbletea components
	input    textarea.Model
	spinner  spinner.Model
	viewport viewport.Model

	// Tool execution state
	activeTools    []ActiveToolInfo // ordered list of in-progress tools with details
	backgroundAgts int              // count of background sub-agents

	// Status bar data
	status StatusInfo

	// Terminal dimensions
	width  int
	height int

	// Pending permission request — huh-based confirm dialog (nil if none)
	permForm *PermissionForm

	// Pending AskUser request — huh-based select/multiselect (nil if none)
	askForm *AskUserForm

	// Connect dialog — multi-step provider connection (nil if none)
	connectDialog *ConnectDialog

	// Whether we're waiting for the engine
	processing bool

	// Cancel function for in-flight prompts
	cancelPrompt context.CancelFunc

	// Engine bridge (set after New)
	bridge *EngineBridge

	// Provider registry for model picker
	registry *provider.Registry

	// Loaded config (for connect dialog custom providers)
	cfg *config.Config

	// Key bindings
	keys KeyMap

	// Whether the viewport has been initialized
	viewportReady bool

	// Accumulated content being streamed (appended live).
	// Must be a pointer — strings.Builder cannot be copied after first use,
	// and Bubbletea copies the model on every Update call.
	streamBuf *strings.Builder

	// Input history (up/down arrow navigation)
	history *History

	// Slash command autocomplete
	autocomplete *Autocomplete

	// @ mention popup
	mentions *MentionPopup

	// Model picker dialog (Ctrl+M) — nil when inactive
	modelPicker *ModelPickerDialog

	// Quick Open file finder dialog (Ctrl+O) — nil when inactive
	quickOpen *QuickOpenDialog

	// Global search dialog (Ctrl+F) — nil when inactive
	globalSearch *GlobalSearchDialog

	// Typeahead ghost text (predicted completion)
	typeahead *Typeahead

	// History search overlay (Ctrl+R)
	historySearch *HistorySearch

	// Input undo/redo stack
	undoStack *UndoStack
	// Theme
	theme Theme

	// Splash screen data (shown when conversation is empty)
	splash *SplashScreen

	// Session start time (for header timer)
	sessionStart time.Time

	// userScrolledUp is true when the user has manually scrolled away from
	// the bottom of the conversation. When true, refreshViewport will not
	// auto-scroll to bottom, letting the user read earlier messages.
	userScrolledUp bool

	// Whether to show the keyboard shortcut overlay
	showHelp bool

	// Emacs keybindings
	emacsKeys *EmacsKeys

	// Draft stash — auto-stashes long input that gets cleared
	peakInputLen      int    // highest char count seen in current input
	stashedDraft      string // the stashed draft text
	hasShownStashHint bool   // only show hint once per session
	showStashHint     bool   // currently showing stash hint
	stashHintTimer    *time.Timer
	prevInputValue    string // previous input value for stash detection

	// Diff dialog (Ctrl+D)
	diffDialog *DiffDialog

	// Dialog queue — centralized dialog coordination
	dialogQueue *DialogQueue

	// Footer navigation pills
	footerNav *FooterNav

	// Cost threshold tracking
	costState *CostThresholdState

	// Active cost dialog (tracked for updateCostDialog)
	activeCostDialog *CostDialog

	// Spinner tips — contextual tips shown below spinner during processing
	spinnerTips *SpinnerTips

	// Idle return dialog — tracks activity and shows dialog after long idle
	idleState *IdleState

	// Streaming complete lines — buffer partial lines to prevent flicker
	partialLineBuf string

	// Input suppression — ignore keys briefly after dialog closes
	inputSuppressed bool
	suppressUntil   time.Time

	// Sticky prompt header — pin last user prompt when scrolled up
	lastUserPromptIdx int    // index of the last user message (-1 if none)
	lastUserPrompt    string // text of the last user message

	// Unseen message divider state: tracks new messages while user is scrolled up.
	// -1 means no divider active.
	unseenDividerIdx int
	unseenCount      int

	// Tasks panel state
	tasksPanel *TasksPanel

	// Animation state
	animations     bool             // whether spring animations are enabled
	scrollCfg      harmonica.Spring // spring config for scroll
	uiCfg          harmonica.Spring // spring config for UI elements
	scrollAnim     SpringState      // viewport scroll position
	acAnim         SpringState      // autocomplete popup height
	mentionAnim    SpringState      // mention popup height
	permAnim       SpringState      // permission dialog entrance height
	collapseAnim   *CollapseAnimation
	collapseSpring SpringState
	animTicking    bool // true when an animation tick is scheduled

	// Central animation clock — single goroutine drives all animations.
	animClock  *AnimClock
	clockSubID int              // subscription ID for Bubbletea event loop
	clockCh    <-chan time.Time // receives ticks from the clock

	// Terminal focus detection
	focus *TerminalFocus

	// Notification queue
	notifications *NotifQueue

	// Stall detection
	stall *StallState

	// Background agent management
	bgState *BackgroundState

	// Input mode (normal, bash, processing, plan)
	inputMode InputMode

	// Auto-approval shimmer animation
	shimmer *ShimmerState

	// Session rule store for editable permission patterns
	sessionRules *SessionRuleStore

	// Selection management — clear text selection on most keystrokes
	selection *SelectionState

	// Scroll coalescing — batch scroll events per frame
	scrollCoalesce *ScrollCoalescer

	// Terminal title — update terminal window/tab title on state changes
	titleState *TitleState

	// Progressive session loading — load messages in batches when resuming
	sessionLoader *SessionLoadState
}

// New creates a new AppModel.
func New(bridge *EngineBridge) AppModel {
	theme := InitTheme()

	ta := textarea.New()
	ta.Placeholder = "Message Forge…"
	ta.Focus()
	ta.CharLimit = 0
	ta.SetWidth(80)
	ta.SetHeight(3)
	ta.ShowLineNumbers = false
	ta.KeyMap.InsertNewline.SetKeys("alt+enter", "ctrl+j")

	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = theme.SpinnerStyle

	registry := NewCommandRegistry()
	hist := NewHistory(500)
	// Optionally load persisted history
	if path := DefaultHistoryPath(); path != "" {
		_ = hist.LoadFromFile(path) // ignore error — file may not exist
	}

	cwd := bridge.eng.Config().Cwd
	mentionSources := []MentionSource{
		&FileMentionSource{Cwd: cwd},
		&SkillMentionSource{Registry: registry},
		&AgentMentionSource{Dirs: agentDirs(cwd)},
	}

	animations := theme.Config.Animations == nil || *theme.Config.Animations

	clock := NewAnimClock()

	return AppModel{
		messages:      []DisplayMessage{},
		input:         ta,
		spinner:       sp,
		viewport:      viewport.New(80, 20),
		keys:          DefaultKeyMap(),
		bridge:        bridge,
		streamBuf:     &strings.Builder{},
		history:       hist,
		autocomplete:  NewAutocomplete(registry),
		mentions:      NewMentionPopup(mentionSources...),
		typeahead:     NewTypeahead(hist),
		historySearch: NewHistorySearch(hist),
		undoStack:     NewUndoStack(50, 1000*time.Millisecond),
		theme:         theme,
		sessionStart:  time.Now(),
		status: StatusInfo{
			Model: bridge.Model(),
		},
		splash: &SplashScreen{
			Info: SplashInfo{
				Version:      "v0.1.0",
				Model:        bridge.Model(),
				Cwd:          cwd,
				CommandCount: len(registry.Commands()),
				SkillNames:   skillNames(registry),
			},
			Theme: theme,
		},
		emacsKeys:         &EmacsKeys{KillRing: NewKillRing(10)},
		animations:        animations,
		scrollCfg:         newScrollSpring(),
		uiCfg:             newUISpring(),
		animClock:         clock,
		focus:             NewTerminalFocus(clock),
		notifications:     NewNotifQueue(),
		stall:             NewStallState(),
		bgState:           NewBackgroundState(),
		inputMode:         ModeNormal,
		unseenDividerIdx:  -1,
		tasksPanel:        NewTasksPanel(),
		spinnerTips:       NewSpinnerTips(),
		idleState:         NewIdleState(),
		lastUserPromptIdx: -1,
		dialogQueue:       NewDialogQueue(),
		footerNav:         NewFooterNav(),
		costState:         NewCostThresholdState(),
		shimmer:           NewShimmerState(),
		sessionRules:      NewSessionRuleStore(),
		selection:         NewSelectionState(),
		scrollCoalesce:    NewScrollCoalescer(3),
		titleState:        NewTitleState(cwd),
	}
}

// SetRegistry sets the provider registry for the model picker.
func (m *AppModel) SetRegistry(reg *provider.Registry) {
	m.registry = reg
}

// SetConfig sets the loaded config for the connect dialog and other features.
func (m *AppModel) SetConfig(cfg *config.Config) {
	m.cfg = cfg
}

// Init starts the spinner and returns any initial commands.
func (m AppModel) Init() tea.Cmd {
	// Set initial terminal title
	m.titleState.SetIdle()
	WriteTitleSequence(m.titleState.Current())

	return tea.Batch(
		textarea.Blink,
		m.spinner.Tick,
	)
}

// Update handles all incoming messages and events.
func (m AppModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {

	// ---- Terminal resize ----
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		headerH := 1 // title
		statusH := 1 // status bar
		inputH := m.inputHeight()
		toolH := len(m.activeTools) + 1
		vpHeight := m.height - headerH - statusH - inputH - toolH - 2
		if vpHeight < 3 {
			vpHeight = 3
		}
		if !m.viewportReady {
			m.viewport = viewport.New(m.width, vpHeight)
			m.viewport.SetContent(renderConversation(m.messages, m.width, nil, m.splash, m.unseenDividerIdx, m.unseenCount))
			m.viewportReady = true
		} else {
			m.viewport.Width = m.width
			m.viewport.Height = vpHeight
		}
		m.input.SetWidth(m.width - 4)
		m.refreshViewport()

	// ---- Keyboard input ----
	case tea.KeyMsg:
		// Record activity for idle detection
		m.idleState.RecordActivity()

		// Selection management: clear selection on most keystrokes
		if shouldClearSelection(msg.String()) {
			m.selection.Clear()
		}
		// Model picker active — it owns all keyboard input
		if m.modelPicker != nil {
			return m.updateModelPicker(msg)
		}

		// Quick Open dialog active — it owns all keyboard input
		if m.quickOpen != nil {
			return m.updateQuickOpen(msg)
		}

		// Global Search dialog active — it owns all keyboard input
		if m.globalSearch != nil {
			return m.updateGlobalSearch(msg)
		}

		// Permission form active — forward keys to huh form
		if m.permForm != nil {
			return m.updatePermForm(msg)
		}

		// AskUser form active — forward keys to huh form
		if m.askForm != nil {
			return m.updateAskForm(msg)
		}

		// Connect dialog active — forward keys to huh form
		if m.connectDialog != nil {
			return m.updateConnectDialog(msg)
		}

		// Idle dialog active — forward keys to huh form
		if m.idleState.IsDialogShowing() {
			return m.updateIdleForm(msg)
		}

		// Cost dialog active — forward keys to cost dialog
		if cd := m.activeCostDialog; cd != nil && m.dialogQueue.ActiveID() == fmt.Sprintf("cost-%.0f", cd.threshold) {
			return m.updateCostDialog(msg, cd)
		}
		// Tasks panel active — forward keys to panel
		if m.tasksPanel != nil && m.tasksPanel.Expanded {
			agents := m.bgState.AllAgents()
			switch msg.String() {
			case "esc":
				m.tasksPanel.Collapse()
			case "up":
				m.tasksPanel.PrevAgent(len(agents))
			case "down":
				m.tasksPanel.NextAgent(len(agents))
			case "x":
				sel := m.tasksPanel.Selected
				if sel >= 0 && sel < len(agents) && !agents[sel].Completed {
					name := agents[sel].Name
					m.bgState.MarkCompleted(name)
					m.addSystemMessage(fmt.Sprintf("  ✗ Stopped agent: %s", name))
				}
			}
			return m, nil
		}

		// Diff dialog active — forward keys to diff dialog
		if m.diffDialog != nil {
			if m.diffDialog.HandleKey(msg.String()) {
				m.diffDialog = nil
			}
			return m, nil
		}

		// History search active — forward keys to history search
		if m.historySearch.Active() {
			return m.updateHistorySearch(msg)
		}
		// Input suppression — ignore non-escape keys briefly after dialog closes
		if m.inputSuppressed && time.Now().Before(m.suppressUntil) {
			if msg.String() != "esc" {
				return m, nil
			}
		} else {
			m.inputSuppressed = false
		}

		// Printable character input (KeyRunes) — always pass to textarea first.
		// This ensures special characters (@, $, #, etc.) are never swallowed
		// by the switch cases below.
		// Footer navigation — typing exits footer mode
		if msg.Type == tea.KeyRunes && m.footerNav.Active() {
			m.footerNav.Exit()
		}

		if msg.Type == tea.KeyRunes && !m.processing {
			// Bash mode: "!" at start of empty input triggers bash mode
			// Only on single-char insert (not paste)
			if len(msg.Runes) == 1 && msg.Runes[0] == '!' && m.input.Value() == "" && m.inputMode == ModeNormal {
				m.inputMode = ModeBash
				// Don't insert the "!" into the input
				return m, nil
			}

			var inputCmd tea.Cmd
			m.input, inputCmd = m.input.Update(msg)
			cmds = append(cmds, inputCmd)

			// Exit bash mode if input is cleared
			if m.inputMode == ModeBash && m.input.Value() == "" {
				m.inputMode = ModeNormal
			}

			m.updateAutocomplete(&cmds)
			m.updateMentions(&cmds)
			m.typeahead.Update(m.input.Value())
			m.undoStack.Push(m.input.Value(), len(m.input.Value()))
			if m.trackDraftStash() {
				cmds = append(cmds, stashHintDismiss())
			}
			return m, tea.Batch(cmds...)
		}

		switch {
		case msg.String() == "ctrl+c":
			if m.processing && m.cancelPrompt != nil {
				// First ctrl+c cancels current prompt
				m.cancelPrompt()
				return m, nil
			}
			return m, tea.Quit

		case msg.String() == "ctrl+_":
			// Toggle shortcut help overlay (Ctrl+/ sends ctrl+_ / 0x1F in most terminals)
			m.showHelp = !m.showHelp
			return m, nil

		// Ctrl+B — background current processing
		case msg.String() == "ctrl+b" && m.processing && !m.bgState.IsBackgrounded():
			m.bgState.Background("main")
			m.bgState.RecordInteraction()
			m.notifications.Push(Notification{
				Key:      "bg-main",
				Message:  "Agent backgrounded — you can type while it works",
				Priority: NotifMedium,
			})
			cmds = append(cmds, bgStallCheck(), bgEvictionCheck())
			return m, tea.Batch(cmds...)

		// Ctrl+M — Model Picker
		case msg.String() == "ctrl+m" && !m.processing:
			if m.registry != nil {
				m.modelPicker = NewModelPickerFromRegistry(m.registry, m.bridge.Model())
			}
			return m, nil

		// Ctrl+O — Quick Open file finder
		case msg.String() == "ctrl+o":
			m.quickOpen = NewQuickOpenDialog(m.bridge.eng.Config().Cwd)
			return m, m.quickOpen.Open()

		// Ctrl+F — Global Search (ripgrep)
		case msg.String() == "ctrl+f":
			m.globalSearch = NewGlobalSearchDialog(m.bridge.eng.Config().Cwd)
			m.globalSearch.Open()
			return m, nil

		// Emacs navigation — Ctrl+A/E (use textarea methods directly)
		case msg.String() == "ctrl+a" && !m.processing:
			m.emacsKeys.KillRing.BreakAccumulate()
			m.input.CursorStart()
			return m, nil
		case msg.String() == "ctrl+e" && !m.processing:
			m.emacsKeys.KillRing.BreakAccumulate()
			m.input.CursorEnd()
			return m, nil
		// Emacs kill/yank — Ctrl+K/U/W/Y
		case msg.String() == "ctrl+k" && !m.processing,
			msg.String() == "ctrl+u" && !m.processing,
			msg.String() == "ctrl+w" && !m.processing,
			msg.String() == "ctrl+y" && !m.processing:
			val := m.input.Value()
			cursor := len(val) // approximate: textarea doesn't expose cursor byte offset
			newVal, newCursor, handled := m.emacsKeys.HandleKey(msg.String(), val, cursor)
			if handled {
				m.input.SetValue(newVal)
				m.input.SetCursor(newCursor)
				if m.trackDraftStash() {
					return m, stashHintDismiss()
				}
			}
			return m, nil

		// Ctrl+Z — undo input, or restore stashed draft as fallback
		case msg.String() == "ctrl+z" && !m.processing:
			if entry, ok := m.undoStack.Undo(); ok {
				m.input.SetValue(entry.Text)
				m.input.SetCursor(entry.Cursor)
				m.typeahead.Update(entry.Text)
			} else if m.stashedDraft != "" {
				m.input.SetValue(m.stashedDraft)
				m.stashedDraft = ""
				m.showStashHint = false
			}
			return m, nil

		// Ctrl+Shift+Z — redo input
		case msg.String() == "ctrl+shift+z" && !m.processing:
			if entry, ok := m.undoStack.Redo(); ok {
				m.input.SetValue(entry.Text)
				m.input.SetCursor(entry.Cursor)
				m.typeahead.Update(entry.Text)
			}
			return m, nil

		// Ctrl+R — open history search
		case msg.String() == "ctrl+r" && !m.processing:
			m.historySearch.Open()
			return m, nil

		// Ctrl+D — open diff dialog
		case msg.String() == "ctrl+d" && !m.processing:
			if d := NewDiffDialog(m.messages, m.width, m.height, m.theme); d != nil {
				m.diffDialog = d
			}
			return m, nil

		// Ctrl+T — toggle tasks/agents panel
		case msg.String() == "ctrl+t":
			m.tasksPanel.Toggle()
			agents := m.bgState.AllAgents()
			m.tasksPanel.ClampSelected(len(agents))
			return m, nil
		// Ctrl+B — toggle footer navigation mode
		case msg.String() == "ctrl+b":
			if m.footerNav.Active() {
				m.footerNav.Exit()
			} else {
				m.footerNav.BuildPills(m.backgroundAgts, m.status)
				m.footerNav.Enter()
			}
			return m, nil

		case msg.String() == "esc":
			if m.inputMode == ModeBash {
				m.inputMode = ModeNormal
				m.input.Reset()
				return m, nil
			}
			if m.showHelp {
				m.showHelp = false
				return m, nil
			}
			if m.mentions.Active() {
				m.mentions.Hide()
				return m, nil
			}
			if m.autocomplete.Visible() {
				m.autocomplete.Hide()
				return m, nil
			}
			if m.processing && m.cancelPrompt != nil {
				m.cancelPrompt()
			}

		// Footer navigation: left/right/enter/esc when in footer mode
		case m.footerNav.Active() && (msg.String() == "left" || msg.String() == "right" || msg.String() == "enter" || msg.String() == "esc"):
			handled, action := m.footerNav.HandleKey(msg)
			if handled && action != "" {
				m.addSystemMessage(fmt.Sprintf("  Footer action: %s", action))
			}
			if handled {
				return m, nil
			}

		// Mention navigation: Tab/Shift+Tab when mention popup is active
		case msg.String() == "tab" && m.mentions.Active():
			m.mentions.Next()
			return m, nil
		case msg.String() == "shift+tab" && m.mentions.Active():
			m.mentions.Prev()
			return m, nil

		// Autocomplete navigation: Tab/Shift+Tab when dropdown is visible
		case msg.String() == "tab" && m.autocomplete.Visible():
			m.autocomplete.Next()
			return m, nil
		case msg.String() == "shift+tab" && m.autocomplete.Visible():
			m.autocomplete.Prev()
			return m, nil

		// Tab accepts typeahead ghost text, or toggles collapse
		case msg.String() == "tab" && !m.processing:
			if m.typeahead.HasGhost() {
				if full, ok := m.typeahead.Accept(m.input.Value()); ok {
					m.input.SetValue(full)
					m.input.SetCursor(len(full))
					m.undoStack.Push(full, len(full))
					return m, nil
				}
			}
			if m.animations {
				if cmd := m.startCollapseAnimation(); cmd != nil {
					return m, cmd
				}
			} else if toggleLastToolCollapse(m.messages) {
				m.refreshViewport()
				return m, nil
			}

		case msg.String() == "enter" && !m.processing:
			// Bash mode: send input directly as a Bash tool call
			if m.inputMode == ModeBash {
				text := strings.TrimSpace(m.input.Value())
				if text == "" {
					return m, nil
				}
				m.input.Reset()
				m.inputMode = ModeNormal
				m.history.Add("!" + text)
				m.history.Reset()
				return m, m.submitBashDirect(text)
			}

			// If mention popup is open, accept selection
			if m.mentions.Active() {
				if sel := m.mentions.Selected(); sel != nil {
					m.insertMention(sel.Value)
					m.mentions.Hide()
					return m, nil
				}
			}
			// If autocomplete is open, accept selection
			if m.autocomplete.Visible() {
				if sel := m.autocomplete.Selected(); sel != nil {
					m.input.SetValue("/" + sel.Name + " ")
					m.autocomplete.Hide()
					return m, nil
				}
			}

			text := strings.TrimSpace(m.input.Value())
			if text == "" {
				return m, nil
			}
			m.input.Reset()
			m.history.Add(text)
			m.history.Reset()
			m.autocomplete.Hide()
			m.mentions.Hide()
			m.typeahead.Clear()
			m.undoStack.Clear()
			// Clear draft stash on submit
			m.stashedDraft = ""
			m.peakInputLen = 0
			m.prevInputValue = ""
			m.dismissStashHint()
			// Check if user was idle — show dialog instead of submitting
			if m.checkIdleOnSubmit() {
				// Put the text back in the input for after the dialog
				m.input.SetValue(text)
				return m, nil
			}
			return m, m.submitPrompt(text)

		// Popup navigation: up/down when mention popup or autocomplete is active
		case msg.String() == "up" && !m.processing && m.mentions.Active():
			m.mentions.Prev()
			return m, nil
		case msg.String() == "down" && !m.processing && m.mentions.Active():
			m.mentions.Next()
			return m, nil
		case msg.String() == "up" && !m.processing && m.autocomplete.Visible():
			m.autocomplete.Prev()
			return m, nil
		case msg.String() == "down" && !m.processing && m.autocomplete.Visible():
			m.autocomplete.Next()
			return m, nil

		// History navigation: up/down when not processing.
		// If history doesn't handle the key, pass to textarea for cursor movement.
		case msg.String() == "up" && !m.processing:
			if entry, ok := m.history.Up(m.input.Value()); ok {
				m.input.SetValue(entry)
				return m, nil
			}
			var inputCmd tea.Cmd
			m.input, inputCmd = m.input.Update(msg)
			cmds = append(cmds, inputCmd)

		case msg.String() == "down" && !m.processing:
			if m.history.Browsing() {
				if entry, ok := m.history.Down(); ok {
					m.input.SetValue(entry)
					return m, nil
				}
			}
			var inputCmd tea.Cmd
			m.input, inputCmd = m.input.Update(msg)
			cmds = append(cmds, inputCmd)

		case msg.String() == "pgup":
			halfPage := m.viewport.Height / 2
			m.scrollCoalesce.Accumulate(-halfPage)
			if m.animations {
				fHP := float64(halfPage)
				if !m.scrollAnim.Active {
					m.scrollAnim.Pos = float64(m.viewport.YOffset)
					m.scrollAnim.Target = float64(m.viewport.YOffset)
				}
				target := m.scrollAnim.Target - fHP
				if target < 0 {
					target = 0
				}
				m.scrollAnim.Start(target)
				m.scheduleAnimTick(&cmds)
			} else if delta := m.scrollCoalesce.Flush(); delta != 0 {
				m.viewport.SetYOffset(m.viewport.YOffset + delta)
			}
			m.userScrolledUp = true

		case msg.String() == "pgdown":
			halfPage := m.viewport.Height / 2
			m.scrollCoalesce.Accumulate(halfPage)
			if m.animations {
				fHP := float64(halfPage)
				if !m.scrollAnim.Active {
					m.scrollAnim.Pos = float64(m.viewport.YOffset)
					m.scrollAnim.Target = float64(m.viewport.YOffset)
				}
				maxOffset := float64(m.viewport.TotalLineCount() - m.viewport.Height)
				if maxOffset < 0 {
					maxOffset = 0
				}
				target := m.scrollAnim.Target + fHP
				if target > maxOffset {
					target = maxOffset
				}
				m.scrollAnim.Start(target)
				m.scheduleAnimTick(&cmds)
			} else if delta := m.scrollCoalesce.Flush(); delta != 0 {
				m.viewport.SetYOffset(m.viewport.YOffset + delta)
			}
			// Update spinner color based on stall level
			if m.processing {
				stallLevel := m.stall.Check(time.Now())
				m.spinner.Style = stallSpinnerStyle(stallLevel, m.theme)
			}
			m.userScrolledUp = !m.viewport.AtBottom()
			if !m.userScrolledUp {
				m.clearUnseenDivider()
			}
		default:
			// Pass all other keys to textarea
			var inputCmd tea.Cmd
			m.input, inputCmd = m.input.Update(msg)
			cmds = append(cmds, inputCmd)
			m.updateAutocomplete(&cmds)
			m.updateMentions(&cmds)
		}

	// ---- Spinner tick ----
	case spinner.TickMsg:
		var spCmd tea.Cmd
		m.spinner, spCmd = m.spinner.Update(msg)
		cmds = append(cmds, spCmd)

	// ---- Clock tick (central animation clock drives all springs) ----
	case ClockTickMsg:
		if m.clockCh != nil {
			cmds = append(cmds, listenForClockTicks(m.clockCh))
		}
		if m.animations {
			m.scrollAnim.Update(m.scrollCfg)
			if m.scrollAnim.Active {
				m.viewport.SetYOffset(int(math.Round(m.scrollAnim.Pos)))
			}
			m.acAnim.Update(m.uiCfg)
			m.mentionAnim.Update(m.uiCfg)
			m.permAnim.Update(m.uiCfg)
			if m.collapseAnim != nil && m.collapseSpring.Update(m.uiCfg) {
				m.collapseAnim.Height = m.collapseSpring.Pos
				m.refreshViewport()
			} else if m.collapseAnim != nil {
				idx := m.collapseAnim.MsgIndex
				if m.collapseAnim.Collapsing {
					m.messages[idx].Collapsed = true
				}
				m.collapseAnim = nil
				m.refreshViewport()
			}
		}

	// ---- Terminal focus/blur ----
	case tea.FocusMsg:
		if m.focus != nil {
			m.focus.HandleFocus()
		}

	case tea.BlurMsg:
		if m.focus != nil {
			m.focus.HandleBlur()
		}

	// ---- Animation tick (60fps spring updates) ----
	case AnimationTickMsg:
		m.animTicking = false
		if !m.animations {
			return m, nil
		}
		anyActive := false

		// Scroll spring
		if m.scrollAnim.Update(m.scrollCfg) {
			m.viewport.SetYOffset(int(math.Round(m.scrollAnim.Pos)))
			anyActive = true
		} else if m.scrollAnim.Pos == m.scrollAnim.Target && m.viewport.YOffset != int(m.scrollAnim.Target) {
			m.viewport.SetYOffset(int(math.Round(m.scrollAnim.Target)))
		}

		// Autocomplete popup spring
		if m.acAnim.Update(m.uiCfg) {
			anyActive = true
		}

		// Mention popup spring
		if m.mentionAnim.Update(m.uiCfg) {
			anyActive = true
		}

		// Permission dialog entrance spring
		if m.permAnim.Update(m.uiCfg) {
			anyActive = true
		}

		// Collapse animation spring
		if m.collapseAnim != nil && m.collapseSpring.Update(m.uiCfg) {
			m.collapseAnim.Height = m.collapseSpring.Pos
			m.refreshViewport()
			anyActive = true
		} else if m.collapseAnim != nil {
			// Settled — finalize collapse state
			idx := m.collapseAnim.MsgIndex
			if m.collapseAnim.Collapsing {
				m.messages[idx].Collapsed = true
			}
			m.collapseAnim = nil
			m.refreshViewport()
		}

		if anyActive {
			m.animTicking = true
			cmds = append(cmds, animationTick())
		}

	// ---- Notification tick (auto-dismiss expired notifications) ----
	case NotifTickMsg:
		m.notifications.Tick(time.Now())
		if m.notifications.Len() > 0 {
			cmds = append(cmds, notifTick())
		}

	// ---- Stall tick (check for stalled processing) ----
	case StallTickMsg:
		if m.processing {
			m.stall.Check(time.Now())
			cmds = append(cmds, stallTick())
		}

	// ---- Auto-background timeout ----
	case AutoBackgroundMsg:
		if m.processing && !m.bgState.IsBackgrounded() {
			if m.bgState.CheckAutoBackground(time.Now()) {
				m.bgState.Background("main")
				m.notifications.Push(Notification{
					Key:      "auto-bg",
					Message:  "Auto-backgrounded after inactivity — you can type while it works",
					Priority: NotifMedium,
				})
				cmds = append(cmds, notifTick(), bgStallCheck(), bgEvictionCheck())
			}
		}

	// ---- Background stall watchdog ----
	case BgStallCheckMsg:
		now := time.Now()
		stalledAgents := m.bgState.CheckStalls(now)
		for _, name := range stalledAgents {
			m.notifications.Push(Notification{
				Key:      "bg-stall-" + name,
				Message:  fmt.Sprintf("Background agent %q may be waiting for input", name),
				Priority: NotifHigh,
			})
			cmds = append(cmds, notifTick())
		}
		if len(m.bgState.Agents) > 0 {
			cmds = append(cmds, bgStallCheck())
		}

	// ---- Background agent eviction ----
	case BgEvictionCheckMsg:
		evicted := m.bgState.EvictCompleted(time.Now())
		_ = evicted
		if len(m.bgState.Agents) > 0 {
			cmds = append(cmds, bgEvictionCheck())
		}

	// ---- Tip rotation tick ----
	case TipRotateMsg:
		if m.processing {
			m.spinnerTips.RotateIfDue(time.Now(), m.activeTools)
			cmds = append(cmds, tipRotateTick())
		}

	// ---- Engine events ----
	case StreamTextMsg:
		m.stall.OnStreamText()
		m.streamBuf.WriteString(msg.Text)
		// Pick a spinner tip on first stream text of the turn
		if !m.spinnerTips.Picked() {
			m.spinnerTips.PickTip(m.activeTools)
			cmds = append(cmds, tipRotateTick())
		}
		// Only display up to the last newline boundary to prevent partial-word flicker
		m.upsertStreamingCompleteLines()
		m.refreshViewport()
		log.Debug("tui: StreamTextMsg len=%d totalBuf=%d messages=%d viewportReady=%v width=%d",
			len(msg.Text), m.streamBuf.Len(), len(m.messages), m.viewportReady, m.width)

	case ToolStartMsg:
		m.stall.OnToolStart()
		// Flush any pending stream buffer into a committed message
		m.flushStreamBuf()
		if !containsTool(m.activeTools, msg.ID) {
			m.activeTools = append(m.activeTools, ActiveToolInfo{
				Name:      msg.Name,
				ID:        msg.ID,
				Detail:    msg.Detail,
				StartTime: time.Now(),
			})
		}
		m.refreshViewport()

	case ToolDoneMsg:
		toolDetail := findToolDetail(m.activeTools, msg.ID)
		m.activeTools = removeTool(m.activeTools, msg.ID)
		m.stall.OnToolDone(len(m.activeTools))
		// Add tool result to conversation — collapsed by default, expanded on error
		m.messages = append(m.messages, DisplayMessage{
			Role:      "tool",
			ToolName:  msg.Name,
			ToolID:    msg.ID,
			Detail:    toolDetail,
			Content:   msg.Result,
			IsError:   msg.IsError,
			Collapsed: !msg.IsError,
		})
		m.trackUnseenMessage()
		m.refreshViewport()

	case PromptDoneMsg:
		m.flushStreamBuf()
		m.spinnerTips.Reset()
		m.processing = false
		m.activeTools = nil
		m.stall.OnProcessingDone()
		m.bgState.OnProcessingDone()
		m.inputMode = ModeNormal
		m.cancelPrompt = nil
		if msg.Result != nil {
			m.status.InputTokens += msg.Result.TotalUsage.InputTokens
			m.status.OutTokens += msg.Result.TotalUsage.OutputTokens
			m.status.CostUSD += msg.Result.TotalCostUSD
		}
		m.status.Processing = false
		// Update terminal title back to idle
		m.titleState.SetIdle()
		WriteTitleSequence(m.titleState.Current())
		m.refreshViewport()

	case CostUpdateMsg:
		m.status.InputTokens = msg.Usage.InputTokens
		m.status.OutTokens = msg.Usage.OutputTokens
		m.status.CostUSD = msg.CostUSD

		// Check cost thresholds
		if threshold := m.costState.CheckThreshold(msg.CostUSD); threshold > 0 {
			cd := NewCostDialog(msg.CostUSD, threshold, m.theme)
			initCmd := cd.form.Init()
			m.activeCostDialog = cd
			dialogCmd := m.dialogQueue.Push(QueuedDialog{
				Priority: DialogCostWarning,
				ID:       fmt.Sprintf("cost-%.0f", threshold),
				Render:   func() string { return cd.form.View() },
				HandleKey: func(key tea.KeyMsg) (bool, bool) {
					return true, false
				},
				InitCmd: initCmd,
			})
			if dialogCmd != nil {
				cmds = append(cmds, dialogCmd)
			}
		}

	case PermissionRequestMsg:
		pf := NewPermissionForm(&msg, m.theme)
		initCmd := pf.form.Init()
		m.permForm = pf
		cmds = append(cmds, initCmd)
		if m.animations {
			rendered := pf.form.View()
			targetHeight := float64(countLines(rendered) + 2)
			m.permAnim.StartFrom(0, targetHeight)
			m.scheduleAnimTick(&cmds)
		}

	case AskUserRequestMsg:
		af := NewAskUserForm(msg.Questions, m.theme)
		initCmd := af.form.Init()
		m.askForm = af
		m.askForm.responseCh = msg.ResponseCh
		cmds = append(cmds, initCmd)

	case AgentSpawnMsg:
		if msg.Background {
			m.backgroundAgts++
		}
		label := msg.Name
		if msg.Background {
			label += " (background)"
		}
		m.addSystemMessage(fmt.Sprintf("  🤖 Agent spawned: %s", label))

	case AgentDoneMsg:
		if m.backgroundAgts > 0 {
			m.backgroundAgts--
		}
		m.addSystemMessage(fmt.Sprintf("  ✓ Agent completed: %s", msg.Name))

	case CompactDoneMsg:
		m.processing = false
		m.status.Processing = false
		m.addSystemMessage("  Conversation compacted.")

	case StashHintDismissMsg:
		m.showStashHint = false

	case AutoApprovalShimmerMsg:
		m.shimmer.Trigger(msg.Command, msg.ToolName)
		cmds = append(cmds, shimmerTick())

	case ShimmerTickMsg:
		if m.shimmer.Active {
			done := m.shimmer.Advance(1.0 / 9.0) // ~300ms at 30fps
			if !done {
				cmds = append(cmds, shimmerTick())
			}
		}
	// ---- Progressive session loading ----
	case SessionLoadBatchMsg:
		if m.sessionLoader != nil && !m.sessionLoader.Done() {
			batch := m.sessionLoader.NextBatch()
			m.messages = append(m.messages, batch...)
			m.refreshViewport()
			if !m.sessionLoader.Done() {
				cmds = append(cmds, scheduleNextBatch())
			}
		}

	case SessionLoadDoneMsg:
		m.sessionLoader = nil
		m.refreshViewport()

	case ErrorMsg:
		m.processing = false
		m.activeTools = nil
		m.cancelPrompt = nil
		m.status.Processing = false
		m.messages = append(m.messages, DisplayMessage{
			Role:    "error",
			Content: msg.Err.Error(),
		})
		m.refreshViewport()

	// ---- Quick Open file scan completed ----
	case quickOpenScanDoneMsg:
		if m.quickOpen != nil {
			m.quickOpen.HandleScanDone(msg)
		}

	// ---- Global Search rg completed ----
	case globalSearchDoneMsg:
		if m.globalSearch != nil {
			m.globalSearch.HandleSearchDone(msg)
		}

	// ---- Global Search debounce fired ----
	case globalSearchDebounceMsg:
		if m.globalSearch != nil && m.globalSearch.query == msg.query {
			return m, m.globalSearch.StartSearch(msg.query)
		}
	}

	// Always tick the spinner while processing
	if m.processing {
		cmds = append(cmds, m.spinner.Tick)
	}

	return m, tea.Batch(cmds...)
}

// View composes the full TUI.
func (m AppModel) View() string {
	if !m.viewportReady {
		return "Initializing…"
	}

	// Full-screen diff dialog
	if m.diffDialog != nil {
		return m.diffDialog.Render()
	}

	var b strings.Builder

	// ---- Header bar ----
	b.WriteString(m.renderHeader())
	b.WriteByte('\n')

	// ---- Sticky prompt header (when scrolled up) ----
	if m.userScrolledUp && m.lastUserPromptIdx >= 0 && m.lastUserPrompt != "" {
		b.WriteString(m.renderStickyPrompt())
		b.WriteByte('\n')
	}

	// ---- Viewport (conversation) or dialog overlay ----
	if m.modelPicker != nil {
		b.WriteString(m.modelPicker.Render(m.width, m.viewport.Height, m.theme))
		b.WriteByte('\n')
	} else if m.quickOpen != nil {
		b.WriteString(m.quickOpen.Render(m.width, m.viewport.Height, m.theme))
		b.WriteByte('\n')
	} else if m.globalSearch != nil {
		b.WriteString(m.globalSearch.Render(m.width, m.viewport.Height, m.theme))
		b.WriteByte('\n')
	} else {
		b.WriteString(m.viewport.View())
		b.WriteByte('\n')
	}

	// ---- Tool status ----
	if len(m.activeTools) > 0 {
		b.WriteString(renderActiveToolsDetailed(m.activeTools, m.spinner.View(), m.width))
	}

	// ---- Spinner tip (below tools during processing) ----
	if m.processing && m.spinnerTips.Current() != "" {
		b.WriteString(renderSpinnerTip(m.spinnerTips.Current(), m.width, m.theme))
	}

	// ---- Idle return dialog ----
	if m.idleState.IsDialogShowing() {
		b.WriteString(renderIdleDialog(m.idleState, m.width, m.theme))
	}

	// ---- Auto-approval shimmer ----
	if m.shimmer.Active {
		b.WriteString(m.shimmer.Render(m.width))
		b.WriteByte('\n')
	}

	// ---- Permission request (huh form) ----
	if m.permForm != nil {
		// Destructive warning banner (above the form)
		if m.permForm.destructiveWarning != nil {
			b.WriteString(m.permForm.destructiveWarning.Render(m.width))
			b.WriteByte('\n')
		}

		rendered := "\n" + m.permForm.form.View() + "\n"
		if m.animations && m.permAnim.Active {
			rendered = clipToLines(rendered, maxInt(1, int(math.Round(m.permAnim.Pos))))
		}
		b.WriteString(rendered)

		// Rule prefix hint (below the form)
		if m.permForm.rulePrefix != "" {
			hintStyle := lipgloss.NewStyle().Faint(true)
			b.WriteString(hintStyle.Render(fmt.Sprintf("  Allow all matching: %s  (press 'a' to add rule)", m.permForm.rulePrefix)))
			b.WriteByte('\n')
		}
	}

	// ---- AskUser form (huh select/multiselect) ----
	if m.askForm != nil {
		b.WriteString("\n" + m.askForm.form.View() + "\n")
	}

	// ---- Connect dialog (huh select/input) ----
	if m.connectDialog != nil {
		b.WriteString("\n" + m.connectDialog.form.View() + "\n")
	}

	// ---- Cost dialog (from dialog queue) ----
	if m.activeCostDialog != nil && m.dialogQueue != nil && m.dialogQueue.ActiveID() == fmt.Sprintf("cost-%.0f", m.activeCostDialog.threshold) {
		b.WriteString("\n" + m.activeCostDialog.form.View() + "\n")
	}

	// ---- Shortcut help overlay ----
	if m.showHelp {
		b.WriteString(m.renderShortcutHelp())
	}

	// ---- Autocomplete dropdown (above input) ----
	if m.autocomplete.Visible() {
		rendered := m.autocomplete.Render(m.width, m.theme)
		if m.animations && m.acAnim.Active {
			rendered = clipToLines(rendered, maxInt(1, int(math.Round(m.acAnim.Pos))))
		}
		b.WriteString(rendered)
	}

	// ---- Mention popup (above input) ----
	if m.mentions.Active() {
		rendered := m.mentions.Render(m.width, m.theme)
		if m.animations && m.mentionAnim.Active {
			rendered = clipToLines(rendered, maxInt(1, int(math.Round(m.mentionAnim.Pos))))
		}
		b.WriteString(rendered)
	}

	// ---- Notifications (above input) ----
	if m.notifications.Len() > 0 {
		b.WriteString(renderNotifications(m.notifications, m.width, m.theme))
	}

	// ---- History search overlay (Ctrl+R) ----
	if m.historySearch.Active() {
		b.WriteString(m.historySearch.Render(m.width, m.theme))
	}

	// ---- Draft stash hint ----
	if m.showStashHint {
		hint := lipgloss.NewStyle().Faint(true).Foreground(lipgloss.Color("8")).
			Render("  Draft stashed — Ctrl+Z to restore")
		b.WriteString(hint + "\n")
	}

	// ---- Input area ----
	b.WriteString(m.renderInput())
	// ---- Typeahead ghost text ----
	if m.typeahead.HasGhost() && !m.processing && !m.autocomplete.Visible() && !m.mentions.Active() && !m.historySearch.Active() {
		b.WriteString(m.typeahead.RenderGhost(m.theme))
	}
	b.WriteByte('\n')

	// ---- Tasks panel (between input and status bar) ----
	if m.tasksPanel != nil && m.tasksPanel.Expanded {
		agents := m.bgState.AllAgents()
		if panel := m.tasksPanel.Render(agents, m.width); panel != "" {
			b.WriteString(panel)
		}
	}
	// ---- Status bar ----
	status := m.status
	status.Processing = m.processing
	status.BackgroundAgts = m.backgroundAgts
	b.WriteString(renderStatusBar(status, m.width))

	// ---- Footer pills (when in footer navigation mode) ----
	if m.footerNav.Active() && len(m.footerNav.Pills) > 0 {
		b.WriteByte('\n')
		b.WriteString(RenderFooterPills(m.footerNav, m.width, m.theme))
	}

	return b.String()
}

// renderInput renders the textarea with mode-aware border styling and label.
func (m AppModel) renderInput() string {
	return renderInputWithMode(m.input.View(), m.inputMode, m.width, m.theme)
}

// renderHeader renders the top header bar with model name, session timer, and cost.
func (m AppModel) renderHeader() string {
	if m.width <= 0 {
		return ""
	}

	left := m.theme.HeaderStyle.Render("Forge")

	var parts []string
	if m.status.Model != "" {
		parts = append(parts, m.theme.HeaderDimStyle.Render(abbreviateModel(m.status.Model)))
	}

	elapsed := time.Since(m.sessionStart).Truncate(time.Second)
	parts = append(parts, m.theme.HeaderDimStyle.Render(elapsed.String()))

	if m.status.CostUSD > 0 {
		parts = append(parts, m.theme.HeaderDimStyle.Render(fmt.Sprintf("$%.4f", m.status.CostUSD)))
	}

	right := strings.Join(parts, m.theme.HeaderDimStyle.Render(" · "))

	leftLen := lipgloss.Width(left)
	rightLen := lipgloss.Width(right)
	gap := m.width - leftLen - rightLen - 4
	if gap < 1 {
		gap = 1
	}

	return "  " + left + strings.Repeat(" ", gap) + right + "  "
}

// renderShortcutHelp renders the keyboard shortcut overlay.
func (m AppModel) renderShortcutHelp() string {
	shortcuts := ShortcutHelp()

	var sb strings.Builder
	for _, s := range shortcuts {
		key := m.theme.AutocompleteSelectedStyle.Render(s.Key)
		action := m.theme.AutocompleteDimStyle.Render(s.Action)
		sb.WriteString("  " + key)
		// Pad key column to 16 chars
		pad := 16 - len(s.Key)
		if pad < 2 {
			pad = 2
		}
		sb.WriteString(strings.Repeat(" ", pad))
		sb.WriteString(action + "\n")
	}

	content := strings.TrimRight(sb.String(), "\n")
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(m.theme.Config.AccentColor)).
		Padding(1, 2).
		Width(m.width - 8).
		Render(m.theme.HeaderStyle.Render("Keyboard Shortcuts") + "\n\n" + content)

	return "\n" + box + "\n"
}

// updateHistorySearch handles all key events while the history search overlay is active.
func (m AppModel) updateHistorySearch(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case msg.String() == "esc", msg.String() == "ctrl+r":
		m.historySearch.Close()
		return m, nil
	case msg.String() == "enter":
		if sel := m.historySearch.Selected(); sel != "" {
			m.input.SetValue(sel)
			m.input.SetCursor(len(sel))
			m.typeahead.Update(sel)
			m.undoStack.Push(sel, len(sel))
		}
		m.historySearch.Close()
		return m, nil
	case msg.String() == "up":
		m.historySearch.Prev()
		return m, nil
	case msg.String() == "down":
		m.historySearch.Next()
		return m, nil
	case msg.String() == "backspace":
		m.historySearch.Backspace()
		return m, nil
	case msg.Type == tea.KeyRunes:
		for _, r := range msg.Runes {
			m.historySearch.TypeChar(r)
		}
		return m, nil
	}
	return m, nil
}

// updateAutocomplete checks the current input and shows/hides the autocomplete dropdown.
func (m *AppModel) updateAutocomplete(cmds *[]tea.Cmd) {
	val := m.input.Value()
	wasVisible := m.autocomplete.Visible()
	if strings.HasPrefix(val, "/") && !strings.Contains(val[1:], " ") {
		m.autocomplete.Show(val[1:])
		if m.animations && !wasVisible && m.autocomplete.Visible() {
			lines := minInt(m.autocomplete.FilteredCount(), MaxVisible) + 2
			m.acAnim.StartFrom(0, float64(lines))
			m.scheduleAnimTick(cmds)
		}
	} else {
		m.autocomplete.Hide()
		if m.animations {
			m.acAnim.Snap(0)
		}
	}
}

// updateMentions checks the current input for an @ trigger and shows/hides the mention popup.
func (m *AppModel) updateMentions(cmds *[]tea.Cmd) {
	val := m.input.Value()
	wasActive := m.mentions.Active()
	query, ok := extractMentionQuery(val)
	if ok {
		m.mentions.Show(query)
		if m.animations && !wasActive && m.mentions.Active() {
			lines := minInt(m.mentions.FilteredCount(), MaxMentionVisible) + 2
			m.mentionAnim.StartFrom(0, float64(lines))
			m.scheduleAnimTick(cmds)
		}
	} else {
		m.mentions.Hide()
		if m.animations {
			m.mentionAnim.Snap(0)
		}
	}
}

// extractMentionQuery finds the last @ trigger in the input and returns the query after it.
// Returns ("", false) if no active @ mention is being typed.
func extractMentionQuery(val string) (string, bool) {
	// Find the last @ that could be a mention trigger
	idx := strings.LastIndex(val, "@")
	if idx < 0 {
		return "", false
	}
	// @ must be at start of input or preceded by a space
	if idx > 0 && val[idx-1] != ' ' {
		return "", false
	}
	query := val[idx+1:]
	// If query contains a space, the mention is "closed" — no longer typing
	if strings.Contains(query, " ") {
		return "", false
	}
	return query, true
}

// insertMention replaces the @query with @value in the input.
func (m *AppModel) insertMention(value string) {
	val := m.input.Value()
	idx := strings.LastIndex(val, "@")
	if idx < 0 {
		return
	}
	// Replace from @ to end-of-query with @value
	before := val[:idx]
	m.input.SetValue(before + "@" + value + " ")
}

// submitPrompt adds the user message and starts engine processing.
// Slash commands (e.g. /help, /cost) are intercepted and handled directly
// by the TUI without sending to the engine.
func (m *AppModel) submitPrompt(text string) tea.Cmd {
	// Intercept slash commands before sending to the engine.
	if strings.HasPrefix(text, "/") {
		parts := strings.SplitN(text[1:], " ", 2)
		name := strings.ToLower(strings.TrimSpace(parts[0]))
		args := ""
		if len(parts) > 1 {
			args = strings.TrimSpace(parts[1])
		}
		if cmd, ok := m.handleSlashCommand(name, args); ok {
			return cmd
		}
		// Skill invocation — show indicator for commands handled by the engine
		m.addSystemMessage(fmt.Sprintf("  ⚡ Running /%s...", name))
	}

	// Track last user prompt for sticky header
	m.lastUserPromptIdx = len(m.messages)
	m.lastUserPrompt = text
	m.messages = append(m.messages, DisplayMessage{
		Role:    "user",
		Content: text,
	})
	m.inputMode = ModeProcessing
	m.stall.OnProcessingStart()
	m.bgState.OnProcessingStart()
	m.processing = true
	m.status.Processing = true
	m.userScrolledUp = false // snap back to bottom for response
	m.clearUnseenDivider()
	m.streamBuf.Reset()
	m.partialLineBuf = ""
	// Update terminal title to processing state
	m.titleState.SetProcessing()
	WriteTitleSequence(m.titleState.Current())
	m.refreshViewport()

	// Create a cancellable context so Ctrl+C / Esc can abort the engine call.
	ctx, cancel := context.WithCancel(context.Background())
	m.cancelPrompt = cancel

	// Capture only the bridge pointer and ctx, not the whole model, to avoid
	// holding a reference to the stack-allocated model copy.
	bridge := m.bridge
	return tea.Batch(
		func() tea.Msg {
			return bridge.Submit(ctx, text)
		},
		autoBackgroundTick(m.bgState.AutoTimeout),
	)
}

// upsertStreamingMessage updates the last assistant message in the conversation
// or appends a new one if none exists.
func (m *AppModel) upsertStreamingMessage() {
	content := m.streamBuf.String()
	if len(m.messages) > 0 && m.messages[len(m.messages)-1].Role == "assistant" {
		m.messages[len(m.messages)-1].Content = content
	} else {
		m.messages = append(m.messages, DisplayMessage{
			Role:    "assistant",
			Content: content,
		})
	}
}

// flushStreamBuf commits the current stream buffer as a final assistant message.
func (m *AppModel) flushStreamBuf() {
	// Include any partial line buffer in the final flush
	content := strings.TrimSpace(m.streamBuf.String())
	if content == "" {
		return
	}
	if len(m.messages) > 0 && m.messages[len(m.messages)-1].Role == "assistant" {
		m.messages[len(m.messages)-1].Content = content
	} else {
		m.messages = append(m.messages, DisplayMessage{
			Role:    "assistant",
			Content: content,
		})
	}
	m.streamBuf.Reset()
	m.partialLineBuf = ""
}

// trackUnseenMessage should be called whenever a new message is appended
// while the user is scrolled up.
func (m *AppModel) trackUnseenMessage() {
	if !m.userScrolledUp {
		return
	}
	if m.unseenDividerIdx < 0 {
		m.unseenDividerIdx = len(m.messages) - 1
	}
	m.unseenCount++
}

// clearUnseenDivider resets the unseen message divider state.
func (m *AppModel) clearUnseenDivider() {
	m.unseenDividerIdx = -1
	m.unseenCount = 0
}

// toggleLastToolCollapse toggles the collapsed state of the most recent tool result.
// Returns true if a tool message was found and toggled.
func toggleLastToolCollapse(messages []DisplayMessage) bool {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "tool" {
			messages[i].Collapsed = !messages[i].Collapsed
			return true
		}
	}
	return false
}

// refreshViewport re-renders the conversation into the viewport.
// Only auto-scrolls to bottom if the user hasn't manually scrolled up.
func (m *AppModel) refreshViewport() {
	if m.width <= 0 {
		return // viewport not yet sized — nothing useful to render
	}
	content := renderConversation(m.messages, m.width, m.collapseAnim, m.splash, m.unseenDividerIdx, m.unseenCount)
	m.viewport.SetContent(content)
	if !m.userScrolledUp {
		m.viewport.GotoBottom()
	}
}

// inputHeight returns the rendered height of the input area.
func (m *AppModel) inputHeight() int {
	return m.input.Height() + 2 // +2 for border
}

// Run starts the Bubbletea program with the given model.
func Run(m AppModel) error {
	p := tea.NewProgram(
		m,
		tea.WithAltScreen(),
		// NOTE: tea.WithMouseCellMotion() removed — it enables terminal mouse
		// tracking which prevents users from selecting/copying text with their
		// mouse. Standard terminal text selection is more important than
		// mouse-based scrolling, which can be done via keyboard (pgup/pgdown).
	)
	m.bridge.program = p
	_, err := p.Run()
	// Reset terminal title on exit
	ResetTitle()
	return err
}

// contains checks if s is in ss.
func contains(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}

// remove removes the first occurrence of s from ss.
func remove(ss []string, s string) []string {
	for i, v := range ss {
		if v == s {
			return append(ss[:i], ss[i+1:]...)
		}
	}
	return ss
}

// containsTool checks if a tool with the given ID is in the active list.
func containsTool(tools []ActiveToolInfo, id string) bool {
	for _, t := range tools {
		if t.ID == id {
			return true
		}
	}
	return false
}

// removeTool removes the first tool with the given ID from the list.
func removeTool(tools []ActiveToolInfo, id string) []ActiveToolInfo {
	for i, t := range tools {
		if t.ID == id {
			return append(tools[:i], tools[i+1:]...)
		}
	}
	return tools
}

// findToolDetail returns the Detail string for a tool by ID, or "" if not found.
func findToolDetail(tools []ActiveToolInfo, id string) string {
	for _, t := range tools {
		if t.ID == id {
			return t.Detail
		}
	}
	return ""
}

// scheduleAnimTick schedules an animation tick if one isn't already pending.
func (m *AppModel) scheduleAnimTick(cmds *[]tea.Cmd) {
	if !m.animTicking {
		m.animTicking = true
		*cmds = append(*cmds, animationTick())
	}
}

// startCollapseAnimation begins an animated expand/collapse of the last tool result.
// Returns a tea.Cmd to schedule the animation tick, or nil if no tool message found.
func (m *AppModel) startCollapseAnimation() tea.Cmd {
	for i := len(m.messages) - 1; i >= 0; i-- {
		if m.messages[i].Role == "tool" {
			collapsing := !m.messages[i].Collapsed
			if collapsing {
				// Expanding → collapsing: render expanded to get height, then animate down
				rendered := renderToolResultExpanded(m.messages[i], m.width)
				fullHeight := float64(countLines(rendered))
				m.collapseSpring.StartFrom(fullHeight, 1)
			} else {
				// Collapsed → expanding: render expanded to get target height
				msg := m.messages[i]
				msg.Collapsed = false
				rendered := renderToolResultExpanded(msg, m.width)
				fullHeight := float64(countLines(rendered))
				m.messages[i].Collapsed = false // show expanded during animation
				m.collapseSpring.StartFrom(1, fullHeight)
			}
			m.collapseAnim = &CollapseAnimation{
				MsgIndex:   i,
				Collapsing: collapsing,
				Height:     m.collapseSpring.Pos,
			}
			m.refreshViewport()
			if !m.animTicking {
				m.animTicking = true
				return animationTick()
			}
			return nil
		}
	}
	return nil
}

// anyAnimating returns true if any spring animation is currently active.
func (m *AppModel) anyAnimating() bool {
	return m.scrollAnim.Active || m.acAnim.Active || m.mentionAnim.Active ||
		m.permAnim.Active || (m.collapseAnim != nil && m.collapseSpring.Active)
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// submitBashDirect sends a command directly as a Bash tool call, bypassing the model.
// Routes through the normal prompt path with a "! " prefix.
func (m *AppModel) submitBashDirect(command string) tea.Cmd {
	return m.submitPrompt("! " + command)
}
func skillNames(reg *CommandRegistry) []string {
	var names []string
	for _, cmd := range reg.Commands() {
		if !cmd.Hidden {
			names = append(names, "/"+cmd.Name)
		}
	}
	return names
}

// StashHintDismissMsg auto-dismisses the draft stash hint after timeout.
type StashHintDismissMsg struct{}

// trackDraftStash monitors input length for draft stash triggers.
// When input drops from 20+ chars to <5, the previous long text is stashed.
func (m *AppModel) trackDraftStash() bool {
	val := m.input.Value()
	curLen := len(val)
	prevLen := len(m.prevInputValue)

	if curLen > m.peakInputLen {
		m.peakInputLen = curLen
	}

	triggered := false
	// Trigger stash when dropping from 20+ chars to <5
	if m.peakInputLen >= 20 && curLen < 5 && prevLen >= 20 {
		m.stashedDraft = m.prevInputValue
		m.peakInputLen = curLen
		if !m.hasShownStashHint {
			m.showStashHint = true
			m.hasShownStashHint = true
			triggered = true
		}
	}

	m.prevInputValue = val
	return triggered
}

// stashHintDismiss returns a tea.Cmd that fires StashHintDismissMsg after 5s.
func stashHintDismiss() tea.Cmd {
	return tea.Tick(5000*time.Millisecond, func(t time.Time) tea.Msg {
		return StashHintDismissMsg{}
	})
}

// dismissStashHint clears the stash hint display.
func (m *AppModel) dismissStashHint() {
	m.showStashHint = false
	if m.stashHintTimer != nil {
		m.stashHintTimer.Stop()
		m.stashHintTimer = nil
	}
}

// Ensure AppModel satisfies the tea.Model interface at compile time.
var _ tea.Model = AppModel{}

// upsertStreamingCompleteLines updates the displayed message with only
// complete lines (up to the last newline boundary). Partial words are
// held in partialLineBuf to prevent flicker.
func (m *AppModel) upsertStreamingCompleteLines() {
	full := m.streamBuf.String()
	lastNL := strings.LastIndex(full, "\n")
	if lastNL < 0 {
		// No newline yet — keep everything in partial buf, show nothing
		m.partialLineBuf = full
		return
	}
	// Display up to (and including) the last newline
	displayContent := full[:lastNL+1]
	m.partialLineBuf = full[lastNL+1:]

	if len(m.messages) > 0 && m.messages[len(m.messages)-1].Role == "assistant" {
		m.messages[len(m.messages)-1].Content = displayContent
	} else {
		m.messages = append(m.messages, DisplayMessage{
			Role:    "assistant",
			Content: displayContent,
		})
	}
}

// renderStickyPrompt renders a dim header bar showing the last user prompt.
func (m AppModel) renderStickyPrompt() string {
	prompt := m.lastUserPrompt
	maxLen := 500
	if len(prompt) > maxLen {
		prompt = prompt[:maxLen-3] + "..."
	}
	// Truncate to terminal width
	maxWidth := m.width - 10
	if maxWidth < 20 {
		maxWidth = 20
	}
	if len(prompt) > maxWidth {
		prompt = prompt[:maxWidth-3] + "..."
	}
	style := lipgloss.NewStyle().
		Foreground(lipgloss.Color(m.theme.Config.DimColor)).
		Faint(true).
		Width(m.width)
	return style.Render("> You: " + prompt)
}

// updateIdleForm forwards key events to the idle return dialog.
func (m *AppModel) updateIdleForm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "esc" {
		m.idleState.DismissDialog()
		m.suppressInput()
		return m, nil
	}

	if m.idleState.form == nil {
		return m, nil
	}

	model, cmd := m.idleState.form.Update(msg)
	if f, ok := model.(*huh.Form); ok {
		m.idleState.form = f
	}

	if m.idleState.form.State == huh.StateCompleted {
		choice := m.idleState.HandleChoice()
		m.suppressInput()
		switch choice {
		case IdleClear:
			m.messages = nil
			m.refreshViewport()
		case IdleNeverAsk:
			// Already persisted by HandleChoice
		}
		return m, cmd
	}

	return m, cmd
}

// suppressInput enables temporary input suppression after dialog close.
func (m *AppModel) suppressInput() {
	m.inputSuppressed = true
	m.suppressUntil = time.Now().Add(200 * time.Millisecond)
}

// checkIdleOnSubmit checks if the user has been idle and shows the dialog if needed.
// Returns true if the idle dialog was shown (caller should not proceed with submit).
func (m *AppModel) checkIdleOnSubmit() bool {
	idle, duration := m.idleState.CheckIdle(time.Now())
	if !idle {
		return false
	}
	m.idleState.ShowDialog(duration, m.theme)
	if m.idleState.form != nil {
		m.idleState.form.Init()
	}
	return true
}

// StartSessionLoad begins progressive loading of session messages.
// Messages are loaded in batches to keep the UI responsive during resume.
func (m *AppModel) StartSessionLoad(messages []DisplayMessage) tea.Cmd {
	if len(messages) == 0 {
		return nil
	}
	m.sessionLoader = NewSessionLoadState(messages, 15)
	return scheduleNextBatch()
}

// IsSessionLoading returns true if session messages are still being loaded.
func (m *AppModel) IsSessionLoading() bool {
	return m.sessionLoader != nil && !m.sessionLoader.Done()
}
