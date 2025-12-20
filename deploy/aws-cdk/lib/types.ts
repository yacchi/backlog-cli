/**
 * Backlog OAuth アプリケーション設定
 */
export interface BacklogAppConfig {
  clientId: string;
  clientSecret: string;
}

/**
 * 監査ログ設定
 */
export interface AuditConfig {
  enabled: boolean;
}

/**
 * インライン設定（設定値を直接指定）
 */
export interface InlineConfig {
  source: "inline";

  /** Cookie/JWT 署名用シークレット（32文字以上） */
  cookieSecret: string;

  /** Backlog アプリケーション設定 */
  backlog: {
    jp?: BacklogAppConfig;
    com?: BacklogAppConfig;
  };

  /** 許可するスペース名（空配列 = 全て許可） */
  allowedSpaces?: string[];

  /** 許可するプロジェクトキー（空配列 = 全て許可） */
  allowedProjects?: string[];

  /** 許可するホストパターン（セミコロン区切り、ワイルドカード対応）
   * base_url未設定時、Hostヘッダーからコールバック URL を構築する際の検証に使用
   * 例: "*.lambda-url.*.on.aws;*.run.app"
   */
  allowedHostPatterns?: string;

  /** 監査ログ設定 */
  audit?: AuditConfig;
}

/**
 * Parameter Store 参照設定
 */
export interface ParameterStoreConfig {
  source: "parameter-store";

  /** SSM Parameter Store のパラメーター名 */
  parameterName: string;

  /** パラメーターをデプロイ時に作成するか */
  createParameter?: boolean;

  /** createParameter=true の場合、格納する値 */
  parameterValue?: ParameterStoreValue;
}

/**
 * Parameter Store に格納する JSON の型
 */
export interface ParameterStoreValue {
  cookieSecret: string;
  backlog: {
    jp?: BacklogAppConfig;
    com?: BacklogAppConfig;
  };
  allowedSpaces?: string[];
  allowedProjects?: string[];
  allowedHostPatterns?: string;
  audit?: AuditConfig;
}

/**
 * リレーサーバー設定
 */
export type RelayConfig = InlineConfig | ParameterStoreConfig;

/**
 * 設定がインライン設定かどうかを判定
 */
export function isInlineConfig(config: RelayConfig): config is InlineConfig {
  return config.source === "inline";
}

/**
 * 設定が Parameter Store 参照かどうかを判定
 */
export function isParameterStoreConfig(
  config: RelayConfig,
): config is ParameterStoreConfig {
  return config.source === "parameter-store";
}
