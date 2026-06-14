// CloudFront Functions Runtime 2.0
// Viewer Request:
// 1. Copy Host → x-original-host (X-Forwarded-Host is reserved)
// 2. Copy Authorization → x-mcp-authorization
//    OAC overwrites Authorization with SigV4; this preserves the Bearer token.
// 3. Copy User-Agent → x-original-user-agent
//    OAC replaces User-Agent with "Amazon CloudFront"; this preserves the viewer's UA.

function handler(event) {
  var request = event.request;
  var host = request.headers.host ? request.headers.host.value : '';
  request.headers['x-original-host'] = { value: host };
  if (request.headers.authorization) {
    request.headers['x-mcp-authorization'] = { value: request.headers.authorization.value };
  }
  if (request.headers['user-agent']) {
    request.headers['x-original-user-agent'] = { value: request.headers['user-agent'].value };
  }
  return request;
}
