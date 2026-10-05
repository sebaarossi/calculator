import type { Decimal } from '../api/calculatorApi';
import { MAX_DIGITS } from './reducer';

/** An optional sign, then digits with at most one decimal point. */
const PLAIN_DECIMAL = /^([+-]?)(\d*)(?:\.(\d*))?$/;

/**
 * Validates free-form text, such as a pasted value, as a number the keypad
 * could have typed, and returns it in the keypad's own format. Returns null if
 * the text is not acceptable.
 *
 * It is deliberately strict. Thousands separators, decimal commas, currency
 * symbols and exponents are rejected rather than interpreted: "1,234" means
 * 1234 in some countries and 1.234 in others, and guessing wrong is off by a
 * factor of a thousand.
 */
export function parseDecimalInput(text: string): Decimal | null {
  const match = PLAIN_DECIMAL.exec(text.trim());
  if (match === null) {
    return null;
  }
  const [, sign = '', rawInteger = '', rawFraction = ''] = match;
  if (rawInteger === '' && rawFraction === '') {
    return null; // "", "-", "." and the like
  }

  const integer = rawInteger.replace(/^0+(?=\d)/, '') || '0';
  if (integer.length + rawFraction.length > MAX_DIGITS) {
    return null;
  }

  const magnitude = rawFraction === '' ? integer : `${integer}.${rawFraction}`;
  const isZero = /^[0.]*$/.test(magnitude);
  return sign === '-' && !isZero ? `-${magnitude}` : magnitude;
}
