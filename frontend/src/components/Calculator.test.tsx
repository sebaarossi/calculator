import { act, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { ApiError, type Calculation, type CalculationRequest } from '../api/calculatorApi';
import { Calculator } from './Calculator';

/** The fake backend may use floats: these tests are about the UI, not the arithmetic. */
function compute(request: CalculationRequest): number {
  const a = Number(request.a);
  if (request.operation === 'sqrt') {
    return Math.sqrt(a);
  }
  const b = Number(request.b);
  switch (request.operation) {
    case 'add':
      return a + b;
    case 'subtract':
      return a - b;
    case 'multiply':
      return a * b;
    case 'divide':
      if (b === 0) {
        throw new ApiError('division_by_zero', 'Cannot divide by zero', 422);
      }
      return a / b;
    case 'power':
      return a ** b;
    case 'percentage':
      return (a / 100) * b;
  }
}

let nextId = 1;

function record(request: CalculationRequest, result: number | string): Calculation {
  return { ...request, id: nextId++, result: String(result), createdAt: '2026-10-05T21:30:00Z' };
}

/**
 * Stands in for the backend so tests can assert on what the UI asks it to
 * compute. `history` is what the server already has recorded, newest first.
 */
function setup(history: Calculation[] = []) {
  const calculate = vi.fn(async (request: CalculationRequest) => record(request, compute(request)));
  const listRecent = vi.fn(async (limit: number) => history.slice(0, limit));
  const user = userEvent.setup();
  render(<Calculator api={{ calculate, listRecent }} />);

  const press = async (...names: string[]) => {
    for (const name of names) {
      await user.click(screen.getByRole('button', { name }));
    }
  };
  const display = () => screen.getByRole('status', { name: 'Result' });

  return { calculate, listRecent, user, press, display };
}

describe('Calculator', () => {
  it('starts at zero', () => {
    const { display } = setup();
    expect(display()).toHaveTextContent('0');
  });

  it.each([
    ['adds', ['1', '2', 'Add', '3', 'Equals'], { operation: 'add', a: '12', b: '3' }, '15'],
    ['subtracts', ['9', 'Subtract', '4', 'Equals'], { operation: 'subtract', a: '9', b: '4' }, '5'],
    ['multiplies', ['6', 'Multiply', '7', 'Equals'], { operation: 'multiply', a: '6', b: '7' }, '42'],
    ['divides', ['9', 'Divide', '2', 'Equals'], { operation: 'divide', a: '9', b: '2' }, '4.5'],
    ['raises to a power', ['2', 'Power', '8', 'Equals'], { operation: 'power', a: '2', b: '8' }, '256'],
    [
      'handles decimals and negatives',
      ['1', 'Decimal point', '5', 'Toggle sign', 'Add', '4', 'Equals'],
      { operation: 'add', a: '-1.5', b: '4' },
      '2.5',
    ],
  ])('%s through the backend', async (_, keys, request, result) => {
    const { calculate, press, display } = setup();

    await press(...keys);

    expect(calculate.mock.calls).toEqual([[request]]);
    expect(display()).toHaveTextContent(result);
  });

  it('takes a square root of the current entry', async () => {
    const { calculate, press, display } = setup();

    await press('8', '1', 'Square root');

    expect(calculate.mock.calls).toEqual([[{ operation: 'sqrt', a: '81' }]]);
    expect(display()).toHaveTextContent('9');
  });

  it('treats a standalone percent as a fraction of one', async () => {
    const { calculate, press, display } = setup();

    await press('5', '0', 'Percent');

    expect(calculate.mock.calls).toEqual([[{ operation: 'percentage', a: '50', b: '1' }]]);
    expect(display()).toHaveTextContent('0.5');
  });

  it('treats percent inside an operation as a share of the left operand', async () => {
    const { calculate, press, display } = setup();

    await press('2', '0', '0', 'Add', '1', '0', 'Percent', 'Equals');

    expect(calculate).toHaveBeenNthCalledWith(1, { operation: 'percentage', a: '10', b: '200' });
    expect(calculate).toHaveBeenNthCalledWith(2, { operation: 'add', a: '200', b: '20' });
    expect(display()).toHaveTextContent('220');
  });

  it('evaluates chained operations left to right', async () => {
    const { calculate, press, display } = setup();

    await press('2', 'Add', '3', 'Multiply');
    expect(display()).toHaveTextContent('5');
    expect(screen.getByTestId('expression')).toHaveTextContent('5 ×');

    await press('4', 'Equals');

    expect(calculate).toHaveBeenNthCalledWith(1, { operation: 'add', a: '2', b: '3' });
    expect(calculate).toHaveBeenNthCalledWith(2, { operation: 'multiply', a: '5', b: '4' });
    expect(display()).toHaveTextContent('20');
  });

  it('resolves the pending operation when an operator follows a square root', async () => {
    const { calculate, press, display } = setup();

    await press('9', 'Add', '1', '6', 'Square root', 'Multiply');

    expect(calculate).toHaveBeenNthCalledWith(2, { operation: 'add', a: '9', b: '4' });
    expect(display()).toHaveTextContent('13');
  });

  it('lets the user change the operator before typing the second operand', async () => {
    const { calculate, press } = setup();

    await press('8', 'Add', 'Subtract', '3', 'Equals');

    expect(calculate.mock.calls).toEqual([[{ operation: 'subtract', a: '8', b: '3' }]]);
    expect(screen.getByRole('button', { name: 'Subtract' })).not.toHaveAttribute('aria-pressed');
  });

  it('highlights the pending operator', async () => {
    const { press } = setup();

    await press('8', 'Add');

    expect(screen.getByRole('button', { name: 'Add' })).toHaveAttribute('aria-pressed', 'true');
  });

  it('does nothing on equals without a pending operation', async () => {
    const { calculate, press, display } = setup();

    await press('7', 'Equals');

    expect(calculate).not.toHaveBeenCalled();
    expect(display()).toHaveTextContent('7');
  });

  it('shrinks the text so long numbers stay visible', async () => {
    const { press, display } = setup();

    await press(...'1234567890'.split(''));
    expect(display()).toHaveClass('display__value', { exact: true });

    await press('1');
    expect(display()).toHaveClass('display__value--small');

    await press(...'2345678901'.split(''));
    expect(display()).toHaveClass('display__value--tiny');
  });

  it('sends operands exactly as typed, without converting them to floats', async () => {
    const { calculate, press } = setup();
    const precise = '0.123456789012345678901234567890123';

    await press(...[...precise].map((char) => (char === '.' ? 'Decimal point' : char)), 'Add', '1', 'Equals');

    expect(calculate.mock.calls).toEqual([[{ operation: 'add', a: precise, b: '1' }]]);
  });

  it('shows results exactly as the backend returns them', async () => {
    const { calculate, press, display } = setup();
    const third = '0.3333333333333333333333333333333333';
    calculate.mockResolvedValueOnce(record({ operation: 'divide', a: '1', b: '3' }, third));

    await press('1', 'Divide', '3', 'Equals');

    expect(display()).toHaveTextContent(third);
  });

  describe('history tape', () => {
    it('starts empty', () => {
      setup();
      expect(screen.getByText('Your calculations print here')).toBeInTheDocument();
      expect(screen.queryByRole('list', { name: 'History' })).not.toBeInTheDocument();
    });

    it('prints successful calculations and skips failed ones', async () => {
      const { press } = setup();

      await press('7', 'Multiply', '6', 'Equals', 'Divide', '0', 'Equals', '8', '1', 'Square root');

      const lines = screen.getAllByRole('listitem').map((line) => line.textContent);
      expect(lines).toEqual(['7 × 6 =42', '√81 =9']);
    });

    it('keeps only the most recent calculations', async () => {
      const { press } = setup();

      for (const digit of ['1', '2', '3', '4', '5', '6', '7']) {
        await press(digit, 'Square root');
      }

      const lines = screen.getAllByRole('listitem');
      expect(lines).toHaveLength(5);
      expect(lines[0]).toHaveTextContent('√3');
      expect(lines[4]).toHaveTextContent('√7');
    });

    it('loads the history recorded by the backend, oldest first', async () => {
      const { listRecent } = setup([
        record({ operation: 'sqrt', a: '81' }, 9),
        record({ operation: 'add', a: '2', b: '3' }, 5),
      ]);

      const lines = await screen.findAllByRole('listitem');

      expect(listRecent).toHaveBeenCalledWith(5);
      expect(lines.map((line) => line.textContent)).toEqual(['2 + 3 =5', '√81 =9']);
    });

    it('keeps working when the history cannot be loaded', async () => {
      const { listRecent, press, display } = setup();
      listRecent.mockRejectedValueOnce(new ApiError('network_error', 'Cannot reach the calculator service'));

      await press('2', 'Add', '3', 'Equals');

      expect(display()).toHaveTextContent('5');
      expect(screen.queryByRole('alert')).not.toBeInTheDocument();
    });

    it('copes with repeated IDs after the backend restarts', async () => {
      const errors = vi.spyOn(console, 'error').mockImplementation(() => {});
      const { calculate, press } = setup([{ ...record({ operation: 'sqrt', a: '81' }, 9), id: 1 }]);
      calculate.mockResolvedValueOnce({
        ...record({ operation: 'sqrt', a: '16' }, 4),
        id: 1,
        createdAt: '2026-10-06T08:00:00Z',
      });
      await screen.findAllByRole('listitem');

      await press('1', '6', 'Square root');

      expect(screen.getAllByRole('listitem').map((line) => line.textContent)).toEqual(['√81 =9', '√16 =4']);
      expect(errors).not.toHaveBeenCalled();
    });

    it('reuses a result when its line is picked', async () => {
      const { calculate, press, display } = setup();

      await press('7', 'Multiply', '6', 'Equals', 'Clear', '1', '0', 'Add');
      await press('7 × 6 = 42, use this result');
      expect(display()).toHaveTextContent('42');

      await press('Equals');
      expect(calculate).toHaveBeenLastCalledWith({ operation: 'add', a: '10', b: '42' });
    });
  });

  it('edits the entry with backspace and clear', async () => {
    const { press, display } = setup();

    await press('1', '2', '3', 'Backspace');
    expect(display()).toHaveTextContent('12');

    await press('Clear');
    expect(display()).toHaveTextContent('0');
  });

  it('shows backend errors and recovers on the next key', async () => {
    const { press, display } = setup();

    await press('5', 'Divide', '0', 'Equals');

    expect(screen.getByRole('alert')).toHaveTextContent('Cannot divide by zero');
    expect(screen.getByTestId('expression')).toBeEmptyDOMElement();

    await press('7');

    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
    expect(display()).toHaveTextContent('7');
  });

  it('starts a fresh calculation when an operator follows an error', async () => {
    const { calculate, press, display } = setup();

    await press('5', 'Divide', '0', 'Equals', 'Add', '2', 'Equals');

    expect(calculate).toHaveBeenLastCalledWith({ operation: 'add', a: '0', b: '2' });
    expect(display()).toHaveTextContent('2');
  });

  it('shows a generic message for unexpected failures', async () => {
    const { calculate, press } = setup();
    calculate.mockRejectedValueOnce('boom');

    await press('4', 'Square root');

    expect(screen.getByRole('alert')).toHaveTextContent('Something went wrong');
  });

  describe('while a calculation is in flight', () => {
    /** Makes the next calculation hang until the returned function is called. */
    function holdNextCalculation(calculate: ReturnType<typeof setup>['calculate']) {
      let release: (value: Calculation) => void = () => {};
      calculate.mockReturnValueOnce(new Promise((resolve) => (release = resolve)));
      return (request: CalculationRequest, result: number) => release(record(request, result));
    }

    it('marks the calculator as busy', async () => {
      const { calculate, press, display } = setup();
      const finish = holdNextCalculation(calculate);
      const region = () => screen.getByRole('region', { name: 'Calculator' });

      await press('1', 'Add', '2', 'Equals');
      expect(region()).toHaveAttribute('aria-busy', 'true');
      expect(display()).toHaveTextContent('2');

      finish({ operation: 'add', a: '1', b: '2' }, 3);

      await waitFor(() => expect(display()).toHaveTextContent('3'));
      expect(region()).toHaveAttribute('aria-busy', 'false');
    });

    it('queues key presses instead of dropping them', async () => {
      const { calculate, press, display } = setup();
      const finish = holdNextCalculation(calculate);

      // "× 4 =" is typed before the answer to "2 + 3 =" arrives.
      await press('2', 'Add', '3', 'Equals', 'Multiply', '4', 'Equals');
      expect(calculate).toHaveBeenCalledTimes(1);

      finish({ operation: 'add', a: '2', b: '3' }, 5);

      await waitFor(() => expect(display()).toHaveTextContent('20'));
      expect(calculate.mock.calls).toEqual([
        [{ operation: 'add', a: '2', b: '3' }],
        [{ operation: 'multiply', a: '5', b: '4' }],
      ]);
    });

    it('discards queued key presses when the calculation fails', async () => {
      const { calculate, press } = setup();
      let fail: (error: Error) => void = () => {};
      calculate.mockReturnValueOnce(new Promise((_, reject) => (fail = reject)));

      await press('5', 'Divide', '0', 'Equals', 'Add', '7', 'Equals');
      fail(new ApiError('division_by_zero', 'Cannot divide by zero', 422));

      expect(await screen.findByRole('alert')).toHaveTextContent('Cannot divide by zero');
      expect(calculate).toHaveBeenCalledTimes(1);
    });
  });

  it('handles keys that arrive in the same instant, in order', async () => {
    const { calculate, display } = setup();

    // Dispatched synchronously: no render happens between one key and the next.
    act(() => {
      for (const key of ['2', '+', '3', '*', '4', 'Enter']) {
        window.dispatchEvent(new KeyboardEvent('keydown', { key }));
      }
    });

    await waitFor(() => expect(display()).toHaveTextContent('20'));
    expect(calculate.mock.calls).toEqual([
      [{ operation: 'add', a: '2', b: '3' }],
      [{ operation: 'multiply', a: '5', b: '4' }],
    ]);
  });

  describe('pasting', () => {
    it('accepts a plain number as the current entry', async () => {
      const { calculate, user, press, display } = setup();

      await press('1', '0', 'Add');
      await user.paste(' 1234.56 ');
      expect(display()).toHaveTextContent('1234.56');

      await press('Equals');
      expect(calculate).toHaveBeenLastCalledWith({ operation: 'add', a: '10', b: '1234.56' });
    });

    it.each(['1,234.56', '$100', '2+2', 'hello', ''])('rejects %j with an explanation', async (text) => {
      const { calculate, user, press, display } = setup();

      await press('7');
      await user.paste(text);

      expect(screen.getByRole('alert')).toHaveTextContent('Cannot paste that');
      expect(calculate).not.toHaveBeenCalled();

      await press('8');
      expect(display()).toHaveTextContent('8');
    });
  });

  it('supports the keyboard', async () => {
    const { calculate, user, display } = setup();

    await user.keyboard('12.5*2{Enter}');
    expect(calculate).toHaveBeenLastCalledWith({ operation: 'multiply', a: '12.5', b: '2' });
    expect(display()).toHaveTextContent('25');

    await user.keyboard('{Escape}95{Backspace}0%');
    expect(calculate).toHaveBeenLastCalledWith({ operation: 'percentage', a: '90', b: '1' });

    await user.keyboard('{Delete}');
    expect(display()).toHaveTextContent('0');

    calculate.mockClear();
    await user.keyboard('{Escape}x{Control>}1{/Control}');
    expect(calculate).not.toHaveBeenCalled();
    expect(display()).toHaveTextContent('0');
  });
});
