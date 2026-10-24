import { NavLink, Outlet } from 'react-router'
import styles from './SettingsLayout.module.css'

const links = [
  { to: '/settings/appearance', label: '外观' },
  { to: '/settings/models', label: '模型' },
  { to: '/settings/agents', label: 'Agent' },
]

export function SettingsLayout() {
  return (
    <div className={styles.page} data-test="settings-page">
      <aside className={styles.nav} aria-label="设置分类">
        <h2>设置</h2>
        {links.map((link) => (
          <NavLink
            key={link.to}
            to={link.to}
            className={({ isActive }) => `${styles.link} ${isActive ? styles.active : ''}`}
          >
            {link.label}
          </NavLink>
        ))}
      </aside>
      <main className={styles.main}>
        <Outlet />
      </main>
    </div>
  )
}
