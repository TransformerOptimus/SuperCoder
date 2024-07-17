import React from 'react';
import { useTerminal } from '@/hooks/useTerminal';

const commands = {
  help: 'Available commands: help, echo, clear',
  'echo Hello': 'Hello',
  clear: '\x1Bc',
};

const TerminalComponent: React.FC = () => {
  const terminalRef = useTerminal(commands);

  return (
    <div className="h-full w-full">
      <div
        ref={terminalRef}
        className="h-full w-full rounded-lg bg-black shadow-lg"
      ></div>
    </div>
  );
};

export default TerminalComponent;
