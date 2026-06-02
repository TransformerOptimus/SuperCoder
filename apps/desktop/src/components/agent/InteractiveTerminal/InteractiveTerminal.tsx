import { useEffect, useRef } from 'react';
import { Terminal } from 'xterm';
import { FitAddon } from '@xterm/addon-fit';
import { spawn } from 'tauri-pty';
import type { IPty } from 'tauri-pty';
import 'xterm/css/xterm.css';
import styles from './InteractiveTerminal.module.css';

interface InteractiveTerminalProps {
  workingDir: string;
  onClose: () => void;
}

export default function InteractiveTerminal({ workingDir, onClose }: InteractiveTerminalProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const xtermRef = useRef<Terminal | null>(null);
  const ptyRef = useRef<IPty | null>(null);

  useEffect(() => {
    if (!containerRef.current) return;

    // Create xterm.js terminal
    const term = new Terminal({
      fontSize: 13,
      fontFamily: '"Space Mono", "Menlo", "Monaco", monospace',
      theme: {
        background: '#1e1e1e',
        foreground: '#d4d4d4',
        cursor: '#d4d4d4',
      },
      cursorBlink: true,
    });
    const fitAddon = new FitAddon();
    term.loadAddon(fitAddon);
    term.open(containerRef.current);
    fitAddon.fit();
    xtermRef.current = term;

    // Spawn PTY session
    const pty = spawn('/bin/zsh', [], {
      cwd: workingDir,
      cols: term.cols,
      rows: term.rows,
    });
    ptyRef.current = pty;

    // PTY output → xterm
    const dataDisposable = pty.onData((data) => {
      const text = new TextDecoder().decode(data);
      term.write(text);
    });

    // PTY exit → close
    const exitDisposable = pty.onExit(() => {
      term.write('\r\n[Process exited]\r\n');
    });

    // xterm input → PTY
    const inputDisposable = term.onData((data) => {
      pty.write(data);
    });

    // xterm resize → PTY
    const resizeDisposable = term.onResize(({ cols, rows }) => {
      pty.resize(cols, rows);
    });

    // Container resize → fit
    const observer = new ResizeObserver(() => {
      fitAddon.fit();
    });
    observer.observe(containerRef.current);

    return () => {
      dataDisposable.dispose();
      exitDisposable.dispose();
      inputDisposable.dispose();
      resizeDisposable.dispose();
      observer.disconnect();
      pty.kill();
      term.dispose();
    };
  }, [workingDir]);

  return (
    <div className={styles.panel}>
      <div className={styles.header}>
        <span className={styles.title}>Terminal</span>
        <span className={styles.path}>{workingDir.split('/').slice(-2).join('/')}</span>
        <div style={{ flex: 1 }} />
        <button className={styles.close_btn} onClick={onClose}>&#x2715;</button>
      </div>
      <div ref={containerRef} className={styles.terminal} />
    </div>
  );
}
