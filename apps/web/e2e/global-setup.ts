import { execFileSync } from "node:child_process";
import { existsSync, readFileSync, renameSync, rmSync, writeFileSync } from "node:fs";
import { join } from "node:path";

export default async function globalSetup(): Promise<void> {
  const cwd = process.cwd();
  const envFile = join(cwd, ".env.local");
  const backupEnvFile = join(cwd, ".env.local.e2e-backup");
  const tsconfigFile = join(cwd, "tsconfig.json");
  const nextEnvFile = join(cwd, "next-env.d.ts");
  const originalTSConfig = readFileSync(tsconfigFile, "utf8");
  const originalNextEnv = readFileSync(nextEnvFile, "utf8");

  if (existsSync(envFile)) renameSync(envFile, backupEnvFile);
  try {
    const buildEnv = {
      ...process.env,
      NEXT_DIST_DIR: `.next-e2e-${process.env.PLAYWRIGHT_WEB_PORT ?? "3001"}`,
      NEXT_PUBLIC_API_BASE_URL: process.env.PLAYWRIGHT_API_BASE_URL ?? "http://localhost:8081",
    };
    execFileSync("npm", ["run", "build"], { cwd, stdio: "inherit", env: buildEnv });
  } finally {
    if (existsSync(backupEnvFile)) renameSync(backupEnvFile, envFile);
    writeFileSync(tsconfigFile, originalTSConfig);
    writeFileSync(nextEnvFile, originalNextEnv);
  }
  rmSync(join(cwd, "tsconfig.tsbuildinfo"), { force: true });
  const reportDirectory = join(
    cwd,
    `playwright-report-${process.env.PLAYWRIGHT_WEB_PORT ?? "3001"}`,
  );
  if (existsSync(reportDirectory)) rmSync(reportDirectory, { recursive: true, force: true });
}
