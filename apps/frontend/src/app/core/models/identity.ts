/** Wire types for identity-svc. */

export type Role = 'admin' | 'editor' | 'user';

export interface User {
  id: string;
  username: string;
  email: string;
  role: Role;
  created_at: string;
  // password_hash is never serialised by the API.
}

/** A linked external sign-in provider. */
export interface Identity {
  id: string;
  user_id: string;
  provider: 'google' | 'facebook';
  email?: string;
  created_at: string;
  last_login_at?: string;
}

/**
 * What login, refresh and the OAuth code exchange all return.
 *
 * The access token lives 15 minutes; `expires_in` is seconds, so a client can
 * schedule a refresh without parsing the token.
 */
export interface TokenPair {
  access_token: string;
  refresh_token: string;
  token_type: 'Bearer';
  expires_in: number;
}

/**
 * The claims carried inside the access token.
 *
 * Read for display and for hiding controls the user cannot use. Never trusted
 * as a security boundary — the API re-checks every rule independently, and a
 * token is client-side data.
 */
export interface AccessTokenClaims {
  user_id: string;
  role: Role;
  sub: string;
  exp: number;
  iat: number;
  jti: string;
}
