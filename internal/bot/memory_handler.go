package bot

import (
	"fmt"
	"strings"

	"github.com/kusa/magabot/internal/memory"
)

// MemoryHandler handles memory-related commands
type MemoryHandler struct {
	stores map[string]*memory.Store // userID -> store
	dataDir string
}

// NewMemoryHandler creates a new memory handler
func NewMemoryHandler(dataDir string) *MemoryHandler {
	return &MemoryHandler{
		stores:  make(map[string]*memory.Store),
		dataDir: dataDir,
	}
}

// GetStore gets or creates a memory store for a user
func (h *MemoryHandler) GetStore(userID string) (*memory.Store, error) {
	if store, ok := h.stores[userID]; ok {
		return store, nil
	}
	
	store, err := memory.NewStore(h.dataDir, userID)
	if err != nil {
		return nil, err
	}
	
	h.stores[userID] = store
	return store, nil
}

// HandleCommand processes memory commands
func (h *MemoryHandler) HandleCommand(userID, platform string, args []string) (string, error) {
	store, err := h.GetStore(userID)
	if err != nil {
		return "", err
	}
	
	if len(args) == 0 {
		return h.showHelp(), nil
	}
	
	cmd := strings.ToLower(args[0])
	subArgs := args[1:]
	
	switch cmd {
	case "add", "remember":
		return h.addMemory(store, platform, subArgs)
	case "search", "find", "recall":
		return h.searchMemory(store, subArgs)
	case "list", "ls":
		return h.listMemory(store, subArgs)
	case "delete", "rm", "forget":
		return h.deleteMemory(store, subArgs)
	case "clear":
		return h.clearMemory(store)
	case "stats":
		return h.showStats(store), nil
	case "help":
		return h.showHelp(), nil
	default:
		// Treat as content to remember
		content := strings.Join(args, " ")
		return h.rememberContent(store, platform, content)
	}
}

// addMemory adds a new memory
func (h *MemoryHandler) addMemory(store *memory.Store, platform string, args []string) (string, error) {
	if len(args) == 0 {
		return "Usage: /memory add <content to remember>", nil
	}
	
	content := strings.Join(args, " ")
	mem, err := store.Remember(content, platform)
	if err != nil {
		return "", err
	}
	
	return fmt.Sprintf("🧠 Remembered!\n\n📝 %s\n🏷️ Type: %s\n🔑 ID: %s", 
		mem.Content, mem.Type, mem.ID[:8]), nil
}

// rememberContent is a shortcut to add memory
func (h *MemoryHandler) rememberContent(store *memory.Store, platform, content string) (string, error) {
	mem, err := store.Remember(content, platform)
	if err != nil {
		return "", err
	}
	
	return fmt.Sprintf("🧠 Noted: %s", truncateStr(mem.Content, 50)), nil
}

// searchMemory searches for relevant memories
func (h *MemoryHandler) searchMemory(store *memory.Store, args []string) (string, error) {
	if len(args) == 0 {
		return "Usage: /memory search <query>", nil
	}
	
	query := strings.Join(args, " ")
	memories := store.Search(query, 5)
	
	if len(memories) == 0 {
		return fmt.Sprintf("🔍 No memories found for: %s", query), nil
	}
	
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("🔍 *Memories for: %s*\n\n", query))
	
	for i, mem := range memories {
		sb.WriteString(fmt.Sprintf("%d. [%s] %s\n", i+1, mem.Type, truncateStr(mem.Content, 80)))
		sb.WriteString(fmt.Sprintf("   📅 %s | 🔑 %s\n\n", 
			mem.CreatedAt.Format("Jan 2"), mem.ID[:8]))
	}
	
	return sb.String(), nil
}

// listMemory lists all memories
func (h *MemoryHandler) listMemory(store *memory.Store, args []string) (string, error) {
	memType := ""
	if len(args) > 0 {
		memType = args[0]
	}
	
	memories := store.List(memType)
	
	if len(memories) == 0 {
		return "📋 No memories stored yet.", nil
	}
	
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("📋 *Memories* (%d total)\n\n", len(memories)))
	
	// Show max 10
	limit := 10
	if len(memories) < limit {
		limit = len(memories)
	}
	
	for i := 0; i < limit; i++ {
		mem := memories[i]
		sb.WriteString(fmt.Sprintf("• [%s] %s\n", mem.Type, truncateStr(mem.Content, 60)))
	}
	
	if len(memories) > limit {
		sb.WriteString(fmt.Sprintf("\n... and %d more", len(memories)-limit))
	}
	
	return sb.String(), nil
}

// deleteMemory deletes a memory by ID
func (h *MemoryHandler) deleteMemory(store *memory.Store, args []string) (string, error) {
	if len(args) == 0 {
		return "Usage: /memory delete <id>", nil
	}
	
	// Find memory by partial ID
	id := args[0]
	memories := store.List("")
	
	for _, mem := range memories {
		if strings.HasPrefix(mem.ID, id) {
			if err := store.Delete(mem.ID); err != nil {
				return "", err
			}
			return fmt.Sprintf("🗑️ Deleted: %s", truncateStr(mem.Content, 50)), nil
		}
	}
	
	return fmt.Sprintf("❌ Memory not found: %s", id), nil
}

// clearMemory clears all memories
func (h *MemoryHandler) clearMemory(store *memory.Store) (string, error) {
	stats := store.Stats()
	count := stats["total"]
	
	if count == 0 {
		return "📋 No memories to clear.", nil
	}
	
	if err := store.Clear(); err != nil {
		return "", err
	}
	
	return fmt.Sprintf("🗑️ Cleared %d memories.", count), nil
}

// showStats shows memory statistics
func (h *MemoryHandler) showStats(store *memory.Store) string {
	stats := store.Stats()
	
	var sb strings.Builder
	sb.WriteString("📊 *Memory Stats*\n\n")
	sb.WriteString(fmt.Sprintf("Total: %d\n", stats["total"]))
	
	for k, v := range stats {
		if k != "total" && v > 0 {
			sb.WriteString(fmt.Sprintf("• %s: %d\n", k, v))
		}
	}
	
	return sb.String()
}

// showHelp shows help text
func (h *MemoryHandler) showHelp() string {
	return `🧠 *Memory Commands*

/memory add <text>     Remember something
/memory search <query> Find memories
/memory list [type]    List all memories
/memory delete <id>    Delete a memory
/memory clear          Clear all memories
/memory stats          Show statistics
/memory help           Show this help

*Memory Types:* fact, preference, event, note

*Examples:*
• /memory add Nama saya Kus
• /memory search trading
• /memory list preference
• /memory delete abc123`
}

// GetContext retrieves relevant memories for LLM context
func (h *MemoryHandler) GetContext(userID, query string, maxTokens int) string {
	store, err := h.GetStore(userID)
	if err != nil {
		return ""
	}
	
	return store.GetContext(query, maxTokens)
}

func truncateStr(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}
