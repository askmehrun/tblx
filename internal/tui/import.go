// Package tui contains the Bubble Tea interfaces of the tblx CLI: an
// interactive import wizard that assigns a storage type to every CSV
// column, and a full-screen viewer for .tblx files.
package tui

import (
	"fmt"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	tblx "github.com/askmehrun/libtblx"
)

// Lip Gloss palette (256-color, tuned for dark terminals).
var (
	colAmber  = lipgloss.Color("215")
	colCyan   = lipgloss.Color("116")
	colGreen  = lipgloss.Color("114")
	colCoral  = lipgloss.Color("210")
	colBright = lipgloss.Color("252")
	colDim    = lipgloss.Color("245")
	colPanel  = lipgloss.Color("236")
)

var (
	wizTitle = lipgloss.NewStyle().Bold(true).Foreground(colAmber)
	wizSub   = lipgloss.NewStyle().Foreground(colDim)
	wizHelp  = lipgloss.NewStyle().Foreground(colDim)
	wizName  = lipgloss.NewStyle().Foreground(colBright)
	wizSel   = lipgloss.NewStyle().Bold(true).Foreground(colBright)
	wizSamp  = lipgloss.NewStyle().Foreground(colDim).Italic(true)
	kbd      = lipgloss.NewStyle().
			Foreground(colBright).Background(colPanel).
			Padding(0, 1).MarginRight(1)
)

// typeColor maps a DataType to its accent colour, consistent everywhere
// in the UI: int = amber, float = cyan, string = green.
func typeColor(d tblx.DataType) lipgloss.Color {
	switch d {
	case tblx.DTypeInt:
		return colAmber
	case tblx.DTypeFloat:
		return colCyan
	default:
		return colGreen
	}
}

// ImportModel is the interactive wizard that chooses a DataType for each
// CSV column before the file is written as TBLX.
type ImportModel struct {
	path      string
	headers   []string
	samples   [][]string // up to 3 raw rows, for context
	types     []tblx.DataType
	cursor    int
	confirmed bool
	aborted   bool
	width     int
	height    int
}

// NewImportModel builds the wizard. Initial types are guessed from the
// sample rows (int > float > string) so most tables need zero edits.
func NewImportModel(path string, headers []string, samples [][]string) ImportModel {
	// Transpose the sample rows into column-major form and let the libtblx
	// core infer the narrowest type per column (int > float > string).
	cols := make([][]string, len(headers))
	for c := range headers {
		for _, row := range samples {
			v := ""
			if c < len(row) {
				v = row[c]
			}
			cols[c] = append(cols[c], v)
		}
	}
	types := tblx.GuessTypes(cols)
	return ImportModel{
		path:    path,
		headers: headers,
		samples: samples,
		types:   types,
	}
}

// RunImportWizard runs the wizard interactively and returns the types the
// user confirmed. It returns an error if the user aborts (q / esc / ctrl+c).
func RunImportWizard(path string, headers []string, samples [][]string) ([]tblx.DataType, error) {
	final, err := tea.NewProgram(NewImportModel(path, headers, samples)).Run()
	if err != nil {
		return nil, fmt.Errorf("tblx: import wizard: %w", err)
	}
	m, ok := final.(ImportModel)
	if !ok {
		return nil, fmt.Errorf("tblx: import wizard: unexpected model type %T", final)
	}
	if m.aborted || !m.confirmed {
		return nil, fmt.Errorf("tblx: import aborted — nothing written")
	}
	return m.types, nil
}

// Init implements tea.Model. The wizard needs no startup command.
func (m ImportModel) Init() tea.Cmd { return nil }

// Update implements tea.Model: navigate with up/down, rotate the type of
// the highlighted column with left/right, confirm with enter, abort with
// q / esc / ctrl+c.
func (m ImportModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q", "esc":
			m.aborted = true
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.headers)-1 {
				m.cursor++
			}
		case "left", "h":
			m.types[m.cursor] = m.types[m.cursor].Prev()
		case "right", "l", "tab":
			m.types[m.cursor] = m.types[m.cursor].Next()
		case "enter":
			m.confirmed = true
			return m, tea.Quit
		}
	}
	return m, nil
}

// sampleFor renders up to three quoted sample values of column c.
func (m ImportModel) sampleFor(c int) string {
	var parts []string
	for _, row := range m.samples {
		if c >= len(row) {
			continue
		}
		v := strings.TrimSpace(row[c])
		if v == "" {
			v = "∅"
		}
		parts = append(parts, strconv.Quote(v))
		if len(parts) == 3 {
			break
		}
	}
	if len(parts) == 0 {
		return "no samples"
	}
	s := strings.Join(parts, "  ")
	if len(s) > 42 {
		s = s[:39] + "…"
	}
	return s
}

// View implements tea.Model and renders the wizard screen.
func (m ImportModel) View() string {
	var b strings.Builder

	b.WriteString(wizTitle.Render("TABLIX · IMPORT WIZARD"))
	b.WriteString(wizSub.Render("   step 1/1 — assign column types"))
	b.WriteString("\n")
	b.WriteString(wizSub.Render(fmt.Sprintf("source  %s", m.path)))
	b.WriteString("\n")
	b.WriteString(wizSub.Render("TBLX has no NULL: empty cells become 0 / 0.0 / \"\"."))
	b.WriteString("\n\n")

	nameW := 0
	for _, h := range m.headers {
		if len(h) > nameW {
			nameW = len(h)
		}
	}

	for i, h := range m.headers {
		marker := "  "
		line := wizName.Render(fmt.Sprintf("%2d. %-*s", i+1, nameW, h))
		if i == m.cursor {
			marker = lipgloss.NewStyle().Foreground(colAmber).Bold(true).Render("❯ ")
			line = wizSel.Render(fmt.Sprintf("%2d. %-*s", i+1, nameW, h))
		}
		badge := lipgloss.NewStyle().
			Bold(true).
			Foreground(typeColor(m.types[i])).
			Background(colPanel).
			Padding(0, 1).
			Render(fmt.Sprintf("%-6s", m.types[i]))
		b.WriteString(fmt.Sprintf("%s%s  %s  %s\n",
			marker, line, badge, wizSamp.Render(m.sampleFor(i))))
	}

	b.WriteString("\n")
	b.WriteString(wizHelp.Render(fmt.Sprintf("column %d / %d · types guessed from the first %d rows",
		m.cursor+1, len(m.headers), len(m.samples))))
	b.WriteString("\n\n")

	b.WriteString(kbd.Render("↑") + kbd.Render("↓") + wizHelp.Render("navigate   "))
	b.WriteString(kbd.Render("←") + kbd.Render("→") + wizHelp.Render("cycle type   "))
	b.WriteString(kbd.Render("enter") + wizHelp.Render("confirm   "))
	b.WriteString(kbd.Render("q") + wizHelp.Render("abort"))

	return b.String()
}
