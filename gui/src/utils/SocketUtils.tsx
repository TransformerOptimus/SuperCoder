import { Socket } from 'net';

export const handleStartWorkspace = (
  socket: Socket | null,
  projectId: string,
) => {
  return new Promise<void>((resolve) => {
    if (socket) {
      socket.emit('workspace-start', { project_id: projectId });
      socket.on('workspace-started', (message: string) => {
        console.log(message);
        resolve();
      });
    } else {
      resolve();
    }
  });
};

export const handleCloseWorkspace = (
  socket: Socket | null,
  projectId: string | null,
  disconnectSocket?: () => void,
) => {
  return new Promise<void>((resolve) => {
    if (socket && projectId) {
      socket.emit('workspace-close', { project_id: projectId });
      socket.on('workspace-closed', (message: string) => {
        console.log(message);
        resolve();
      });
    } else {
      resolve();
    }
  });
};
