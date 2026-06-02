import type { Artifact } from "@/types/agent";

interface UnknownArtifactCardProps {
  artifact: Artifact;
}

export default function UnknownArtifactCard({ artifact }: UnknownArtifactCardProps) {
  return (
    <div className="border rounded-lg overflow-hidden border_8">
      <div className="px-3 py-2">
        <span className="text-xs text_secondary">
          Unknown artifact: {artifact.type}
        </span>
      </div>
    </div>
  );
}
