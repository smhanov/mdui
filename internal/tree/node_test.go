package tree

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
)

func TestScanDirFiltersAndSorts(t *testing.T) {
	root := t.TempDir()
	mkdir(t, root, "zdir")
	mkdir(t, root, "adir")
	mkdir(t, root, ".hidden-dir")
	writeNote(t, root, "a10.md")
	writeNote(t, root, "a2.md")
	writeNote(t, root, "README.MD")
	writeNote(t, root, "notes.markdown")
	writeNote(t, root, "日本語.md")
	writeNote(t, root, ".hidden.md")
	writeNote(t, root, "image.png")
	writeNote(t, root, "notes.txt")

	nodes, err := ScanDir(root, false)
	if err != nil {
		t.Fatalf("ScanDir: %v", err)
	}

	want := []string{
		"adir", "zdir",
		"a2.md", "a10.md", "notes.markdown", "README.MD", "日本語.md",
	}
	if got := nodeNames(nodes); !reflect.DeepEqual(got, want) {
		t.Fatalf("visible nodes = %v, want %v", got, want)
	}
	for _, node := range nodes[:2] {
		if !node.IsDir {
			t.Errorf("%q should be a directory", node.Name)
		}
	}

	withHidden, err := ScanDir(root, true)
	if err != nil {
		t.Fatalf("ScanDir(showHidden): %v", err)
	}
	wantHidden := []string{
		".hidden-dir", "adir", "zdir",
		".hidden.md", "a2.md", "a10.md", "notes.markdown", "README.MD", "日本語.md",
	}
	if got := nodeNames(withHidden); !reflect.DeepEqual(got, wantHidden) {
		t.Fatalf("nodes with hidden = %v, want %v", got, wantHidden)
	}
}

func TestCompareNatural(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"a2", "a10", -1},
		{"a10", "a2", 1},
		{"a2", "a2", 0},
		{"a02", "a2", 0},
		{"file1", "file01", 0},
		{"x9y", "x10y", -1},
		{"Foo", "foo", 0},
		{"bar", "Foo", -1},
		{"abc", "abd", -1},
		{"abc", "ab", 1},
		{"2", "10", -1},
		{"v1.2", "v1.10", -1},
	}
	for _, tt := range tests {
		if got := compareNatural(tt.a, tt.b); got != tt.want {
			t.Errorf("compareNatural(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestSortNodesDirsFirstAndStable(t *testing.T) {
	nodes := []*Node{
		{Name: "foo"},
		{Name: "zdir", IsDir: true},
		{Name: "Bar"},
		{Name: "adir", IsDir: true},
		{Name: "foo"},
		{Name: "bar"},
	}
	sortNodes(nodes)

	want := []string{"adir", "zdir", "Bar", "bar", "foo", "foo"}
	if got := nodeNames(nodes); !reflect.DeepEqual(got, want) {
		t.Fatalf("sorted names = %v, want %v", got, want)
	}
}

func TestScanDirUnreadable(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("directory permissions are not enforced for this user")
	}
	root := t.TempDir()
	locked := mkdir(t, root, "locked")
	if err := os.Chmod(locked, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o700) })

	if _, err := ScanDir(locked, false); err == nil {
		t.Fatal("ScanDir(unreadable) = nil error, want failure")
	}
}

func mkdir(t *testing.T, parent, name string) string {
	t.Helper()
	path := filepath.Join(parent, name)
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func writeNote(t *testing.T, dir, name string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("# "+name+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func nodeNames(nodes []*Node) []string {
	names := make([]string, len(nodes))
	for i, node := range nodes {
		names[i] = node.Name
	}
	return names
}
