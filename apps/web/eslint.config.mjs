import { dirname } from "node:path";
import { fileURLToPath } from "node:url";
import { FlatCompat } from "@eslint/eslintrc";

const __dirname = dirname(fileURLToPath(import.meta.url));
const compat = new FlatCompat({ baseDirectory: __dirname });

const eslintConfig = [
  {
    // Only source is linted; matches the gitignored generated output dirs.
    ignores: [
      ".next/**",
      ".next-e2e-*/**",
      "node_modules/**",
      "next-env.d.ts",
      "test-results*/**",
      "playwright-report*/**",
    ],
  },
  ...compat.extends("next/core-web-vitals", "next/typescript"),
];

export default eslintConfig;
