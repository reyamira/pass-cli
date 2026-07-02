// Package skills serves version-matched, agent-facing usage guides embedded in
// the pass-cli binary. The embedded markdown is the single source of truth for
// how an AI agent should drive pass-cli; the `pass-cli skills` command is a thin
// wrapper over the accessors here, and `skills install` writes a discovery stub
// (StubContent) into an agent's skills directory that just points back at
// `pass-cli skills get core`.
package skills

import (
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strings"
)

//go:embed all:data
var dataFS embed.FS

const (
	skillsDir = "data/skills"
	stubPath  = "data/stub.md"
	mdExt     = ".md"
	fullExt   = ".full.md"
)

// Skill is a listable skill: its name and the description parsed from the
// markdown file's frontmatter.
type Skill struct {
	Name        string
	Description string
}

// List returns every listable skill (a `<name>.md` file, excluding the
// `<name>.full.md` companions), sorted by name. The description comes from each
// file's frontmatter.
func List() ([]Skill, error) {
	entries, err := fs.ReadDir(dataFS, skillsDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read embedded skills: %w", err)
	}

	var out []Skill
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, mdExt) || strings.HasSuffix(name, fullExt) {
			continue
		}
		body, err := fs.ReadFile(dataFS, skillsDir+"/"+name)
		if err != nil {
			return nil, fmt.Errorf("failed to read embedded skill %q: %w", name, err)
		}
		fm, _ := splitFrontmatter(string(body))
		out = append(out, Skill{
			Name:        strings.TrimSuffix(name, mdExt),
			Description: fm["description"],
		})
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// Get returns the body of a skill (frontmatter stripped). When full is true and
// a `<name>.full.md` companion exists, it is appended. An unknown name returns
// an error that lists the valid names.
func Get(name string, full bool) (string, error) {
	body, err := fs.ReadFile(dataFS, skillsDir+"/"+name+mdExt)
	if err != nil {
		return "", unknownSkillError(name)
	}
	_, content := splitFrontmatter(string(body))
	out := strings.Trim(content, "\n")

	if full {
		if extra, err := fs.ReadFile(dataFS, skillsDir+"/"+name+fullExt); err == nil {
			_, extraContent := splitFrontmatter(string(extra))
			out += "\n\n" + strings.Trim(extraContent, "\n")
		}
	}

	return out + "\n", nil
}

// StubContent returns the discovery-stub markdown written by `skills install`.
func StubContent() (string, error) {
	b, err := fs.ReadFile(dataFS, stubPath)
	if err != nil {
		return "", fmt.Errorf("failed to read embedded stub: %w", err)
	}
	return string(b), nil
}

func unknownSkillError(name string) error {
	list, err := List()
	if err != nil {
		return fmt.Errorf("unknown skill %q", name)
	}
	names := make([]string, len(list))
	for i, s := range list {
		names[i] = s.Name
	}
	return fmt.Errorf("unknown skill %q; available: %s", name, strings.Join(names, ", "))
}

// splitFrontmatter separates a leading `---`-delimited YAML frontmatter block
// from the markdown body. It returns the parsed frontmatter fields (only simple
// single-line `key: value` pairs are recognized) and the remaining body. When
// there is no frontmatter, the fields map is empty and the whole input is the
// body.
func splitFrontmatter(s string) (map[string]string, string) {
	fields := map[string]string{}

	// Normalize CRLF so the delimiter check is newline-agnostic.
	norm := strings.ReplaceAll(s, "\r\n", "\n")
	if !strings.HasPrefix(norm, "---\n") {
		return fields, s
	}

	rest := norm[len("---\n"):]
	end := strings.Index(rest, "\n---")
	if end < 0 {
		// No closing delimiter — treat the whole thing as body.
		return fields, s
	}

	block := rest[:end]
	for _, line := range strings.Split(block, "\n") {
		key, val, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)
		val = strings.Trim(val, `"'`)
		if key != "" {
			fields[key] = val
		}
	}

	// Body starts after the closing "---" line.
	body := rest[end+len("\n---"):]
	body = strings.TrimPrefix(body, "\n")
	return fields, body
}
