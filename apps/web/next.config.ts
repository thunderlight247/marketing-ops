import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  ...(process.env.NEXT_DIST_DIR ? { distDir: process.env.NEXT_DIST_DIR } : {}),
  reactStrictMode: true,
  // Internal tool: avoid advertising the framework in response headers.
  poweredByHeader: false,
};

export default nextConfig;
