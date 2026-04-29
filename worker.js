const DEFAULT_UPSTREAM_ORIGIN = "https://chat.z.ai";

const HOP_BY_HOP_HEADERS = [
  "connection",
  "keep-alive",
  "proxy-authenticate",
  "proxy-authorization",
  "te",
  "trailer",
  "transfer-encoding",
  "upgrade",
];

const CF_REQUEST_HEADERS = [
  "cf-connecting-ip",
  "cf-ipcountry",
  "cf-ray",
  "cf-visitor",
  "cdn-loop",
  "x-forwarded-for",
  "x-real-ip",
];

function cleanRequestHeaders(requestHeaders) {
  const headers = new Headers(requestHeaders);

  for (const name of HOP_BY_HOP_HEADERS) {
    headers.delete(name);
  }
  for (const name of CF_REQUEST_HEADERS) {
    headers.delete(name);
  }

  headers.delete("host");
  return headers;
}

function cleanResponseHeaders(responseHeaders) {
  const headers = new Headers(responseHeaders);

  for (const name of HOP_BY_HOP_HEADERS) {
    headers.delete(name);
  }

  return headers;
}

export default {
  async fetch(request, env) {
    const upstreamOrigin = env.UPSTREAM_ORIGIN || DEFAULT_UPSTREAM_ORIGIN;
    const incomingURL = new URL(request.url);
    const upstreamURL = new URL(upstreamOrigin);

    upstreamURL.pathname = incomingURL.pathname;
    upstreamURL.search = incomingURL.search;

    const upstreamRequest = new Request(upstreamURL.toString(), {
      method: request.method,
      headers: cleanRequestHeaders(request.headers),
      body: request.body,
      redirect: "manual",
    });

    const upstreamResponse = await fetch(upstreamRequest);

    return new Response(upstreamResponse.body, {
      status: upstreamResponse.status,
      statusText: upstreamResponse.statusText,
      headers: cleanResponseHeaders(upstreamResponse.headers),
    });
  },
};
