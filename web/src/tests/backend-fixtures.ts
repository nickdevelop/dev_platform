// Backend metadata as the platform's backend list reports it. Tests that mount
// components reading the catalog facts (labels, order, service support) seed
// the agents store with this list instead of stubbing the network.
import type { AgentBackend, Capabilities } from '../api/agents'

const none: Capabilities = {
  streaming: false,
  sessionDiscovery: false,
  sessionContinue: false,
  stop: false,
  interrupt: false,
  modelSelection: false,
  toolCallEvents: false,
  toolConfirmation: false,
  sessionHistory: false,
  rawEvents: false,
}

export function backendFixtures(): AgentBackend[] {
  return [
    {
      backend: 'opencode-v2',
      capabilities: { ...none, streaming: true, stop: true, toolConfirmation: true, rawEvents: true },
      enabled: true,
      label: 'OpenCode',
      shortName: 'OC',
      order: 1,
      serviceSupported: true,
      color: '#10B981',
    },
    {
      backend: 'pi',
      capabilities: { ...none, streaming: true, stop: true, toolCallEvents: true },
      enabled: true,
      label: 'Pi Agent',
      shortName: 'PI',
      order: 2,
      serviceSupported: false,
      color: '#F97316',
    },
  ]
}
