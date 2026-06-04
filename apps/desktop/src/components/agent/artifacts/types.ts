import type { Artifact } from "@/types/agent";

export interface ArtifactCardProps {
  artifact: Artifact;
  onExpand?: (artifactId: string) => void;
}
