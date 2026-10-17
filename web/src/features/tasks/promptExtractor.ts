import type { ChatEntry } from '../chat/state/sessionLog'

/**
 * Patterns matching standard execution plan headers produced by the Agent during analysis.
 */
const PLAN_HEADER_PATTERNS = [
  /(?:^|\n)(?:#{1,4}\s*|【|\*\*)?(?:直接执行方案|可直接执行方案|执行方案|执行计划|实施方案|分步方案|操作计划)(?:】|\*\*)?[:：]?\s*\n([\s\S]+)$/i,
  /(?:^|\n)(?:#{1,4}\s*|【|\*\*)?(?:分步步骤|详细步骤|执行步骤)(?:】|\*\*)?[:：]?\s*\n([\s\S]+)$/i,
]

/**
 * Extracts a refined, clean execution prompt from a session's chat entries.
 * Prioritizes the latest assistant turn and parses structured execution plans.
 */
export function extractExecutionPrompt(
  entries: ChatEntry[],
  fallback?: { title?: string; description?: string },
): string {
  // Scan backwards for the latest assistant messages
  const assistantEntries = entries
    .filter((e) => e.kind === 'message' && e.role === 'assistant' && Boolean(e.text && e.text.trim()))
    .reverse()

  for (const entry of assistantEntries) {
    const text = entry.text.trim()
    if (!text) continue

    // 1. Try to match specific execution plan sections
    for (const pattern of PLAN_HEADER_PATTERNS) {
      const match = text.match(pattern)
      if (match && match[1] && match[1].trim().length > 10) {
        return cleanExtractedPlan(match[1].trim())
      }
    }

    // 2. If the message starts with or contains numbered lists or code blocks that represent steps
    if (/^\s*(?:1[\.\、]|[-*]\s*\[\s*\])\s+/m.test(text)) {
      return cleanExtractedPlan(text)
    }

    // 3. Fallback to using the entire assistant text (stripping common preamble conversational filler)
    const stripped = stripConversationalPreamble(text)
    if (stripped.length > 20) {
      return stripped
    }
  }

  // If no assistant text was found, construct a helpful placeholder if fallback given
  if (fallback?.title) {
    let out = `【执行方案】${fallback.title}`
    if (fallback.description) {
      out += `\n需求与目标：${fallback.description}`
    }
    return out
  }

  return ''
}

/**
 * Strips conversational pleasantries that Agents frequently prepend to their plan outputs.
 */
function stripConversationalPreamble(text: string): string {
  return text
    .replace(
      /^(?:好的|收到|经过仔细分析|已完成任务分析|根据上述需求与确认)[，,。!\s\n]*(?:以下是.*?(?:执行方案|执行计划|具体步骤)[：:\n]*)?/i,
      '',
    )
    .trim()
}

/**
 * Cleans extracted plan markdown, ensuring it is ready for direct prompt execution.
 */
function cleanExtractedPlan(plan: string): string {
  // Strip trailing polite wrap-ups like "如有疑问随时沟通" / "请确认后开始执行"
  const cleaned = plan.replace(
    /\n+(?:如有任何疑问|若无异议|请确认以上方案|确认后即可开始执行)[\s\S]*$/i,
    '',
  )
  return cleaned.trim()
}
