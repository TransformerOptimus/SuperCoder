import type { CodeChangesArtifact } from "@/types/agent";

export interface CodeChangesCardProps {
  artifact: CodeChangesArtifact;
  onExpand?: (artifactId: string) => void;
}
