import { StrictMode } from 'react'
import { beforeEach, describe, expect, it } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import { resetProbe } from '../spike/strictModeProbe'
import { StrictModeProbe } from '../spike/StrictModeProbe'

describe('StrictMode singleton probe', () => {
  beforeEach(() => {
    resetProbe()
  })

  it('reuses one window-level connection across StrictMode effect replay', async () => {
    render(
      <StrictMode>
        <StrictModeProbe />
      </StrictMode>,
    )

    await waitFor(() => {
      expect(screen.getByTestId('probe-total')).toHaveTextContent('1')
    })
    expect(screen.getByTestId('probe-live')).toHaveTextContent('1')
  })
})
