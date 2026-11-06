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

	"github.com/smhanov/mdui/internal/style"
)

const (
	// MaxRenderBytes is the largest file rendered automatically. Bigger files
	// get a warning and offer their raw source instead.
	MaxRenderBytes = 2 << 20

	// CacheSize is the number of rendered documents kept in memory.
	CacheSize = 8
)

var defaultCache = NewCache(CacheSize)

// Style is a Glamour style: a built-in name such as "dark", an embedded mdui
// stylesheet, or a path to a JSON stylesheet. It is an alias so callers can
// pass plain strings.
type Style = string

const (
	// Dark is Glamour's built-in dark-terminal stylesheet.
	Dark Style = "dark"
	// Light is Glamour's built-in light-terminal stylesheet.
	Light Style = "light"
	// MduiDark is mdui's tuned dark stylesheet.
	MduiDark Style = "mdui-dark"
	// MduiLight is mdui's tuned light stylesheet.
	MduiLight Style = "mdui-light"
)

// Options carries the renderer settings that are not already expressed by the
// style name or the target width.
type Options struct {
	Style Style
	Emoji bool
}

// ResolveStyle picks the document style from the environment and terminal
// background: GLAMOUR_STYLE first, then auto-detection with mdui's embedded
// dark/light stylesheets. The value "auto" behaves as if the variable were
// unset. A nil hasDark function also falls back to dark.
func ResolveStyle(getenv func(string) string, hasDark func() bool) Style {
	return ResolveStylePrecedence("", "", getenv, hasDark)
}

// ResolveStylePrecedence applies the full startup precedence: CLI flag, then
// config file, then GLAMOUR_STYLE, then terminal background detection. An
// "auto" or empty value at any level falls through to the next.
func ResolveStylePrecedence(cli, config Style, getenv func(string) string, hasDark func() bool) Style {
	for _, candidate := range []Style{cli, config} {
		if value := strings.TrimSpace(candidate); value != "" && !strings.EqualFold(value, "auto") {
			return Style(value)
		}
	}
	if getenv != nil {
		if value := strings.TrimSpace(getenv("GLAMOUR_STYLE")); value != "" && !strings.EqualFold(value, "auto") {
			return Style(value)
		}
	}
	if hasDark != nil && !hasDark() {
		return MduiLight
	}
	return MduiDark
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

// NewCmd renders the markdown file at path asynchronously with default
// options. It stats the file before reading so files above MaxRenderBytes come
// back as Large without loading the document.
func NewCmd(reqID uint64, path string, width int, style Style) tea.Cmd {
	return NewCmdWith(reqID, path, width, Options{Style: style})
}

// NewCmdWith renders the markdown file at path asynchronously. Results for the
// same path, mtime, width, style, and emoji setting are served from the render
// cache.
func NewCmdWith(reqID uint64, path string, width int, opts Options) tea.Cmd {
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
				Style: opts.Style,
				Err:   err,
			}
		}
		if info.Size() > MaxRenderBytes {
			return RenderedMsg{
				ReqID: reqID,
				Path:  path,
				Width: width,
				Style: opts.Style,
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
				Style: opts.Style,
				Size:  info.Size(),
				Err:   err,
			}
		}

		key := Key{
			Path:    absPath,
			ModTime: info.ModTime().UnixNano(),
			Width:   width,
			Style:   opts.Style,
			Emoji:   opts.Emoji,
		}
		rendered, err := renderDocument(key, data, width, opts)
		return RenderedMsg{
			ReqID:    reqID,
			Path:     path,
			Width:    width,
			Style:    opts.Style,
			Rendered: rendered,
			Raw:      string(data),
			Size:     info.Size(),
			Err:      err,
		}
	}
}

// BytesCmdWith is BytesCmd with explicit renderer options.
func BytesCmdWith(reqID uint64, name string, data []byte, width int, opts Options) tea.Cmd {
	return func() tea.Msg {
		key := Key{
			Path:  name,
			Width: width,
			Style: opts.Style,
			Emoji: opts.Emoji,
		}
		rendered, err := renderDocument(key, data, width, opts)
		return RenderedMsg{
			ReqID:    reqID,
			Path:     name,
			Width:    width,
			Style:    opts.Style,
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

// RenderBytes renders an in-memory document synchronously. It is used for the
// help overlay, which is small and rendered once when opened.
func RenderBytes(data []byte, width int, opts Options) (string, error) {
	return renderMarkdown(data, width, opts)
}

func renderDocument(key Key, data []byte, width int, opts Options) (string, error) {
	if cached, ok := defaultCache.Get(key); ok {
		return cached, nil
	}

	rendered, err := renderMarkdown(data, width, opts)
	if err != nil {
		return "", err
	}
	defaultCache.Put(key, rendered)
	return rendered, nil
}

func renderMarkdown(data []byte, width int, opts Options) (string, error) {
	if width < 1 {
		width = 1
	}

	rendererOpts := []glamour.TermRendererOption{
		styleOption(opts.Style),
		glamour.WithWordWrap(width),
		glamour.WithInlineTableLinks(false),
		glamour.WithTableWrap(true),
		glamour.WithChromaFormatter("terminal16m"),
	}
	if opts.Emoji {
		rendererOpts = append(rendererOpts, glamour.WithEmoji())
	}

	renderer, err := glamour.NewTermRenderer(rendererOpts...)
	if err != nil {
		return "", fmt.Errorf("render: %w", err)
	}

	rendered, err := renderer.Render(string(joinSoftBreaks(data)))
	if err != nil {
		return "", fmt.Errorf("render: %w", err)
	}
	return strings.Trim(rendered, "\n"), nil
}

// styleOption resolves embedded mdui stylesheets first, then Glamour's
// built-in styles, then arbitrary stylesheet paths.
func styleOption(docStyle Style) glamour.TermRendererOption {
	name := string(docStyle)
	if data, ok := style.LookupStylesheet(name); ok {
		return glamour.WithStylesFromJSONBytes(data)
	}
	if _, ok := styles.DefaultStyles[name]; ok {
		return glamour.WithStandardStyle(name)
	}
	return glamour.WithStylePath(name)
}
