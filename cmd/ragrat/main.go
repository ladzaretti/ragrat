package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const (
	listWidth  = 24
	extraLines = 2 // spinner, status
)

var (
	// https://catppuccin.com/palette/
	mochaSubtext0 = "#a6adc8"
	mochaOverlay2 = "#9399b2"
	mochaGreen    = "#a6e3a1"
	mochaMauve    = "#cba6f7"
	mochaSurface0 = "#313244"
	mochaBase     = "#1e1e2e" // text-fg when we invert the bubble

	keyStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color(mochaSubtext0)).Bold(true)
	dimStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color(mochaOverlay2))
	boldStyle    = lipgloss.NewStyle().Bold(true)
	promptSymbol = lipgloss.NewStyle().Foreground(lipgloss.Color(mochaGreen)).Render("❯ ")
	barStyle     = lipgloss.NewStyle().Background(lipgloss.Color(mochaSurface0))

	chipStyle = lipgloss.NewStyle().
			Background(lipgloss.Color(mochaGreen)).
			Foreground(lipgloss.Color(mochaBase)).
			Bold(true).
			Padding(0, 1)

	legend = lipgloss.JoinHorizontal(lipgloss.Left,
		keyStyle.Render("up/down"), dimStyle.Render(" scroll - "),
		keyStyle.Render("ctrl+s"), dimStyle.Render(" send - "),
		keyStyle.Render("ctrl+l"), dimStyle.Render(" focus list - "),
		keyStyle.Render("ctrl+g"), dimStyle.Render(" focus history - "),
		keyStyle.Render("esc"), dimStyle.Render(" cancel - "),
		keyStyle.Render("ctrl+c"), dimStyle.Render(" quit"),
	)

	spinnerCol = lipgloss.NewStyle().Foreground(lipgloss.Color(mochaMauve))
)

type doneMsg struct{}

func waitCmd(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(time.Time) tea.Msg { return doneMsg{} })
}

type listItem string

func (i listItem) Title() string       { return string(i) }
func (i listItem) FilterValue() string { return string(i) }
func (listItem) Description() string   { return "" }

type focus int

const (
	_ = iota
	focusTextarea
	focusViewport
	focusModelList
)

func (f focus) String() string {
	switch f {
	case focusViewport:
		return "history"
	case focusModelList:
		return "models"
	case focusTextarea:
		return "input"
	default:
	}

	return ""
}

type model struct {
	viewport viewport.Model
	textarea textarea.Model
	spinner  spinner.Model

	messages []string
	loading  bool
	ready    bool

	modelList     list.Model
	selectedModel string

	legendWrapped string
	legendHeight  int

	currentFocus focus
}

func initialModel() *model {
	ta := textarea.New()
	ta.Placeholder = "Ask anything"
	ta.Focus()
	ta.Prompt = ""
	ta.FocusedStyle.CursorLine = lipgloss.NewStyle()
	// keep textarea to a single visible line
	ta.SetHeight(1)
	ta.ShowLineNumbers = false

	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = spinnerCol

	models := []string{"gpt-3.5-turbo", "gpt-4o", "llama3"}
	items := make([]list.Item, 0, len(models))

	for _, m := range models {
		items = append(items, listItem(m))
	}

	lm := list.New(items, list.NewDefaultDelegate(), listWidth, 10)
	lm.Title = "Models"
	lm.SetFilteringEnabled(false)
	lm.SetShowStatusBar(false)
	lm.SetShowHelp(false)

	return &model{
		viewport:      viewport.New(0, 0),
		modelList:     lm,
		textarea:      ta,
		spinner:       sp,
		messages:      make([]string, 0, 128),
		selectedModel: models[0],
		legendWrapped: legend,
		legendHeight:  1,
		currentFocus:  focusTextarea,
	}
}

func (*model) Init() tea.Cmd { return textinput.Blink }

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if km, ok := msg.(tea.KeyMsg); ok {
		return m.handleKey(km)
	}

	return m.handleNonKey(msg)
}

func (m *model) handleKey(keyMsg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.currentFocus {
	case focusViewport:
		return m.handleViewport(keyMsg)
	case focusModelList:
		return m.handleModelList(keyMsg)
	default:
	}

	return m.handleTextarea(keyMsg)
}

func (m *model) handleNonKey(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		return m.resize(msg)

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)

		if m.loading {
			return m, cmd
		}

		return m, nil

	case doneMsg:
		m.loading = false
		m.messages = append(m.messages, boldStyle.Render("llm("+m.selectedModel+"):")+" <dummy response>")
		m.refreshViewport()
		m.viewport.GotoBottom()

		return m, nil

	default:
	}

	var (
		taCmd tea.Cmd
		vpCmd tea.Cmd
		mlCmd tea.Cmd
	)

	m.textarea, taCmd = m.textarea.Update(msg)
	m.viewport, vpCmd = m.viewport.Update(msg)
	m.modelList, mlCmd = m.modelList.Update(msg)

	return m, tea.Batch(vpCmd, taCmd, mlCmd)
}

func (m *model) handleViewport(keyMsg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch keyMsg.String() {
	case "esc", "ctrl+g": //nolint:goconst
		m.currentFocus = focusTextarea

		m.textarea.Focus()

		return m, textinput.Blink
	default:
	}

	var cmd tea.Cmd

	m.viewport, cmd = m.viewport.Update(keyMsg)

	return m, cmd
}

func (m *model) handleModelList(keyMsg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch keyMsg.String() {
	case "esc", "ctrl+l", "enter":
		if it, ok := m.modelList.SelectedItem().(listItem); ok {
			m.selectedModel = string(it)
		}

		m.currentFocus = focusTextarea

		m.textarea.Focus()

		return m, textinput.Blink

	default:
	}

	var cmd tea.Cmd

	m.modelList, cmd = m.modelList.Update(keyMsg)

	return m, cmd
}

func (m *model) handleTextarea(keyMsg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch keyMsg.String() {
	case "ctrl+c", "esc":
		return m, tea.Quit

	case "ctrl+g":
		m.currentFocus = focusViewport

		m.textarea.Blur()

		return m, nil

	case "ctrl+l":
		m.currentFocus = focusModelList

		m.textarea.Blur()

		return m, nil

	case "ctrl+s":
		if m.loading {
			return m, nil
		}

		prompt := strings.TrimSpace(m.textarea.Value())
		if prompt == "" {
			return m, nil
		}

		return m.sendPrompt(prompt)
	default:
	}

	var cmd tea.Cmd

	m.textarea, cmd = m.textarea.Update(keyMsg)

	return m, cmd
}

func (m *model) sendPrompt(p string) (tea.Model, tea.Cmd) {
	m.loading = true
	m.messages = append(m.messages, boldStyle.Render("you:")+" "+p)

	m.refreshViewport()
	m.textarea.Reset()
	m.viewport.GotoBottom()

	return m, tea.Batch(m.spinner.Tick, waitCmd(time.Second))
}

func (m *model) resize(w tea.WindowSizeMsg) (tea.Model, tea.Cmd) {
	m.ready = true

	viewportWidth := max(w.Width-listWidth, 1)

	m.viewport.Width = viewportWidth

	m.textarea.SetWidth(viewportWidth)
	m.textarea.SetHeight(1)

	// rewrap legend to the terminal width
	m.legendWrapped = lipgloss.NewStyle().Width(w.Width).Render(legend)
	m.legendHeight = lipgloss.Height(m.legendWrapped)

	availableHeight := w.Height -
		m.textarea.Height() -
		m.legendHeight -
		extraLines

	m.viewport.Height = max(availableHeight, 1)
	m.modelList.SetSize(listWidth, availableHeight)

	m.refreshViewport()
	m.viewport.GotoBottom()

	return m, nil
}

func (m *model) refreshViewport() {
	joined := lipgloss.JoinVertical(lipgloss.Left, m.messages...)
	wrapped := lipgloss.NewStyle().Width(m.viewport.Width).Render(joined)

	m.viewport.SetContent(wrapped)
}

func (m *model) View() string {
	if !m.ready {
		return "\n  Initializing…"
	}

	mainArea := lipgloss.JoinHorizontal(lipgloss.Top, m.viewport.View(), m.modelList.View())
	footerContent := lipgloss.JoinHorizontal(
		lipgloss.Left,
		chipStyle.Render(m.currentFocus.String()),
	)

	status := barStyle.Width(m.viewport.Width + m.modelList.Width()).Render(footerContent)

	var b strings.Builder

	b.WriteString(mainArea)
	b.WriteString("\n")

	if m.loading {
		b.WriteString(m.spinner.View())
	}

	b.WriteString("\n")
	b.WriteString(promptSymbol)
	b.WriteString(m.textarea.View())
	b.WriteString("\n")
	b.WriteString(m.legendWrapped)
	b.WriteString("\n")
	b.WriteString(status)

	return b.String()
}

func main() {
	p := tea.NewProgram(initialModel(), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "ragrat-tui: %v\n", err)
		os.Exit(1)
	}
}
