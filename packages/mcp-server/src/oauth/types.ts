export interface DcrRequest {
    redirect_uris: string[];
    client_name?: string;
    grant_types?: string[];
    response_types?: string[];
    token_endpoint_auth_method?: string;
}

export interface DcrResponse {
    client_id: string;
    client_name?: string;
    redirect_uris: string[];
    grant_types: string[];
    response_types: string[];
    token_endpoint_auth_method: string;
}

export interface TokenRequest {
    grant_type: string;
    code?: string;
    redirect_uri?: string;
    client_id?: string;
    code_verifier?: string;
    refresh_token?: string;
}

export interface TokenResponse {
    access_token: string;
    token_type: string;
    expires_in: number;
    refresh_token: string;
}

export interface TokenErrorResponse {
    error: string;
    error_description?: string;
}

export interface ClientIdPayload {
    redirect_uris: string[];
    client_name?: string;
    iat: number;
}

export interface SpaceRef {
    space: string;
}

export interface AuthorizeState {
    client_id: string;
    redirect_uri: string;
    code_challenge: string;
    code_challenge_method: string;
    state: string;
    space: string;
    requiredSpaces?: SpaceRef[];
    popup?: boolean;
}
