import { useEffect, useRef, useState } from 'react';
import { Terminal } from 'xterm';
import { FitAddon } from '@xterm/addon-fit';
import 'xterm/css/xterm.css';

type Command = {
  content: string;
  type: 'input' | 'output';
};

export const useTerminal = (commands: Command[] = []) => {
  const terminalRef = useRef<HTMLDivElement>(null);
  const terminalInstance = useRef<Terminal | null>(null);
  const fitAddon = useRef<FitAddon | null>(null);
  const lastCommandIndex = useRef(0);
  const [inputBuffer, setInputBuffer] = useState('');

  useEffect(() => {
    terminalInstance.current = new Terminal({
      fontFamily: "Menlo, Monaco, 'Courier New', monospace",
      fontSize: 12,
      theme: {
        background: '#262626',
        foreground: '#ffffff',
        cursor: 'rgba(248,28,229,0.8)',
      },
    });

    fitAddon.current = new FitAddon();
    terminalInstance.current.loadAddon(fitAddon.current);

    if (terminalRef.current) {
      terminalInstance.current.open(terminalRef.current);
      fitAddon.current.fit();
      terminalInstance.current.write('$ ');
    }

    const resizeObserver = new ResizeObserver(() => {
      fitAddon.current?.fit();
    });

    if (terminalRef.current) {
      resizeObserver.observe(terminalRef.current);
    }

    terminalInstance.current?.onData((data) => {
      handleInput(data);
    });

    return () => {
      terminalInstance.current?.dispose();
      resizeObserver.disconnect();
    };
  }, []);

  useEffect(() => {
    if (terminalInstance.current && commands.length > 0) {
      for (let i = lastCommandIndex.current; i < commands.length; i += 1) {
        const command = commands[i];
        const lines = command.content.split('\n');
        lines.forEach((line: string) => {
          terminalInstance.current?.writeln(line);
        });
        if (command.type === 'output') {
          terminalInstance.current.write('\n$ ');
        }
      }
      lastCommandIndex.current = commands.length;
    }
  }, [commands]);

  const handleInput = (data: string) => {
    const code = data.charCodeAt(0);

    if (code === 13) {
      // Enter key
      terminalInstance.current?.writeln('');
      // Here you can handle the input command, e.g., send it to a server or process it
      terminalInstance.current?.write('$ ');
      setInputBuffer('');
    } else if (code === 127) {
      // Backspace key
      if (inputBuffer.length > 0) {
        terminalInstance.current?.write('\b \b');
        setInputBuffer(inputBuffer.slice(0, -1));
      }
    } else {
      terminalInstance.current?.write(data);
      setInputBuffer(inputBuffer + data);
    }
  };

  return terminalRef;
};
