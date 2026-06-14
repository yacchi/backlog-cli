// CloudFront Functions Runtime 2.0
// Viewer Request:
// 1. Copy Host → x-original-host (X-Forwarded-Host is reserved)
// 2. Copy Authorization → x-mcp-authorization
//    OAC overwrites Authorization with SigV4; this preserves the Bearer token.
// 3. Copy User-Agent → x-original-user-agent
//    OAC replaces User-Agent with "Amazon CloudFront"; this preserves the viewer's UA.
// 4. Copy viewer IP → x-viewer-ip (event.viewer.ip is the true client IP)
//    X-Forwarded-For at origin contains the CloudFront edge IP, not the viewer's.

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
  if (event.viewer && event.viewer.ip) {
    request.headers['x-viewer-ip'] = { value: event.viewer.ip };
  }
  return request;
}
