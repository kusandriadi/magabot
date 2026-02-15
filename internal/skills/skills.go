// Package skills provides a plugin system for extending LLM capabilities
package skills

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

// Skill represents a loaded skill
type Skill struct {
	// Metadata
	Name        string   `yaml:"name"`
	Description string   `yaml:"description"`
	Version     string   `yaml:"version"`
	Author      string   `yaml:"author"`
	Tags        []string `yaml:"tags"`

	// Triggers
	Triggers Triggers `yaml:"triggers"`

	// Actions
	Actions Actions `yaml:"actions"`

	// Context injection
	SystemPrompt string `yaml:"system_prompt"`

	// File path (set at load time)
	Path string `yaml:"-"`
}

// Triggers define when a skill is activated
type Triggers struct {
	// Commands like /weather, /translate
	Commands []string `yaml:"commands"`

	// Patterns (regex) to match in user messages
	Patterns []string `yaml:"patterns"`

	// Keywords to detect
	Keywords []string `yaml:"keywords"`

	// Always active (inject system prompt)
	Always bool `yaml:"always"`
}

// Actions define what the skill does
type Actions struct {
	// Type: prompt, script, api, function
	Type string `yaml:"type"`

	// For prompt type - additional instructions for LLM
	Prompt string `yaml:"prompt"`

	// For script type - shell command to execute
	Script string `yaml:"script"`

	// For API type
	API *APIAction `yaml:"api,omitempty"`

	// Response template
	ResponseTemplate string `yaml:"response_template"`
}

// APIAction for API-based skills
type APIAction struct {
	URL     string            `yaml:"url"`
	Method  string            `yaml:"method"`
	Headers map[string]string `yaml:"headers"`
	Body    string            `yaml:"body"`
}

// Manager manages all loaded skills
type Manager struct {
	skills      map[string]*Skill
	skillsDir   string
	mu          sync.RWMutex
	compiledRe  map[string]*regexp.Regexp
}

// NewManager creates a new skill manager
func NewManager(skillsDir string) *Manager {
	return &Manager{
		skills:     make(map[string]*Skill),
		skillsDir:  skillsDir,
		compiledRe: make(map[string]*regexp.Regexp),
	}
}

// LoadAll loads all skills from the skills directory
func (m *Manager) LoadAll() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Create skills directory if it doesn't exist
	if err := os.MkdirAll(m.skillsDir, 0755); err != nil {
		return err
	}

	// Find all skill.yaml files
	err := filepath.Walk(m.skillsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.Name() == "skill.yaml" || info.Name() == "SKILL.yaml" {
			skill, err := m.loadSkill(path)
			if err != nil {
				fmt.Printf("Warning: failed to load skill %s: %v\n", path, err)
				return nil
			}
			m.skills[skill.Name] = skill
		}

		return nil
	})

	return err
}

// loadSkill loads a single skill from file
func (m *Manager) loadSkill(path string) (*Skill, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var skill Skill
	if err := yaml.Unmarshal(data, &skill); err != nil {
		return nil, err
	}

	skill.Path = filepath.Dir(path)

	// Compile regex patterns
	for _, pattern := range skill.Triggers.Patterns {
		re, err := regexp.Compile(pattern)
		if err != nil {
			return nil, fmt.Errorf("invalid pattern %s: %w", pattern, err)
		}
		m.compiledRe[skill.Name+":"+pattern] = re
	}

	return &skill, nil
}

// Load loads a single skill by name
func (m *Manager) Load(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	path := filepath.Join(m.skillsDir, name, "skill.yaml")
	skill, err := m.loadSkill(path)
	if err != nil {
		return err
	}

	m.skills[skill.Name] = skill
	return nil
}

// Unload removes a skill
func (m *Manager) Unload(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.skills, name)
}

// Get returns a skill by name
func (m *Manager) Get(name string) (*Skill, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	skill, ok := m.skills[name]
	return skill, ok
}

// List returns all loaded skills
func (m *Manager) List() []*Skill {
	m.mu.RLock()
	defer m.mu.RUnlock()

	skills := make([]*Skill, 0, len(m.skills))
	for _, s := range m.skills {
		skills = append(skills, s)
	}
	return skills
}

// Match finds skills that match the given message
func (m *Manager) Match(message string) []*Skill {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var matched []*Skill
	messageLower := strings.ToLower(message)

	for _, skill := range m.skills {
		if m.matchSkill(skill, message, messageLower) {
			matched = append(matched, skill)
		}
	}

	return matched
}

// matchSkill checks if a message matches a skill's triggers
func (m *Manager) matchSkill(skill *Skill, message, messageLower string) bool {
	// Always active skills
	if skill.Triggers.Always {
		return true
	}

	// Command triggers
	for _, cmd := range skill.Triggers.Commands {
		if strings.HasPrefix(messageLower, cmd) {
			return true
		}
	}

	// Pattern triggers
	for _, pattern := range skill.Triggers.Patterns {
		re := m.compiledRe[skill.Name+":"+pattern]
		if re != nil && re.MatchString(message) {
			return true
		}
	}

	// Keyword triggers
	for _, keyword := range skill.Triggers.Keywords {
		if strings.Contains(messageLower, strings.ToLower(keyword)) {
			return true
		}
	}

	return false
}

// GetSystemPrompts returns combined system prompts from all always-active skills
func (m *Manager) GetSystemPrompts() string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var prompts []string
	for _, skill := range m.skills {
		if skill.Triggers.Always && skill.SystemPrompt != "" {
			prompts = append(prompts, fmt.Sprintf("# %s\n%s", skill.Name, skill.SystemPrompt))
		}
	}

	return strings.Join(prompts, "\n\n")
}

// GetMatchedPrompts returns prompts for skills that match the message
func (m *Manager) GetMatchedPrompts(message string) string {
	matched := m.Match(message)

	var prompts []string
	for _, skill := range matched {
		if skill.Actions.Type == "prompt" && skill.Actions.Prompt != "" {
			prompts = append(prompts, skill.Actions.Prompt)
		} else if skill.SystemPrompt != "" {
			prompts = append(prompts, skill.SystemPrompt)
		}
	}

	return strings.Join(prompts, "\n\n")
}

// Execute executes a skill's action
func (m *Manager) Execute(ctx context.Context, skill *Skill, message string) (string, error) {
	switch skill.Actions.Type {
	case "prompt":
		// Return prompt to be injected (handled by LLM)
		return skill.Actions.Prompt, nil

	case "script":
		return m.executeScript(ctx, skill, message)

	case "api":
		return m.executeAPI(ctx, skill, message)

	default:
		return "", fmt.Errorf("unknown action type: %s", skill.Actions.Type)
	}
}

// executeScript runs a shell script
func (m *Manager) executeScript(ctx context.Context, skill *Skill, message string) (string, error) {
	// Security: Scripts run in skill directory with limited env
	// This is a placeholder - full implementation would use exec.CommandContext
	return fmt.Sprintf("Script execution: %s", skill.Actions.Script), nil
}

// executeAPI calls an external API
func (m *Manager) executeAPI(ctx context.Context, skill *Skill, message string) (string, error) {
	if skill.Actions.API == nil {
		return "", fmt.Errorf("API configuration missing")
	}

	// This is a placeholder - full implementation would use http.Client
	return fmt.Sprintf("API call: %s %s", skill.Actions.API.Method, skill.Actions.API.URL), nil
}

// Install installs a skill from a URL or path
func (m *Manager) Install(source string) error {
	// Placeholder for skill installation
	// Could support:
	// - Git URLs
	// - Local directories
	// - Zip files
	return fmt.Errorf("not implemented")
}

// CreateTemplate creates a new skill template
func (m *Manager) CreateTemplate(name string) error {
	skillDir := filepath.Join(m.skillsDir, name)
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		return err
	}

	template := &Skill{
		Name:        name,
		Description: "Description of your skill",
		Version:     "1.0.0",
		Author:      "Your Name",
		Tags:        []string{"example"},
		Triggers: Triggers{
			Commands: []string{"/" + name},
			Keywords: []string{name},
		},
		Actions: Actions{
			Type:   "prompt",
			Prompt: "You are now in " + name + " mode. Help the user with...",
		},
		SystemPrompt: "Additional context for the LLM when this skill is active.",
	}

	data, err := yaml.Marshal(template)
	if err != nil {
		return err
	}

	readmePath := filepath.Join(skillDir, "README.md")
	readme := fmt.Sprintf("# %s\n\n%s\n\n## Usage\n\nTrigger with `/%s` or mention \"%s\" in your message.\n",
		name, template.Description, name, name)
	if err := os.WriteFile(readmePath, []byte(readme), 0644); err != nil {
		return fmt.Errorf("write README: %w", err)
	}

	return os.WriteFile(filepath.Join(skillDir, "skill.yaml"), data, 0644)
}
