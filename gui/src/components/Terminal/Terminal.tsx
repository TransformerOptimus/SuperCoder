import React from 'react';
import { useTerminal } from '@/hooks/useTerminal';

type Command = {
  content: string;
  type: 'input' | 'output';
};

const TerminalComponent: React.FC = () => {
  const commands: Command[] = [
    { content: 'echo Hello, World!', type: 'output' },
  ];
  const terminalRef = useTerminal(commands);

  return <div ref={terminalRef} style={{ height: '100%', width: '100%' }} />;
};

export default TerminalComponent;
