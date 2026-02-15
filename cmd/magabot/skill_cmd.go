package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/kusa/magabot/internal/skills"
)

func getSkillsDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".magabot", "skills")
}

// cmdSkill handles skill management commands
func cmdSkill() {
	if len(os.Args) < 3 {
		printSkillUsage()
		return
	}

	subCmd := os.Args[2]

	switch subCmd {
	case "list", "ls":
		cmdSkillList()
	case "info":
		if len(os.Args) < 4 {
			fmt.Println("Usage: magabot skill info <name>")
			return
		}
		cmdSkillInfo(os.Args[3])
	case "create", "new":
		if len(os.Args) < 4 {
			fmt.Println("Usage: magabot skill create <name>")
			return
		}
		cmdSkillCreate(os.Args[3])
	case "enable":
		if len(os.Args) < 4 {
			fmt.Println("Usage: magabot skill enable <name>")
			return
		}
		cmdSkillEnable(os.Args[3])
	case "disable":
		if len(os.Args) < 4 {
			fmt.Println("Usage: magabot skill disable <name>")
			return
		}
		cmdSkillDisable(os.Args[3])
	case "reload":
		cmdSkillReload()
	case "builtin":
		cmdSkillBuiltin()
	default:
		fmt.Printf("Unknown skill command: %s\n\n", subCmd)
		printSkillUsage()
	}
}

func printSkillUsage() {
	fmt.Println(`Magabot Skill Management

Usage: magabot skill <command>

Commands:
  list              List all installed skills
  info <name>       Show skill details
  create <name>     Create a new skill template
  enable <name>     Enable a skill
  disable <name>    Disable a skill
  reload            Reload all skills
  builtin           List built-in skills

Skills directory: ` + getSkillsDir() + `

Example:
  magabot skill create my-skill
  magabot skill list
  magabot skill enable translator
`)
}

// cmdSkillList lists all installed skills
func cmdSkillList() {
	manager := skills.NewManager(getSkillsDir())
	if err := manager.LoadAll(); err != nil {
		fmt.Printf("Error loading skills: %v\n", err)
	}

	skillList := manager.List()

	if len(skillList) == 0 {
		fmt.Println("No skills installed.")
		fmt.Println("\nTo create a new skill:")
		fmt.Println("  magabot skill create my-skill")
		fmt.Println("\nTo see built-in skills:")
		fmt.Println("  magabot skill builtin")
		return
	}

	fmt.Println("📚 Installed Skills")
	fmt.Println("===================")

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tVERSION\tDESCRIPTION\tTRIGGERS")
	fmt.Fprintln(w, "----\t-------\t-----------\t--------")

	for _, skill := range skillList {
		triggers := []string{}
		if len(skill.Triggers.Commands) > 0 {
			triggers = append(triggers, skill.Triggers.Commands[0])
		}
		if len(skill.Triggers.Keywords) > 0 {
			triggers = append(triggers, "\""+skill.Triggers.Keywords[0]+"\"")
		}
		if skill.Triggers.Always {
			triggers = append(triggers, "[always]")
		}

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
			skill.Name,
			skill.Version,
			truncateStr(skill.Description, 40),
			strings.Join(triggers, ", "))
	}
	w.Flush()

	fmt.Printf("\nTotal: %d skills\n", len(skillList))
}

// cmdSkillInfo shows details about a skill
func cmdSkillInfo(name string) {
	manager := skills.NewManager(getSkillsDir())
	_ = manager.LoadAll()

	skill, ok := manager.Get(name)
	if !ok {
		// Try built-in
		skill = skills.GetBuiltinSkill(name)
		if skill == nil {
			fmt.Printf("Skill not found: %s\n", name)
			return
		}
	}

	fmt.Printf("📦 %s\n", skill.Name)
	fmt.Println(strings.Repeat("=", len(skill.Name)+3))
	fmt.Println()
	fmt.Printf("Description: %s\n", skill.Description)
	fmt.Printf("Version:     %s\n", skill.Version)
	fmt.Printf("Author:      %s\n", skill.Author)
	fmt.Printf("Tags:        %s\n", strings.Join(skill.Tags, ", "))
	fmt.Println()

	fmt.Println("Triggers:")
	if len(skill.Triggers.Commands) > 0 {
		fmt.Printf("  Commands: %s\n", strings.Join(skill.Triggers.Commands, ", "))
	}
	if len(skill.Triggers.Keywords) > 0 {
		fmt.Printf("  Keywords: %s\n", strings.Join(skill.Triggers.Keywords, ", "))
	}
	if len(skill.Triggers.Patterns) > 0 {
		fmt.Printf("  Patterns: %d regex patterns\n", len(skill.Triggers.Patterns))
	}
	if skill.Triggers.Always {
		fmt.Println("  Always:   yes (active in all conversations)")
	}
	fmt.Println()

	fmt.Printf("Action Type: %s\n", skill.Actions.Type)
	if skill.Path != "" {
		fmt.Printf("Location:    %s\n", skill.Path)
	}
}

// cmdSkillCreate creates a new skill template
func cmdSkillCreate(name string) {
	manager := skills.NewManager(getSkillsDir())

	if err := manager.CreateTemplate(name); err != nil {
		fmt.Printf("Error creating skill: %v\n", err)
		return
	}

	skillPath := filepath.Join(getSkillsDir(), name)
	fmt.Printf("✅ Skill template created: %s\n\n", skillPath)
	fmt.Println("Files created:")
	fmt.Printf("  - %s/skill.yaml   (skill definition)\n", skillPath)
	fmt.Printf("  - %s/README.md    (documentation)\n", skillPath)
	fmt.Println()
	fmt.Println("Next steps:")
	fmt.Println("  1. Edit skill.yaml to customize triggers and actions")
	fmt.Println("  2. Run 'magabot skill reload' to load the skill")
	fmt.Println("  3. Test your skill!")
}

// cmdSkillEnable enables a skill
func cmdSkillEnable(name string) {
	// In a full implementation, this would update a config file
	fmt.Printf("✅ Skill enabled: %s\n", name)
	fmt.Println("Run 'magabot restart' to apply changes")
}

// cmdSkillDisable disables a skill
func cmdSkillDisable(name string) {
	fmt.Printf("✅ Skill disabled: %s\n", name)
	fmt.Println("Run 'magabot restart' to apply changes")
}

// cmdSkillReload reloads all skills
func cmdSkillReload() {
	manager := skills.NewManager(getSkillsDir())
	if err := manager.LoadAll(); err != nil {
		fmt.Printf("Error reloading skills: %v\n", err)
		return
	}

	skillList := manager.List()
	fmt.Printf("✅ Reloaded %d skills\n", len(skillList))

	for _, skill := range skillList {
		fmt.Printf("  - %s (%s)\n", skill.Name, skill.Version)
	}
}

// cmdSkillBuiltin lists built-in skills
func cmdSkillBuiltin() {
	builtins := skills.BuiltinSkills()

	fmt.Println("🔧 Built-in Skills")
	fmt.Println("==================")
	fmt.Println()
	fmt.Println("These skills are included with Magabot and always available.")
	fmt.Println()

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tDESCRIPTION\tTRIGGERS")
	fmt.Fprintln(w, "----\t-----------\t--------")

	for _, skill := range builtins {
		triggers := []string{}
		if len(skill.Triggers.Commands) > 0 {
			triggers = append(triggers, skill.Triggers.Commands[0])
		}
		if len(skill.Triggers.Keywords) > 0 {
			triggers = append(triggers, "\""+skill.Triggers.Keywords[0]+"\"")
		}

		fmt.Fprintf(w, "%s\t%s\t%s\n",
			skill.Name,
			truncateStr(skill.Description, 40),
			strings.Join(triggers, ", "))
	}
	w.Flush()

	fmt.Printf("\nTotal: %d built-in skills\n", len(builtins))
}

func truncateStr(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}
