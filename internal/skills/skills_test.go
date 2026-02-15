package skills

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func writeSkillYAML(t *testing.T, dir, name, content string) string {
	t.Helper()
	skillDir := filepath.Join(dir, name)
	if err := os.MkdirAll(skillDir, 0750); err != nil {
		t.Fatalf("mkdir %s: %v", skillDir, err)
	}
	path := filepath.Join(skillDir, "skill.yaml")
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}

const validSkillYAML = `name: test-skill
description: A test skill
version: "1.0.0"
author: tester
tags:
  - testing
triggers:
  commands: ["/test"]
  keywords: ["weather"]
  patterns: ["\\d+ degrees"]
actions:
  type: prompt
  prompt: "test prompt"
system_prompt: "You are a test assistant."
`

func TestLoadAll_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir)

	if err := m.LoadAll(); err != nil {
		t.Fatalf("LoadAll on empty dir should not error: %v", err)
	}

	if got := len(m.List()); got != 0 {
		t.Errorf("expected 0 skills, got %d", got)
	}
}

func TestLoadAll_ValidSkill(t *testing.T) {
	dir := t.TempDir()
	writeSkillYAML(t, dir, "test-skill", validSkillYAML)

	m := NewManager(dir)
	if err := m.LoadAll(); err != nil {
		t.Fatalf("LoadAll failed: %v", err)
	}

	skills := m.List()
	if len(skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(skills))
	}

	s := skills[0]
	if s.Name != "test-skill" {
		t.Errorf("expected name 'test-skill', got %q", s.Name)
	}
	if s.Description != "A test skill" {
		t.Errorf("expected description 'A test skill', got %q", s.Description)
	}
	if s.Version != "1.0.0" {
		t.Errorf("expected version '1.0.0', got %q", s.Version)
	}
	if s.Author != "tester" {
		t.Errorf("expected author 'tester', got %q", s.Author)
	}
	if len(s.Tags) != 1 || s.Tags[0] != "testing" {
		t.Errorf("unexpected tags: %v", s.Tags)
	}
	if s.Actions.Type != "prompt" {
		t.Errorf("expected action type 'prompt', got %q", s.Actions.Type)
	}
	if s.Actions.Prompt != "test prompt" {
		t.Errorf("expected prompt 'test prompt', got %q", s.Actions.Prompt)
	}
	if s.SystemPrompt != "You are a test assistant." {
		t.Errorf("unexpected system prompt: %q", s.SystemPrompt)
	}
}

func TestLoadAll_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	writeSkillYAML(t, dir, "bad-skill", `{{{not valid yaml`)

	m := NewManager(dir)
	// LoadAll should not return an error for invalid YAML; it prints a warning and skips.
	if err := m.LoadAll(); err != nil {
		t.Fatalf("LoadAll should not error on invalid YAML: %v", err)
	}
	if got := len(m.List()); got != 0 {
		t.Errorf("expected 0 skills (invalid YAML skipped), got %d", got)
	}
}

func TestLoadAll_MultipleSkills(t *testing.T) {
	dir := t.TempDir()
	writeSkillYAML(t, dir, "skill-a", `name: skill-a
description: Skill A
version: "1.0.0"
triggers:
  commands: ["/a"]
actions:
  type: prompt
  prompt: "A"
`)
	writeSkillYAML(t, dir, "skill-b", `name: skill-b
description: Skill B
version: "2.0.0"
triggers:
  keywords: ["hello"]
actions:
  type: prompt
  prompt: "B"
`)

	m := NewManager(dir)
	if err := m.LoadAll(); err != nil {
		t.Fatalf("LoadAll failed: %v", err)
	}
	if got := len(m.List()); got != 2 {
		t.Errorf("expected 2 skills, got %d", got)
	}
}

func TestGet_Found(t *testing.T) {
	dir := t.TempDir()
	writeSkillYAML(t, dir, "test-skill", validSkillYAML)

	m := NewManager(dir)
	if err := m.LoadAll(); err != nil {
		t.Fatalf("LoadAll failed: %v", err)
	}

	s, ok := m.Get("test-skill")
	if !ok {
		t.Fatal("expected Get to return true for loaded skill")
	}
	if s.Name != "test-skill" {
		t.Errorf("expected name 'test-skill', got %q", s.Name)
	}
}

func TestGet_NotFound(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir)

	s, ok := m.Get("nonexistent")
	if ok {
		t.Error("expected Get to return false for missing skill")
	}
	if s != nil {
		t.Error("expected nil skill for missing name")
	}
}

func TestLoad_SingleSkill(t *testing.T) {
	dir := t.TempDir()
	writeSkillYAML(t, dir, "my-skill", validSkillYAML)

	m := NewManager(dir)

	// Load by directory name
	if err := m.Load("my-skill"); err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	s, ok := m.Get("test-skill") // YAML name is "test-skill"
	if !ok {
		t.Fatal("expected skill to be loaded")
	}
	if s.Name != "test-skill" {
		t.Errorf("expected name 'test-skill', got %q", s.Name)
	}
}

func TestLoad_NonexistentDir(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir)

	if err := m.Load("does-not-exist"); err == nil {
		t.Error("expected error loading from nonexistent directory")
	}
}

func TestUnload(t *testing.T) {
	dir := t.TempDir()
	writeSkillYAML(t, dir, "test-skill", validSkillYAML)

	m := NewManager(dir)
	if err := m.LoadAll(); err != nil {
		t.Fatalf("LoadAll failed: %v", err)
	}

	_, ok := m.Get("test-skill")
	if !ok {
		t.Fatal("skill should exist before unload")
	}

	m.Unload("test-skill")

	_, ok = m.Get("test-skill")
	if ok {
		t.Error("skill should not exist after unload")
	}

	if got := len(m.List()); got != 0 {
		t.Errorf("expected 0 skills after unload, got %d", got)
	}
}

func TestUnload_NonexistentSkill(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir)

	// Should not panic
	m.Unload("nonexistent")
}

func TestMatch_Command(t *testing.T) {
	dir := t.TempDir()
	writeSkillYAML(t, dir, "test-skill", validSkillYAML)

	m := NewManager(dir)
	if err := m.LoadAll(); err != nil {
		t.Fatalf("LoadAll failed: %v", err)
	}

	matched := m.Match("/test something")
	if len(matched) != 1 {
		t.Fatalf("expected 1 match for command trigger, got %d", len(matched))
	}
	if matched[0].Name != "test-skill" {
		t.Errorf("expected 'test-skill', got %q", matched[0].Name)
	}
}

func TestMatch_Keyword(t *testing.T) {
	dir := t.TempDir()
	writeSkillYAML(t, dir, "test-skill", validSkillYAML)

	m := NewManager(dir)
	if err := m.LoadAll(); err != nil {
		t.Fatalf("LoadAll failed: %v", err)
	}

	matched := m.Match("what's the weather today?")
	if len(matched) != 1 {
		t.Fatalf("expected 1 match for keyword trigger, got %d", len(matched))
	}
	if matched[0].Name != "test-skill" {
		t.Errorf("expected 'test-skill', got %q", matched[0].Name)
	}
}

func TestMatch_KeywordCaseInsensitive(t *testing.T) {
	dir := t.TempDir()
	writeSkillYAML(t, dir, "test-skill", validSkillYAML)

	m := NewManager(dir)
	if err := m.LoadAll(); err != nil {
		t.Fatalf("LoadAll failed: %v", err)
	}

	matched := m.Match("WEATHER forecast")
	if len(matched) != 1 {
		t.Fatalf("expected 1 match for case-insensitive keyword, got %d", len(matched))
	}
}

func TestMatch_Pattern(t *testing.T) {
	dir := t.TempDir()
	writeSkillYAML(t, dir, "test-skill", validSkillYAML)

	m := NewManager(dir)
	if err := m.LoadAll(); err != nil {
		t.Fatalf("LoadAll failed: %v", err)
	}

	matched := m.Match("it is 72 degrees outside")
	if len(matched) != 1 {
		t.Fatalf("expected 1 match for pattern trigger, got %d", len(matched))
	}
	if matched[0].Name != "test-skill" {
		t.Errorf("expected 'test-skill', got %q", matched[0].Name)
	}
}

func TestMatch_AlwaysActive(t *testing.T) {
	dir := t.TempDir()
	writeSkillYAML(t, dir, "always-skill", `name: always-skill
description: Always active
version: "1.0.0"
triggers:
  always: true
actions:
  type: prompt
  prompt: "always here"
`)

	m := NewManager(dir)
	if err := m.LoadAll(); err != nil {
		t.Fatalf("LoadAll failed: %v", err)
	}

	matched := m.Match("any random message")
	if len(matched) != 1 {
		t.Fatalf("expected 1 match for always-active skill, got %d", len(matched))
	}
	if matched[0].Name != "always-skill" {
		t.Errorf("expected 'always-skill', got %q", matched[0].Name)
	}
}

func TestMatch_NoMatch(t *testing.T) {
	dir := t.TempDir()
	writeSkillYAML(t, dir, "test-skill", validSkillYAML)

	m := NewManager(dir)
	if err := m.LoadAll(); err != nil {
		t.Fatalf("LoadAll failed: %v", err)
	}

	matched := m.Match("completely unrelated message about cats")
	if len(matched) != 0 {
		t.Errorf("expected 0 matches, got %d", len(matched))
	}
}

func TestMatch_EmptySkills(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir)

	matched := m.Match("anything")
	if len(matched) != 0 {
		t.Errorf("expected 0 matches with no skills loaded, got %d", len(matched))
	}
}

func TestCreateTemplate(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir)

	if err := m.CreateTemplate("my-new-skill"); err != nil {
		t.Fatalf("CreateTemplate failed: %v", err)
	}

	// Verify skill directory was created
	skillDir := filepath.Join(dir, "my-new-skill")
	info, err := os.Stat(skillDir)
	if err != nil {
		t.Fatalf("skill directory not created: %v", err)
	}
	if !info.IsDir() {
		t.Error("expected skill path to be a directory")
	}

	// Verify skill.yaml was created
	yamlPath := filepath.Join(skillDir, "skill.yaml")
	if _, err := os.Stat(yamlPath); err != nil {
		t.Fatalf("skill.yaml not created: %v", err)
	}

	// Verify README.md was created
	readmePath := filepath.Join(skillDir, "README.md")
	if _, err := os.Stat(readmePath); err != nil {
		t.Fatalf("README.md not created: %v", err)
	}

	// Verify the created skill can be loaded
	if err := m.LoadAll(); err != nil {
		t.Fatalf("LoadAll after CreateTemplate failed: %v", err)
	}

	s, ok := m.Get("my-new-skill")
	if !ok {
		t.Fatal("created template skill should be loadable")
	}
	if s.Name != "my-new-skill" {
		t.Errorf("expected name 'my-new-skill', got %q", s.Name)
	}
	if s.Actions.Type != "prompt" {
		t.Errorf("expected action type 'prompt', got %q", s.Actions.Type)
	}
}

func TestExecute_Prompt(t *testing.T) {
	dir := t.TempDir()
	writeSkillYAML(t, dir, "test-skill", validSkillYAML)

	m := NewManager(dir)
	if err := m.LoadAll(); err != nil {
		t.Fatalf("LoadAll failed: %v", err)
	}

	s, ok := m.Get("test-skill")
	if !ok {
		t.Fatal("skill not found")
	}

	result, err := m.Execute(context.Background(), s, "hello")
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result != "test prompt" {
		t.Errorf("expected 'test prompt', got %q", result)
	}
}

func TestExecute_Script(t *testing.T) {
	dir := t.TempDir()
	writeSkillYAML(t, dir, "script-skill", `name: script-skill
description: Script skill
version: "1.0.0"
triggers:
  commands: ["/run"]
actions:
  type: script
  script: "echo hello"
`)

	m := NewManager(dir)
	if err := m.LoadAll(); err != nil {
		t.Fatalf("LoadAll failed: %v", err)
	}

	s, ok := m.Get("script-skill")
	if !ok {
		t.Fatal("skill not found")
	}

	result, err := m.Execute(context.Background(), s, "run")
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result == "" {
		t.Error("expected non-empty result from script execution")
	}
}

func TestExecute_UnknownType(t *testing.T) {
	skill := &Skill{
		Name: "unknown",
		Actions: Actions{
			Type: "telepathy",
		},
	}

	dir := t.TempDir()
	m := NewManager(dir)

	_, err := m.Execute(context.Background(), skill, "hello")
	if err == nil {
		t.Error("expected error for unknown action type")
	}
}

func TestExecute_APIMissingConfig(t *testing.T) {
	skill := &Skill{
		Name: "api-missing",
		Actions: Actions{
			Type: "api",
			API:  nil,
		},
	}

	dir := t.TempDir()
	m := NewManager(dir)

	_, err := m.Execute(context.Background(), skill, "hello")
	if err == nil {
		t.Error("expected error for API action with no API config")
	}
}

func TestExecute_APIWithConfig(t *testing.T) {
	skill := &Skill{
		Name: "api-skill",
		Actions: Actions{
			Type: "api",
			API: &APIAction{
				URL:    "https://example.com/api",
				Method: "GET",
			},
		},
	}

	dir := t.TempDir()
	m := NewManager(dir)

	result, err := m.Execute(context.Background(), skill, "hello")
	if err != nil {
		t.Fatalf("Execute API failed: %v", err)
	}
	if result == "" {
		t.Error("expected non-empty result from API execution")
	}
}

func TestList_Empty(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir)

	skills := m.List()
	if skills == nil {
		t.Error("List should return non-nil slice")
	}
	if len(skills) != 0 {
		t.Errorf("expected 0 skills, got %d", len(skills))
	}
}

func TestGetSystemPrompts(t *testing.T) {
	dir := t.TempDir()
	writeSkillYAML(t, dir, "always-skill", `name: always-skill
description: Always active skill
version: "1.0.0"
triggers:
  always: true
actions:
  type: prompt
  prompt: "always prompt"
system_prompt: "System context for always-skill."
`)

	m := NewManager(dir)
	if err := m.LoadAll(); err != nil {
		t.Fatalf("LoadAll failed: %v", err)
	}

	prompts := m.GetSystemPrompts()
	if prompts == "" {
		t.Error("expected non-empty system prompts for always-active skill")
	}
	if !contains(prompts, "always-skill") {
		t.Errorf("system prompt should contain skill name, got %q", prompts)
	}
	if !contains(prompts, "System context for always-skill.") {
		t.Errorf("system prompt should contain the skill's system_prompt, got %q", prompts)
	}
}

func TestGetMatchedPrompts(t *testing.T) {
	dir := t.TempDir()
	writeSkillYAML(t, dir, "test-skill", validSkillYAML)

	m := NewManager(dir)
	if err := m.LoadAll(); err != nil {
		t.Fatalf("LoadAll failed: %v", err)
	}

	prompts := m.GetMatchedPrompts("/test help me")
	if prompts != "test prompt" {
		t.Errorf("expected 'test prompt', got %q", prompts)
	}
}

func TestGetMatchedPrompts_NoMatch(t *testing.T) {
	dir := t.TempDir()
	writeSkillYAML(t, dir, "test-skill", validSkillYAML)

	m := NewManager(dir)
	if err := m.LoadAll(); err != nil {
		t.Fatalf("LoadAll failed: %v", err)
	}

	prompts := m.GetMatchedPrompts("no triggers here")
	if prompts != "" {
		t.Errorf("expected empty prompts for non-matching message, got %q", prompts)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstr(s, substr))
}

func containsSubstr(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
