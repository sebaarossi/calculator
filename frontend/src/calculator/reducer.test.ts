import {
  calculatorReducer,
  initialState,
  MAX_DIGITS,
  toOperand,
  type CalculatorAction,
  type CalculatorState,
} from './reducer';

/** Applies a sequence of actions starting from the initial state. */
function run(...actions: CalculatorAction[]): CalculatorState {
  return actions.reduce(calculatorReducer, initialState);
}

const digits = (text: string): CalculatorAction[] =>
  [...text].map((char) => (char === '.' ? { type: 'decimal' } : { type: 'digit', digit: char }));

describe('typing an operand', () => {
  it('replaces the initial zero', () => {
    expect(run(...digits('42')).entry).toBe('42');
  });

  it('does not accumulate leading zeros', () => {
    expect(run(...digits('007')).entry).toBe('7');
  });

  it('accepts a single decimal point', () => {
    expect(run(...digits('1.5.2')).entry).toBe('1.52');
  });

  it('starts a decimal with a leading zero', () => {
    expect(run({ type: 'decimal' }, ...digits('5')).entry).toBe('0.5');
  });

  it(`ignores digits beyond ${MAX_DIGITS}`, () => {
    const state = run(...digits('1'.repeat(MAX_DIGITS + 3)));
    expect(state.entry).toBe('1'.repeat(MAX_DIGITS));
    expect(calculatorReducer(state, { type: 'decimal' })).toBe(state);
  });
});

describe('backspace', () => {
  it('removes the last character', () => {
    expect(run(...digits('123'), { type: 'backspace' }).entry).toBe('12');
  });

  it('falls back to zero when the entry is emptied', () => {
    expect(run(...digits('7'), { type: 'backspace' }).entry).toBe('0');
    expect(run(...digits('7'), { type: 'toggleSign' }, { type: 'backspace' }).entry).toBe('0');
  });

  it('does not edit a result', () => {
    const state = run({ type: 'resolved', value: '123', nextOperation: null }, { type: 'backspace' });
    expect(state.entry).toBe('123');
  });
});

describe('toggleSign', () => {
  it('negates and restores the entry', () => {
    expect(run(...digits('5'), { type: 'toggleSign' }).entry).toBe('-5');
    expect(run(...digits('5'), { type: 'toggleSign' }, { type: 'toggleSign' }).entry).toBe('5');
  });

  it('leaves zero unsigned', () => {
    expect(run({ type: 'toggleSign' }).entry).toBe('0');
  });

  it('keeps the sign while typing a negative decimal', () => {
    expect(run({ type: 'decimal' }, { type: 'toggleSign' }, ...digits('5')).entry).toBe('-0.5');
    expect(run(...digits('0'), { type: 'decimal' }, { type: 'toggleSign' }, { type: 'backspace' }).entry).toBe('-0');
    expect(
      run({ type: 'decimal' }, { type: 'toggleSign' }, { type: 'backspace' }, ...digits('7')).entry,
    ).toBe('-7');
  });
});

describe('choosing an operator', () => {
  it('stores the entry as the left operand', () => {
    const state = run(...digits('12'), { type: 'operatorChosen', operation: 'add' });
    expect(state).toMatchObject({ accumulator: '12', pendingOperation: 'add', awaitingOperand: true, entry: '12' });
  });

  it('starts a new entry for the right operand', () => {
    const state = run(...digits('12'), { type: 'operatorChosen', operation: 'add' }, ...digits('3'));
    expect(state).toMatchObject({ accumulator: '12', entry: '3', awaitingOperand: false });
  });

  it('swaps the operator when no right operand was typed', () => {
    const state = run(
      ...digits('12'),
      { type: 'operatorChosen', operation: 'add' },
      { type: 'operatorChosen', operation: 'multiply' },
    );
    expect(state).toMatchObject({ accumulator: '12', pendingOperation: 'multiply' });
  });

  it('keeps every typed digit of the left operand', () => {
    const state = run(...digits('0.1234567890123456789'), { type: 'operatorChosen', operation: 'add' });
    expect(state.accumulator).toBe('0.1234567890123456789');
  });

  it('drops an unfinished decimal point from the left operand', () => {
    const state = run(...digits('5.'), { type: 'operatorChosen', operation: 'add' });
    expect(state.accumulator).toBe('5');
  });

  it('starts the right operand with "0." when a decimal point comes first', () => {
    const state = run(...digits('12'), { type: 'operatorChosen', operation: 'add' }, { type: 'decimal' });
    expect(state).toMatchObject({ entry: '0.', awaitingOperand: false });
  });
});

describe('results', () => {
  it('shows the result and clears the pending operation', () => {
    const state = run(
      ...digits('2'),
      { type: 'operatorChosen', operation: 'add' },
      ...digits('3'),
      { type: 'resolved', value: '5', nextOperation: null },
    );
    expect(state).toEqual({ ...initialState, entry: '5', overwrite: true });
  });

  it('chains into the next operation', () => {
    const state = run({ type: 'resolved', value: '5', nextOperation: 'multiply' });
    expect(state).toMatchObject({ entry: '5', accumulator: '5', pendingOperation: 'multiply', awaitingOperand: true });
  });

  it('starts a new entry when a digit follows a result', () => {
    expect(run({ type: 'resolved', value: '5', nextOperation: null }, ...digits('9')).entry).toBe('9');
  });

  it('replaces the entry in place and keeps the pending operation', () => {
    const state = run(
      ...digits('9'),
      { type: 'operatorChosen', operation: 'add' },
      ...digits('16'),
      { type: 'entryReplaced', value: '4' },
    );
    expect(state).toMatchObject({ entry: '4', accumulator: '9', pendingOperation: 'add', awaitingOperand: false });
  });
});

describe('errors', () => {
  const failed = run(...digits('1'), { type: 'operatorChosen', operation: 'divide' }, ...digits('0'), {
    type: 'failed',
    message: 'Cannot divide by zero',
  });

  it('shows the message and discards the calculation', () => {
    expect(failed).toEqual({ ...initialState, error: 'Cannot divide by zero' });
  });

  it('recovers on the next key press', () => {
    expect(calculatorReducer(failed, { type: 'digit', digit: '7' })).toEqual({ ...initialState, entry: '7' });
    expect(calculatorReducer(failed, { type: 'clear' })).toEqual(initialState);
  });
});

describe('clear', () => {
  it('resets everything', () => {
    expect(run(...digits('12'), { type: 'operatorChosen', operation: 'add' }, { type: 'clear' })).toEqual(
      initialState,
    );
  });
});

describe('toOperand', () => {
  it.each([
    ['12', '12'],
    ['5.', '5'],
    ['-0.', '-0'],
    ['0.50', '0.50'],
  ])('reads %s as %s', (entry, expected) => {
    expect(toOperand(entry)).toBe(expected);
  });
});
