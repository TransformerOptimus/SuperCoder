import React from 'react';
import { useTerminal } from '@/hooks/useTerminal';

const TerminalComponent: React.FC = () => {
  const terminalRef = useTerminal();

  return (
    <div
      ref={terminalRef}
      className={'w-full'}
      style={{ height: 'calc(100vh - 546px)' }}
    />
  );
};

export default TerminalComponent;
