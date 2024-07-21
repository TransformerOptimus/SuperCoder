import React, { useEffect, useRef } from 'react';
import { Terminal } from 'xterm';
import { AttachAddon } from 'xterm-addon-attach';
import { FitAddon } from 'xterm-addon-fit';
import { WebLinksAddon } from 'xterm-addon-web-links';
import { Unicode11Addon } from 'xterm-addon-unicode11';
import { SerializeAddon } from 'xterm-addon-serialize';
import 'xterm/css/xterm.css';

const CustomTerminal: React.FC = () => {
  const terminalRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (terminalRef.current) {
      const terminal = new Terminal({
        screenKeys: true,
        useStyle: true,
        cursorBlink: true,
        fullscreenWin: true,
        maximizeWin: true,
        screenReaderMode: true,
        cols: 128,
        allowProposedApi: true,
      });

      terminal.open(terminalRef.current);

      const protocol = location.protocol === 'https:' ? 'wss://' : 'ws://';
      const url = `${protocol}localhost:8084/api/terminal`;
      const ws = new WebSocket(url);

      const attachAddon = new AttachAddon(ws);
      const fitAddon = new FitAddon();
      const webLinksAddon = new WebLinksAddon();
      const unicode11Addon = new Unicode11Addon();
      const serializeAddon = new SerializeAddon();

      terminal.loadAddon(fitAddon);
      terminal.loadAddon(webLinksAddon);
      terminal.loadAddon(unicode11Addon);
      terminal.loadAddon(serializeAddon);

      ws.onclose = (event) => {
        console.log(event);
        terminal.write(
          '\r\n\nconnection has been terminated from the server-side (hit refresh to restart)\n',
        );
      };

      ws.onopen = () => {
        terminal.loadAddon(attachAddon);
        (terminal as any)._initialized = true; // TypeScript workaround
        terminal.focus();
        setTimeout(() => {
          fitAddon.fit();
        });

        terminal.onResize((event) => {
          const rows = event.rows;
          const cols = event.cols;
          const size = JSON.stringify({ cols, rows: rows + 1 });
          const send = new TextEncoder().encode(`\x01${size}`);
          console.log('resizing to', size);
          ws.send(send);
        });

        terminal.onTitleChange((event) => {
          console.log(event);
        });

        window.onresize = () => {
          fitAddon.fit();
        };
      };
    }
  }, []);

  return <div ref={terminalRef} style={{ width: '100%', height: '100%' }} />;
};

export default CustomTerminal;
