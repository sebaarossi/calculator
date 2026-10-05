import { paymentPlanApi, type PaymentPlan } from './paymentPlanApi';

const plan: PaymentPlan = {
  plan: 'pay_in_2',
  amount: '10.01',
  currency: 'USD',
  interest: '0.00',
  intervalDays: 14,
  installments: [
    { number: 1, dueDate: '2026-10-05', amount: '5.01' },
    { number: 2, dueDate: '2026-10-19', amount: '5.00' },
  ],
};

const request = { plan: 'pay_in_2', amount: '10.01', purchaseDate: '2026-10-05' } as const;

function mockFetch(status: number, body: unknown) {
  const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify(body), { status }));
  vi.stubGlobal('fetch', fetchMock);
  return fetchMock;
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('createPlan', () => {
  it('posts the request and returns the schedule', async () => {
    const fetchMock = mockFetch(200, plan);

    await expect(paymentPlanApi.createPlan(request)).resolves.toEqual(plan);

    expect(fetchMock).toHaveBeenCalledWith(
      '/api/v1/payment-plans',
      expect.objectContaining({
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(request),
      }),
    );
  });

  it('translates the amount error into a friendly message', async () => {
    mockFetch(422, { error: { code: 'amount_out_of_range', message: 'amount is too small to split' } });

    await expect(paymentPlanApi.createPlan(request)).rejects.toMatchObject({
      code: 'amount_out_of_range',
      message: 'That amount cannot be split into this plan',
      status: 422,
    });
  });

  it('passes other validation messages through', async () => {
    mockFetch(400, { error: { code: 'invalid_request', message: 'field "purchaseDate": not a date' } });

    await expect(paymentPlanApi.createPlan(request)).rejects.toMatchObject({
      message: 'field "purchaseDate": not a date',
    });
  });

  it.each([
    ['an unknown plan', { ...plan, plan: 'pay_in_3' }],
    ['no installments', { ...plan, installments: [] }],
    ['installments that are not a list', { ...plan, installments: 'none' }],
    ['an installment amount sent as a number', { ...plan, installments: [{ number: 1, dueDate: '2026-10-05', amount: 5.01 }] }],
    ['an installment without a date', { ...plan, installments: [{ number: 1, amount: '5.01' }] }],
    ['a missing total', { ...plan, amount: undefined }],
    ['a null body', null],
  ])('rejects a response with %s', async (_, body) => {
    mockFetch(200, body);

    await expect(paymentPlanApi.createPlan(request)).rejects.toMatchObject({ code: 'invalid_response' });
  });
});
