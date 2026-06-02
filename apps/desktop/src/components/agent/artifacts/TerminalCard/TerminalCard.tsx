import { Card } from "antd";
import { Terminal } from "lucide-react";
import type { TerminalArtifact } from "@/types/agent";

interface TerminalCardProps {
  artifact: TerminalArtifact;
}

export default function TerminalCard({ artifact }: TerminalCardProps) {
  return (
    <Card size="small">
      <div className="flex items-center gap-2">
        <Terminal className="w-4 h-4 text-[var(--text-secondary)] shrink-0" />
        <span className="text-xs text-[var(--text-secondary)] font-mono truncate">
          {artifact.command}
        </span>
        <span
          className={`ml-auto text-xs font-mono ${
            artifact.exit_code === 0 ? "text-green-600" : "text-red-500"
          }`}
        >
          exit {artifact.exit_code}
        </span>
      </div>
    </Card>
  );
}
