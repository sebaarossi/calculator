import { includes, invalidResponse, isRecord, post, send } from './http';

export { ApiError, REQUEST_TIMEOUT_MS } from './http';

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

export const calculatorApi: CalculatorApi = {
  async calculate(request) {
    const body = await post('/calculations', request);
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
