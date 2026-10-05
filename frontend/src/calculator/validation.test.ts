import { MAX_DIGITS } from './reducer';
import { parseDecimalInput } from './validation';

describe('parseDecimalInput', () => {
  it.each([
    ['42', '42'],
    ['1234.56', '1234.56'],
    ['-0.5', '-0.5'],
    ['+7', '7'],
    ['  12.5\n', '12.5'],
    ['.5', '0.5'],
    ['-.5', '-0.5'],
    ['5.', '5'],
    ['007', '7'],
    ['000.10', '0.10'],
    ['0', '0'],
    ['-0', '0'],
    ['-0.00', '0.00'],
    ['9'.repeat(MAX_DIGITS), '9'.repeat(MAX_DIGITS)],
  ])('accepts %j as %s', (text, expected) => {
    expect(parseDecimalInput(text)).toBe(expected);
  });

  it.each([
    ['nothing', ''],
    ['only spaces', '   '],
    ['a lone sign', '-'],
    ['a lone decimal point', '.'],
    ['words', 'twelve'],
    ['a thousands separator', '1,234.56'],
    ['a decimal comma', '1,5'],
    ['a currency symbol', '$100'],
    ['a percent sign', '15%'],
    ['an exponent', '1e5'],
    ['two decimal points', '1.2.3'],
    ['two signs', '--5'],
    ['an expression', '2+2'],
    ['inner spaces', '1 000'],
    ['hexadecimal', '0x10'],
    ['non-ASCII digits', '１２'],
    ['Infinity', 'Infinity'],
    ['NaN', 'NaN'],
    ['markup', '<b>5</b>'],
    ['too many digits', '9'.repeat(MAX_DIGITS + 1)],
  ])('rejects %s', (_, text) => {
    expect(parseDecimalInput(text)).toBeNull();
  });
});
