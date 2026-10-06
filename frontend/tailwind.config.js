/** @type {import('tailwindcss').Config} */
export default {
  content: [
    "./index.html",
    "./src/**/*.{vue,js,ts,jsx,tsx}",
  ],
  darkMode: 'class',
  theme: {
    extend: {
      colors: {
        // Deeper than the server dashboard: on a large TV panel the extra
        // darkness is what makes the status colours read as signal.
        wall: {
          950: '#05080f',
          900: '#0a0f1a',
          850: '#101827',
          800: '#18202f',
          700: '#243044',
        },
        signal: {
          up: '#10b981',
          warn: '#f59e0b',
          down: '#f43f5e',
          idle: '#64748b',
          accent: '#22d3ee',
        }
      },
      fontFamily: {
        mono: ['"JetBrains Mono"', 'Consolas', 'Menlo', 'Monaco', 'Courier New', 'monospace'],
        sans: ['"Inter"', 'system-ui', '-apple-system', 'sans-serif'],
      },
      fontSize: {
        // Steps sized for a wall panel read from three metres away.
        'kpi': ['3.25rem', { lineHeight: '1', letterSpacing: '-0.03em' }],
        'kpi-sm': ['2.25rem', { lineHeight: '1', letterSpacing: '-0.02em' }],
      },
      animation: {
        'ticker': 'ticker 48s linear infinite',
      },
      keyframes: {
        ticker: {
          '0%': { transform: 'translateX(0)' },
          '100%': { transform: 'translateX(-50%)' },
        }
      }
    },
  },
  plugins: [],
}
