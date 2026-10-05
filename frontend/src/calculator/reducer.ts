import type { Decimal } from '../api/calculatorApi';

/** Operations that sit between two operands on the keypad. */
export type InfixOperation = 'add' | 'subtract' | 'multiply' | 'divide' | 'power';

/** Maximum digits in a typed operand: the precision the backend calculates with. */
export const MAX_DIGITS = 34;

export interface CalculatorState {
  /** Text on the display: the operand being typed, or the last result. */
  entry: string;
  /** Left operand of the pending operation. */
  accumulator: Decimal | null;
  pendingOperation: InfixOperation | null;
  /** An operator was just chosen and the right operand has not been provided yet. */
  awaitingOperand: boolean;
  /** The next digit starts a new entry instead of appending to the current one. */
  overwrite: boolean;
  error: string | null;
}

export type CalculatorAction =
  | { type: 'digit'; digit: string }
  | { type: 'decimal' }
  | { type: 'backspace' }
  | { type: 'toggleSign' }
  | { type: 'clear' }
  | { type: 'operatorChosen'; operation: InfixOperation }
  /** The pending operation was evaluated; optionally chains into the next one. */
  | { type: 'resolved'; value: Decimal; nextOperation: InfixOperation | null }
  /** The current entry was transformed in place (square root, percentage). */
  | { type: 'entryReplaced'; value: Decimal }
  | { type: 'failed'; message: string };

export const initialState: CalculatorState = {
  entry: '0',
  accumulator: null,
  pendingOperation: null,
  awaitingOperand: false,
  overwrite: false,
  error: null,
};

/**
 * Pure state machine for the keypad. It never performs arithmetic and never
 * converts an operand to a JavaScript number: operands are text, and results
 * arrive from the backend through the `resolved` and `entryReplaced` actions.
 */
export function calculatorReducer(current: CalculatorState, action: CalculatorAction): CalculatorState {
  // Any key press after an error starts over from a clean slate.
  const state = current.error === null ? current : initialState;

  switch (action.type) {
    case 'digit': {
      if (state.overwrite) {
        return { ...state, entry: action.digit, overwrite: false, awaitingOperand: false };
      }
      if (state.entry === '0') {
        return { ...state, entry: action.digit };
      }
      if (state.entry === '-0') {
        return { ...state, entry: `-${action.digit}` };
      }
      if (countDigits(state.entry) >= MAX_DIGITS) {
        return state;
      }
      return { ...state, entry: state.entry + action.digit };
    }

    case 'decimal': {
      if (state.overwrite) {
        return { ...state, entry: '0.', overwrite: false, awaitingOperand: false };
      }
      if (state.entry.includes('.') || countDigits(state.entry) >= MAX_DIGITS) {
        return state;
      }
      return { ...state, entry: `${state.entry}.` };
    }

    case 'backspace': {
      // A result is not editable digit by digit.
      if (state.overwrite) {
        return state;
      }
      const shortened = state.entry.slice(0, -1);
      return { ...state, entry: shortened === '' || shortened === '-' ? '0' : shortened };
    }

    case 'toggleSign': {
      // Zero has no sign, but "0." may be the start of a negative decimal.
      if (state.entry === '0') {
        return state;
      }
      const entry = state.entry.startsWith('-') ? state.entry.slice(1) : `-${state.entry}`;
      return { ...state, entry };
    }

    case 'clear':
      return initialState;

    case 'operatorChosen': {
      // Pressing a second operator before typing an operand just swaps it.
      if (state.pendingOperation !== null && state.awaitingOperand) {
        return { ...state, pendingOperation: action.operation };
      }
      return {
        ...state,
        accumulator: toOperand(state.entry),
        pendingOperation: action.operation,
        awaitingOperand: true,
        overwrite: true,
      };
    }

    case 'resolved': {
      const chaining = action.nextOperation !== null;
      return {
        entry: action.value,
        accumulator: chaining ? action.value : null,
        pendingOperation: action.nextOperation,
        awaitingOperand: chaining,
        overwrite: true,
        error: null,
      };
    }

    case 'entryReplaced':
      return { ...state, entry: action.value, awaitingOperand: false, overwrite: true };

    case 'failed':
      return { ...initialState, error: action.message };
  }
}

/** Turns what is on the display into an operand, dropping an unfinished decimal point ("5." → "5"). */
export function toOperand(entry: string): Decimal {
  return entry.endsWith('.') ? entry.slice(0, -1) : entry;
}

function countDigits(entry: string): number {
  return entry.replace(/[^0-9]/g, '').length;
}
