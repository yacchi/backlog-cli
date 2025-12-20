/** @type {import('tailwindcss').Config} */
export default {
  content: [
    './index.html',
    './src/**/*.{js,ts,jsx,tsx}',
  ],
  theme: {
    extend: {
      colors: {
        ink: 'var(--ink)',
        brand: 'var(--brand)',
        'brand-strong': 'var(--brand-strong)',
        accent: 'var(--accent)',
        surface: 'var(--surface)',
        outline: 'var(--outline)',
      },
      boxShadow: {
        glow: '0 20px 40px rgba(15, 23, 42, 0.18)',
      },
    },
  },
  plugins: [],
}
