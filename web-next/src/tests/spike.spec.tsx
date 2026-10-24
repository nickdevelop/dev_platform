import { beforeEach, describe, expect, it } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { AppProviders } from '../app/providers'
import { useAppearanceStore } from '../stores/appearance'
import { SpikePage } from '../spike/SpikePage'

function renderSpike() {
  return render(
    <AppProviders>
      <SpikePage />
    </AppProviders>,
  )
}

describe('antd P0 spike', () => {
  beforeEach(() => {
    useAppearanceStore.getState().reset()
  })

  it('renders the table and file tree from real antd components', () => {
    renderSpike()
    expect(screen.getByText('pi/gpt-5')).toBeInTheDocument()
    expect(screen.getByText('src')).toBeInTheDocument()
    expect(screen.getByText('README.md')).toBeInTheDocument()
  })

  it('validates the form before submitting', async () => {
    const user = userEvent.setup()
    renderSpike()

    await user.click(screen.getByTestId('submit-form'))
    expect(await screen.findByText('请输入会话名称')).toBeInTheDocument()
    expect(await screen.findByText('请选择 Agent 后端')).toBeInTheDocument()
  })

  it('moves focus into the modal and closes it from the cancel action', async () => {
    const user = userEvent.setup()
    renderSpike()

    await user.click(screen.getByTestId('open-modal'))
    const dialog = await screen.findByRole('dialog')
    expect(dialog).toBeInTheDocument()
    await waitFor(() => {
      expect(screen.getByTestId('modal-input')).toHaveFocus()
    })
    expect(screen.getByTestId('modal-state')).toHaveTextContent('open')

    await user.click(screen.getByRole('button', { name: /取\s*消/ }))
    await waitFor(() => {
      expect(screen.getByTestId('modal-state')).toHaveTextContent('closed')
    })
  })

  it('switches themes through the shared appearance store', async () => {
    const user = userEvent.setup()
    renderSpike()

    await user.click(screen.getByText('深色'))
    await waitFor(() => {
      expect(document.documentElement.dataset.theme).toBe('dark')
    })
    expect(document.documentElement.style.getPropertyValue('--color-bg')).not.toBe('')
  })
})
