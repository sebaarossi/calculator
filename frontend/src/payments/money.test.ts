import { formatDate, formatMoney, todayISO, validateAmount } from './money';

describe('validateAmount', () => {
  it.each([
    ['100', '100'],
    ['99.99', '99.99'],
    ['99.9', '99.9'],
    ['0.01', '0.01'],
    ['  250.00 ', '250.00'],
    ['1000000', '1000000'],
    ['1000000.00', '1000000.00'],
  ])('accepts %j', (text, amount) => {
    expect(validateAmount(text)).toEqual({ ok: true, amount });
  });

  it.each([
    ['', 'Enter the purchase amount'],
    ['   ', 'Enter the purchase amount'],
    ['0', 'Enter an amount greater than zero'],
    ['0.00', 'Enter an amount greater than zero'],
    ['10.005', 'Use at most two decimals'],
    ['1,000.00', 'Use digits only, such as 1234.56'],
    ['1.000,00', 'Use digits only, such as 1234.56'],
    ['$100', 'Use digits only, such as 1234.56'],
    ['-5', 'Use digits only, such as 1234.56'],
    ['.5', 'Use digits only, such as 1234.56'],
    ['5.', 'Use digits only, such as 1234.56'],
    ['1e3', 'Use digits only, such as 1234.56'],
    ['ten', 'Use digits only, such as 1234.56'],
    ['１００', 'Use digits only, such as 1234.56'],
    ['1000000.01', 'The maximum is $1,000,000.00'],
    ['99999999', 'The maximum is $1,000,000.00'],
  ])('rejects %j: %s', (text, error) => {
    expect(validateAmount(text)).toEqual({ ok: false, error });
  });
});

describe('formatMoney', () => {
  it.each([
    ['25.02', '$25.02'],
    ['0.01', '$0.01'],
    ['999.00', '$999.00'],
    ['1000.00', '$1,000.00'],
    ['250000.00', '$250,000.00'],
    ['1000000.00', '$1,000,000.00'],
  ])('formats %s as %s', (amount, expected) => {
    expect(formatMoney(amount)).toBe(expected);
  });
});

describe('formatDate', () => {
  it('writes the date out without shifting the day', () => {
    expect(formatDate('2026-10-19')).toBe('Mon, Oct 19, 2026');
    expect(formatDate('2027-01-01')).toBe('Fri, Jan 1, 2027');
  });
});

describe('todayISO', () => {
  it('uses the local calendar date, zero-padded', () => {
    expect(todayISO(new Date(2026, 0, 5, 23, 59))).toBe('2026-01-05');
    expect(todayISO()).toMatch(/^\d{4}-\d{2}-\d{2}$/);
  });
});
