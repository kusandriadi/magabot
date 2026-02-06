// Built-in skills that come with Magabot
package skills

// BuiltinSkills returns the built-in skill definitions
func BuiltinSkills() []*Skill {
	return []*Skill{
		weatherSkill(),
		translateSkill(),
		summarizeSkill(),
		codeSkill(),
		mathSkill(),
		searchSkill(),
	}
}

func weatherSkill() *Skill {
	return &Skill{
		Name:        "weather",
		Description: "Get weather information for any location",
		Version:     "1.0.0",
		Tags:        []string{"utility", "weather"},
		Triggers: Triggers{
			Commands: []string{"/weather", "/cuaca"},
			Keywords: []string{"weather", "cuaca", "forecast", "temperature", "suhu"},
			Patterns: []string{`(?i)weather\s+(in|at|for)\s+\w+`, `(?i)cuaca\s+(di|untuk)\s+\w+`},
		},
		Actions: Actions{
			Type:   "prompt",
			Prompt: "The user is asking about weather. Use the weather tool to get current weather data. Format the response nicely with emojis.",
		},
	}
}

func translateSkill() *Skill {
	return &Skill{
		Name:        "translate",
		Description: "Translate text between languages",
		Version:     "1.0.0",
		Tags:        []string{"utility", "language"},
		Triggers: Triggers{
			Commands: []string{"/translate", "/terjemah"},
			Keywords: []string{"translate", "terjemahkan", "translation"},
			Patterns: []string{`(?i)translate\s+.+\s+to\s+\w+`, `(?i)terjemahkan\s+.+\s+ke\s+\w+`},
		},
		Actions: Actions{
			Type:   "prompt",
			Prompt: "The user wants to translate text. Identify the source language (auto-detect if not specified) and target language. Provide the translation clearly.",
		},
	}
}

func summarizeSkill() *Skill {
	return &Skill{
		Name:        "summarize",
		Description: "Summarize text, articles, or conversations",
		Version:     "1.0.0",
		Tags:        []string{"utility", "text"},
		Triggers: Triggers{
			Commands: []string{"/summarize", "/summary", "/ringkas"},
			Keywords: []string{"summarize", "summary", "tldr", "ringkas", "rangkum"},
		},
		Actions: Actions{
			Type:   "prompt",
			Prompt: "The user wants a summary. Provide a concise summary with key points. Use bullet points for clarity.",
		},
	}
}

func codeSkill() *Skill {
	return &Skill{
		Name:        "code",
		Description: "Help with coding, debugging, and code review",
		Version:     "1.0.0",
		Tags:        []string{"development", "coding"},
		Triggers: Triggers{
			Commands: []string{"/code", "/debug", "/review"},
			Keywords: []string{"bug", "error", "code review", "refactor"},
			Patterns: []string{`(?i)(fix|debug|review)\s+(this|my)\s+code`},
		},
		Actions: Actions{
			Type: "prompt",
			Prompt: `You are a senior software engineer. Help with:
- Code review: Point out issues and suggest improvements
- Debugging: Identify bugs and explain fixes
- Best practices: Suggest cleaner patterns
Format code blocks properly with language tags.`,
		},
	}
}

func mathSkill() *Skill {
	return &Skill{
		Name:        "math",
		Description: "Solve math problems and calculations",
		Version:     "1.0.0",
		Tags:        []string{"utility", "math"},
		Triggers: Triggers{
			Commands: []string{"/math", "/calc", "/hitung"},
			Keywords: []string{"calculate", "solve", "equation", "hitung"},
			Patterns: []string{`\d+\s*[\+\-\*\/\^]\s*\d+`, `(?i)(what|berapa)\s+is\s+\d+`},
		},
		Actions: Actions{
			Type:   "prompt",
			Prompt: "The user has a math question. Show the step-by-step solution clearly. Use proper mathematical notation.",
		},
	}
}

func searchSkill() *Skill {
	return &Skill{
		Name:        "search",
		Description: "Search the web for information",
		Version:     "1.0.0",
		Tags:        []string{"utility", "search"},
		Triggers: Triggers{
			Commands: []string{"/search", "/google", "/cari"},
			Keywords: []string{"search for", "look up", "find info", "cari tentang"},
			Patterns: []string{`(?i)(search|cari|google)\s+(for\s+)?".+"`},
		},
		Actions: Actions{
			Type:   "prompt",
			Prompt: "The user wants to search for information. Use the search tool to find relevant results and summarize the key findings.",
		},
	}
}

// GetBuiltinSkill returns a built-in skill by name
func GetBuiltinSkill(name string) *Skill {
	for _, skill := range BuiltinSkills() {
		if skill.Name == name {
			return skill
		}
	}
	return nil
}
