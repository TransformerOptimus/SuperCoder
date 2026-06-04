import { Card } from "antd";
import type { TextArtifact } from "@/types/agent";

interface TextCardProps {
  artifact: TextArtifact;
}

export default function TextCard({ artifact }: TextCardProps) {
  return (
    <Card size="small">
      <p className="text-xs text-[var(--text-color)] whitespace-pre-wrap line-clamp-4 m-0">
        {artifact.content}
      </p>
    </Card>
  );
}
