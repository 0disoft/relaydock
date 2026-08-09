import { defineConfig, presetUno } from 'unocss';

export default defineConfig({
  presets: [presetUno()],
  shortcuts: {
    panel: 'rounded-xl border border-[var(--border)] bg-[var(--panel)]',
    button: 'rounded-lg px-3 py-2 text-sm font-600 transition disabled:cursor-not-allowed disabled:opacity-45',
    'button-primary': 'button bg-[var(--accent)] text-white hover:brightness-110',
    'button-secondary': 'button border border-[var(--border)] bg-transparent hover:bg-[var(--hover)]'
  }
});
