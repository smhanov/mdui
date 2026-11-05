package mdutil

import (
	"fmt"
	"net/url"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// LinkKind classifies a markdown link destination for dispatch.
type LinkKind int

const (
	// LinkOther is anything mdui refuses to follow (mailto:, ftp:, unknown
	// relative files, and malformed destinations).
	LinkOther LinkKind = iota
	// LinkFile is a relative or absolute markdown document.
	LinkFile
	// LinkAnchor is an in-document #heading reference.
	LinkAnchor
	// LinkRemote is an http(s) URL that may be opened externally.
	LinkRemote
)

func (k LinkKind) String() string {
	switch k {
	case LinkFile:
		return "file"
	case LinkAnchor:
		return "anchor"
	case LinkRemote:
		return "remote"
	default:
		return "other"
	}
}

// Link is one parsed table footnote line.
type Link struct {
	Number    int
	Text      string
	URL       string
	Line      int // 0-based rendered line index
	Truncated bool
}

// LinkTarget is a resolved link ready for dispatch. Path is only set for
// LinkFile; Anchor is set for LinkAnchor and may also accompany LinkFile.
type LinkTarget struct {
	Kind   LinkKind
	Path   string
	Anchor string
	URL    string
}

var footnotePattern = regexp.MustCompile(`^\s*\[\s*(\d+)\s*\]:\s*(.*?)\s*$`)

// ParseLinkFootnotes returns the numbered link footnotes Glamour emits below
// tables (WithInlineTableLinks(false)). Both "[1]: text url" and padded
// "[ 1]: text url" forms are accepted, as well as the case where the only
// token after the colon is the URL. ANSI styling is ignored.
func ParseLinkFootnotes(rendered string) []Link {
	lines := strings.Split(ansi.Strip(rendered), "\n")
	var links []Link
	for i, line := range lines {
		match := footnotePattern.FindStringSubmatch(line)
		if match == nil {
			continue
		}
		number, err := strconv.Atoi(match[1])
		if err != nil {
			continue
		}
		body := strings.TrimSpace(match[2])
		if body == "" {
			continue
		}
		text := ""
		destination := body
		if cut := strings.LastIndexAny(body, " \t"); cut >= 0 {
			text = strings.TrimSpace(body[:cut])
			destination = strings.TrimSpace(body[cut+1:])
		}
		if destination == "" {
			continue
		}
		links = append(links, Link{
			Number:    number,
			Text:      text,
			URL:       destination,
			Line:      i,
			Truncated: strings.HasSuffix(destination, "…"),
		})
	}
	return links
}

// IsFootnoteLine reports whether a rendered line is a numbered table footnote.
func IsFootnoteLine(line string) bool {
	return footnotePattern.MatchString(ansi.Strip(line))
}

// LinkAtLine returns the footnote whose rendered line contains row.
func LinkAtLine(links []Link, line int) (Link, bool) {
	for _, link := range links {
		if link.Line == line {
			return link, true
		}
	}
	return Link{}, false
}

// ClassifyLink classifies a raw markdown link destination. The classification
// is purely lexical; it never touches the filesystem or network.
func ClassifyLink(raw string) LinkKind {
	destination := strings.TrimSpace(raw)
	if destination == "" {
		return LinkOther
	}
	if strings.HasPrefix(destination, "#") {
		return LinkAnchor
	}
	if scheme, ok := schemeOf(destination); ok {
		switch strings.ToLower(scheme) {
		case "http", "https":
			return LinkRemote
		default:
			return LinkOther
		}
	}

	parsed, err := url.Parse(destination)
	if err != nil {
		return LinkOther
	}
	if strings.HasSuffix(strings.ToLower(parsed.Path), ".md") {
		return LinkFile
	}
	return LinkOther
}

// schemeOf returns the URL scheme when destination starts with one. It
// deliberately rejects Windows-style drive letters and bare relative paths.
func schemeOf(destination string) (string, bool) {
	colon := strings.IndexByte(destination, ':')
	if colon <= 0 {
		return "", false
	}
	if slash := strings.IndexByte(destination, '/'); slash >= 0 && slash < colon {
		return "", false
	}
	scheme := destination[:colon]
	for i, r := range scheme {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z':
		case i > 0 && r >= '0' && r <= '9', i > 0 && (r == '+' || r == '-' || r == '.'):
		default:
			return "", false
		}
	}
	return scheme, true
}

// ResolveLink turns a raw destination into a dispatch target. Relative
// markdown paths are resolved against the directory of currentFile. The
// returned error is informational; callers should show a status hint rather
// than crash.
func ResolveLink(currentFile, raw string) (LinkTarget, error) {
	destination := strings.TrimSpace(raw)
	target := LinkTarget{Kind: ClassifyLink(destination)}

	switch target.Kind {
	case LinkAnchor:
		target.Anchor = unescape(destination[1:])
		return target, nil
	case LinkRemote, LinkOther:
		target.URL = destination
		return target, nil
	}

	parsed, err := url.Parse(destination)
	if err != nil {
		return LinkTarget{Kind: LinkOther, URL: destination}, fmt.Errorf("invalid link: %w", err)
	}
	path := unescape(parsed.Path)
	if path == "" {
		return LinkTarget{Kind: LinkOther, URL: destination}, fmt.Errorf("empty link path")
	}
	if !filepath.IsAbs(path) {
		if currentFile == "" {
			return LinkTarget{Kind: LinkOther, URL: destination}, fmt.Errorf("no document to resolve the link against")
		}
		path = filepath.Join(filepath.Dir(currentFile), path)
	}
	target.Path = filepath.Clean(path)
	target.Anchor = unescape(parsed.Fragment)
	return target, nil
}

func unescape(value string) string {
	if unescaped, err := url.PathUnescape(value); err == nil {
		return unescaped
	}
	return value
}
