import { useState, type FormEvent } from 'react';
import { paymentPlanApi, PLANS, type PaymentPlan, type PaymentPlanApi, type Plan } from '../api/paymentPlanApi';
import { formatDate, formatMoney, todayISO, validateAmount } from '../payments/money';

const PLAN_LABELS: Record<Plan, string> = {
  pay_in_2: 'Pay in 2',
  pay_in_4: 'Pay in 4',
  pay_in_5: 'Pay in 5',
};

interface PaymentPlannerProps {
  /** Overrides how the backend is reached; used by tests. */
  api?: PaymentPlanApi;
  /** The default purchase date, as YYYY-MM-DD; used by tests. */
  today?: string;
}

/** Splits a purchase into interest-free installments and shows the schedule. */
export function PaymentPlanner({ api = paymentPlanApi, today = todayISO() }: PaymentPlannerProps) {
  const [plan, setPlan] = useState<Plan>('pay_in_4');
  const [amount, setAmount] = useState('');
  const [purchaseDate, setPurchaseDate] = useState(today);
  const [schedule, setSchedule] = useState<PaymentPlan | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [isLoading, setIsLoading] = useState(false);

  /** Wraps a setter so that editing any input discards a schedule that no longer matches it. */
  const editing =
    <T,>(set: (value: T) => void) =>
    (value: T) => {
      set(value);
      setSchedule(null);
      setError(null);
    };

  const submit = async (event: FormEvent) => {
    event.preventDefault();
    if (isLoading) {
      return;
    }
    const validation = validateAmount(amount);
    if (!validation.ok) {
      setError(validation.error);
      return;
    }
    if (purchaseDate === '') {
      setError('Choose the purchase date');
      return;
    }

    setIsLoading(true);
    setError(null);
    try {
      setSchedule(await api.createPlan({ plan, amount: validation.amount, purchaseDate }));
    } catch (failure) {
      setSchedule(null);
      setError(failure instanceof Error ? failure.message : 'Something went wrong');
    } finally {
      setIsLoading(false);
    }
  };

  return (
    <div className="machine">
      <form className="planner" aria-label="Payment planner" aria-busy={isLoading} onSubmit={submit} noValidate>
        <fieldset className="planner__plans">
          <legend>Plan</legend>
          {PLANS.map((option) => (
            <label key={option} className="planner__plan">
              <input
                type="radio"
                name="plan"
                value={option}
                checked={plan === option}
                onChange={() => editing(setPlan)(option)}
              />
              <span>{PLAN_LABELS[option]}</span>
            </label>
          ))}
        </fieldset>

        <div className="planner__field">
          <label htmlFor="planner-amount">Purchase amount</label>
          <span className="planner__money">
            <span aria-hidden="true">$</span>
            <input
              id="planner-amount"
              type="text"
              inputMode="decimal"
              autoComplete="off"
              placeholder="0.00"
              value={amount}
              aria-invalid={error !== null}
              aria-describedby={error !== null ? 'planner-error' : undefined}
              onChange={(event) => editing(setAmount)(event.target.value)}
            />
          </span>
        </div>

        <div className="planner__field">
          <label htmlFor="planner-date">Purchase date</label>
          <input
            id="planner-date"
            type="date"
            value={purchaseDate}
            min="2000-01-01"
            max="2100-12-31"
            onChange={(event) => editing(setPurchaseDate)(event.target.value)}
          />
        </div>

        {error !== null && (
          <p id="planner-error" className="planner__error" role="alert">
            {error}
          </p>
        )}

        <button type="submit" className="key key--equals planner__submit">
          Split it
        </button>
      </form>

      {schedule !== null && <Receipt schedule={schedule} />}
    </div>
  );
}

function Receipt({ schedule }: { schedule: PaymentPlan }) {
  const count = schedule.installments.length;
  return (
    <section className="receipt" aria-label="Payment schedule">
      <h2 className="receipt__title">
        {PLAN_LABELS[schedule.plan]} <span>{formatMoney(schedule.amount)}</span>
      </h2>
      <ol className="receipt__lines">
        {schedule.installments.map((installment) => (
          <li key={installment.number} className="receipt__line">
            <span>
              {installment.number === 1 ? 'At checkout' : `Payment ${installment.number}`}
              <small>{formatDate(installment.dueDate)}</small>
            </span>
            <strong>{formatMoney(installment.amount)}</strong>
          </li>
        ))}
      </ol>
      <p className="receipt__footer">
        {count} payments, {schedule.intervalDays} days apart · {formatMoney(schedule.interest)} interest
      </p>
    </section>
  );
}
