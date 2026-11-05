// Package render owns markdown rendering: Glamour setup, background style
// resolution, and the cache shared by asynchronous render jobs.
package render

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
	glamour "charm.land/glamour/v2"
	"charm.land/glamour/v2/styles"
	"charm.land/lipgloss/v2"
)

const (
	// MaxRenderBytes is the largest file rendered automatically. Bigger files
	// get a warning and offer their raw source instead.
	MaxRenderBytes = 2 << 20

	// CacheSize is the number of rendered documents kept in memory.
	CacheSize = 8
)

var defaultCache = NewCache(CacheSize)

// Style is a Glamour style: a built-in name such as "dark" or "light", or a
// path to a JSON stylesheet. It is an alias so callers can pass plain strings.
type Style = string

const (
	// Dark is Glamour's dark-terminal stylesheet.
	Dark Style = "dark"
	// Light is Glamour's light-terminal stylesheet.
	Light Style = "light"
)

// ResolveStyle picks the document style in priority order: the GLAMOUR_STYLE
// environment variable, then the terminal background, then dark. The value
// "auto" in GLAMOUR_STYLE behaves as if the variable were unset. A nil
// hasDark function also falls back to dark.
func ResolveStyle(getenv func(string) string, hasDark func() bool) Style {
	if getenv != nil {
		if value := strings.TrimSpace(getenv("GLAMOUR_STYLE")); value != "" && !strings.EqualFold(value, "auto") {
			return Style(value)
		}
	}
	if hasDark != nil && !hasDark() {
		return Light
	}
	return Dark
}

// DefaultStyle resolves the startup style against the real environment and
// terminal. It queries the terminal, so call it before starting the TUI rather
// than from an Update method.
func DefaultStyle() Style {
	return ResolveStyle(os.Getenv, func() bool {
		return lipgloss.HasDarkBackground(os.Stdin, os.Stdout)
	})
}

// RenderedMsg is the result of a render, raw-load, or size-check job. RawOnly
// marks a response to RawCmd; those results use the viewer's raw request
// counter instead of its render counter.
type RenderedMsg struct {
	ReqID    uint64
	Path     string
	Width    int
	Style    Style
	Rendered string
	Raw      string
	Size     int64
	Large    bool
	RawOnly  bool
	Err      error
}

// NewCmd renders the markdown file at path asynchronously. It stats the file
// before reading so files above MaxRenderBytes come back as Large without
// loading the document. Results for the same path, mtime, width, and style are
// served from the render cache.
func NewCmd(reqID uint64, path string, width int, style Style) tea.Cmd {
	return func() tea.Msg {
		absPath, err := filepath.Abs(path)
		if err != nil {
			absPath = filepath.Clean(path)
		}

		info, err := os.Stat(absPath)
		if err != nil {
			return RenderedMsg{
				ReqID: reqID,
				Path:  path,
				Width: width,
				Style: style,
				Err:   err,
			}
		}
		if info.Size() > MaxRenderBytes {
			return RenderedMsg{
				ReqID: reqID,
				Path:  path,
				Width: width,
				Style: style,
				Size:  info.Size(),
				Large: true,
			}
		}

		data, err := os.ReadFile(absPath)
		if err != nil {
			return RenderedMsg{
				ReqID: reqID,
				Path:  path,
				Width: width,
				Style: style,
				Size:  info.Size(),
				Err:   err,
			}
		}

		key := Key{
			Path:    absPath,
			ModTime: info.ModTime().UnixNano(),
			Width:   width,
			Style:   style,
		}
		rendered, err := renderDocument(key, data, width, style)
		return RenderedMsg{
			ReqID:    reqID,
			Path:     path,
			Width:    width,
			Style:    style,
			Rendered: rendered,
			Raw:      string(data),
			Size:     info.Size(),
			Err:      err,
		}
	}
}

// BytesCmd renders an embedded document, such as the viewer's welcome note,
// through the same pipeline and cache as files. name is only used as the cache
// identity.
func BytesCmd(reqID uint64, name string, data []byte, width int, style Style) tea.Cmd {
	return func() tea.Msg {
		key := Key{
			Path:  name,
			Width: width,
			Style: style,
		}
		rendered, err := renderDocument(key, data, width, style)
		return RenderedMsg{
			ReqID:    reqID,
			Path:     name,
			Width:    width,
			Style:    style,
			Rendered: rendered,
			Raw:      string(data),
			Size:     int64(len(data)),
			Err:      err,
		}
	}
}

// RawCmd loads a file's source without rendering it. It backs the raw view for
// files too large to render.
func RawCmd(reqID uint64, path string) tea.Cmd {
	return func() tea.Msg {
		data, err := os.ReadFile(path)
		return RenderedMsg{
			ReqID:   reqID,
			Path:    path,
			Raw:     string(data),
			Size:    int64(len(data)),
			RawOnly: true,
			Err:     err,
		}
	}
}

func renderDocument(key Key, data []byte, width int, style Style) (string, error) {
	if cached, ok := defaultCache.Get(key); ok {
		return cached, nil
	}

	rendered, err := renderMarkdown(data, width, style)
	if err != nil {
		return "", err
	}
	defaultCache.Put(key, rendered)
	return rendered, nil
}

func renderMarkdown(data []byte, width int, style Style) (string, error) {
	if width < 1 {
		width = 1
	}

	renderer, err := glamour.NewTermRenderer(
		styleOption(style),
		glamour.WithWordWrap(width),
		glamour.WithInlineTableLinks(false),
		glamour.WithTableWrap(true),
		glamour.WithChromaFormatter("terminal16m"),
	)
	if err != nil {
		return "", fmt.Errorf("render: %w", err)
	}

	rendered, err := renderer.Render(string(data))
	if err != nil {
		return "", fmt.Errorf("render: %w", err)
	}
	return strings.Trim(rendered, "\n"), nil
}

// styleOption uses Glamour's built-in stylesheet loader for standard names and
// the path loader for user stylesheets.
func styleOption(style Style) glamour.TermRendererOption {
	name := string(style)
	if _, ok := styles.DefaultStyles[name]; ok {
		return glamour.WithStandardStyle(name)
	}
	return glamour.WithStylePath(name)
}
