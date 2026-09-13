import type { Config } from "tailwindcss";

export default {
  content: ["./index.html", "./src/**/*.{ts,tsx}"],
  theme: {
    screens: {
      // Matches MOCKUP.html's rail-collapse breakpoint (max-width: 900px).
      md: "901px",
    },
    extend: {
      colors: {
        ink: "var(--c-ink)",
        hull: "var(--c-hull)",
        steel: "var(--c-steel)",
        railtext: "var(--c-railtext)",
        paper: "var(--c-paper)",
        panel: "var(--c-panel)",
        line: "var(--c-line)",
        linesoft: "var(--c-linesoft)",
        text: "var(--c-text)",
        muted: "var(--c-muted)",
        run: "var(--c-run)",
        pause: "var(--c-pause)",
        fail: "var(--c-fail)",
        stop: "var(--c-stop)",
        link: "var(--c-link)",
      },
      fontFamily: {
        sans: ["Archivo", "system-ui", "sans-serif"],
        mono: ['"IBM Plex Mono"', "ui-monospace", "monospace"],
      },
      spacing: {
        row: "32px",
      },
      borderRadius: {
        DEFAULT: "3px",
        md: "4px",
      },
    },
  },
  plugins: [],
} satisfies Config;
