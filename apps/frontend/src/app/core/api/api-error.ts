import { HttpErrorResponse } from '@angular/common/http';

/**
 * The error codes the API returns. Every failure carries one, so the UI
 * branches on this rather than on status numbers scattered through components.
 */
export type ApiErrorCode =
  | 'invalid'
  | 'unauthorized'
  | 'forbidden'
  | 'not_found'
  | 'conflict'
  | 'rate_limited'
  | 'internal'
  | 'network';

/** The body shape the backend guarantees for every failed request. */
interface ApiErrorBody {
  error?: string;
  code?: string;
  request_id?: string;
}

/**
 * A failed request, normalised.
 *
 * The backend never leaks driver or SQL detail into `message`, so it is always
 * safe to show a user. `requestId` is worth surfacing on internal errors —
 * it is the same id in the service logs, so a user can quote it and it can be
 * found.
 */
export class ApiError extends Error {
  constructor(
    readonly code: ApiErrorCode,
    override readonly message: string,
    readonly status: number,
    readonly requestId?: string,
  ) {
    super(message);
    this.name = 'ApiError';
  }

  /** True when retrying the identical request could plausibly succeed. */
  get isRetryable(): boolean {
    return this.code === 'network' || this.code === 'internal' || this.code === 'rate_limited';
  }

  /** True when the user could fix this by changing their input. */
  get isUserFixable(): boolean {
    return this.code === 'invalid' || this.code === 'conflict';
  }

  static fromHttp(response: HttpErrorResponse): ApiError {
    // status 0 means the request never reached the server at all — offline,
    // DNS, or a CORS rejection. Reporting that as "internal server error"
    // would send someone looking at server logs that contain nothing.
    if (response.status === 0) {
      return new ApiError('network', 'Could not reach the server. Check your connection.', 0);
    }

    const body = (response.error ?? {}) as ApiErrorBody;
    const code = isApiErrorCode(body.code) ? body.code : codeForStatus(response.status);

    return new ApiError(
      code,
      body.error ?? defaultMessageFor(code),
      response.status,
      body.request_id,
    );
  }
}

function isApiErrorCode(value: unknown): value is ApiErrorCode {
  return (
    typeof value === 'string' &&
    ['invalid', 'unauthorized', 'forbidden', 'not_found', 'conflict', 'rate_limited', 'internal']
      .includes(value)
  );
}

/**
 * Fallback for a response that did not come from our API — a gateway timeout,
 * or a proxy error page.
 */
function codeForStatus(status: number): ApiErrorCode {
  switch (status) {
    case 400: return 'invalid';
    case 401: return 'unauthorized';
    case 403: return 'forbidden';
    case 404: return 'not_found';
    case 409: return 'conflict';
    case 429: return 'rate_limited';
    default: return 'internal';
  }
}

function defaultMessageFor(code: ApiErrorCode): string {
  switch (code) {
    case 'invalid': return 'That request was not valid.';
    case 'unauthorized': return 'Please sign in.';
    case 'forbidden': return 'You do not have permission to do that.';
    case 'not_found': return 'Not found.';
    case 'conflict': return 'That conflicts with something that already exists.';
    case 'rate_limited': return 'Too many attempts. Wait a moment and try again.';
    case 'network': return 'Could not reach the server.';
    default: return 'Something went wrong. Please try again.';
  }
}
