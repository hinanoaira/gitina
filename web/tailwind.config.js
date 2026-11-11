/** @type {import('tailwindcss').Config} */
export default {
  content: [
    "./index.html",
    "./src/**/*.{vue,js,ts,jsx,tsx}",
  ],
  theme: {
    extend: {
      colors: {
        github: {
          dark: '#0d1117',
          darker: '#010409',
          border: '#30363d',
          subtle: '#161b22',
          muted: '#8b949e',
          text: '#c9d1d9',
          blue: '#58a6ff',
          green: '#238636',
          greenHover: '#2ea043',
          red: '#da3633',
        }
      }
    },
  },
  plugins: [],
}
