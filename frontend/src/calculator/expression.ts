import type { CalculationRequest } from '../api/calculatorApi';
import type { InfixOperation } from './reducer';

export const OPERATOR_SYMBOLS: Record<InfixOperation, string> = {
  add: '+',
  subtract: '−',
  multiply: '×',
  divide: '÷',
  power: '^',
};

/** Writes a calculation the way a person would, for the history tape. */
export function describeCalculation(request: CalculationRequest): string {
  switch (request.operation) {
    case 'sqrt':
      return `√${request.a}`;
    case 'percentage':
      return request.b === '1' ? `${request.a}%` : `${request.a}% of ${request.b}`;
    default:
      return `${request.a} ${OPERATOR_SYMBOLS[request.operation]} ${request.b}`;
  }
}
