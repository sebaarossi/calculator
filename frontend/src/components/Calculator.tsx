import { useEffect } from 'react';
import type { CalculatorApi } from '../api/calculatorApi';
import { OPERATOR_SYMBOLS } from '../calculator/expression';
import type { InfixOperation } from '../calculator/reducer';
import { useCalculator } from '../calculator/useCalculator';
import { Tape } from './Tape';

const KEYBOARD_OPERATORS: Record<string, InfixOperation> = {
  '+': 'add',
  '-': 'subtract',
  '*': 'multiply',
  '/': 'divide',
  '^': 'power',
};

/** Picks a text size so that even a 34-digit number stays fully visible. */
function displaySize(entry: string): string {
  if (entry.length > 20) return 'display__value display__value--tiny';
  if (entry.length > 10) return 'display__value display__value--small';
  return 'display__value';
}

type KeyVariant = 'digit' | 'function' | 'operator' | 'equals';

interface KeyButtonProps {
  label: string;
  /** Accessible name, when the visible label is only a symbol. */
  name?: string;
  variant?: KeyVariant;
  wide?: boolean;
  active?: boolean;
  onPress: () => void;
}

function KeyButton({ label, name, variant = 'digit', wide = false, active = false, onPress }: KeyButtonProps) {
  const classes = ['key', `key--${variant}`, wide && 'key--wide', active && 'key--active'].filter(Boolean).join(' ');
  return (
    <button type="button" className={classes} aria-label={name} aria-pressed={active || undefined} onClick={onPress}>
      {label}
    </button>
  );
}

interface CalculatorProps {
  /** Overrides how the backend is reached; used by tests. */
  api?: CalculatorApi;
}

export function Calculator({ api }: CalculatorProps) {
  const { state, isCalculating, tape, press } = useCalculator(api);

  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.ctrlKey || event.metaKey || event.altKey) {
        return;
      }
      const { key } = event;
      const operation = KEYBOARD_OPERATORS[key];

      if (/^[0-9]$/.test(key)) press({ type: 'digit', digit: key });
      else if (key === '.' || key === ',') press({ type: 'decimal' });
      else if (operation) press({ type: 'operator', operation });
      else if (key === '%') press({ type: 'percent' });
      else if (key === 'Enter' || key === '=') press({ type: 'equals' });
      else if (key === 'Backspace') press({ type: 'backspace' });
      else if (key === 'Escape' || key === 'Delete') press({ type: 'clear' });
      else return;

      // Stops "/" opening quick-find and Enter re-activating a focused button.
      event.preventDefault();
    };
    const onPaste = (event: ClipboardEvent) => {
      press({ type: 'paste', text: event.clipboardData?.getData('text/plain') ?? '' });
      event.preventDefault();
    };

    window.addEventListener('keydown', onKeyDown);
    window.addEventListener('paste', onPaste);
    return () => {
      window.removeEventListener('keydown', onKeyDown);
      window.removeEventListener('paste', onPaste);
    };
  }, [press]);

  const pending = state.error === null ? state.pendingOperation : null;
  const expression =
    pending !== null && state.accumulator !== null
      ? `${state.accumulator} ${OPERATOR_SYMBOLS[pending]}`
      : '';

  const operatorKey = (operation: InfixOperation, name: string, label = OPERATOR_SYMBOLS[operation]) => (
    <KeyButton
      label={label}
      name={name}
      variant="operator"
      active={pending === operation && state.awaitingOperand}
      onPress={() => press({ type: 'operator', operation })}
    />
  );
  const digitKey = (digit: string, wide = false) => (
    <KeyButton label={digit} wide={wide} onPress={() => press({ type: 'digit', digit })} />
  );

  return (
    <div className="machine">
      <Tape lines={tape} onRecall={(value) => press({ type: 'recall', value })} />
      <section className="calculator" aria-label="Calculator" aria-busy={isCalculating}>
        <div className="display">
          <div className="display__expression" data-testid="expression">
            {expression}
          </div>
          {state.error !== null ? (
            <div className="display__value display__value--error" role="alert">
              {state.error}
            </div>
          ) : (
            <output className={displaySize(state.entry)} aria-label="Result" aria-live="polite">
              {state.entry}
            </output>
          )}
        </div>

        <div className="keypad">
          <KeyButton label="C" name="Clear" variant="function" onPress={() => press({ type: 'clear' })} />
          <KeyButton label="⌫" name="Backspace" variant="function" onPress={() => press({ type: 'backspace' })} />
          <KeyButton label="%" name="Percent" variant="function" onPress={() => press({ type: 'percent' })} />
          {operatorKey('divide', 'Divide')}

          <KeyButton label="√" name="Square root" variant="function" onPress={() => press({ type: 'squareRoot' })} />
          {operatorKey('power', 'Power', 'xʸ')}
          <KeyButton label="±" name="Toggle sign" variant="function" onPress={() => press({ type: 'toggleSign' })} />
          {operatorKey('multiply', 'Multiply')}

          {digitKey('7')}
          {digitKey('8')}
          {digitKey('9')}
          {operatorKey('subtract', 'Subtract')}

          {digitKey('4')}
          {digitKey('5')}
          {digitKey('6')}
          {operatorKey('add', 'Add')}

          {digitKey('1')}
          {digitKey('2')}
          {digitKey('3')}
          <KeyButton label="=" name="Equals" variant="equals" onPress={() => press({ type: 'equals' })} />

          {digitKey('0', true)}
          <KeyButton label="." name="Decimal point" onPress={() => press({ type: 'decimal' })} />
        </div>
      </section>
    </div>
  );
}
