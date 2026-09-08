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
        ink: "#0B1F2A",
        hull: "#12303F",
        steel: "#24505F",
        railtext: "#A9C2CC",
        paper: "#EDF1F0",
        panel: "#FFFFFF",
        line: "#CBD8D6",
        linesoft: "#E1E8E7",
        text: "#12303F",
        muted: "#5C7480",
        run: "#1F7A5C",
        pause: "#C8860D",
        fail: "#B7362A",
        stop: "#8DA3AC",
        link: "#1F5E7E",
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
