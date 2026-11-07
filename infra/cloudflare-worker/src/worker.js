// Stable entrypoint for the Marketing Ops platform.
//
// Cloudflare gives free *.workers.dev hostnames to Workers, but not to
// Tunnels (a tunnel public hostname requires a verified domain zone). This
// Worker is the fix: it forwards every request to the quick-tunnel URL
// configured as TARGET in wrangler.toml.
//
// Pass-through semantics:
// - every method, every path, query string preserved;
// - headers forwarded except hop-specific ones (host/cf-*); the browser's
//   Origin header is kept so the web app's middleware sees the *stable* URL
//   (WEB_ORIGIN must be set to this Worker's public URL for non-GET APIs);
// - streaming responses (e.g. agent SSE) pass through untouched;
// - redirect: manual so redirects to relative paths stay on this domain.

const HOP_HEADERS = new Set([
  "host",
  "connection",
  "keep-alive",
  "transfer-encoding",
  "upgrade",
  "cf-connecting-ip",
  "cf-ray",
  "cf-ipcountry",
  "cf-visitor",
  "cdn-loop",
  "x-forwarded-proto",
  "x-forwarded-host",
  "x-real-ip",
]);

export default {
  async fetch(request, env) {
    const incoming = new URL(request.url);
    const target = new URL(`${new String(env.TARGET).replace(/\/+$/, "")}${incoming.pathname}${incoming.search}`);

    const headers = new Headers();
    for (const [name, value] of request.headers) {
      if (!HOP_HEADERS.has(name.toLowerCase())) headers.set(name, value);
    }

    if (request.method === "GET" || request.method === "HEAD") {
      const upstream = await fetch(target, {
        headers,
        redirect: "manual",
        cf: { cacheTtl: 0, cacheEverything: false },
      });
      return cloneResponse(upstream);
    }

    const body = await request.arrayBuffer();
    const upstream = await fetch(target, {
      method: request.method,
      headers,
      body: body.byteLength ? body : undefined,
      redirect: "manual",
      cf: { cacheTtl: 0, cacheEverything: false },
    });
    return cloneResponse(upstream);
  },
};

function cloneResponse(upstream) {
  const out = new Headers();
  for (const [name, value] of upstream.headers) {
    out.set(name, value);
  }
  // Manifest/HTML caching headers from the origin must not pin a rotating URL.
  out.delete("set-cookie");
  return new Response(upstream.body, {
    status: upstream.status,
    statusText: upstream.statusText,
    headers: out,
  });
}
