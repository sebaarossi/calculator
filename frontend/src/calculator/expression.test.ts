import type { CalculationRequest } from '../api/calculatorApi';
import { describeCalculation } from './expression';

describe('describeCalculation', () => {
  it.each<[CalculationRequest, string]>([
    [{ operation: 'add', a: '2', b: '3' }, '2 + 3'],
    [{ operation: 'subtract', a: '2', b: '-3' }, '2 − -3'],
    [{ operation: 'multiply', a: '1.5', b: '4' }, '1.5 × 4'],
    [{ operation: 'divide', a: '9', b: '2' }, '9 ÷ 2'],
    [{ operation: 'power', a: '2', b: '10' }, '2 ^ 10'],
    [{ operation: 'sqrt', a: '81' }, '√81'],
    [{ operation: 'percentage', a: '10', b: '200' }, '10% of 200'],
    [{ operation: 'percentage', a: '50', b: '1' }, '50%'],
  ])('describes %j as %s', (request, expected) => {
    expect(describeCalculation(request)).toBe(expected);
  });
});
