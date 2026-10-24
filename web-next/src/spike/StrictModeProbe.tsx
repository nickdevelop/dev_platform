import { useEffect, useState } from 'react'
import { acquireProbe, probeSnapshot, type ProbeSnapshot } from './strictModeProbe'

export function StrictModeProbe() {
  const [snapshot, setSnapshot] = useState<ProbeSnapshot>(() => probeSnapshot())

  useEffect(() => {
    const update = () => setSnapshot(probeSnapshot())
    return acquireProbe(update)
  }, [])

  return (
    <dl className="p0-probe" data-test="strictmode-probe">
      <div>
        <dt>累计连接</dt>
        <dd data-test="probe-total">{snapshot.totalConnections}</dd>
      </div>
      <div>
        <dt>活跃监听</dt>
        <dd data-test="probe-live">{snapshot.liveListeners}</dd>
      </div>
      <div>
        <dt>连接 ID</dt>
        <dd>{snapshot.connectionId ?? '—'}</dd>
      </div>
    </dl>
  )
}
