// Wire contract with the Go agent bridge. These shapes mirror
// companion/internal/phoneruntime/agentbridge/shapes.go verbatim; change
// them in both places or not at all.

import { readFileSync } from "node:fs";
import { request as httpsRequest, type RequestOptions } from "node:https";
import { URL } from "node:url";

export interface VerbDescriptor {
  name: string;
  requiresPreview: boolean;
}

export interface ToolDescriptor {
  name: string;
  description: string;
  verbs: VerbDescriptor[];
  ceiling: string;
  inputSchema?: Record<string, unknown>;
}

export interface ToolListResult {
  tools: ToolDescriptor[];
}

export interface ToolCallRequest {
  adapter: string;
  verb: string;
  subject?: string;
  handle?: string;
  body?: string;
  fields?: Record<string, string>;
  // Current turn's session id, omitempty. The bridge's exfiltration gate
  // buckets calls by turn; a request without it falls into one shared
  // always-suspicious bucket.
  turnKey?: string;
}

export interface PreviewSummary {
  headline: string;
  lines?: string[];
  confirm?: string;
}

export interface CallError {
  code: string;
  message: string;
}

export interface ToolCallResult {
  ok: boolean;
  reached?: string;
  done?: boolean;
  handedOffTo?: string;
  detail?: string;
  preview?: PreviewSummary;
  error?: CallError;
  // Id of the approval gate raised for this call, omitempty; set together
  // with error.code === "approval_required" so the owner's answer on the
  // launcher can be correlated back to this call.
  gateId?: string;
}

export interface BridgeClientConfig {
  // Base URL of the bridge, e.g. https://127.0.0.1:9443. Loopback only.
  bridgeUrl: string;
  // Path to the bearer token file minted by the phone runtime.
  tokenPath: string;
  // Path to the runtime's exported certificate PEM. The client refuses any
  // peer whose certificate is not byte-identical to this one — never
  // rejectUnauthorized:false, never NODE_TLS_REJECT_UNAUTHORIZED.
  certPath: string;
}

export interface BridgeClient {
  listTools(): Promise<ToolListResult>;
  // Resolves with the ToolCallResult envelope on every HTTP status the
  // bridge produces (it sends the envelope even on errors); rejects only on
  // transport failures, bad JSON, or an unpinned peer certificate.
  callTool(request: ToolCallRequest): Promise<ToolCallResult>;
}

// rawRequest issues one HTTPS call against the bridge, pinning the peer
// certificate to the exact bytes at certPath. The token and cert are read
// fresh from disk on every call: the phone runtime re-mints both whenever it
// restarts, so a long-lived client must not cache stale credentials.
function rawRequest(
  config: BridgeClientConfig,
  method: string,
  urlPath: string,
  body?: string,
): Promise<{ status: number; body: string }> {
  const token = readFileSync(config.tokenPath, "utf8").trim();
  const certPem = readFileSync(config.certPath, "utf8");
  const target = new URL(urlPath, config.bridgeUrl);

  const headers: Record<string, string> = {
    authorization: `Bearer ${token}`,
  };
  if (body !== undefined) {
    headers["content-type"] = "application/json";
    headers["content-length"] = String(Buffer.byteLength(body));
  }

  // The runtime's certificate is self-signed and CN-only, with no SAN, so
  // Node's default hostname verification would reject it outright. `ca`
  // pins that exact self-signed certificate as the sole trust anchor (any
  // other peer certificate fails chain validation and the request errors
  // out); `checkServerIdentity` is a no-op because the pinned certificate
  // bytes *are* the identity check here — same model as the Go client's
  // TLS config in runtime.go:765-790. This is not rejectUnauthorized:false
  // and does not touch NODE_TLS_REJECT_UNAUTHORIZED.
  const options: RequestOptions = {
    method,
    hostname: target.hostname,
    port: target.port,
    path: `${target.pathname}${target.search}`,
    headers,
    ca: [certPem],
    checkServerIdentity: () => undefined,
  };

  return new Promise((resolve, reject) => {
    const req = httpsRequest(options, (res) => {
      let data = "";
      res.setEncoding("utf8");
      res.on("data", (chunk) => {
        data += chunk;
      });
      res.on("end", () => {
        resolve({ status: res.statusCode ?? 0, body: data });
      });
      res.on("error", reject);
    });
    req.on("error", reject);
    if (body !== undefined) {
      req.write(body);
    }
    req.end();
  });
}

export function createBridgeClient(config: BridgeClientConfig): BridgeClient {
  return {
    async listTools(): Promise<ToolListResult> {
      const { status, body } = await rawRequest(config, "GET", "/v1/agent-tools/list");
      if (status < 200 || status >= 300) {
        throw new Error(`agent bridge listTools failed with status ${status}: ${body}`);
      }
      return JSON.parse(body) as ToolListResult;
    },

    async callTool(request: ToolCallRequest): Promise<ToolCallResult> {
      const { body } = await rawRequest(
        config,
        "POST",
        "/v1/agent-tools/call",
        JSON.stringify(request),
      );
      // The bridge sends the ToolCallResult envelope on error statuses too,
      // so any status is decoded the same way; only transport/JSON failures
      // reject.
      return JSON.parse(body) as ToolCallResult;
    },
  };
}
