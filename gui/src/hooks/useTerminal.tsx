import { useEffect, useRef } from 'react';
import { Terminal } from 'xterm';
import { AttachAddon } from 'xterm-addon-attach';
import { FitAddon } from 'xterm-addon-fit';
import { WebLinksAddon } from 'xterm-addon-web-links';
import { Unicode11Addon } from 'xterm-addon-unicode11';
import { SerializeAddon } from 'xterm-addon-serialize';
import 'xterm/css/xterm.css';

export const useTerminal = () => {
  const terminalRef = useRef<HTMLDivElement | null>(null);
  const xtermRef = useRef<Terminal | null>(null);

  useEffect(() => {
    if (terminalRef.current && !xtermRef.current) {
      const xterm = new Terminal({
        cursorBlink: true,
        cursorWidth: 120,
        fontFamily: 'Space Mono',
        fontSize: 12,
        fontWeight: '400',
        allowProposedApi: true,
      });

      const fitAddon = new FitAddon();
      const socket = new WebSocket(
        'ws://localhost:8084/api/terminal?EIO=4&transport=websocket',
      );
      const attachAddon = new AttachAddon(socket);
      const webLinksAddon = new WebLinksAddon();
      const unicode11Addon = new Unicode11Addon();
      const serializeAddon = new SerializeAddon();

      xterm.loadAddon(fitAddon);
      xterm.loadAddon(attachAddon);
      xterm.loadAddon(webLinksAddon);
      xterm.loadAddon(unicode11Addon);
      xterm.loadAddon(serializeAddon);

      xterm.open(terminalRef.current);
      fitAddon.fit();

      xtermRef.current = xterm;

      xterm.prompt = () => {
        xterm.write('\r\n$ ');
      };

      xterm.prompt();

      window.addEventListener('resize', fitAddon.fit);

      return () => {
        xtermRef.current?.dispose();
        window.removeEventListener('resize', fitAddon.fit);
      };
    }
  }, []);

  return terminalRef;
};
