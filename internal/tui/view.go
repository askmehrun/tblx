package tui

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	tblx "github.com/askmehrun/libtblx"
)

// chromeLines is the number of screen rows reserved for the title bar,
// the frozen header, the separator and the status bar.
const chromeLines = 4

var (
	viewTitle  = lipgloss.NewStyle().Bold(true).Foreground(colAmber)
	viewHead   = lipgloss.NewStyle().Bold(true).Foreground(colBright)
	viewSep    = lipgloss.NewStyle().Foreground(lipgloss.Color("238"))
	viewGutter = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	viewStatus = lipgloss.NewStyle().Foreground(colDim)
)

// ViewModel renders a .tblx file as a scrollable table with a frozen,
// pan-synced header row and a sticky row-number gutter.
type ViewModel struct {
	path     string
	colNames []string
	colTypes []tblx.DataType
	nRows    int
	header   string   // full-width padded header line
	body     []string // full-width padded data lines
	widths   []int
	gutterW  int

	viewport viewport.Model
	xOff     int
	contentW int
	width    int
	height   int
	ready    bool
}

// NewViewModel pre-renders the table once; scrolling and panning then
// never touch the decoded data again.
func NewViewModel(path string, colNames []string, colTypes []tblx.DataType, rows [][]string) ViewModel {
	nCols := len(colNames)
	widths := make([]int, nCols)
	for c := range colNames {
		widths[c] = utf8.RuneCountInString(colNames[c])
	}
	for _, row := range rows {
		for c, cell := range row {
			if c < nCols && utf8.RuneCountInString(cell) > widths[c] {
				widths[c] = utf8.RuneCountInString(cell)
			}
		}
	}
	for c := range widths {
		if widths[c] > 30 {
			widths[c] = 30 // keep absurd cells readable
		}
	}

	contentW := 0
	for c, w := range widths {
		contentW += w
		if c < nCols-1 {
			contentW += 2
		}
	}

	m := ViewModel{
		path:     path,
		colNames: colNames,
		colTypes: colTypes,
		nRows:    len(rows),
		widths:   widths,
		contentW: contentW,
		gutterW:  len(strconv.Itoa(len(rows))) + 2,
	}
	if m.gutterW < 4 {
		m.gutterW = 4
	}

	m.header = m.renderLine(colNames)
	m.body = make([]string, len(rows))
	for ri, row := range rows {
		m.body[ri] = m.renderLine(row)
	}
	return m
}

// renderLine pads every cell to its column width and joins with 2 spaces.
func (m ViewModel) renderLine(cells []string) string {
	var parts []string
	for c, w := range m.widths {
		cell := ""
		if c < len(cells) {
			cell = cells[c]
		}
		parts = append(parts, padCell(cell, w))
	}
	return strings.Join(parts, "  ")
}

func padCell(s string, w int) string {
	rs := []rune(s)
	if len(rs) > w {
		return string(rs[:w-1]) + "…"
	}
	return s + strings.Repeat(" ", w-len(rs))
}

// sliceLine applies the horizontal pan offset to a full-width line.
func (m ViewModel) sliceLine(s string) string {
	vw := m.viewport.Width
	if vw <= 0 {
		return ""
	}
	rs := []rune(s)
	if m.xOff >= len(rs) {
		return strings.Repeat(" ", vw)
	}
	end := m.xOff + vw
	if end > len(rs) {
		end = len(rs)
	}
	line := string(rs[m.xOff:end])
	return line + strings.Repeat(" ", vw-utf8.RuneCountInString(line))
}

// refreshContent re-renders the viewport body for the current pan offset.
func (m *ViewModel) refreshContent() {
	sliced := make([]string, len(m.body))
	for i, line := range m.body {
		sliced[i] = m.sliceLine(line)
	}
	m.viewport.SetContent(strings.Join(sliced, "\n"))
}

func (m *ViewModel) clampPan() {
	max := m.contentW - m.viewport.Width
	if max < 0 {
		max = 0
	}
	if m.xOff < 0 {
		m.xOff = 0
	}
	if m.xOff > max {
		m.xOff = max
	}
}

// RunViewer opens path, decodes it and runs the full-screen viewer.
func RunViewer(path string) error {
	r, err := tblx.NewReader(path)
	if err != nil {
		return err
	}
	defer r.Close()

	data, err := r.ReadAll()
	if err != nil {
		return err
	}
	rows := make([][]string, len(data))
	for ri, row := range data {
		cells := make([]string, r.NCols)
		for ci, name := range r.ColNames {
			cells[ci] = tblx.FormatValue(row[name])
		}
		rows[ri] = cells
	}

	m := NewViewModel(path, r.ColNames, r.ColTypes, rows)
	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("tblx: viewer: %w", err)
	}
	return nil
}

// Init implements tea.Model.
func (m ViewModel) Init() tea.Cmd { return nil }

// Update implements tea.Model: q/esc quits, j/k and arrows scroll,
// g/G jump to the edges, and left/right pan horizontally.
func (m ViewModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		vw := msg.Width - m.gutterW
		vh := msg.Height - chromeLines
		if vw < 1 {
			vw = 1
		}
		if vh < 1 {
			vh = 1
		}
		if !m.ready {
			m.viewport = viewport.New(vw, vh)
			m.viewport.MouseWheelEnabled = true
			m.ready = true
		} else {
			m.viewport.Width = vw
			m.viewport.Height = vh
		}
		m.clampPan()
		m.refreshContent()

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q", "esc":
			return m, tea.Quit
		case "up", "k":
			m.viewport.LineUp(1)
		case "down", "j":
			m.viewport.LineDown(1)
		case "pgup":
			// HalfPageUp/HalfPageDown only exist in newer bubbles;
			// LineUp/LineDown are available in every release.
			m.viewport.LineUp(m.viewport.Height / 2)
		case "pgdown":
			m.viewport.LineDown(m.viewport.Height / 2)
		case "g":
			m.viewport.GotoTop()
		case "G":
			m.viewport.GotoBottom()
		case "left", "h":
			m.xOff -= 8
			m.clampPan()
			m.refreshContent()
		case "right", "l":
			m.xOff += 8
			m.clampPan()
			m.refreshContent()
		}
	}
	return m, nil
}

// View implements tea.Model: title bar, frozen header, viewport body
// with a sticky row-number gutter, and a status bar.
func (m ViewModel) View() string {
	if !m.ready {
		return "opening…"
	}

	title := viewTitle.Render(" tblx view ") +
		viewHead.Render(" "+m.path+" ") +
		viewStatus.Render(fmt.Sprintf(" %d cols · %d rows · TBLX", len(m.colNames), m.nRows))

	headLine := viewHead.Render(m.sliceLine(m.header))
	sep := viewSep.Render(strings.Repeat("─", m.width))

	// Sticky gutter, aligned with the viewport's current vertical offset.
	var gut strings.Builder
	for i := 0; i < m.viewport.Height; i++ {
		n := m.viewport.YOffset + i + 1
		cell := ""
		if n <= m.nRows {
			cell = strconv.Itoa(n)
		}
		gut.WriteString(viewGutter.Render(fmt.Sprintf("%*s ", m.gutterW-1, cell)))
		if i < m.viewport.Height-1 {
			gut.WriteString("\n")
		}
	}
	body := lipgloss.JoinHorizontal(lipgloss.Top, gut.String(), m.viewport.View())

	lo := m.viewport.YOffset + 1
	hi := m.viewport.YOffset + m.viewport.Height
	if hi > m.nRows {
		hi = m.nRows
	}
	if m.nRows == 0 {
		lo = 0
	}
	status := viewStatus.Render(fmt.Sprintf(
		" q quit · ↑/↓ j/k scroll · ←/→ pan · g/G jump          x+%d · rows %d–%d/%d · %s",
		m.xOff, lo, hi, m.nRows, schemaSummary(m.colNames, m.colTypes)))

	return lipgloss.JoinVertical(lipgloss.Left, title, headLine, sep, body, status)
}

// schemaSummary renders "name:str · age:int · score:float" for the status bar.
func schemaSummary(names []string, types []tblx.DataType) string {
	parts := make([]string, len(names))
	for i := range names {
		parts[i] = names[i] + ":" + types[i].String()
	}
	s := strings.Join(parts, " · ")
	if len(s) > 48 {
		s = s[:45] + "…"
	}
	return s
}
