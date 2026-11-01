import { NextRequest, NextResponse } from "next/server";

// Basic authentication is intended for a single operator over HTTPS. The API
// key itself never leaves the server. An explicit bypass exists only for local
// testing; never enable it on a publicly reachable deployment.
async function equalDigest(left: string, right: string): Promise<boolean> {
  const encoder = new TextEncoder();
  const [a, b] = await Promise.all([
    crypto.subtle.digest("SHA-256", encoder.encode(left)),
    crypto.subtle.digest("SHA-256", encoder.encode(right)),
  ]);
  const x = new Uint8Array(a);
  const y = new Uint8Array(b);
  let difference = 0;
  for (let i = 0; i < x.length; i++) difference |= x[i] ^ y[i];
  return difference === 0;
}

export async function middleware(request: NextRequest) {
  const disabled = process.env.WEB_AUTH_DISABLED === "true";
  const username = process.env.OPERATOR_USERNAME;
  const password = process.env.OPERATOR_PASSWORD;

  if (!disabled) {
    // Fail closed on missing or invalid configuration, including when the app
    // is started with `next start` without its production environment.
    if (
      !username ||
      !password ||
      username.includes(":") ||
      !/^[\x20-\x7e]+$/.test(username + password)
    ) {
      return new NextResponse("Operator authentication is not configured.", { status: 503 });
    }
    const expected = `Basic ${btoa(`${username}:${password}`)}`;
    if (!(await equalDigest(request.headers.get("authorization") ?? "", expected))) {
      return new NextResponse("Authentication required.", {
        status: 401,
        headers: {
          "WWW-Authenticate": 'Basic realm="Marketing Operations", charset="UTF-8"',
          "Cache-Control": "no-store",
        },
      });
    }
  }

  if (
    request.nextUrl.pathname.startsWith("/api/v1/") &&
    !["GET", "HEAD", "OPTIONS"].includes(request.method)
  ) {
    const origin = request.headers.get("origin");
    const expectedOrigin = process.env.WEB_ORIGIN || (disabled ? request.nextUrl.origin : "");
    if (!origin || !expectedOrigin || origin !== expectedOrigin) {
      return new NextResponse("Invalid request origin.", { status: 403 });
    }
  }
  return NextResponse.next();
}
