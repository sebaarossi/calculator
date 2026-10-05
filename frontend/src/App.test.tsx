import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { App } from './App';

beforeEach(() => {
  // The tools load from the backend on mount; these tests only switch tabs.
  vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(JSON.stringify({ calculations: [] }))));
});

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('App', () => {
  it('opens on the calculator', () => {
    render(<App />);

    expect(screen.getByRole('tab', { name: 'Normal Mode' })).toHaveAttribute('aria-selected', 'true');
    expect(screen.getByRole('region', { name: 'Calculator' })).toBeInTheDocument();
    expect(screen.getByText(/Keyboard:/)).toBeInTheDocument();
  });

  it('switches to the payment planner and back', async () => {
    const user = userEvent.setup();
    render(<App />);

    await user.click(screen.getByRole('tab', { name: 'Sezzle Mode' }));

    expect(screen.getByRole('tab', { name: 'Sezzle Mode' })).toHaveAttribute('aria-selected', 'true');
    expect(screen.getByRole('form', { name: 'Payment planner' })).toBeInTheDocument();
    expect(screen.queryByRole('region', { name: 'Calculator' })).not.toBeInTheDocument();
    expect(screen.queryByText(/Keyboard:/)).not.toBeInTheDocument();

    await user.click(screen.getByRole('tab', { name: 'Normal Mode' }));

    expect(screen.getByRole('region', { name: 'Calculator' })).toBeInTheDocument();
  });

  it('does not let typing in the planner reach the calculator', async () => {
    const user = userEvent.setup();
    render(<App />);

    await user.click(screen.getByRole('tab', { name: 'Sezzle Mode' }));
    await user.type(screen.getByLabelText('Purchase amount'), '12');
    await user.click(screen.getByRole('tab', { name: 'Normal Mode' }));

    expect(screen.getByRole('status', { name: 'Result' })).toHaveTextContent('0');
  });
});
