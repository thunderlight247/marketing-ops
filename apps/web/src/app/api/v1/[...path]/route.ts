import { NextRequest, NextResponse } from "next/server";

export const dynamic = "force-dynamic";

async function proxy(request: NextRequest) {
  const base = process.env.API_BASE_URL;
  const key = process.env.API_KEY;
  if (!base || (!key && process.env.WEB_AUTH_DISABLED !== "true")) {
    return NextResponse.json({ error: "API proxy is not configured." }, { status: 503 });
  }

  let target: URL;
  try {
    const configured = new URL(base);
    if (
      !["http:", "https:"].includes(configured.protocol) ||
      configured.username ||
      configured.password ||
      configured.search ||
      configured.hash
    ) {
      throw new Error("Invalid API URL");
    }
    target = new URL(
      `${configured.origin}${configured.pathname.replace(/\/+$/, "")}${request.nextUrl.pathname}${request.nextUrl.search}`,
    );
  } catch {
    return NextResponse.json({ error: "API proxy is not configured." }, { status: 503 });
  }

  const headers = new Headers();
  for (const name of ["accept", "content-type"]) {
    const value = request.headers.get(name);
    if (value) headers.set(name, value);
  }
  if (key) headers.set("authorization", `Bearer ${key}`);

  try {
    // Live event streams stay open indefinitely; they must not hit the 120s abort below.
    const isSSE =
      request.headers.get("accept")?.includes("text/event-stream") ||
      /\/events$/.test(request.nextUrl.pathname);
    const response = await fetch(target, {
      method: request.method,
      headers,
      body: ["GET", "HEAD"].includes(request.method) ? undefined : await request.arrayBuffer(),
      redirect: "manual",
      cache: "no-store",
      signal: isSSE ? undefined : AbortSignal.timeout(120_000),
    });
    const responseHeaders = new Headers({ "Cache-Control": "no-store" });
    const contentType = response.headers.get("content-type");
    if (contentType) responseHeaders.set("Content-Type", contentType);
    if (response.headers.get("x-accel-buffering")) {
      responseHeaders.set("X-Accel-Buffering", response.headers.get("x-accel-buffering") || "no");
    }
    return new Response(response.body, { status: response.status, headers: responseHeaders });
  } catch {
    return NextResponse.json({ error: "Could not reach the API." }, { status: 502 });
  }
}

export const GET = proxy;
export const HEAD = proxy;
export const POST = proxy;
export const PUT = proxy;
export const PATCH = proxy;
export const DELETE = proxy;
