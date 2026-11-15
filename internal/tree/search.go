package tree

import (
	"path/filepath"

	"github.com/smhanov/mdui/internal/finder"
)

// Search returns the best matching visible row, the number of matching rows,
// and whether any row matched. Warning and root rows are not searchable.
func (m Model) Search(query string) (best, count int, ok bool) {
	if query == "" {
		return -1, 0, false
	}

	best = -1
	bestScore := 0
	for i, row := range m.rows {
		if row.Warning != "" || i == 0 {
			continue
		}
		candidate := row.Name
		if row.Path != "" && m.root != nil {
			if rel, err := filepath.Rel(m.root.Path, row.Path); err == nil {
				candidate = filepath.ToSlash(rel)
			}
		}
		score, _, matched := finder.Score(candidate, query)
		if !matched {
			continue
		}
		count++
		if best < 0 || score > bestScore {
			best = i
			bestScore = score
		}
	}
	return best, count, count > 0
}

// CursorIndex returns the current cursor's row index.
func (m Model) CursorIndex() int {
	return m.cursor
}
