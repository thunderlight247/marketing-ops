import { afterEach, describe, expect, it, vi } from "vitest";
import { pairOwnWhatsApp } from "./leadEngine";

function jsonRes(status: number, body: unknown): Response {
  return {
    ok: status >= 200 && status < 300,
    status,
    json: async () => body,
    text: async () => JSON.stringify(body),
  } as unknown as Response;
}

describe("WhatsApp phone-number pairing", () => {
  afterEach(() => vi.unstubAllGlobals());

  it("posts the number to the workspace-scoped own-number pairing endpoint", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      jsonRes(200, {
        status: "qr_ready",
        qr: "data:image/png;base64,qr",
        pairingPhone: "233241234567",
        pairingCode: "ABCD1234",
        source: "own",
      }),
    );
    vi.stubGlobal("fetch", fetchMock);

    const result = await pairOwnWhatsApp("", "ws-1", "233241234567");

    expect(fetchMock.mock.calls[0][0]).toBe(
      "/api/v1/workspaces/ws-1/lead-engine/whatsapp/pair?own=1",
    );
    expect(fetchMock.mock.calls[0][1].method).toBe("POST");
    expect(JSON.parse(fetchMock.mock.calls[0][1].body)).toEqual({
      phone: "233241234567",
    });
    expect(result.pairingCode).toBe("ABCD1234");
    expect(result.source).toBe("own");
  });

  it("surfaces a dispatcher refusal instead of pretending a code was sent", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        jsonRes(400, {
          error: "Enter a phone number with its country code, digits only (8–15 digits).",
        }),
      ),
    );

    await expect(pairOwnWhatsApp("", "ws-1", "123")).rejects.toThrow(
      "Enter a phone number with its country code",
    );
  });
});
