package skills

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"path"
	"strings"

	"gopkg.in/yaml.v3"
)

//go:embed all:*
var embeddedFS embed.FS

type Frontmatter struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
}

type Skill struct {
	Name        string
	Description string
	Location    string
}

func LoadAll() ([]Skill, error) {
	entries, err := fs.ReadDir(embeddedFS, ".")
	if err != nil {
		return nil, fmt.Errorf("read embedded skills: %w", err)
	}
	var skills []Skill
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		filePath := path.Join(entry.Name(), "SKILL.md")
		raw, err := fs.ReadFile(embeddedFS, filePath)
		if err != nil {
			continue
		}
		fm := parseFrontmatter(string(raw))
		name := fm.Name
		if name == "" {
			name = entry.Name()
		}
		skills = append(skills, Skill{
			Name:        name,
			Description: fm.Description,
			Location:    filePath,
		})
	}
	return skills, nil
}

func ReadSkill(name string) (string, error) {
	filePath := path.Join(name, "SKILL.md")
	raw, err := fs.ReadFile(embeddedFS, filePath)
	if err != nil {
		return "", fmt.Errorf("skill %q not found: %w", name, err)
	}
	_, body := splitFrontmatter(string(raw))
	return strings.TrimSpace(body), nil
}

func ReadSkillRaw(name string) ([]byte, error) {
	filePath := path.Join(name, "SKILL.md")
	raw, err := fs.ReadFile(embeddedFS, filePath)
	if err != nil {
		return nil, fmt.Errorf("skill %q not found: %w", name, err)
	}
	return raw, nil
}

func ReadSchema(name string) (map[string]any, error) {
	filePath := path.Join(name, "schema.json")
	raw, err := fs.ReadFile(embeddedFS, filePath)
	if err != nil {
		return nil, fmt.Errorf("schema %q not found: %w", name, err)
	}
	var schema map[string]any
	if err := json.Unmarshal(raw, &schema); err != nil {
		return nil, fmt.Errorf("schema %q invalid JSON: %w", name, err)
	}
	return schema, nil
}

func ReadSchemaRaw(name string) ([]byte, error) {
	filePath := path.Join(name, "schema.json")
	raw, err := fs.ReadFile(embeddedFS, filePath)
	if err != nil {
		return nil, fmt.Errorf("schema %q not found: %w", name, err)
	}
	return raw, nil
}

func parseFrontmatter(raw string) Frontmatter {
	normalized := strings.ReplaceAll(raw, "\r\n", "\n")
	if !strings.HasPrefix(normalized, "---\n") {
		return Frontmatter{}
	}
	end := strings.Index(normalized[4:], "\n---")
	if end < 0 {
		return Frontmatter{}
	}
	var fm Frontmatter
	_ = yaml.Unmarshal([]byte(normalized[4:4+end]), &fm)
	return fm
}

func splitFrontmatter(raw string) (string, string) {
	normalized := strings.ReplaceAll(raw, "\r\n", "\n")
	if !strings.HasPrefix(normalized, "---\n") {
		return "", raw
	}
	end := strings.Index(normalized[4:], "\n---")
	if end < 0 {
		return "", raw
	}
	yamlBlock := normalized[4 : 4+end]
	body := normalized[4+end:]
	body = strings.TrimPrefix(body, "\n---")
	body = strings.TrimPrefix(body, "---")
	body = strings.TrimPrefix(body, "\n")
	return yamlBlock, body
}
