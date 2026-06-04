import type { Artifact, ArtifactType, CodeChangesArtifact } from "@/types/agent";
import CodeChangesCard from "./CodeChangesCard/CodeChangesCard";
import TerminalCard from "./TerminalCard/TerminalCard";
import FileCard from "./FileCard/FileCard";
import TextCard from "./TextCard/TextCard";
import InlineDiffPreview from "./InlineDiffPreview/InlineDiffPreview";
import UnknownArtifactCard from "./UnknownArtifactCard";

type RendererComponent = React.ComponentType<{ artifact: never; onExpand?: (id: string) => void }>;

const ARTIFACT_RENDERERS: Partial<Record<ArtifactType, RendererComponent>> = {
  code_changes: CodeChangesCard as RendererComponent,
  terminal: TerminalCard as RendererComponent,
  file: FileCard as RendererComponent,
  text: TextCard as RendererComponent,
};

interface ArtifactRendererProps {
  artifact: Artifact;
  inline?: boolean;
  onExpand?: (artifactId: string) => void;
}

export default function ArtifactRenderer({ artifact, inline, onExpand }: ArtifactRendererProps) {
  if (inline && artifact.type === "code_changes") {
    return (
      <InlineDiffPreview
        artifact={artifact as CodeChangesArtifact}
        onExpand={onExpand}
      />
    );
  }

  const Component = ARTIFACT_RENDERERS[artifact.type];
  if (!Component) return <UnknownArtifactCard artifact={artifact} />;
  return <Component artifact={artifact as never} onExpand={onExpand} />;
}
