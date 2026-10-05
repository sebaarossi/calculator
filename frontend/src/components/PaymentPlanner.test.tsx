import { render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { ApiError } from '../api/http';
import type { PaymentPlan, PaymentPlanRequest } from '../api/paymentPlanApi';
import { PaymentPlanner } from './PaymentPlanner';

const payIn4: PaymentPlan = {
  plan: 'pay_in_4',
  amount: '99.99',
  currency: 'USD',
  interest: '0.00',
  intervalDays: 14,
  installments: [
    { number: 1, dueDate: '2026-10-05', amount: '25.02' },
    { number: 2, dueDate: '2026-10-19', amount: '24.99' },
    { number: 3, dueDate: '2026-11-02', amount: '24.99' },
    { number: 4, dueDate: '2026-11-16', amount: '24.99' },
  ],
};

function setup() {
  const createPlan = vi.fn(async (_request: PaymentPlanRequest) => payIn4);
  const user = userEvent.setup();
  render(<PaymentPlanner api={{ createPlan }} today="2026-10-05" />);

  const amount = () => screen.getByLabelText('Purchase amount');
  const submit = () => user.click(screen.getByRole('button', { name: 'Split it' }));
  const schedule = () => screen.queryByRole('region', { name: 'Payment schedule' });

  return { createPlan, user, amount, submit, schedule };
}

describe('PaymentPlanner', () => {
  it('starts on Pay in 4 with today as the purchase date and no schedule', () => {
    const { schedule } = setup();

    expect(screen.getByRole('radio', { name: 'Pay in 4' })).toBeChecked();
    expect(screen.getByLabelText('Purchase date')).toHaveValue('2026-10-05');
    expect(schedule()).not.toBeInTheDocument();
  });

  it('asks the backend for the plan and shows every installment', async () => {
    const { createPlan, user, amount, submit, schedule } = setup();

    await user.type(amount(), '99.99');
    await submit();

    expect(createPlan.mock.calls).toEqual([[{ plan: 'pay_in_4', amount: '99.99', purchaseDate: '2026-10-05' }]]);

    const lines = within(schedule()!).getAllByRole('listitem').map((line) => line.textContent);
    expect(lines).toEqual([
      'At checkoutMon, Oct 5, 2026$25.02',
      'Payment 2Mon, Oct 19, 2026$24.99',
      'Payment 3Mon, Nov 2, 2026$24.99',
      'Payment 4Mon, Nov 16, 2026$24.99',
    ]);
    expect(schedule()).toHaveTextContent('Pay in 4 $99.99');
    expect(schedule()).toHaveTextContent('4 payments, 14 days apart · $0.00 interest');
  });

  it('sends the chosen plan and date', async () => {
    const { createPlan, user, amount, submit } = setup();

    await user.click(screen.getByRole('radio', { name: 'Pay in 2' }));
    await user.type(amount(), '50');
    await user.clear(screen.getByLabelText('Purchase date'));
    await user.type(screen.getByLabelText('Purchase date'), '2026-12-25');
    await submit();

    expect(createPlan).toHaveBeenCalledWith({ plan: 'pay_in_2', amount: '50', purchaseDate: '2026-12-25' });
  });

  it('submits with Enter from the amount field', async () => {
    const { createPlan, user, amount } = setup();

    await user.type(amount(), '100{Enter}');

    expect(createPlan).toHaveBeenCalledTimes(1);
  });

  it.each([
    ['', 'Enter the purchase amount'],
    ['0', 'Enter an amount greater than zero'],
    ['10.005', 'Use at most two decimals'],
    ['1,000', 'Use digits only, such as 1234.56'],
  ])('explains an invalid amount %j without calling the backend', async (text, message) => {
    const { createPlan, user, amount, submit } = setup();

    if (text !== '') {
      await user.type(amount(), text);
    }
    await submit();

    expect(screen.getByRole('alert')).toHaveTextContent(message);
    expect(amount()).toHaveAttribute('aria-invalid', 'true');
    expect(createPlan).not.toHaveBeenCalled();
  });

  it('requires a purchase date', async () => {
    const { createPlan, user, amount, submit } = setup();

    await user.type(amount(), '100');
    await user.clear(screen.getByLabelText('Purchase date'));
    await submit();

    expect(screen.getByRole('alert')).toHaveTextContent('Choose the purchase date');
    expect(createPlan).not.toHaveBeenCalled();
  });

  it('shows errors from the backend', async () => {
    const { createPlan, user, amount, submit, schedule } = setup();
    createPlan.mockRejectedValueOnce(new ApiError('amount_out_of_range', 'That amount cannot be split into this plan', 422));

    await user.type(amount(), '0.03');
    await submit();

    expect(screen.getByRole('alert')).toHaveTextContent('That amount cannot be split into this plan');
    expect(schedule()).not.toBeInTheDocument();
  });

  it('shows a generic message for unexpected failures', async () => {
    const { createPlan, user, amount, submit } = setup();
    createPlan.mockRejectedValueOnce('boom');

    await user.type(amount(), '100');
    await submit();

    expect(screen.getByRole('alert')).toHaveTextContent('Something went wrong');
  });

  it('discards the schedule and the error as soon as an input changes', async () => {
    const { user, amount, submit, schedule } = setup();

    await user.type(amount(), '99.99');
    await submit();
    expect(schedule()).toBeInTheDocument();

    await user.click(screen.getByRole('radio', { name: 'Pay in 5' }));
    expect(schedule()).not.toBeInTheDocument();

    await user.clear(amount());
    await submit();
    expect(screen.getByRole('alert')).toBeInTheDocument();

    await user.type(amount(), '1');
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
  });

  it('ignores a second submit while the first is in flight', async () => {
    const { createPlan, user, amount, submit, schedule } = setup();
    let finish: (plan: PaymentPlan) => void = () => {};
    createPlan.mockReturnValueOnce(new Promise((resolve) => (finish = resolve)));

    await user.type(amount(), '99.99');
    await submit();
    await submit();

    expect(screen.getByRole('form', { name: 'Payment planner' })).toHaveAttribute('aria-busy', 'true');
    expect(createPlan).toHaveBeenCalledTimes(1);

    finish(payIn4);
    expect(await screen.findByRole('region', { name: 'Payment schedule' })).toBe(schedule());
  });
});
