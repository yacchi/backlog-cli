import type {
  CloudFrontRequestEvent,
  CloudFrontRequestHandler,
  CloudFrontRequestResult,
} from "aws-lambda";
import { createHash } from "crypto";

/**
 * Lambda@Edge: Origin Request
 *
 * POST/PUT リクエストのボディから SHA256 ハッシュを計算し、
 * x-amz-content-sha256 ヘッダーに追加（OAC 署名用）
 *
 * 注意:
 * - x-original-host ヘッダーは CloudFront Function (viewer-request) で設定済み
 * - この Lambda@Edge はコンテンツハッシュ計算のみを担当
 */
export const handler: CloudFrontRequestHandler = async (
  event: CloudFrontRequestEvent,
): Promise<CloudFrontRequestResult> => {
  const request = event.Records[0].cf.request;

  // ボディがある場合のみハッシュを計算
  if (request.body?.data) {
    // Base64 エンコードされたボディをデコード
    const bodyBuffer =
      request.body.encoding === "base64"
        ? Buffer.from(request.body.data, "base64")
        : Buffer.from(request.body.data, "utf-8");

    const hash = createHash("sha256").update(bodyBuffer).digest("hex");

    // x-amz-content-sha256 ヘッダーを追加
    request.headers["x-amz-content-sha256"] = [
      { key: "x-amz-content-sha256", value: hash },
    ];
  }

  return request;
};
