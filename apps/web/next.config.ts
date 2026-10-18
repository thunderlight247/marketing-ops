import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  reactStrictMode: true,
  // Internal tool: avoid advertising the framework in response headers.
  poweredByHeader: false,
};

export default nextConfig;
