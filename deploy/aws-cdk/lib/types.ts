/**
 * JWK (JSON Web Key)
 */
export interface JWK {
  kty: string;
  kid?: string;
  use?: string;
  alg?: string;
  // OKP (Ed25519, X25519, etc.)
  crv?: string;
  x?: string;
  d?: string;
  // RSA
  n?: string;
  e?: string;
  // EC
  y?: string;
  // その他のパラメータ
  [key: string]: unknown;
}

/**
 * JWKS (JSON Web Key Set)
 */
export interface JWKS {
  keys: JWK[];
}

/**
 * Backlog OAuth アプリケーション設定
 */
export interface BacklogAppConfig {
  client_id: string;
  client_secret: string;
  domain?: string;
}

/**
 * テナント設定
 */
export interface TenantConfig {
  /** 許可するドメイン (spaceid.backlog.jp) */
  allowed_domain: string;

  /**
   * 秘密鍵を含む JWK セット
   * - JWKS オブジェクト: デプロイ時に JSON 文字列化
   */
  jwks: JWKS;

  /** 署名に使う kid (カンマ区切り) */
  active_keys: string;

  /** info の TTL (秒) */
  info_ttl?: number;

  /** ポータル用パスフレーズの bcrypt ハッシュ */
  passphrase_hash?: string;
}

/**
 * Parameter Store 参照設定
 */
export interface ParameterStoreConfig {
  /** SSM Parameter Store のパラメーター名 */
  parameterName: string;

  /** Parameter Store に格納する値 */
  parameterValue?: ParameterStoreValue;
}

/**
 * CloudFront キャッシュ設定
 */
export interface CloudFrontCacheConfig {
  /**
   * 静的アセット (/assets/*) のキャッシュ TTL (秒)
   * @default 31536000 (365日)
   */
  assetsMaxAge?: number;

  /**
   * API (certs/info) のデフォルト TTL (秒)
   * オリジンが Cache-Control を返さない場合に使用
   * @default 3600 (1時間)
   */
  apiDefaultTtl?: number;

  /**
   * API (certs/info) の最大 TTL (秒)
   * オリジンの max-age がこれを超えても、この値でキャップされる
   * @default 86400 (24時間)
   */
  apiMaxTtl?: number;

  /**
   * API (certs/info) の最小 TTL (秒)
   * Lambda への負荷軽減のため、最低限キャッシュする時間
   * 鍵ローテーションは移行期間を設けて行うため、数分のキャッシュは問題ない
   * @default 300 (5分)
   */
  apiMinTtl?: number;
}

/**
 * CloudFront 設定
 */
export interface CloudFrontConfig {
  /**
   * CloudFront を有効化
   * true の場合、CloudFront 経由でアクセス（キャッシュ有効）
   */
  enabled: boolean;

  /**
   * カスタムドメイン名 (例: relay.example.com)
   * 未指定の場合は CloudFront デフォルトドメイン（*.cloudfront.net）を使用
   */
  domainName?: string;

  /**
   * ACM 証明書の ARN（us-east-1 リージョン）
   * domainName を指定する場合は必須
   */
  certificateArn?: string;

  /**
   * Route 53 ホストゾーン ID（オプション）
   * 指定すると DNS レコードを自動作成
   */
  hostedZoneId?: string;

  /**
   * キャッシュ設定
   * 未指定の場合は推奨デフォルト値を使用
   */
  cache?: CloudFrontCacheConfig;
}

export interface AccessControlConfig {
  allowed_spaces?: string[];
  allowed_projects?: string[];
  allowed_cidrs?: string[];
}

export interface AuditConfig {
  enabled: boolean;
  output?: string;
  file_path?: string;
  webhook_url?: string;
  webhook_timeout?: number;
}

/**
 * certs/info エンドポイントの HTTP キャッシュ設定
 */
export interface ServerCacheConfig {
  /**
   * certs エンドポイントの Cache-Control max-age (秒)
   * 公開鍵は頻繁に変わらないため長めに設定可能
   * @default 86400 (24時間)
   */
  certs_ttl?: number;

  /**
   * info エンドポイントの Cache-Control max-age (秒)
   * 署名付き情報のため、certsより短めを推奨
   * @default 3600 (1時間)
   */
  info_ttl?: number;
}

export interface RelayServerConfig {
  backlog?: Record<string, BacklogAppConfig>;
  allowed_host_patterns?: string;
  access_control?: AccessControlConfig;
  audit?: AuditConfig;
  tenants?: Record<string, TenantConfig>;
  /** certs/info エンドポイントの HTTP キャッシュ設定 */
  cache?: ServerCacheConfig;
}

/**
 * Parameter Store に格納する JSON の型
 */
export interface ParameterStoreValue {
  server?: RelayServerConfig;
}

/**
 * リレーサーバー設定
 */
export interface RelayConfig extends ParameterStoreConfig {
  /** CloudFront 設定（オプション） */
  cloudFront?: CloudFrontConfig;
}
