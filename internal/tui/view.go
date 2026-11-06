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

// ViewModel renders a .tblx file with lazy, windowed row decoding: only the
// rows visible in the viewport are ever decoded, so viewing a table with
// millions of rows never buffers the whole table in memory.
type ViewModel struct {
	path     string
	colNames []string
	colTypes []tblx.DataType
	nRows    int
	widths   []int
	gutterW  int

	reader   *tblx.Reader
	rows     [][]string // decoded rows currently in the viewport
	rowStart uint64     // index of rows[0]
	itRow    uint64     // next row index the iterator will produce
	itReady  bool
	it       tblx.RowIterator
	err      error

	y          int // absolute index of the first visible row
	xOff       int
	contentW   int
	width      int
	height     int
	ready      bool
	viewport   viewport.Model
}

// measureWidths streams every row once to find the widest cell per column
// so the table can be aligned. It keeps only nCols widths in memory.
func measureWidths(r *tblx.Reader) []int {
	widths := make([]int, r.NCols)
	for c := range r.ColNames {
		widths[c] = utf8.RuneCountInString(r.ColNames[c])
	}
	it := r.Rows()
	defer it.Close()
	for it.Next() {
		row := it.Row()
		for c, name := range r.ColNames {
			if w := utf8.RuneCountInString(tblx.FormatValue(row[name])); w > widths[c] {
				widths[c] = w
			}
		}
	}
	for c := range widths {
		if widths[c] > 30 {
			widths[c] = 30 // keep absurd cells readable
		}
	}
	return widths
}

// newViewModel opens path and measures column widths. Row data is decoded
// lazily as the user scrolls (see ensureWindow), so the whole table is
// never held in memory.
func newViewModel(path string) (*ViewModel, error) {
	r, err := tblx.NewReader(path)
	if err != nil {
		return nil, err
	}
	return &ViewModel{
		path:     path,
		reader:   r,
		colNames: r.ColNames,
		colTypes: r.ColTypes,
		nRows:    int(r.NRows),
		widths:   measureWidths(r),
		gutterW:  len(strconv.Itoa(int(r.NRows))) + 2,
	}, nil
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

// ensureWindow decodes exactly the rows inside the viewport. It is cheap
// when the scroll position is unchanged: a single streaming iterator is
// advanced forward as the user scrolls down and only restarted when the
// user scrolls past rows that were already decoded away. Peak memory is
// O(viewport height) plus the column widths, independent of row count.
func (m *ViewModel) ensureWindow() {
	if m.err != nil || !m.ready {
		return
	}
	start := m.y
	if start < 0 {
		start = 0
	}
	end := start + m.viewport.Height
	if end > m.nRows {
		end = m.nRows
	}
	if start >= end {
		m.rows = nil
		return
	}

	// Drop rows that scrolled out of view at the top.
	for uint64(start) > m.rowStart && len(m.rows) > 0 {
		m.rows = m.rows[1:]
		m.rowStart++
	}

	// Restart the iterator only when the window needs rows we no longer
	// hold (scrolling up) or rows we have not yet reached (scrolling past
	// the end of what we decoded).
	if !m.itReady || m.itRow < uint64(start) || uint64(start) < m.rowStart {
		if m.itReady {
			m.it.Close()
		}
		m.it = m.reader.Rows()
		m.rows = nil
		m.rowStart = uint64(start)
		m.itRow = uint64(start)
		m.itReady = true
	}

	for m.itRow < uint64(start) {
		if !m.it.Next() {
			break
		}
		m.it.Row()
		m.itRow++
	}
	if err := m.it.Err(); err != nil {
		m.err = err
		return
	}

	for m.itRow < uint64(end) {
		if !m.it.Next() {
			break
		}
		row := m.it.Row()
		cells := make([]string, m.reader.NCols)
		for ci, name := range m.reader.ColNames {
			cells[ci] = tblx.FormatValue(row[name])
		}
		m.rows = append(m.rows, cells)
		m.itRow++
	}
}

// refreshContent renders the current viewport window into the viewport.
func (m *ViewModel) refreshContent() {
	var content string
	if len(m.rows) > 0 {
		lines := make([]string, len(m.rows))
		for i, cells := range m.rows {
			lines[i] = m.sliceLine(m.renderLine(cells))
		}
		content = strings.Join(lines, "\n")
	}
	m.viewport.SetContent(content)
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

// RunViewer opens path and runs the full-screen viewer. Rows are decoded
// lazily as the user scrolls, so even very large files stay light in memory.
func RunViewer(path string) error {
	m, err := newViewModel(path)
	if err != nil {
		return err
	}
	defer func() {
		if m.itReady {
			m.it.Close()
		}
		if m.reader != nil {
			m.reader.Close()
		}
	}()

	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("tblx: viewer: %w", err)
	}
	return nil
}

// Init implements tea.Model.
func (m ViewModel) Init() tea.Cmd { return nil }

// Update implements tea.Model: q/esc quits, j/k and arrows scroll,
// g/G jump to the edges, left/right pan horizontally, and the mouse wheel
// scrolls. Vertical scrolling advances a single streaming iterator rather
// than decoding the whole table up front.
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
			m.ready = true
		} else {
			m.viewport.Width = vw
			m.viewport.Height = vh
		}
		m.clampPan()
		m.ensureWindow()
		m.refreshContent()

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q", "esc":
			return m, tea.Quit
		case "up", "k":
			m.y--
			m.ensureWindow()
			m.refreshContent()
		case "down", "j":
			m.y++
			m.ensureWindow()
			m.refreshContent()
		case "pgup":
			m.y -= m.viewport.Height / 2
			m.ensureWindow()
			m.refreshContent()
		case "pgdown":
			m.y += m.viewport.Height / 2
			m.ensureWindow()
			m.refreshContent()
		case "g":
			m.y = 0
			m.ensureWindow()
			m.refreshContent()
		case "G":
			m.y = m.nRows - m.viewport.Height
			if m.y < 0 {
				m.y = 0
			}
			m.ensureWindow()
			m.refreshContent()
		case "left", "h":
			m.xOff -= 8
			m.clampPan()
			m.refreshContent()
		case "right", "l":
			m.xOff += 8
			m.clampPan()
			m.refreshContent()
		}

	case tea.MouseMsg:
		switch msg.Button {
		case tea.MouseButtonWheelUp:
			m.y--
			m.ensureWindow()
			m.refreshContent()
		case tea.MouseButtonWheelDown:
			m.y++
			m.ensureWindow()
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
	m.ensureWindow()

	title := viewTitle.Render(" tblx view ") +
		viewHead.Render(" "+m.path+" ") +
		viewStatus.Render(fmt.Sprintf(" %d cols · %d rows · TBLX", len(m.colNames), m.nRows))

	headLine := viewHead.Render(m.sliceLine(m.renderLine(m.colNames)))
	sep := viewSep.Render(strings.Repeat("─", m.width))

	// Sticky gutter, aligned with the current vertical offset.
	var gut strings.Builder
	for i := 0; i < m.viewport.Height; i++ {
		n := m.y + i + 1
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

	lo := m.y + 1
	hi := m.y + m.viewport.Height
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
