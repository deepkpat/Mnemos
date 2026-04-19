package adk

import "sync"

// Agent is the main agent type that manages conversation state and executes prompts
type Agent struct {
	mu    sync.RWMutex
	state AgentState
}
