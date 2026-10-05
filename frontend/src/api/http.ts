// Shared plumbing for talking to the backend: one place for timeouts, error
// handling and response checks, used by every API client.

/** An error the user can be shown: either reported by the API or a transport failure. */
export class ApiError extends Error {
  constructor(
    readonly code: string,
    message: string,
    readonly status?: number,
  ) {
    super(message);
    this.name = 'ApiError';
  }
}

const BASE_URL: string = import.meta.env.VITE_API_BASE_URL ?? '/api/v1';

/** How long to wait for the backend before giving up, so the UI never hangs. */
export const REQUEST_TIMEOUT_MS = 10_000;

/** Friendlier wording for the error codes the API is known to return. */
const MESSAGES: Record<string, string> = {
  division_by_zero: 'Cannot divide by zero',
  negative_square_root: 'Cannot take the square root of a negative number',
  undefined_result: 'Result is undefined',
  result_out_of_range: 'Result is too large',
  amount_out_of_range: 'That amount cannot be split into this plan',
};

/** Sends a JSON body with POST. */
export function post(path: string, body: unknown): Promise<unknown> {
  return send(path, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  });
}

/** Calls the API and returns the parsed body, turning every failure into an ApiError. */
export async function send(path: string, init?: RequestInit): Promise<unknown> {
  const controller = new AbortController();
  const timeout = setTimeout(() => controller.abort(), REQUEST_TIMEOUT_MS);

  let response: Response;
  let body: unknown;
  try {
    response = await fetch(BASE_URL + path, { ...init, signal: controller.signal });
    body = await response.json().catch(() => null);
  } catch {
    throw controller.signal.aborted
      ? new ApiError('timeout', 'The calculator service took too long to answer')
      : new ApiError('network_error', 'Cannot reach the calculator service');
  } finally {
    clearTimeout(timeout);
  }

  if (!response.ok) {
    const { code, message } = parseError(body, response.status);
    throw new ApiError(code, MESSAGES[code] ?? message, response.status);
  }
  return body;
}

export function invalidResponse(): ApiError {
  return new ApiError('invalid_response', 'Unexpected response from the calculator service');
}

function parseError(body: unknown, status: number): { code: string; message: string } {
  // A 5xx without the API's error shape comes from a proxy or gateway in
  // front of the service, which usually means the service itself is down.
  const fallback =
    status >= 500
      ? { code: 'service_unavailable', message: 'The calculator service is unavailable' }
      : { code: 'unknown_error', message: 'The calculator service returned an error' };
  if (!isRecord(body) || !isRecord(body.error)) {
    return fallback;
  }
  const { code, message } = body.error;
  return {
    code: typeof code === 'string' ? code : fallback.code,
    message: typeof message === 'string' ? message : fallback.message,
  };
}

export function includes(list: readonly string[], value: unknown): boolean {
  return typeof value === 'string' && list.includes(value);
}

export function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null;
}
