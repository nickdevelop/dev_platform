import type { CSSProperties } from 'react'
import { useAgentsStore } from '../stores/agents'
import styles from './BackendTag.module.css'

interface BackendTagProps {
  backend: string
  label?: string
}

export function BackendTag({ backend, label }: BackendTagProps) {
  const storeLabel = useAgentsStore((state) => state.backendLabel)
  const backendColor = useAgentsStore((state) => state.backendColor)
  const color = backendColor(backend)
  const text = label ?? storeLabel(backend)
  const style = color ? ({ '--tag-base': color } as CSSProperties) : undefined

  return (
    <span
      className={`${styles.tag} ${color ? styles.hasColor : styles.neutral}`}
      style={style}
      data-backend-color={color || undefined}
    >
      {text}
    </span>
  )
}
