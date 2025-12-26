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

  /** 秘密鍵を含む JWK セット (JSON 文字列) */
  jwks: string;

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

export interface RelayServerConfig {
  backlog?: Record<string, BacklogAppConfig>;
  allowed_host_patterns?: string;
  access_control?: AccessControlConfig;
  audit?: AuditConfig;
  tenants?: Record<string, TenantConfig>;
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
