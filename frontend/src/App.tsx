import { useState } from 'react';
import { Calculator } from './components/Calculator';
import { PaymentPlanner } from './components/PaymentPlanner';

const TABS = [
  { id: 'calculator', label: 'Calculator' },
  { id: 'planner', label: 'Split payments' },
] as const;

type TabId = (typeof TABS)[number]['id'];

export function App() {
  const [active, setActive] = useState<TabId>('calculator');

  return (
    <main className="app">
      <h1 className="app__title">Calculator</h1>

      <div className="tabs" role="tablist" aria-label="Tools">
        {TABS.map((tab) => (
          <button
            key={tab.id}
            type="button"
            role="tab"
            id={`tab-${tab.id}`}
            className="tabs__tab"
            aria-selected={active === tab.id}
            aria-controls="tool"
            onClick={() => setActive(tab.id)}
          >
            {tab.label}
          </button>
        ))}
      </div>

      {/* Only the active tool is mounted, so the calculator's keyboard shortcuts
          cannot fire while the user types an amount into the planner. */}
      <div className="app__tool" id="tool" role="tabpanel" aria-labelledby={`tab-${active}`}>
        {active === 'calculator' ? <Calculator /> : <PaymentPlanner />}
      </div>

      {active === 'calculator' && (
        <p className="app__hint">
          Keyboard: digits, <kbd>+</kbd> <kbd>-</kbd> <kbd>*</kbd> <kbd>/</kbd> <kbd>^</kbd> <kbd>%</kbd>,{' '}
          <kbd>Enter</kbd>, <kbd>Backspace</kbd>, <kbd>Esc</kbd>, paste
        </p>
      )}
    </main>
  );
}
