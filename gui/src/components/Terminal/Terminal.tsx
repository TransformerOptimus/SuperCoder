import React, { useEffect, useRef } from 'react';
import { Terminal } from 'xterm';
import 'xterm/css/xterm.css';
import styles from './terminal.module.css';

const TerminalComponent: React.FC = () => {
  const terminalRef = useRef<HTMLDivElement | null>(null);

  useEffect(() => {
    const terminal = new Terminal();
    if (terminalRef.current) {
      terminal.open(terminalRef.current);
      terminal.write('Hello from xterm.js!\r\n');
    }

    return () => {
      terminal.dispose();
    };
  }, []);

  return <div ref={terminalRef} className={styles.terminal} />;
};

export default TerminalComponent;
