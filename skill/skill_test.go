package skill

import (
	"os"
	"path/filepath"
	"testing"
)

// TestLoadDir verifies front-matter and Markdown loading.
// @param t test state.
// @return none.
func TestLoadDir(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "review")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "---\nname: code-review\ndescription: Review code\nmax_steps: 8\ntools:\n  - get_diff\n  - get_file\n---\nRead the diff first.\n"
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	skills, err := LoadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	skill := skills["code-review"]
	if skill.MaxSteps != 8 || len(skill.Tools) != 2 || skill.Instructions != "Read the diff first." {
		t.Fatalf("skill = %#v", skill)
	}
}
