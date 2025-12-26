import type {
  CloudFrontRequestEvent,
  CloudFrontRequestHandler,
  CloudFrontRequestResult,
} from "aws-lambda";
import { createHash } from "crypto";

/**
 * Lambda@Edge: Origin Request
 *
 * 1. X-Forwarded-Host ヘッダーを追加（CloudFront ドメイン → Lambda へ転送）
 * 2. POST/PUT リクエストのボディから SHA256 ハッシュを計算し、
 *    x-amz-content-sha256 ヘッダーに追加（OAC 署名用）
 */
export const handler: CloudFrontRequestHandler = async (
  event: CloudFrontRequestEvent,
): Promise<CloudFrontRequestResult> => {
  const request = event.Records[0].cf.request;

  // X-Forwarded-Host ヘッダーを追加
  // CloudFront の Host ヘッダー（カスタムドメインまたは *.cloudfront.net）を転送
  const host = request.headers.host?.[0]?.value || "";
  if (host) {
    request.headers["x-forwarded-host"] = [
      { key: "X-Forwarded-Host", value: host },
    ];
  }

  // ボディがある場合のみハッシュを計算
  if (request.body?.data) {
    // Base64 エンコードされたボディをデコード
    const body =
      request.body.encoding === "base64"
        ? Buffer.from(request.body.data, "base64").toString("utf-8")
        : request.body.data;

    // SHA256 ハッシュを計算
    const hash = createHash("sha256").update(body).digest("hex");

    // x-amz-content-sha256 ヘッダーを追加
    request.headers["x-amz-content-sha256"] = [
      { key: "x-amz-content-sha256", value: hash },
    ];
  }

  return request;
};
