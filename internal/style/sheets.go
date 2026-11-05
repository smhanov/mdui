package style

import (
	"embed"
	"path"
	"strings"

	"charm.land/lipgloss/v2"
)

//go:embed styles/*.json
var stylesheets embed.FS

//go:embed help.md
var helpMarkdown []byte

// defaultStylesheets lists the stylesheets compiled into mdui.
var defaultStylesheets = []string{"mdui-dark", "mdui-light"}

// DefaultStyleNames returns the names of the stylesheets shipped with mdui.
func DefaultStyleNames() []string {
	return append([]string(nil), defaultStylesheets...)
}

// LookupStylesheet returns the embedded stylesheet for name, which may be
// given with or without the .json suffix.
func LookupStylesheet(name string) ([]byte, bool) {
	name = strings.TrimSpace(name)
	name = strings.TrimSuffix(name, ".json")
	for _, candidate := range defaultStylesheets {
		if name == candidate {
			data, err := stylesheets.ReadFile(path.Join("styles", candidate+".json"))
			if err != nil {
				return nil, false
			}
			return data, true
		}
	}
	return nil, false
}

// HelpMarkdown is the embedded keybinding cheatsheet shown by the help
// overlay.
func HelpMarkdown() []byte {
	return helpMarkdown
}

var (
	// Backdrop dims the normal layout behind an overlay.
	Backdrop = lipgloss.NewStyle().
			Faint(true)

	// HelpBorder frames the help overlay.
	HelpBorder = lipgloss.NewStyle().
			Foreground(lipgloss.Color(muted))

	// MouseHint is the bar shown while mouse capture is off.
	MouseHint = lipgloss.NewStyle().
			Foreground(lipgloss.Color("3")).
			Bold(true)
)
