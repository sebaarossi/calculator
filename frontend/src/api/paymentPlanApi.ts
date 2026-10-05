import { includes, invalidResponse, isRecord, post } from './http';

export const PLANS = ['pay_in_2', 'pay_in_4', 'pay_in_5'] as const;
export type Plan = (typeof PLANS)[number];

export interface PaymentPlanRequest {
  plan: Plan;
  /** Dollars with at most two decimals, as text: "99.99". */
  amount: string;
  /** The day of the purchase, as YYYY-MM-DD. */
  purchaseDate: string;
}

export interface Installment {
  /** 1 is the installment charged at checkout. */
  number: number;
  dueDate: string;
  amount: string;
}

export interface PaymentPlan {
  plan: Plan;
  amount: string;
  currency: string;
  interest: string;
  intervalDays: number;
  installments: Installment[];
}

/** Everything the payment planner needs from the backend; lets tests inject a fake. */
export interface PaymentPlanApi {
  createPlan(request: PaymentPlanRequest): Promise<PaymentPlan>;
}

export const paymentPlanApi: PaymentPlanApi = {
  async createPlan(request) {
    const body = await post('/payment-plans', request);
    if (!isPaymentPlan(body)) {
      throw invalidResponse();
    }
    return body;
  },
};

function isPaymentPlan(value: unknown): value is PaymentPlan {
  return (
    isRecord(value) &&
    includes(PLANS, value.plan) &&
    typeof value.amount === 'string' &&
    typeof value.currency === 'string' &&
    typeof value.interest === 'string' &&
    typeof value.intervalDays === 'number' &&
    Array.isArray(value.installments) &&
    value.installments.length > 0 &&
    value.installments.every(isInstallment)
  );
}

function isInstallment(value: unknown): value is Installment {
  return (
    isRecord(value) &&
    typeof value.number === 'number' &&
    typeof value.dueDate === 'string' &&
    typeof value.amount === 'string'
  );
}
