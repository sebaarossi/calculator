import { useCallback, useEffect, useReducer, useState } from 'react';
import {
  calculatorApi,
  type Calculation,
  type CalculationRequest,
  type CalculatorApi,
  type Decimal,
} from '../api/calculatorApi';
import { calculatorReducer, initialState, toOperand, type CalculatorAction, type InfixOperation } from './reducer';
import { parseDecimalInput } from './validation';

/** How many calculations the tape shows; older ones scroll off. */
export const MAX_TAPE_LINES = 5;

/** Something the user did: a key on the keypad or keyboard, a tape line, a paste. */
export type Key =
  | { type: 'digit'; digit: string }
  | { type: 'decimal' }
  | { type: 'backspace' }
  | { type: 'toggleSign' }
  | { type: 'clear' }
  | { type: 'operator'; operation: InfixOperation }
  | { type: 'equals' }
  | { type: 'squareRoot' }
  | { type: 'percent' }
  /** Bring a result from the tape back as the current entry. */
  | { type: 'recall'; value: Decimal }
  | { type: 'paste'; text: string };

/**
 * Connects the keypad state machine to the backend: synchronous key presses go
 * straight to the reducer, and anything that needs arithmetic is sent to the
 * API and fed back as an action.
 *
 * Key presses are queued and handled strictly one at a time, each seeing the
 * result of the one before it. A key pressed while a calculation is in flight
 * therefore waits for the result instead of being lost or applied to stale
 * state, however fast the user types and however slow the network is.
 */
export function useCalculator(api: CalculatorApi = calculatorApi) {
  const [state, dispatch] = useReducer(calculatorReducer, initialState);
  const [isCalculating, setIsCalculating] = useState(false);
  const [queue, setQueue] = useState<Key[]>([]);
  /** The latest calculations recorded by the backend, oldest first. */
  const [tape, setTape] = useState<Calculation[]>([]);

  // The history lives on the server, so the tape survives a page reload.
  useEffect(() => {
    api
      .listRecent(MAX_TAPE_LINES)
      .then((recent) => {
        // Keep anything calculated while the history was still loading.
        setTape((lines) => (lines.length > 0 || recent.length === 0 ? lines : [...recent].reverse()));
      })
      .catch(() => {
        // The tape is a convenience: without it the calculator still works.
      });
  }, [api]);

  const request = async (calculation: CalculationRequest, onResult: (value: Decimal) => CalculatorAction) => {
    setIsCalculating(true);
    try {
      const recorded = await api.calculate(calculation);
      dispatch(onResult(recorded.result));
      setTape((lines) => [...lines, recorded].slice(-MAX_TAPE_LINES));
    } catch (error) {
      dispatch({ type: 'failed', message: error instanceof Error ? error.message : 'Something went wrong' });
      // Keys typed after the failed one assumed it would succeed.
      setQueue([]);
    } finally {
      setIsCalculating(false);
    }
  };

  const handle = (key: Key) => {
    // After an error the reducer starts over, so pending work must be ignored too.
    const { entry, accumulator, pendingOperation, awaitingOperand } = state.error === null ? state : initialState;
    const operand = toOperand(entry);

    /** Evaluates the pending operation, then optionally sets up the next one. */
    const resolvePending = (operation: InfixOperation, a: Decimal, nextOperation: InfixOperation | null) =>
      request({ operation, a, b: operand }, (value) => ({ type: 'resolved', value, nextOperation }));

    switch (key.type) {
      case 'operator':
        if (pendingOperation !== null && accumulator !== null && !awaitingOperand) {
          void resolvePending(pendingOperation, accumulator, key.operation);
        } else {
          dispatch({ type: 'operatorChosen', operation: key.operation });
        }
        break;

      case 'equals':
        if (pendingOperation !== null && accumulator !== null) {
          void resolvePending(pendingOperation, accumulator, null);
        }
        break;

      case 'squareRoot':
        void request({ operation: 'sqrt', a: operand }, (value) => ({ type: 'entryReplaced', value }));
        break;

      // Pocket-calculator semantics: within an operation the entry becomes that
      // percentage of the left operand (200 + 10 % → 200 + 20); on its own it
      // is a percentage of one (50 % → 0.5).
      case 'percent': {
        const base = pendingOperation !== null && accumulator !== null ? accumulator : '1';
        void request({ operation: 'percentage', a: operand, b: base }, (value) => ({ type: 'entryReplaced', value }));
        break;
      }

      case 'recall':
        dispatch({ type: 'entryReplaced', value: key.value });
        break;

      case 'paste': {
        const value = parseDecimalInput(key.text);
        dispatch(
          value === null
            ? { type: 'failed', message: 'Cannot paste that: it is not a plain number like 1234.56' }
            : { type: 'entryReplaced', value },
        );
        break;
      }

      default:
        dispatch(key);
    }
  };

  // Take one key per render, so the next one sees the state this one produced.
  useEffect(() => {
    const next = queue[0];
    if (next === undefined || isCalculating) {
      return;
    }
    setQueue((keys) => keys.slice(1));
    handle(next);
  });

  const press = useCallback((key: Key) => setQueue((keys) => [...keys, key]), []);

  return { state, isCalculating, tape, press };
}
