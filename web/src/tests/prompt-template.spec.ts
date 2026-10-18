import { describe, expect, it } from 'vitest'
import {
  applyUserPromptTemplate,
  hasUserPromptPlaceholder,
  USER_PROMPT_PLACEHOLDER,
} from '../features/prompts/promptTemplate'

describe('promptTemplate utility', () => {
  it('correctly detects {user_prompt} placeholder presence', () => {
    expect(hasUserPromptPlaceholder('Hello {user_prompt}')).toBe(true)
    expect(hasUserPromptPlaceholder('{user_prompt}')).toBe(true)
    expect(hasUserPromptPlaceholder('No placeholder here')).toBe(false)
  })

  it('replaces single {user_prompt} placeholder with user input', () => {
    const template = 'Please analyze the following:\n\n{user_prompt}\n\nProvide recommendations.'
    const userPrompt = 'const a = 123;'
    const result = applyUserPromptTemplate(template, userPrompt)
    expect(result).toBe('Please analyze the following:\n\nconst a = 123;\n\nProvide recommendations.')
  })

  it('replaces multiple {user_prompt} placeholders with user input', () => {
    const template = 'First: {user_prompt}, Second: {user_prompt}'
    const userPrompt = 'code'
    const result = applyUserPromptTemplate(template, userPrompt)
    expect(result).toBe('First: code, Second: code')
  })

  it('replaces placeholder with empty string when user prompt is empty', () => {
    const template = 'Prefix: {user_prompt} Postfix'
    const result = applyUserPromptTemplate(template, '')
    expect(result).toBe('Prefix:  Postfix')
  })

  it('appends user prompt when placeholder is absent and user prompt is non-empty', () => {
    const template = 'You are a code expert. Always reply concisely.'
    const userPrompt = 'How do I invert a binary tree?'
    const result = applyUserPromptTemplate(template, userPrompt)
    expect(result).toBe('You are a code expert. Always reply concisely.\n\nHow do I invert a binary tree?')
  })

  it('returns template as-is when placeholder is absent and user prompt is empty', () => {
    const template = 'You are a code expert.'
    const result = applyUserPromptTemplate(template, '   ')
    expect(result).toBe('You are a code expert.')
  })

  it('returns user prompt directly if template is empty', () => {
    expect(applyUserPromptTemplate('', 'Hello')).toBe('Hello')
    expect(applyUserPromptTemplate('   ', 'Hello')).toBe('Hello')
  })
})
