import { useState, useCallback } from "react";
import { useAppStore } from "@/store";
import { createInitialStreamingState } from "@/store/agentSlice";
import { agentTauriService } from "@/services/agentTauriService";
import { buildAgentMessage } from "@/utils/agentMessageAdapter";
import type { Attachment } from "@/types/chat";

interface UseAgentSendOpts {
  /** The session this composer sends into (the open thread). */
  sessionId: string | null;
}

interface UseAgentSendReturn {
  send: (text: string, mode?: string, attachments?: Attachment[]) => Promise<void>;
  cancel: () => Promise<void>;
  isBusy: boolean;
  isSending: boolean;
}

export function useAgentSend({ sessionId }: UseAgentSendOpts): UseAgentSendReturn {
  const [isSending, setSending] = useState(false);
  const agentStreaming = useAppStore((s) => s.agentStreaming);

  const isBusy = sessionId ? !!agentStreaming[sessionId]?.isStreaming : false;

  const send = useCallback(
    async (text: string, mode?: string, attachments?: Attachment[]) => {
      if (!text || !sessionId || isSending || isBusy) return;
      setSending(true);
      const store = useAppStore.getState();
      try {
        // Optimistic user message.
        store.addMessageToThread(
          sessionId,
          buildAgentMessage(`agent-user-${sessionId}-${Date.now()}`, text, "user", sessionId, ""),
        );
        store.setActiveSession(sessionId, sessionId);
        store.setAgentStreaming(sessionId, createInitialStreamingState());
        if (mode) store.setSessionMode(sessionId, mode);

        await agentTauriService.sendMessage(sessionId, text, mode, attachments);
      } catch (err) {
        console.error("[useAgentSend] error:", err);
        store.setStreamingError(sessionId, err instanceof Error ? err.message : String(err));
        store.clearActiveSession(sessionId);
      } finally {
        setSending(false);
      }
    },
    [sessionId, isSending, isBusy],
  );

  const cancel = useCallback(async () => {
    if (!sessionId) return;
    const store = useAppStore.getState();
    const loopId = store.activeSessionIds[sessionId] ?? sessionId;
    try {
      await agentTauriService.cancelSession(loopId);

      const streaming = store.agentStreaming[sessionId];
      if (streaming?.textBuffer) {
        store.addMessageToThread(
          sessionId,
          buildAgentMessage(`agent-stopped-${Date.now()}`, streaming.textBuffer, "agent", sessionId, ""),
        );
      }
      store.addMessageToThread(
        sessionId,
        buildAgentMessage(`interrupted-${Date.now()}`, "Session interrupted by user", "agent", sessionId, ""),
      );
      store.clearAgentStreaming(sessionId);
      store.clearActiveSession(sessionId);
    } catch (err) {
      console.error("[useAgentSend] Failed to cancel:", err);
    }
  }, [sessionId]);

  return { send, cancel, isBusy, isSending };
}
