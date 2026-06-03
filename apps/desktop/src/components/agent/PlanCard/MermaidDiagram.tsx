import { useEffect, useRef, useState } from 'react';
import mermaid from 'mermaid';
import { useTheme } from '../../../context/ThemeContext';

let mermaidCounter = 0;

interface MermaidDiagramProps {
  chart: string;
}

export default function MermaidDiagram({ chart }: MermaidDiagramProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const [error, setError] = useState<string | null>(null);
  const idRef = useRef(`mermaid-${++mermaidCounter}`);
  const { dark } = useTheme();

  useEffect(() => {
    if (!containerRef.current || !chart.trim()) return;
    let cancelled = false;

    (async () => {
      try {
        // Re-init per render so the diagram tracks the active app theme.
        mermaid.initialize({
          startOnLoad: false,
          theme: dark ? 'dark' : 'default',
          securityLevel: 'loose',
        });
        const { svg } = await mermaid.render(idRef.current, chart.trim());
        if (!cancelled && containerRef.current) {
          containerRef.current.innerHTML = svg;
          setError(null);
        }
      } catch (err) {
        if (!cancelled) {
          console.error('[MermaidDiagram] render failed', { error: err, chart });
          setError(String(err));
        }
      }
    })();

    return () => { cancelled = true; };
  }, [chart, dark]);

  if (error) {
    return (
      <div className="text-xs bg-[var(--bg-secondary)] p-3 rounded-md overflow-x-auto space-y-2">
        <div className="text-[var(--diff-del-color)] font-mono whitespace-pre-wrap">{error}</div>
        <pre className="text-[var(--text-secondary)] whitespace-pre-wrap">{chart}</pre>
      </div>
    );
  }

  return <div ref={containerRef} className="overflow-x-auto py-2" />;
}
