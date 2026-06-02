import { useRef, useState, useCallback } from "react";
import { Bot } from "lucide-react";
import SessionListSidebar from "../components/agent/SessionListSidebar/SessionListSidebar";
import AgentThreadPanel from "../components/agent/AgentThreadPanel/AgentThreadPanel";
import PlanThreadPanel from "../components/agent/PlanCard/PlanThreadPanel";
import DiffReviewPanel from "../components/agent/DiffReviewPanel/DiffReviewPanel";
import FileExplorerPanel from "../components/agent/FileExplorerPanel/FileExplorerPanel";
import InteractiveTerminal from "../components/agent/InteractiveTerminal/InteractiveTerminal";
import { useAppStore } from "../store";

export default function Workspace() {
  const agentViewMode = useAppStore((s) => s.agentViewMode);
  const activeAgentThreadId = useAppStore((s) => s.activeAgentThreadId);
  const activePlanProjectPath = useAppStore((s) => s.activePlanProjectPath);
  const agentThreads = useAppStore((s) => s.agentThreads);
  const showFileExplorer = useAppStore((s) => s.showFileExplorer);
  const showTerminal = useAppStore((s) => s.showTerminal);
  const toggleTerminal = useAppStore((s) => s.toggleTerminal);
  const sidebarCollapsed = useAppStore((s) => s.sidebarCollapsed);

  const terminalWorkingDir = activeAgentThreadId ? agentThreads[activeAgentThreadId]?.folder_path : undefined;

  const [sidebarWidth, setSidebarWidth] = useState(300);
  const [agentThreadWidth, setAgentThreadWidth] = useState(480);
  const resizingRef = useRef<"sidebar" | "agentThread" | null>(null);
  const startXRef = useRef(0);
  const startWidthRef = useRef(0);

  const handleResizeMouseDown = useCallback(
    (e: React.MouseEvent, panel: "sidebar" | "agentThread") => {
      e.preventDefault();
      resizingRef.current = panel;
      startXRef.current = e.clientX;
      startWidthRef.current = panel === "sidebar" ? sidebarWidth : agentThreadWidth;

      const handleMouseMove = (ev: MouseEvent) => {
        const delta = ev.clientX - startXRef.current;
        if (resizingRef.current === "sidebar") {
          setSidebarWidth(Math.max(220, Math.min(500, startWidthRef.current + delta)));
        } else if (resizingRef.current === "agentThread") {
          setAgentThreadWidth(Math.max(320, Math.min(720, startWidthRef.current + delta)));
        }
      };
      const handleMouseUp = () => {
        resizingRef.current = null;
        document.removeEventListener("mousemove", handleMouseMove);
        document.removeEventListener("mouseup", handleMouseUp);
        document.body.style.cursor = "";
        document.body.style.userSelect = "";
      };
      document.body.style.cursor = "col-resize";
      document.body.style.userSelect = "none";
      document.addEventListener("mousemove", handleMouseMove);
      document.addEventListener("mouseup", handleMouseUp);
    },
    [sidebarWidth, agentThreadWidth],
  );

  return (
    <div className="flex-1 flex overflow-hidden min-h-0">
      <div style={{ width: sidebarCollapsed ? 56 : sidebarWidth }} className="shrink-0 relative transition-[width] duration-150">
        <SessionListSidebar />
        {!sidebarCollapsed && (
          <div
            onMouseDown={(e) => handleResizeMouseDown(e, "sidebar")}
            className="absolute top-0 right-0 w-1 h-full cursor-col-resize hover:bg-blue-400/50 active:bg-blue-500/50 z-10"
          />
        )}
      </div>

      {agentViewMode === "diff_review" ? (
        <div className="flex-1 flex flex-col overflow-hidden min-h-0">
          <div className="flex-1 flex overflow-hidden min-h-0">
            <div style={{ width: agentThreadWidth }} className="shrink-0 relative">
              <AgentThreadPanel />
              <div
                onMouseDown={(e) => handleResizeMouseDown(e, "agentThread")}
                className="absolute top-0 right-0 w-1 h-full cursor-col-resize hover:bg-blue-400/50 active:bg-blue-500/50 z-10"
              />
            </div>
            <DiffReviewPanel />
            {showFileExplorer && <FileExplorerPanel />}
          </div>
          {showTerminal && terminalWorkingDir && (
            <InteractiveTerminal workingDir={terminalWorkingDir} onClose={toggleTerminal} />
          )}
        </div>
      ) : activeAgentThreadId ? (
        <div className="flex-1 flex overflow-hidden min-h-0">
          <AgentThreadPanel />
        </div>
      ) : activePlanProjectPath ? (
        <div className="flex-1 flex overflow-hidden min-h-0">
          <PlanThreadPanel />
        </div>
      ) : (
        <div className="flex-1 flex flex-col items-center justify-center text-center px-6">
          <Bot className="w-12 h-12 text-[var(--text-secondary)] opacity-40 mb-4" />
          <h2 className="text-lg font-semibold text-[var(--text-primary)] mb-1">No session open</h2>
          <p className="text-sm text-[var(--text-secondary)] max-w-sm">
            Create a new session from the sidebar to start an Ask, Plan, or Coding session on a local project folder.
          </p>
        </div>
      )}
    </div>
  );
}
