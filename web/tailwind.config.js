/** @type {import('tailwindcss').Config} */
export default {
  content: ['./index.html', './src/**/*.{vue,js,ts,jsx,tsx}'],
  darkMode: 'class',
  theme: {
    extend: {
      fontFamily: {
        // 平台只带两张脸（src/fonts：Google Sans Code + Noto Sans SC），Tailwind 的
        // 默认栈会把 `font-mono` 掉回 ui-monospace / Menlo / Consolas，界面上就会出现
        // 一批「看着像另一种字」的等宽片段。sans 与 mono 都必须指回 style.css 的
        // 两个变量——等宽栈同样以 Google Sans Code 领头（它本身是等宽 face）。
        sans: ['var(--font-family)'],
        mono: ['var(--font-mono)'],
      },
      borderRadius: {
        DEFAULT: '6px',
        sm: '4px',
        md: '6px',
        lg: '8px',
        xl: '12px',
        full: '9999px',
      },
      fontSize: {
        // Small steps follow the root font but keep a px floor so the 12px
        // appearance tier stays readable.
        '3xs': ['var(--text-size-10)', { lineHeight: '1.5' }],
        '2xs': ['var(--text-size-11)', { lineHeight: '1.5' }],
        xs: ['max(10.5px, 0.75rem)', { lineHeight: '1rem' }],
        sm: ['max(11px, 0.875rem)', { lineHeight: '1.25rem' }],
        '2xl': ['var(--text-size-22)', { lineHeight: '1.4' }],
      },
      colors: {
        platform: {
          bg: 'var(--color-bg)',
          surface: 'var(--color-surface)',
          'surface-muted': 'var(--color-surface-muted)',
          text: 'var(--color-text)',
          'text-muted': 'var(--color-text-muted)',
          border: 'var(--color-border)',
          'border-strong': 'var(--color-border-strong)',
          primary: 'var(--color-primary)',
          'primary-hover': 'var(--color-primary-hover)',
          'primary-soft': 'var(--color-primary-soft)',
          danger: 'var(--color-danger)',
          'danger-soft': 'var(--color-danger-soft)',
          success: 'var(--color-success)',
          warning: 'var(--color-warning)',
          info: 'var(--color-info)',
        },
      },
    },
  },
  plugins: [],
}
