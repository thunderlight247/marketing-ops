import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  ...(process.env.NEXT_DIST_DIR ? { distDir: process.env.NEXT_DIST_DIR } : {}),
  reactStrictMode: true,
  // Do not enable `output: "standalone"` on Next 15.5: its build-trace pass
  // crashes on the generated /_not-found route. The Dockerfile runs the normal
  // production build with node_modules instead.
  // The repository root has its own lockfile for tooling; keep tracing inside
  // the web app instead of letting Next infer the monorepo root.
  outputFileTracingRoot: process.cwd(),
  // Internal tool: avoid advertising the framework in response headers.
  poweredByHeader: false,
};

export default nextConfig;
