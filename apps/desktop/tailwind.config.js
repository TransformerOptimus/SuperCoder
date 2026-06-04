/** @type {import('tailwindcss').Config} */
export default {
  content: ['./index.html', './src/**/*.{js,ts,jsx,tsx}'],
  darkMode: 'class',
  theme: {
    fontFamily: {
      sans: ['"Proxima Nova"', 'sans-serif'],
      mono: ['"Space Mono"', 'monospace'],
    },
    extend: {
      colors: {
        dark: {
          bg: '#1a1a1a',
          surface: '#242424',
          hover: '#2e2e2e',
          border: '#333333',
        },
      },
    },
  },
  plugins: [],
};
