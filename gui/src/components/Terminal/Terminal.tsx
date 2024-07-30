import React from 'react';
import { useTerminal } from '@/hooks/useTerminal';

const TerminalComponent: React.FC = () => {
  const terminalRef = useTerminal();

  return (
    <div
      ref={terminalRef}
      className={'w-full overflow-hidden rounded-b-lg'}
      style={{ height: 'calc(100vh - 532px)' }}
    />
  );
};

export default TerminalComponent;
