import { HttpErrorResponse, HttpInterceptorFn } from '@angular/common/http';
import { catchError, throwError } from 'rxjs';

import { ApiError } from './api-error';

/**
 * Turns every failure into an {@link ApiError} before it reaches a caller.
 *
 * Without this, each component would branch on status codes and dig into
 * `error.error.message` itself, and the shape the backend guarantees would be
 * re-learned in a dozen places. One translation here means a facade catches a
 * typed error with a `code` it can switch on.
 */
export const errorInterceptor: HttpInterceptorFn = (req, next) =>
  next(req).pipe(
    catchError((error: unknown) =>
      throwError(() => (error instanceof HttpErrorResponse ? ApiError.fromHttp(error) : error)),
    ),
  );

/** The header the gateway and both services use to correlate a request. */
export const REQUEST_ID_HEADER = 'X-Request-ID';

/**
 * Attaches a correlation id to every request.
 *
 * The gateway generates one when a client does not supply it, so this is not
 * required — but supplying it means the id is known *before* the response
 * arrives. When a request fails or hangs, that id can be quoted against the
 * service logs, which is the difference between finding the failure and
 * guessing at it.
 */
export const correlationIdInterceptor: HttpInterceptorFn = (req, next) =>
  next(req.clone({ setHeaders: { [REQUEST_ID_HEADER]: newRequestId() } }));

function newRequestId(): string {
  // Available in browsers and in Node 24, but only over https or localhost in
  // the browser — so a plain-http staging host would throw without the guard.
  if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') {
    return crypto.randomUUID().replace(/-/g, '');
  }
  return Math.random().toString(16).slice(2) + Date.now().toString(16);
}
