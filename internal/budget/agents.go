package budget

// AgentBudgets is the configurable per-agent description-byte budget table.
var AgentBudgets = map[string]int{
	"codex":  5440,
	"claude": 8000, // TODO: replace with a probed Claude Code hard limit.
}
