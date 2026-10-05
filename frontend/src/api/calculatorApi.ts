const BINARY_OPERATIONS = ['add', 'subtract', 'multiply', 'divide', 'power', 'percentage'] as const;
const UNARY_OPERATIONS = ['sqrt'] as const;

export type BinaryOperation = (typeof BINARY_OPERATIONS)[number];
export type UnaryOperation = (typeof UNARY_OPERATIONS)[number];

/**
 * A decimal number as text, such as "12.5". Operands and results stay strings
 * from the keypad to the backend and back: converting them to a JavaScript
 * number would reintroduce binary floating-point error.
 */
export type Decimal = string;

export type CalculationRequest =
  | { operation: BinaryOperation; a: Decimal; b: Decimal }
  | { operation: UnaryOperation; a: Decimal };

/** A calculation the backend performed and recorded in its history. */
export type Calculation = CalculationRequest & {
  id: number;
  result: Decimal;
  createdAt: string;
};

/** Everything the UI needs from the backend; lets tests inject a fake. */
export interface CalculatorApi {
  /** Performs a calculation, which the backend also adds to the history. */
  calculate(request: CalculationRequest): Promise<Calculation>;
  /** Returns the most recent calculations, newest first. */
  listRecent(limit: number): Promise<Calculation[]>;
}

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
};

export const calculatorApi: CalculatorApi = {
  async calculate(request) {
    const body = await send('/calculations', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(request),
    });
    if (!isCalculation(body)) {
      throw invalidResponse();
    }
    return body;
  },

  async listRecent(limit) {
    const body = await send(`/calculations?limit=${limit}`);
    if (!isRecord(body) || !Array.isArray(body.calculations) || !body.calculations.every(isCalculation)) {
      throw invalidResponse();
    }
    return body.calculations;
  },
};

/** Calls the API and returns the parsed body, turning every failure into an ApiError. */
async function send(path: string, init?: RequestInit): Promise<unknown> {
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

function invalidResponse(): ApiError {
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

/** Checks a calculation from the backend field by field: responses are untrusted input too. */
function isCalculation(value: unknown): value is Calculation {
  return (
    isRecord(value) &&
    typeof value.id === 'number' &&
    typeof value.a === 'string' &&
    typeof value.result === 'string' &&
    typeof value.createdAt === 'string' &&
    (includes(UNARY_OPERATIONS, value.operation) ||
      (includes(BINARY_OPERATIONS, value.operation) && typeof value.b === 'string'))
  );
}

function includes(list: readonly string[], value: unknown): boolean {
  return typeof value === 'string' && list.includes(value);
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null;
}
