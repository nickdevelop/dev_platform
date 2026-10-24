// Pure utility functions for prompt templates and placeholder replacement.

export const USER_PROMPT_PLACEHOLDER = '{user_prompt}'

/**
 * Checks whether a template string contains the {user_prompt} placeholder.
 */
export function hasUserPromptPlaceholder(template: string): boolean {
  return template.includes(USER_PROMPT_PLACEHOLDER)
}

/**
 * Combines a prompt template with user-provided text.
 * - If the template contains {user_prompt}, all occurrences are replaced by userPrompt.
 * - If the template does not contain {user_prompt}:
 *   - When userPrompt is non-empty, it is appended with two newlines.
 *   - When userPrompt is empty, the template is returned as-is.
 */
export function applyUserPromptTemplate(template: string, userPrompt: string): string {
  const trimmedTemplate = template.trim()
  const trimmedUser = userPrompt.trim()

  if (!trimmedTemplate) {
    return userPrompt
  }

  if (hasUserPromptPlaceholder(trimmedTemplate)) {
    return trimmedTemplate.replaceAll(USER_PROMPT_PLACEHOLDER, userPrompt)
  }

  if (trimmedUser !== '') {
    return `${trimmedTemplate}\n\n${userPrompt}`
  }

  return trimmedTemplate
}
