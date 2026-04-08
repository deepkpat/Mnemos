# pi/ai — Provider-Agnostic LLM Abstraction Layer

This document explains the architecture and abstractions found in `context/ai` (sourced from the pi coding agent's `@mariozechner/pi-ai` package). The goal is to provide a unified interface for interacting with any LLM provider — streaming responses, handling tool calls, managing conversation context — without coupling application logic to a specific vendor.

---

## 1. High-Level Architecture

```
┌─────────────────────────────────────────────────────┐
│                   Application Code                  │
│         stream(model, context, options)              │
│         complete(model, context, options)            │
└───────────────────┬─────────────────────────────────┘
                    │
┌───────────────────▼─────────────────────────────────┐
│               API Registry                          │
│   Maps API name → { stream, streamSimple }          │
│   e.g. "openai-completions" → OpenAI provider       │
│   e.g. "anthropic-messages" → Anthropic provider    │
└───────────────────┬─────────────────────────────────┘
                    │
┌───────────────────▼─────────────────────────────────┐
│            Provider Implementation                  │
│  Translates unified types ↔ vendor-specific API     │
│  Returns AssistantMessageEventStream                │
└───────────────────┬─────────────────────────────────┘
                    │
┌───────────────────▼─────────────────────────────────┐
│          AssistantMessageEventStream                │
│  Async-iterable push-based stream of events         │
│  Events: start → text/thinking/toolcall deltas      │
│          → done | error                             │
│  .result() → Promise<AssistantMessage>              │
└─────────────────────────────────────────────────────┘
```

---

## 2. Core Types (`types.ts`)

### 2.1 Content Blocks

Messages contain typed content blocks — the atomic units of LLM input/output:

| Type | Fields | Purpose |
|------|--------|---------|
| `TextContent` | `text`, `textSignature?` | Plain text output |
| `ThinkingContent` | `thinking`, `thinkingSignature?`, `redacted?` | Chain-of-thought reasoning (hidden from user in some UIs) |
| `ImageContent` | `data` (base64), `mimeType` | Image input (e.g. screenshots for vision models) |
| `ToolCall` | `id`, `name`, `arguments`, `thoughtSignature?` | LLM requesting a tool execution |

### 2.2 Messages

Three message roles form a conversation:

- **`UserMessage`** — `role: "user"`, content is string or array of `TextContent | ImageContent`
- **`AssistantMessage`** — `role: "assistant"`, content is array of `TextContent | ThinkingContent | ToolCall`. Also carries: `api`, `provider`, `model`, `usage`, `stopReason`, `errorMessage?`, `responseId?`, `timestamp`
- **`ToolResultMessage`** — `role: "toolResult"`, carries `toolCallId`, `toolName`, content array, `isError` flag

The union `Message = UserMessage | AssistantMessage | ToolResultMessage` is what flows through the system.

### 2.3 Model

A `Model<TApi>` describes a specific LLM endpoint:

```typescript
interface Model<TApi extends Api> {
  id: string;           // e.g. "gpt-4o", "claude-sonnet-4-20250514"
  name: string;         // Human-friendly name
  api: TApi;            // Which API protocol ("openai-completions", "anthropic-messages", etc.)
  provider: Provider;   // Which vendor ("openai", "anthropic", "ollama", etc.)
  baseUrl: string;      // API endpoint URL
  reasoning: boolean;   // Supports extended thinking?
  input: ("text" | "image")[];
  cost: { input, output, cacheRead, cacheWrite };  // $/million tokens
  contextWindow: number;
  maxTokens: number;
  headers?: Record<string, string>;
  compat?: ...;         // Provider-specific compatibility overrides
}
```

### 2.4 Context

The full conversation state sent to the LLM on each request:

```typescript
interface Context {
  systemPrompt?: string;
  messages: Message[];
  tools?: Tool[];
}
```

### 2.5 Tool

```typescript
interface Tool<TParameters extends TSchema = TSchema> {
  name: string;
  description: string;
  parameters: TParameters;  // JSON Schema (via TypeBox)
}
```

### 2.6 Usage & Cost

```typescript
interface Usage {
  input: number;       // input tokens (excluding cache)
  output: number;      // output tokens
  cacheRead: number;   // tokens served from cache
  cacheWrite: number;  // tokens written to cache
  totalTokens: number;
  cost: { input, output, cacheRead, cacheWrite, total };  // in dollars
}
```

### 2.7 StopReason

Why the model stopped generating:
- `"stop"` — Natural completion
- `"length"` — Hit max token limit
- `"toolUse"` — Model wants to call tool(s)
- `"error"` — Provider/network error
- `"aborted"` — Cancelled by caller (e.g. AbortSignal)

### 2.8 StreamOptions

```typescript
interface StreamOptions {
  temperature?: number;
  maxTokens?: number;
  signal?: AbortSignal;
  apiKey?: string;
  cacheRetention?: "none" | "short" | "long";
  sessionId?: string;
  headers?: Record<string, string>;
  maxRetryDelayMs?: number;
  metadata?: Record<string, unknown>;
  onPayload?: (payload, model) => payload | undefined;
}
```

`SimpleStreamOptions` extends this with `reasoning?: ThinkingLevel` and `thinkingBudgets?`.

---

## 3. Event Stream (`utils/event-stream.ts`)

### 3.1 Generic EventStream

`EventStream<T, R>` is a push-based async-iterable stream:

- **`push(event)`** — Enqueues an event. If a consumer is waiting, delivers immediately.
- **`end(result?)`** — Marks stream as done, resolves final result.
- **`[Symbol.asyncIterator]()`** — Allows `for await (const event of stream)`.
- **`result()`** — Returns `Promise<R>` that resolves when stream completes.

Internally uses a queue + waiting-consumers pattern (no external dependencies).

### 3.2 AssistantMessageEventStream

Specialized `EventStream<AssistantMessageEvent, AssistantMessage>` that:
- Completes when event type is `"done"` or `"error"`
- Extracts the final `AssistantMessage` from terminal events

### 3.3 Event Protocol

Events flow in this order:

```
start
├── text_start → text_delta* → text_end
├── thinking_start → thinking_delta* → thinking_end
├── toolcall_start → toolcall_delta* → toolcall_end
└── (repeat for each content block)
done { message } | error { error }
```

Each event carries a `partial` (the in-progress `AssistantMessage`) and a `contentIndex` identifying which content block is being streamed.

---

## 4. API Registry (`api-registry.ts`)

A global map from API protocol names to provider implementations:

```typescript
interface ApiProvider<TApi, TOptions> {
  api: TApi;                              // e.g. "openai-completions"
  stream: StreamFunction<TApi, TOptions>; // Raw streaming
  streamSimple: StreamFunction<TApi, SimpleStreamOptions>; // Simplified
}
```

Key functions:
- **`registerApiProvider(provider, sourceId?)`** — Register a provider
- **`getApiProvider(api)`** — Look up provider by API name
- **`unregisterApiProviders(sourceId)`** — Remove providers by source
- **`clearApiProviders()`** — Remove all

This decouples the entry-point functions (`stream`, `complete`) from any specific provider.

---

## 5. Entry Points (`stream.ts`)

Four top-level functions that look up the provider from the registry and delegate:

| Function | Returns | Purpose |
|----------|---------|---------|
| `stream(model, ctx, opts)` | `AssistantMessageEventStream` | Raw streaming with provider-specific options |
| `complete(model, ctx, opts)` | `Promise<AssistantMessage>` | Awaits stream to completion |
| `streamSimple(model, ctx, opts)` | `AssistantMessageEventStream` | Streaming with simplified reasoning options |
| `completeSimple(model, ctx, opts)` | `Promise<AssistantMessage>` | Awaits simplified stream |

---

## 6. Provider Implementation Pattern

Each provider (e.g. `openai-completions.ts`, `anthropic.ts`) follows the same pattern:

1. **Export a `stream` function** `(model, context, options?) → AssistantMessageEventStream`
2. **Create an `AssistantMessageEventStream`**, then start an async IIFE
3. **Convert** unified `Context` → vendor-specific request format
4. **Call** the vendor API (streaming)
5. **Parse** vendor-specific SSE/chunks into unified `AssistantMessageEvent`s, pushing to the stream
6. **On completion**, push `done` event; **on error**, push `error` event with a proper `AssistantMessage`
7. **Return** the stream immediately (async work happens in background)

### Key Conversion Responsibilities:
- **Messages**: `UserMessage` → vendor user format, `AssistantMessage` → vendor assistant format, `ToolResultMessage` → vendor tool result format
- **Tools**: `Tool` → vendor function/tool definition format
- **Thinking blocks**: Some providers need them as text, others have native support
- **Tool call IDs**: Normalized across providers (different ID formats/lengths)
- **Usage**: Vendor-specific token counts → unified `Usage` struct
- **Stop reasons**: Vendor-specific finish reasons → unified `StopReason`

---

## 7. Supporting Utilities

### 7.1 Message Transform (`transform-messages.ts`)
Normalizes messages for cross-provider compatibility:
- Drops redacted thinking blocks when switching models
- Converts thinking blocks to text for cross-model replay
- Normalizes tool call IDs
- Inserts synthetic tool results for orphaned tool calls
- Skips errored/aborted assistant messages

### 7.2 Simple Options (`simple-options.ts`)
- `buildBaseOptions()` — Extracts common options from `SimpleStreamOptions`
- `adjustMaxTokensForThinking()` — Allocates token budget between thinking and output

### 7.3 Validation (`validation.ts`)
- Validates tool call arguments against JSON Schema using AJV
- Coerces types where possible

### 7.4 Context Overflow (`overflow.ts`)
- Detects context window overflow via error message pattern matching
- Handles silent overflow (some providers accept but return bad results)

### 7.5 Streaming JSON (`json-parse.ts`)
- Parses partial/incomplete JSON during streaming (tool call arguments arrive incrementally)

---

## 8. Lazy Loading & Registration (`register-builtins.ts`)

Providers are lazily loaded on first use to avoid importing heavy SDKs upfront:

1. Each provider has a `loadXxxProviderModule()` that does a dynamic `import()`
2. `createLazyStream()` wraps this: creates an outer `AssistantMessageEventStream`, loads the module, then `forwardStream()` pipes the inner stream's events to the outer
3. `registerBuiltInApiProviders()` registers all lazy-wrapped providers at module load time

---

## 9. Design Principles

1. **Provider agnostic** — Application code uses `Model` + `Context`, never vendor types
2. **Streaming-first** — Every interaction returns an `EventStream`; non-streaming is just `stream().result()`
3. **Push-based events** — Fine-grained events (`text_delta`, `thinking_delta`, `toolcall_delta`) enable real-time UIs
4. **Registry pattern** — Providers are pluggable; register/unregister at runtime
5. **Unified message format** — One `Message` union type for all providers
6. **Tool call support** — First-class tool definitions, validation, streaming argument parsing
7. **Error handling** — Errors are encoded as events in the stream, never thrown from stream functions
8. **Cross-provider replay** — Message transform layer handles thinking blocks, tool IDs, etc. when switching models mid-conversation

---

## 10. Go Implementation Plan

For the Go variant in `pkg/ai`, we preserve the core abstractions while adapting to Go idioms:

```
pkg/ai/
├── types.go        # Core types: Message, Content blocks, Model, Context, Tool, Usage
├── event.go        # AssistantMessageEvent + EventStream (channel-based)
├── provider.go     # Provider interface + registry
├── stream.go       # Top-level Stream/Complete entry points
└── ollama/
    └── ollama.go   # Ollama provider (OpenAI-compatible chat completions API)
```

### Key Go Adaptations:
- **Channels** replace the push-based EventStream (idiomatic Go concurrency)
- **Interfaces** replace TypeScript generics for the Provider contract
- **`context.Context`** replaces `AbortSignal` for cancellation
- **JSON Schema as `map[string]any`** replaces TypeBox schemas
- **Error values** in events rather than thrown exceptions
