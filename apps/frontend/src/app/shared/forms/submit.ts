import { ApiError } from '../../core/api/api-error';

/**
 * Turning a rejected save into something to show the user.
 *
 * The API's messages are written for people — "from_team_id does not match the
 * player's current club", not a constraint name — and it never leaks driver or
 * SQL detail into them, so passing one straight through is both safe and more
 * useful than any generic sentence a form could substitute.
 *
 * Two codes are worth rewording, because the API's phrasing is correct but
 * says nothing about what to do next.
 */
export function messageFor(error: unknown, fallback = 'Could not save. Please try again.'): string {
  if (!(error instanceof ApiError)) {
    return error instanceof Error ? error.message : fallback;
  }

  switch (error.code) {
    case 'forbidden':
      // Reached when a permission check here disagreed with the server's —
      // most often a grant revoked since sign-in. The API is the authority.
      return 'You do not have permission to change this. If that is unexpected, sign in again.';
    case 'unauthorized':
      return 'Your session has expired. Sign in and try again.';
    default:
      return error.message || fallback;
  }
}

/**
 * The correlation id, on failures where it helps.
 *
 * Only for internal errors: quoting a reference for "name is required" is
 * noise, but for a 500 it is the one thing that makes the failure findable in
 * the service logs.
 */
export function requestIdFor(error: unknown): string | null {
  if (!(error instanceof ApiError)) return null;
  return error.code === 'internal' ? (error.requestId ?? null) : null;
}
