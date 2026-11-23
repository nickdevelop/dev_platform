package agentloop

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

var (
	// ErrContextCancelled indicates the loop was aborted because the context was cancelled.
	ErrContextCancelled = errors.New("agentloop: context cancelled")
	// ErrNoCompleter indicates no Completer was provided to the runner.
	ErrNoCompleter = errors.New("agentloop: completer is required")
)

// Runner drives the unified ReAct (Reasoning + Acting) loop.
type Runner struct {
	completer Completer
	registry  Registry
}

// NewRunner creates a unified ReAct Loop Runner.
func NewRunner(completer Completer, registry Registry) *Runner {
	return &Runner{
		completer: completer,
		registry:  registry,
	}
}

// Run executes the ReAct loop until completion, user-interruption, step limit exhaustion, or suspension.
func (r *Runner) Run(ctx context.Context, baseReq CompletionRequest, opts LoopOptions) (RunOutcome, error) {
	if r.completer == nil {
		return RunOutcome{}, ErrNoCompleter
	}

	maxSteps := opts.MaxSteps
	if maxSteps <= 0 {
		maxSteps = 10
	}

	outcome := RunOutcome{
		Messages: append([]CompletionMessage(nil), baseReq.Messages...),
	}

	stepCount := 0
	var finalAssistantText strings.Builder
	var finalReasoning strings.Builder

	for {
		if err := ctx.Err(); err != nil {
			return outcome, fmt.Errorf("%w: %v", ErrContextCancelled, err)
		}

		// Prepare request for this iteration
		req := baseReq
		req.Messages = outcome.Messages

		// If a tool registry is attached, expose its tool definitions
		if r.registry != nil {
			req.Tools = r.registry.Definitions()
		}

		// Check if we hit the step limit
		if stepCount >= maxSteps {
			if opts.ForceFinal {
				// Strip tools to force the model to provide a concluding text answer
				req.Tools = nil
			} else {
				// Stop immediately upon reaching step limit
				break
			}
		}

		var receivedToolCalls []ToolCall
		var currentStepText strings.Builder
		var currentStepReasoning strings.Builder

		streamErr := r.completer.Stream(ctx, req, func(delta CompletionDelta) error {
			if delta.Usage != nil {
				outcome.TotalUsage.PromptTokens += delta.Usage.PromptTokens
				outcome.TotalUsage.CompletionTokens += delta.Usage.CompletionTokens
				outcome.TotalUsage.TotalTokens += delta.Usage.TotalTokens
			}
			if len(delta.ToolCalls) > 0 {
				receivedToolCalls = append(receivedToolCalls, delta.ToolCalls...)
			}
			if delta.Text != "" {
				currentStepText.WriteString(delta.Text)
				finalAssistantText.WriteString(delta.Text)
			}
			if delta.Reasoning != "" {
				currentStepReasoning.WriteString(delta.Reasoning)
				finalReasoning.WriteString(delta.Reasoning)
			}
			if opts.OnDelta != nil {
				opts.OnDelta(delta)
			}
			return nil
		})

		if streamErr != nil {
			return outcome, fmt.Errorf("agentloop: completion stream error: %w", streamErr)
		}

		// No tool calls requested: the turn has converged on a natural final text answer.
		if len(receivedToolCalls) == 0 {
			outcome.Messages = append(outcome.Messages, CompletionMessage{
				Role: RoleAssistant,
				Text: currentStepText.String(),
			})
			break
		}

		// The model invoked tools: append assistant handoff message containing tool calls
		outcome.Messages = append(outcome.Messages, CompletionMessage{
			Role:      RoleAssistant,
			Text:      currentStepText.String(),
			ToolCalls: receivedToolCalls,
		})

		// If tools were called after reaching the limit (and ForceFinal wasn't respected), break out
		if stepCount >= maxSteps || r.registry == nil {
			break
		}

		// Execute all requested tool calls sequentially
		hasSuspended := false
		hasApprovalReq := false
		var triggerCall *ToolCall

		for _, call := range receivedToolCalls {
			if opts.OnToolStart != nil {
				opts.OnToolStart(call)
			}

			res, _ := r.registry.Execute(ctx, call)

			if opts.OnToolEnd != nil {
				opts.OnToolEnd(call, res)
			}

			// Append the tool execution result message
			outcome.Messages = append(outcome.Messages, CompletionMessage{
				Role:       RoleTool,
				ToolCallID: call.ID,
				Text:       res.Output,
			})

			// Check for flow suspension (e.g. asking a clarifying question to user)
			if res.Status == ToolStatusSuspended {
				hasSuspended = true
				callCopy := call
				triggerCall = &callCopy
				break
			}

			// Check for human approval requirement (e.g. high-risk bash / file command)
			if res.Status == ToolStatusRequireApproval {
				hasApprovalReq = true
				callCopy := call
				triggerCall = &callCopy
				break
			}
		}

		if opts.OnStepFinish != nil {
			opts.OnStepFinish(stepCount, receivedToolCalls)
		}

		stepCount++
		outcome.StepsTaken = stepCount

		if hasSuspended {
			outcome.Suspended = true
			outcome.PendingCall = triggerCall
			break
		}
		if hasApprovalReq {
			outcome.ApprovalReq = true
			outcome.PendingCall = triggerCall
			break
		}
	}

	outcome.FinalText = finalAssistantText.String()
	outcome.Reasoning = finalReasoning.String()
	return outcome, nil
}
