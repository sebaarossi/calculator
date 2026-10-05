/** The largest purchase the backend will split, in dollars. */
const MAX_DOLLARS = 1_000_000;

/** Dollars, then optionally a point and one or two digits of cents. */
const AMOUNT = /^(\d+)(?:\.(\d{1,2}))?$/;

export type AmountValidation = { ok: true; amount: string } | { ok: false; error: string };

/**
 * Validates a purchase amount typed by the user, so obvious mistakes are
 * explained immediately instead of after a round trip. The backend applies the
 * same rules again: it never trusts this check.
 *
 * Like the backend, it refuses to interpret or round: "1,000" and "10.005" are
 * errors, not 1000 and 10.01.
 */
export function validateAmount(text: string): AmountValidation {
  const trimmed = text.trim();
  if (trimmed === '') {
    return { ok: false, error: 'Enter the purchase amount' };
  }
  if (/^\d+\.\d{3,}$/.test(trimmed)) {
    return { ok: false, error: 'Use at most two decimals' };
  }
  const match = AMOUNT.exec(trimmed);
  if (match === null) {
    return { ok: false, error: 'Use digits only, such as 1234.56' };
  }

  // Safe as plain numbers: these are whole dollars and whole cents, not fractions.
  const dollars = Number(match[1]);
  const cents = Number((match[2] ?? '').padEnd(2, '0'));
  if (dollars === 0 && cents === 0) {
    return { ok: false, error: 'Enter an amount greater than zero' };
  }
  if (dollars > MAX_DOLLARS || (dollars === MAX_DOLLARS && cents > 0)) {
    return { ok: false, error: `The maximum is ${formatMoney(`${MAX_DOLLARS}.00`)}` };
  }
  return { ok: true, amount: trimmed };
}

/** Formats an amount from the backend for display: "250000.00" → "$250,000.00". */
export function formatMoney(amount: string): string {
  const [dollars = '0', cents = '00'] = amount.split('.');
  return `$${dollars.replace(/\B(?=(\d{3})+$)/g, ',')}.${cents}`;
}

const DATE_FORMAT = new Intl.DateTimeFormat('en-US', {
  weekday: 'short',
  month: 'short',
  day: 'numeric',
  year: 'numeric',
  timeZone: 'UTC',
});

/** Formats a calendar date from the backend: "2026-10-19" → "Mon, Oct 19, 2026". */
export function formatDate(isoDate: string): string {
  // Parsed and formatted in UTC, so the day never shifts with the viewer's time zone.
  return DATE_FORMAT.format(new Date(`${isoDate}T00:00:00Z`));
}

/** Today's date where the user is, as YYYY-MM-DD. */
export function todayISO(now = new Date()): string {
  const pad = (n: number) => String(n).padStart(2, '0');
  return `${now.getFullYear()}-${pad(now.getMonth() + 1)}-${pad(now.getDate())}`;
}
