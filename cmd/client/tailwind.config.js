import primeui from 'tailwindcss-primeui'

/** @type {import('tailwindcss').Config} */
export default {
  theme: {
    extend: {}
  },
  content: ['./index.html', './src/**/*.{vue,ts}'],
  plugins: [primeui]
}
