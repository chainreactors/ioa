package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chainreactors/ioa/skills"
)

func TestInitExportsSkills(t *testing.T) {
	for _, tc := range []struct {
		name      string
		requested []string
		want      []string
	}{
		{name: "all", want: []string{"checkpoint", "handoff", "swarm", "team"}},
		{name: "selected", requested: []string{"swarm"}, want: []string{"swarm"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := initCmd{Output: t.TempDir()}
			cmd.Positional.Skills = tc.requested
			if err := runInit(cmd); err != nil {
				t.Fatal(err)
			}
			entries, err := os.ReadDir(cmd.Output)
			if err != nil || len(entries) != len(tc.want) {
				t.Fatalf("exported directories: %v, err=%v", entries, err)
			}
			for _, name := range tc.want {
				for _, resource := range []struct {
					file string
					read func(string) ([]byte, error)
				}{
					{file: "SKILL.md", read: skills.ReadSkillRaw},
					{file: "schema.json", read: skills.ReadSchemaRaw},
				} {
					want, err := resource.read(name)
					if err != nil {
						t.Fatal(err)
					}
					got, err := os.ReadFile(filepath.Join(cmd.Output, name, resource.file))
					if err != nil || !bytes.Equal(got, want) {
						t.Fatalf("export %s/%s differs from embedded resource: %v", name, resource.file, err)
					}
				}
			}
		})
	}
}

func TestInitRejectsUnknownSkillBeforeWriting(t *testing.T) {
	cmd := initCmd{Output: t.TempDir()}
	cmd.Positional.Skills = []string{"swarm", "missing"}
	if err := runInit(cmd); err == nil || !strings.Contains(err.Error(), "unknown skill: missing") {
		t.Fatalf("expected unknown skill error, got %v", err)
	}
	entries, err := os.ReadDir(cmd.Output)
	if err != nil || len(entries) != 0 {
		t.Fatalf("unknown skill left partial output: %v, err=%v", entries, err)
	}
}
