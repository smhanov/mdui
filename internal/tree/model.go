package tree

import (
	"os"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// SelectionMsg reports that the cursor landed on a markdown file. The root
// model uses it for the status bar now and for viewer loading later.
type SelectionMsg struct {
	Path string
}

// Row is one flattened, currently visible line in the tree. Selection is an
// index into the row slice, which also makes hit-testing straightforward once
// mouse support lands.
type Row struct {
	Path     string
	Name     string
	Depth    int
	IsDir    bool
	Expanded bool
	Warning  string

	node *Node
}

// Model is the left-hand file tree pane.
type Model struct {
	root       *Node
	rows       []Row
	cursor     int
	offset     int
	height     int
	showHidden bool
	sortBy     string
	hideEmpty  bool
	scan       Scanner
}

// Option configures a tree at construction time.
type Option func(*Model)

// WithHidden shows dot-prefixed entries from the first scan.
func WithHidden(hidden bool) Option {
	return func(m *Model) {
		m.showHidden = hidden
	}
}

// WithSortBy selects the file order: "name", "mtime", or "size". Directories
// always stay first and are ordered by name.
func WithSortBy(mode string) Option {
	return func(m *Model) {
		switch strings.ToLower(strings.TrimSpace(mode)) {
		case "mtime":
			m.sortBy = "mtime"
		case "size":
			m.sortBy = "size"
		default:
			m.sortBy = "name"
		}
	}
}

// WithHideEmpty hides directories that contain no markdown anywhere beneath
// them.
func WithHideEmpty(hide bool) Option {
	return func(m *Model) {
		m.hideEmpty = hide
	}
}

// New scans the root's first level and returns a tree ready to render.
// Subdirectories stay unloaded until they are expanded.
func New(root string, opts ...Option) Model {
	return newModel(root, ScanDir, opts...)
}

func newModel(root string, scan Scanner, opts ...Option) Model {
	m := Model{scan: scan, sortBy: "name"}
	for _, opt := range opts {
		opt(&m)
	}
	m.root = &Node{
		Name:     rootLabel(root),
		Path:     root,
		IsDir:    true,
		Expanded: true,
		Loaded:   true,
	}
	m.readChildren(m.root)
	m.rebuild()
	return m
}

// SetHeight records how many rows fit in the pane. Cursor windowing and page
// keys use it.
func (m *Model) SetHeight(height int) {
	if height < 0 {
		height = 0
	}
	m.height = height
	m.ensureCursorVisible()
}

// Update implements the tree's share of the root model's key routing.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	key, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return m, nil
	}

	switch key.String() {
	case "up", "k":
		return m, m.move(-1)
	case "down", "j":
		return m, m.move(1)
	case "left", "h":
		return m, m.collapseOrParent()
	case "right", "l":
		return m, m.expandOrDescend()
	case "enter":
		return m, m.toggleOrSelect()
	case "g":
		return m, m.moveTo(0, false)
	case "G":
		return m, m.moveTo(len(m.rows)-1, true)
	case "pgup":
		return m, m.movePage(-1)
	case "pgdown":
		return m, m.movePage(1)
	case ".":
		return m, m.toggleHidden()
	default:
		return m, nil
	}
}

func (m *Model) move(delta int) tea.Cmd {
	if len(m.rows) == 0 {
		return nil
	}
	target := m.cursor + delta
	if target < 0 {
		target = 0
	}
	if target >= len(m.rows) {
		target = len(m.rows) - 1
	}
	return m.moveTo(target, delta < 0)
}

// Scroll moves the cursor by delta rows, positive downward. It is the mouse
// wheel equivalent of repeated j/k presses; the cursor (and therefore the
// selection preview) follows the visible window.
func (m *Model) Scroll(delta int) tea.Cmd {
	if delta == 0 {
		return nil
	}
	return m.move(delta)
}

func (m *Model) movePage(direction int) tea.Cmd {
	page := m.height
	if page < 1 {
		page = 1
	}
	return m.move(direction * page)
}

// moveTo places the cursor on index, skipping informational warning rows in
// the direction of travel. It returns the selection command for file rows.
func (m *Model) moveTo(index int, backwards bool) tea.Cmd {
	if len(m.rows) == 0 {
		return nil
	}
	if index < 0 {
		index = 0
	}
	if index >= len(m.rows) {
		index = len(m.rows) - 1
	}

	if backwards {
		for index > 0 && m.rows[index].Warning != "" {
			index--
		}
	} else {
		for index < len(m.rows)-1 && m.rows[index].Warning != "" {
			index++
		}
	}
	if m.rows[index].Warning != "" {
		return nil
	}

	m.cursor = index
	m.ensureCursorVisible()
	return m.selectionCmd()
}

func (m *Model) toggleOrSelect() tea.Cmd {
	row := m.currentRow()
	if row == nil {
		return nil
	}
	if !row.IsDir {
		return m.selectionCmd()
	}

	if row.node.Expanded {
		row.node.Expanded = false
	} else {
		if !row.node.Loaded {
			m.load(row.node)
		}
		row.node.Expanded = true
	}
	m.rebuild()
	return nil
}

func (m *Model) expandOrDescend() tea.Cmd {
	row := m.currentRow()
	if row == nil {
		return nil
	}
	if !row.IsDir {
		return m.selectionCmd()
	}

	if !row.node.Expanded {
		if !row.node.Loaded {
			m.load(row.node)
		}
		row.node.Expanded = true
		m.rebuild()
		return nil
	}

	// Already open: step into the first selectable child, if any.
	for i := m.cursor + 1; i < len(m.rows); i++ {
		if m.rows[i].Depth <= row.Depth {
			break
		}
		if m.rows[i].Warning == "" {
			return m.moveTo(i, false)
		}
	}
	return nil
}

func (m *Model) collapseOrParent() tea.Cmd {
	row := m.currentRow()
	if row == nil {
		return nil
	}

	if row.IsDir && row.node.Expanded {
		row.node.Expanded = false
		m.rebuild()
		return nil
	}
	if row.node == nil || row.node.parent == nil {
		return nil
	}
	if index := m.rowIndex(row.node.parent); index >= 0 {
		return m.moveTo(index, true)
	}
	return nil
}

func (m *Model) currentRow() *Row {
	if m.cursor < 0 || m.cursor >= len(m.rows) {
		return nil
	}
	return &m.rows[m.cursor]
}

func (m *Model) selectionCmd() tea.Cmd {
	row := m.currentRow()
	if row == nil || row.IsDir || row.Warning != "" {
		return nil
	}
	path := row.Path
	return func() tea.Msg {
		return SelectionMsg{Path: path}
	}
}

// load reads one directory level. It refuses to descend into a directory that
// resolves to an ancestor, which keeps symlink cycles from recursing forever.
func (m *Model) load(node *Node) {
	if node.Loaded {
		return
	}

	info, err := os.Stat(node.Path)
	if err != nil {
		node.Err = errorText(err)
		node.Loaded = true
		return
	}
	for ancestor := node.parent; ancestor != nil; ancestor = ancestor.parent {
		ancestorInfo, statErr := os.Stat(ancestor.Path)
		if statErr == nil && os.SameFile(info, ancestorInfo) {
			node.Err = "symlink cycle"
			node.cycle = true
			node.Loaded = true
			return
		}
	}

	children, err := m.scan(node.Path, m.showHidden)
	if err != nil {
		node.Err = errorText(err)
		node.Loaded = true
		return
	}

	node.Err = ""
	m.attach(node, m.refine(children))
	node.Loaded = true
}

func (m *Model) readChildren(node *Node) {
	children, err := m.scan(node.Path, m.showHidden)
	if err != nil {
		node.Err = errorText(err)
		return
	}
	node.Err = ""
	m.attach(node, m.refine(children))
}

// refine applies the configured empty-directory filter and sort order to a
// freshly scanned level.
func (m *Model) refine(nodes []*Node) []*Node {
	if m.hideEmpty {
		filtered := nodes[:0]
		for _, node := range nodes {
			if node.IsDir && !hasMarkdownBeneath(node.Path, m.showHidden) {
				continue
			}
			filtered = append(filtered, node)
		}
		nodes = filtered
	}
	sortNodesBy(nodes, m.sortBy)
	return nodes
}

func (m *Model) attach(node *Node, children []*Node) {
	for _, child := range children {
		child.parent = node
	}
	node.Children = children
}

// Reload rescans every directory that has already been loaded and returns a
// selection command when the cursor moved to a different file. Matching
// children keep their expansion and load state, so a background filesystem
// change does not collapse the tree. The cursor is restored by path; when the
// previously selected file vanished it falls back to the nearest row.
//
// affected optionally narrows the empty-directory re-evaluation: when
// hide_empty_dirs is on, only directories on or below a changed path (or
// ancestors of one) are walked again. Other directories keep their previous
// filtered state, so a reload stays proportional to the change, not the tree.
func (m *Model) Reload(affected ...string) tea.Cmd {
	path := m.cursorPath()
	index := m.cursor

	m.reload(m.root, affected)
	m.rebuild()

	if i := m.rowIndexByPath(path); i >= 0 {
		m.cursor = i
	} else if index < len(m.rows) {
		m.cursor = index
	}
	m.clampCursor()

	if path != "" && m.cursorPath() == path {
		return nil
	}
	return m.selectionCmd()
}

// LoadedDirs returns every directory the tree has scanned: the root plus each
// directory that has ever been expanded. Collapsed-but-loaded directories stay
// in the list because their children are still cached and can change on disk.
func (m Model) LoadedDirs() []string {
	var dirs []string
	var walk func(*Node)
	walk = func(node *Node) {
		if node == nil {
			return
		}
		if node.IsDir && node.Loaded && !node.cycle {
			dirs = append(dirs, node.Path)
		}
		for _, child := range node.Children {
			walk(child)
		}
	}
	walk(m.root)
	return dirs
}

// toggleHidden re-scans every loaded directory with the new visibility rule.
// Matching children keep their expansion state, so toggling does not collapse
// the tree or lose the cursor.
func (m *Model) toggleHidden() tea.Cmd {
	m.showHidden = !m.showHidden
	return m.Reload()
}

func (m *Model) reload(node *Node, affected []string) {
	if !node.IsDir || !node.Loaded || node.cycle {
		return
	}

	oldChildren := node.Children
	children, err := m.scan(node.Path, m.showHidden)
	if err != nil {
		node.Err = errorText(err)
		return
	}

	existing := make(map[string]*Node, len(oldChildren))
	for _, child := range oldChildren {
		existing[child.Path] = child
	}
	for _, child := range children {
		old, ok := existing[child.Path]
		if !ok || old.IsDir != child.IsDir {
			continue
		}
		child.Expanded = old.Expanded
		child.Loaded = old.Loaded
		child.Err = old.Err
		child.cycle = old.cycle
		child.Children = old.Children
		for _, grandchild := range child.Children {
			grandchild.parent = child
		}
	}

	node.Err = ""
	m.attach(node, m.refineReload(oldChildren, children, affected))
	for _, child := range children {
		m.reload(child, affected)
	}
}

// refineReload applies the empty-directory filter to a rescan. With a scoped
// change set it avoids walking every directory: a directory that was already
// visible and is unrelated to the change keeps that state, while directories
// whose contents changed are re-walked so a now-empty directory can disappear
// and a newly populated one can appear.
func (m *Model) refineReload(oldChildren, nodes []*Node, affected []string) []*Node {
	if !m.hideEmpty || len(affected) == 0 {
		return m.refine(nodes)
	}

	previouslyVisible := make(map[string]bool, len(oldChildren))
	for _, child := range oldChildren {
		previouslyVisible[child.Path] = child.IsDir
	}

	filtered := nodes[:0]
	for _, node := range nodes {
		if node.IsDir {
			touched := touchedBy(node.Path, affected)
			visible, existed := previouslyVisible[node.Path]
			switch {
			case touched:
				if !hasMarkdownBeneath(node.Path, m.showHidden) {
					continue
				}
			case existed && visible:
				// Unchanged directory that the user could already see.
			default:
				// An unrelated new directory stays hidden until a full
				// reload (or its own change event) re-evaluates it.
				continue
			}
		}
		filtered = append(filtered, node)
	}
	sortNodesBy(filtered, m.sortBy)
	return filtered
}

// touchedBy reports whether path contains a changed path or is contained by
// one. Ancestors are included so deleting the last markdown file below a
// visible directory can hide it; descendants are included so a newly created
// directory tree is evaluated once.
func touchedBy(path string, affected []string) bool {
	path = filepath.Clean(path)
	for _, changed := range affected {
		if changed == "" {
			continue
		}
		changed = filepath.Clean(changed)
		if path == changed || isUnder(path, changed) || isUnder(changed, path) {
			return true
		}
	}
	return false
}

func isUnder(path, parent string) bool {
	rel, err := filepath.Rel(parent, path)
	if err != nil {
		return false
	}
	return rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func (m *Model) rebuild() {
	m.rows = m.rows[:0]
	m.appendNode(m.root, 0)
	m.clampCursor()
}

func (m *Model) appendNode(node *Node, depth int) {
	m.rows = append(m.rows, Row{
		Path:     node.Path,
		Name:     node.Name,
		Depth:    depth,
		IsDir:    node.IsDir,
		Expanded: node.Expanded,
		node:     node,
	})
	if !node.IsDir || !node.Expanded {
		return
	}
	for _, child := range node.Children {
		m.appendNode(child, depth+1)
	}
	if node.Err != "" {
		m.rows = append(m.rows, Row{
			Name:    node.Name,
			Depth:   depth + 1,
			Warning: node.Err,
			node:    node,
		})
	}
}

func (m *Model) clampCursor() {
	if len(m.rows) == 0 {
		m.cursor = 0
		m.offset = 0
		return
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor >= len(m.rows) {
		m.cursor = len(m.rows) - 1
	}
	if m.rows[m.cursor].Warning != "" {
		m.cursor = m.nearestSelectable(m.cursor)
	}
	m.ensureCursorVisible()
}

func (m *Model) nearestSelectable(index int) int {
	for i := index; i >= 0; i-- {
		if m.rows[i].Warning == "" {
			return i
		}
	}
	for i := index; i < len(m.rows); i++ {
		if m.rows[i].Warning == "" {
			return i
		}
	}
	return 0
}

func (m *Model) ensureCursorVisible() {
	if len(m.rows) == 0 || m.height <= 0 || m.height >= len(m.rows) {
		m.offset = 0
		return
	}
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+m.height {
		m.offset = m.cursor - m.height + 1
	}
	if m.offset > len(m.rows)-m.height {
		m.offset = len(m.rows) - m.height
	}
	if m.offset < 0 {
		m.offset = 0
	}
}

func (m *Model) rowIndex(node *Node) int {
	for i := range m.rows {
		if m.rows[i].node == node {
			return i
		}
	}
	return -1
}

func (m *Model) rowIndexByPath(path string) int {
	if path == "" {
		return -1
	}
	for i := range m.rows {
		if m.rows[i].Warning == "" && m.rows[i].Path == path {
			return i
		}
	}
	return -1
}

func (m *Model) cursorPath() string {
	row := m.currentRow()
	if row == nil || row.Warning != "" {
		return ""
	}
	return row.Path
}

func rootLabel(path string) string {
	clean := filepath.Clean(path)
	base := filepath.Base(clean)
	if base == "." || base == string(filepath.Separator) || base == "" {
		return clean
	}
	return base
}

func displayName(row Row) string {
	if !row.IsDir || strings.HasSuffix(row.Name, string(filepath.Separator)) {
		return row.Name
	}
	return row.Name + string(filepath.Separator)
}
