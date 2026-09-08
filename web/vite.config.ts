import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

export default defineConfig({
  base: "./",
  plugins: [react()],
  server: {
    proxy: {
      "/api": "http://127.0.0.1:7373",
    },
  },
  build: {
    outDir: "dist",
    emptyOutDir: true,
  },
});
