// CloudFront Functions Runtime 2.0
// Viewer Response: Restore WWW-Authenticate header
// Lambda Function URL remaps WWW-Authenticate to x-amzn-remapped-www-authenticate.
// MCP OAuth clients (RFC 9728) require the original header name.

function handler(event) {
  var response = event.response;
  var remapped = response.headers['x-amzn-remapped-www-authenticate'];
  if (remapped) {
    response.headers['www-authenticate'] = remapped;
    delete response.headers['x-amzn-remapped-www-authenticate'];
  }
  return response;
}
