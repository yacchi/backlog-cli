// CloudFront Functions Runtime 2.0
// Viewer Request: Copy Host header to x-original-host
// Note: X-Forwarded-Host is a reserved header and cannot be used

function handler(event) {
  const request = event.request;
  const host = request.headers.host ? request.headers.host.value : '';
  request.headers['x-original-host'] = { value: host };
  return request;
}
