package adk

import (
	"context"
	"sync"
)

// Agent is the main agent type that manages conversation state and executes prompts
// PENDING!!
type Agent struct {
	mu    sync.RWMutex
	state AgentState // PENDING!!

	// run context for cancellations
	runCtx    context.Context
	runCancel context.CancelFunc

	// thing i do NOT understand YET
	steeringQueue *Queue[Message]
	followUpQueue *Queue[Message]
}
