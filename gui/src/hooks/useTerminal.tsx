import { useEffect, useRef } from 'react';
import { Terminal } from '@xterm/xterm';
import '@xterm/xterm/css/xterm.css';

export const useTerminal = (commands: { [key: string]: string }) => {
  const terminalRef = useRef<HTMLDivElement | null>(null);
  const xtermRef = useRef<Terminal | null>(null);

  useEffect(() => {
    if (terminalRef.current && !xtermRef.current) {
      const xterm = new Terminal({
        cursorBlink: true,
        theme: {
          background: '#1e1e1e', // Dark background
          foreground: '#ffffff', // Light text
        },
      });
      xterm.open(terminalRef.current);
      xtermRef.current = xterm;

      xterm.onKey(({ key, domEvent }) => {
        const printable =
          !domEvent.altKey && !domEvent.ctrlKey && !domEvent.metaKey;

        if (domEvent.keyCode === 13) {
          handleCommand(xterm);
        } else if (domEvent.keyCode === 8) {
          // Do not delete the prompt
          if (xterm.buffer.active.cursorX > 2) {
            xterm.write('\b \b');
          }
        } else if (printable) {
          xterm.write(key);
        }
      });

      xterm.prompt = () => {
        xterm.write('\r\n$ ');
      };

      xterm.prompt();
    }

    return () => {
      xtermRef.current?.dispose();
    };
  }, []);

  const handleCommand = (xterm: Terminal) => {
    const input = xterm.buffer.active
      .getLine(xterm.buffer.active.cursorY)
      ?.translateToString()
      .trim();
    const command = input?.split('$ ')[1];

    if (command && commands[command]) {
      xterm.write(`\r\n${commands[command]}`);
    } else {
      xterm.write('\r\nCommand not found');
    }

    xterm.prompt();
  };

  return terminalRef;
};
