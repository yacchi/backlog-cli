// CloudFront Functions Runtime 2.0
// Viewer Request:
// 1. Copy Host → x-original-host (X-Forwarded-Host is reserved)
// 2. Copy Authorization → x-mcp-authorization
//    OAC overwrites Authorization with SigV4; this preserves the Bearer token.

function handler(event) {
  const request = event.request;
  const host = request.headers.host ? request.headers.host.value : '';
  request.headers['x-original-host'] = { value: host };
  if (request.headers.authorization) {
    request.headers['x-mcp-authorization'] = { value: request.headers.authorization.value };
  }
  return request;
}
