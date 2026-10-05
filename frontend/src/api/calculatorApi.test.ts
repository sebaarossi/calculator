import { ApiError, calculatorApi, REQUEST_TIMEOUT_MS, type Calculation } from './calculatorApi';

const { calculate, listRecent } = calculatorApi;

const record: Calculation = {
  id: 1,
  operation: 'add',
  a: '2',
  b: '3',
  result: '5',
  createdAt: '2026-10-05T21:30:00Z',
};

function mockFetch(status: number, body: unknown) {
  const fetchMock = vi.fn().mockResolvedValue(
    new Response(typeof body === 'string' ? body : JSON.stringify(body), { status }),
  );
  vi.stubGlobal('fetch', fetchMock);
  return fetchMock;
}

const requestInit = (init: Record<string, unknown> = {}) => ({ ...init, signal: expect.any(AbortSignal) });

afterEach(() => {
  vi.unstubAllGlobals();
  vi.useRealTimers();
});

describe('calculate', () => {
  it('posts the request as JSON and returns the recorded calculation', async () => {
    const fetchMock = mockFetch(201, record);

    await expect(calculate({ operation: 'add', a: '2', b: '3' })).resolves.toEqual(record);

    expect(fetchMock).toHaveBeenCalledWith(
      '/api/v1/calculations',
      requestInit({
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ operation: 'add', a: '2', b: '3' }),
      }),
    );
  });

  it('translates known error codes into friendly messages', async () => {
    mockFetch(422, { error: { code: 'division_by_zero', message: 'division by zero' } });

    await expect(calculate({ operation: 'divide', a: '1', b: '0' })).rejects.toMatchObject({
      name: 'ApiError',
      code: 'division_by_zero',
      message: 'Cannot divide by zero',
      status: 422,
    });
  });

  it('falls back to the server message for other error codes', async () => {
    mockFetch(400, { error: { code: 'invalid_request', message: 'field "b" is required' } });

    await expect(calculate({ operation: 'sqrt', a: '4' })).rejects.toMatchObject({
      code: 'invalid_request',
      message: 'field "b" is required',
    });
  });

  it.each([
    ['a non-JSON body', 'Bad Gateway'],
    ['an unexpected JSON shape', { error: 'nope' }],
    ['an error without code or message', { error: {} }],
  ])('reports a server error with %s as the service being unavailable', async (_, body) => {
    mockFetch(502, body);

    await expect(calculate({ operation: 'sqrt', a: '4' })).rejects.toMatchObject({
      code: 'service_unavailable',
      message: 'The calculator service is unavailable',
      status: 502,
    });
  });

  it('reports an unrecognised client error generically', async () => {
    mockFetch(418, 'I am a teapot');

    await expect(calculate({ operation: 'sqrt', a: '4' })).rejects.toMatchObject({
      code: 'unknown_error',
      status: 418,
    });
  });

  it('gives up when the service does not answer in time', async () => {
    vi.useFakeTimers();
    vi.stubGlobal(
      'fetch',
      vi.fn(
        (_url: string, init: RequestInit) =>
          new Promise((_, reject) => init.signal?.addEventListener('abort', () => reject(new Error('aborted')))),
      ),
    );

    const pending = calculate({ operation: 'sqrt', a: '4' }).catch((e: unknown) => e);
    await vi.advanceTimersByTimeAsync(REQUEST_TIMEOUT_MS);

    expect(await pending).toMatchObject({ code: 'timeout', message: 'The calculator service took too long to answer' });
  });

  it.each([
    ['a result sent as a JSON number', { ...record, result: 5 }],
    ['a missing id', { ...record, id: undefined }],
    ['an unknown operation', { ...record, operation: 'modulo' }],
    ['a binary operation without b', { ...record, b: undefined }],
    ['a missing timestamp', { ...record, createdAt: undefined }],
    ['no body', ''],
  ])('rejects a successful response with %s', async (_, body) => {
    mockFetch(201, body);

    await expect(calculate({ operation: 'add', a: '2', b: '3' })).rejects.toMatchObject({ code: 'invalid_response' });
  });

  it('reports network failures', async () => {
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new TypeError('Failed to fetch')));

    const error = await calculate({ operation: 'sqrt', a: '4' }).catch((e: unknown) => e);

    expect(error).toBeInstanceOf(ApiError);
    expect(error).toMatchObject({ code: 'network_error', message: 'Cannot reach the calculator service' });
  });
});

describe('listRecent', () => {
  it('accepts unary calculations, which have no b', async () => {
    const unary = { id: 2, operation: 'sqrt', a: '81', result: '9', createdAt: record.createdAt };
    mockFetch(200, { calculations: [unary] });

    await expect(listRecent(5)).resolves.toEqual([unary]);
  });

  it('requests the newest calculations', async () => {
    const fetchMock = mockFetch(200, { calculations: [record], nextCursor: '1' });

    await expect(listRecent(5)).resolves.toEqual([record]);

    expect(fetchMock).toHaveBeenCalledWith('/api/v1/calculations?limit=5', requestInit());
  });

  it.each([
    ['a missing list', {}],
    ['a list that is not an array', { calculations: 'none' }],
    ['a malformed entry', { calculations: [record, { id: 'x' }] }],
  ])('rejects a response with %s', async (_, body) => {
    mockFetch(200, body);

    await expect(listRecent(5)).rejects.toMatchObject({ code: 'invalid_response' });
  });

  it('reports server errors', async () => {
    mockFetch(500, { error: { code: 'internal_error', message: 'internal server error' } });

    await expect(listRecent(5)).rejects.toMatchObject({ code: 'internal_error', status: 500 });
  });
});
