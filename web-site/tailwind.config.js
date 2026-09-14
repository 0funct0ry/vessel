/** @type {import('tailwindcss').Config} */
module.exports = {
  content: ['./index.html'],
  darkMode: 'media',
  theme: {
    extend: {
      colors: {
        ink: '#0B1F2A',
        hull: '#12303F',
        steel: '#24505F',
        paper: '#EDF1F0',
        panel: '#FFFFFF',
        line: '#CBD8D6',
        'line-soft': '#E1E8E7',
        text: '#12303F',
        muted: '#5C7480',
        run: '#1F7A5C',
        pause: '#C8860D',
        fail: '#B7362A',
        stop: '#8DA3AC',
        link: '#1F5E7E',
      },
      fontFamily: {
        sans: ['Archivo', 'system-ui', 'sans-serif'],
        mono: ['IBM Plex Mono', 'ui-monospace', 'monospace'],
      },
    },
  },
  plugins: [],
}
