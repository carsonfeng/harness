// Package skill loads reusable Markdown instructions for agent runs.
package skill

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Skill contains reusable instructions, a tool allowlist, and a run limit.
type Skill struct {
	Name         string
	Description  string
	Instructions string
	Tools        []string
	MaxSteps     int
}

// LoadDir reads SKILL.md from each immediate child directory.
// @param root directory containing skill folders.
// @return skills by name or loading error.
func LoadDir(root string) (map[string]Skill, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("load skills: %w", err)
	}
	skills := make(map[string]Skill)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		path := filepath.Join(root, entry.Name(), "SKILL.md")
		data, err := os.ReadFile(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("load skill %s: %w", entry.Name(), err)
		}
		skill, err := parse(entry.Name(), string(data))
		if err != nil {
			return nil, fmt.Errorf("load skill %s: %w", entry.Name(), err)
		}
		if _, exists := skills[skill.Name]; exists {
			return nil, fmt.Errorf("load skills: duplicate name %q", skill.Name)
		}
		skills[skill.Name] = skill
	}
	return skills, nil
}

// parse reads supported front matter and Markdown instructions.
// @param defaultName directory-derived skill name.
// @param content SKILL.md content.
// @return parsed skill or syntax error.
func parse(defaultName, content string) (Skill, error) {
	skill := Skill{Name: defaultName}
	content = strings.ReplaceAll(content, "\r\n", "\n")
	if !strings.HasPrefix(content, "---\n") {
		skill.Instructions = strings.TrimSpace(content)
		return skill, nil
	}
	parts := strings.SplitN(strings.TrimPrefix(content, "---\n"), "\n---\n", 2)
	if len(parts) != 2 {
		return Skill{}, errors.New("unterminated front matter")
	}
	var list string
	scanner := bufio.NewScanner(strings.NewReader(parts[0]))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "-") {
			if list != "tools" {
				return Skill{}, fmt.Errorf("unexpected list item %q", line)
			}
			name := strings.TrimSpace(strings.TrimPrefix(line, "-"))
			if !validName(name) {
				return Skill{}, fmt.Errorf("invalid tool name %q", name)
			}
			skill.Tools = append(skill.Tools, name)
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			return Skill{}, fmt.Errorf("invalid front matter line %q", line)
		}
		list = ""
		value = strings.Trim(strings.TrimSpace(value), "\"'")
		switch strings.TrimSpace(key) {
		case "name":
			skill.Name = value
		case "description":
			skill.Description = value
		case "max_steps":
			steps, err := strconv.Atoi(value)
			if err != nil || steps < 1 {
				return Skill{}, errors.New("max_steps must be a positive integer")
			}
			skill.MaxSteps = steps
		case "tools":
			list = "tools"
		default:
			return Skill{}, fmt.Errorf("unsupported key %q", strings.TrimSpace(key))
		}
	}
	if err := scanner.Err(); err != nil {
		return Skill{}, err
	}
	if !validName(skill.Name) {
		return Skill{}, fmt.Errorf("invalid skill name %q", skill.Name)
	}
	skill.Instructions = strings.TrimSpace(parts[1])
	return skill, nil
}

// validName checks portable skill and tool names.
// @param name candidate name.
// @return true when valid.
func validName(name string) bool {
	if name == "" {
		return false
	}
	for i, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (i > 0 && r >= '0' && r <= '9') || (i > 0 && (r == '_' || r == '-')) {
			continue
		}
		return false
	}
	return true
}
