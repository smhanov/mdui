package style

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestEmbeddedStylesheets(t *testing.T) {
	names := DefaultStyleNames()
	if len(names) != 2 {
		t.Fatalf("DefaultStyleNames = %v, want two sheets", names)
	}
	for _, name := range names {
		data, ok := LookupStylesheet(name)
		if !ok {
			t.Fatalf("missing embedded sheet %q", name)
		}
		var parsed map[string]any
		if err := json.Unmarshal(data, &parsed); err != nil {
			t.Fatalf("%s is not valid JSON: %v", name, err)
		}
		table, ok := parsed["table"].(map[string]any)
		if !ok {
			t.Fatalf("%s has no table style", name)
		}
		if table["bold"] != true {
			t.Errorf("%s table is not bold", name)
		}
		if table["center_separator"] != "┼" {
			t.Errorf("%s table separators were not tuned", name)
		}
	}

	if _, ok := LookupStylesheet("mdui-dark.json"); !ok {
		t.Error("LookupStylesheet should accept a .json suffix")
	}
	if _, ok := LookupStylesheet("missing"); ok {
		t.Error("LookupStylesheet found a style that does not exist")
	}
}

func TestHelpMarkdownListsImplementedKeys(t *testing.T) {
	help := string(HelpMarkdown())
	for _, want := range []string{
		"`q`", "`ctrl+c`", "`?`", "`m`", "`tab`", "`shift+tab`",
		"`r`", "`H`", "`L`", "`=`", "`g`", "`G`", "`ctrl+d`", "`ctrl+u`",
		"`←`", "`→`", "`enter`", "`pgup`", "`pgdn`", "`.`", "double-click",
	} {
		if !strings.Contains(help, want) {
			t.Errorf("help cheatsheet missing %q", want)
		}
	}
	for _, unwanted := range []string{"fuzzy", "`e`", "`t`", "`/`", "`f`"} {
		if strings.Contains(help, unwanted) {
			t.Errorf("help advertises unimplemented feature %q", unwanted)
		}
	}
}
