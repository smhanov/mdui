package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	tea "charm.land/bubbletea/v2"

	"github.com/smhanov/mdui/internal/app"
	"github.com/smhanov/mdui/internal/config"
	"github.com/smhanov/mdui/internal/render"
)

func main() {
	var (
		styleFlag   = flag.String("style", "", "document style: auto, dark, light, a name, or a JSON path")
		mouseFlag   = flag.Bool("mouse", false, "enable mouse capture (default: config)")
		noMouseFlag = flag.Bool("no-mouse", false, "disable mouse capture")
		widthFlag   = flag.Int("width", -1, "reflow width in columns; 0 uses the pane width")
		emojiFlag   = flag.Bool("emoji", false, "render :emoji: shortcodes as unicode")
		noEmojiFlag = flag.Bool("no-emoji", false, "leave :emoji: shortcodes as written")
		hiddenFlag  = flag.Bool("hidden", false, "show dot-prefixed entries")
		sortFlag    = flag.String("sort", "", "tree order: name, mtime, or size")
	)

	flag.Usage = func() {
		out := flag.CommandLine.Output()
		fmt.Fprintf(out, "usage: mdui [flags] [folder]\n\n")
		fmt.Fprintln(out, "Open a folder of markdown files in a two-pane terminal viewer.")
		fmt.Fprintln(out, "The folder defaults to the current directory.")
		fmt.Fprintln(out)
		fmt.Fprintln(out, "Flags override ~/.config/mdui/config.toml. Press ? in the viewer")
		fmt.Fprintln(out, "for the full key list.")
		fmt.Fprintln(out)
		flag.PrintDefaults()
	}
	flag.Parse()

	if flag.NArg() > 1 {
		flag.Usage()
		os.Exit(2)
	}

	root := "."
	if flag.NArg() == 1 {
		root = flag.Arg(0)
	}

	if err := validateDir(root); err != nil {
		fmt.Fprintf(os.Stderr, "mdui: %v\n", err)
		os.Exit(1)
	}

	if abs, err := filepath.Abs(root); err == nil {
		root = abs
	}

	cfg := loadConfig()
	set := make(map[string]bool)
	flag.Visit(func(f *flag.Flag) { set[f.Name] = true })

	if set["mouse"] && set["no-mouse"] {
		fmt.Fprintln(os.Stderr, "mdui: use only one of --mouse and --no-mouse")
		os.Exit(2)
	}
	if set["mouse"] {
		cfg.Mouse = *mouseFlag
	}
	if set["no-mouse"] && *noMouseFlag {
		cfg.Mouse = false
	}
	if set["width"] {
		cfg.WrapWidth = max(0, *widthFlag)
	}
	if set["emoji"] {
		cfg.Emoji = *emojiFlag
	}
	if set["no-emoji"] && *noEmojiFlag {
		cfg.Emoji = false
	}
	if set["hidden"] {
		cfg.HiddenFiles = *hiddenFlag
	}
	if set["sort"] && *sortFlag != "" {
		cfg.SortBy = *sortFlag
	}

	stylesDir, err := config.StylesPath()
	if err != nil {
		stylesDir = ""
	}
	cliStyle := ""
	if set["style"] {
		cliStyle = *styleFlag
	}
	statePath, err := config.StatePath()
	if err != nil {
		statePath = ""
	}
	ratio := config.StartupTreeRatio(cfg, statePath)

	// Probe the terminal background only if the style precedence actually
	// needs detection: an explicit flag, config, or GLAMOUR_STYLE skips the
	// query entirely. The probe is bounded, so a terminal that never
	// answers capability queries cannot stall startup or swallow
	// keystrokes typed while mdui starts; whatever it read is replayed
	// into the program's input below so no typed key is lost.
	var probeConsumed []byte
	hasDark := func() bool {
		dark, consumed := render.ProbeBackground(os.Stdin, os.Stdout)
		probeConsumed = consumed
		return dark
	}
	docStyle := config.ResolveStyle(cliStyle, cfg.Style, os.Getenv, hasDark, stylesDir)

	model := app.New(
		root,
		app.WithStyle(docStyle),
		app.WithConfig(cfg),
		app.WithTreeRatio(ratio),
		app.WithStatePath(statePath),
	)
	opts := []tea.ProgramOption{}
	if len(probeConsumed) > 0 {
		opts = append(opts, tea.WithInput(newReplayInput(os.Stdin, probeConsumed)))
	}
	if _, err := tea.NewProgram(model, opts...).Run(); err != nil {
		if msg, ok := userTTYError(err); ok {
			fmt.Fprintf(os.Stderr, "mdui: %s\n", msg)
		} else {
			fmt.Fprintf(os.Stderr, "mdui: %v\n", err)
		}
		os.Exit(1)
	}
}

// loadConfig creates the first-run config directory and falls back to the
// built-in defaults when the config cannot be read or written.
func loadConfig() config.Config {
	path, err := config.ConfigPath()
	if err != nil {
		fmt.Fprintf(os.Stderr, "mdui: config: %v\n", err)
		return config.DefaultConfig()
	}
	cfg, err := config.LoadOrCreate(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "mdui: config: %v\n", err)
	}
	return cfg
}

// validateDir makes sure path exists, is a directory, and can be listed before
// the TUI takes over the terminal. Errors read as "<path>: <reason>".
func validateDir(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return pathError(path, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("%s: not a directory", path)
	}

	f, err := os.Open(path)
	if err != nil {
		return pathError(path, err)
	}
	defer f.Close()

	if _, err := f.Readdirnames(1); err != nil && !errors.Is(err, io.EOF) {
		return pathError(path, err)
	}
	return nil
}

// pathError strips the OS's redundant operation/path prefix so the caller can
// report the offending path exactly once.
func pathError(path string, err error) error {
	var pe *os.PathError
	if errors.As(err, &pe) {
		err = pe.Err
	}
	return fmt.Errorf("%s: %w", path, err)
}

// userTTYError recognizes the error bubbletea returns when mdui starts without
// a terminal to attach to and maps it to a single friendly line. The ok result
// is false for any other error, which callers report verbatim.
func userTTYError(err error) (string, bool) {
	const msg = "no interactive terminal available (mdui needs a TTY to display its interface)"
	if errors.Is(err, syscall.ENXIO) || errors.Is(err, syscall.ENOENT) ||
		strings.Contains(err.Error(), "could not open TTY") {
		return msg, true
	}
	return "", false
}

// replayInput is the program's input reader: it first returns the bytes the
// background probe consumed from the terminal, then keeps reading stdin
// directly. Keystrokes typed while the probe was waiting therefore reach
// the app instead of being dropped. Embedding the file keeps bubbletea's
// raw-mode setup working, which needs the real file descriptor.
type replayInput struct {
	*os.File
	pending []byte
}

func newReplayInput(in *os.File, replay []byte) *replayInput {
	return &replayInput{File: in, pending: replay}
}

func (r *replayInput) Read(p []byte) (int, error) {
	if len(r.pending) > 0 {
		n := copy(p, r.pending)
		r.pending = r.pending[n:]
		return n, nil
	}
	return r.File.Read(p)
}
