package ai

// StopReason indicates why the model stopped generating.
type StopReason string

const (
	StopReasonStop    StopReason = "stop"    // Natural completion
	StopReasonLength  StopReason = "length"  // Hit max token limit
	StopReasonToolUse StopReason = "toolUse" // Model wants to call tool(s)
	StopReasonError   StopReason = "error"   // Provider/network error
	StopReasonAborted StopReason = "aborted" // Cancelled by caller
)

// ThinkingLevel controls how much reasoning effort the model should use.
type ThinkingLevel string

const (
	ThinkingMinimal ThinkingLevel = "minimal"
	ThinkingLow     ThinkingLevel = "low"
	ThinkingMedium  ThinkingLevel = "medium"
	ThinkingHigh    ThinkingLevel = "high"
)
