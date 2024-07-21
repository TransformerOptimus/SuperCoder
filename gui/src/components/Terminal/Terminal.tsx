import React from 'react';
import { useTerminal } from '@/hooks/useTerminal';

const TerminalComponent: React.FC = () => {
  const terminalRef = useTerminal();

  return <div ref={terminalRef} className={'h-[200px] w-full rounded-lg'} />;
};

export default TerminalComponent;
