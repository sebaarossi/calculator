import type { Calculation, Decimal } from '../api/calculatorApi';
import { describeCalculation } from '../calculator/expression';

interface TapeProps {
  /** Calculations to print, oldest first. */
  lines: Calculation[];
  /** Called with a line's result when the user picks it to reuse. */
  onRecall: (value: Decimal) => void;
}

/** The paper tape: a short history of calculations whose results can be reused. */
export function Tape({ lines, onRecall }: TapeProps) {
  return (
    <div className="tape">
      {lines.length === 0 ? (
        <p className="tape__empty">Your calculations print here</p>
      ) : (
        <ol className="tape__lines" aria-label="History">
          {/* IDs restart when the backend does, so the timestamp keeps keys unique. */}
          {lines.map((line) => {
            const expression = describeCalculation(line);
            return (
              <li key={`${line.id}-${line.createdAt}`}>
                <button
                  type="button"
                  className="tape__line"
                  title="Use this result"
                  aria-label={`${expression} = ${line.result}, use this result`}
                  onClick={() => onRecall(line.result)}
                >
                  <span>{expression} =</span>
                  <strong>{line.result}</strong>
                </button>
              </li>
            );
          })}
        </ol>
      )}
    </div>
  );
}
