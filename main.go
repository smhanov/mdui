package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	tea "charm.land/bubbletea/v2"

	"mdui/internal/app"
	"mdui/internal/render"
)

func main() {
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "usage: mdui [folder]\n\n")
		fmt.Fprintln(flag.CommandLine.Output(), "Open a folder of markdown files in a two-pane terminal viewer.")
		fmt.Fprintln(flag.CommandLine.Output(), "The folder defaults to the current directory.")
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

	if _, err := tea.NewProgram(app.New(root, app.WithStyle(render.DefaultStyle()))).Run(); err != nil {
		fmt.Fprintf(os.Stderr, "mdui: %v\n", err)
		os.Exit(1)
	}
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
