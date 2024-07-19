'use client';
import React, {
  createContext,
  useContext,
  useEffect,
  useRef,
  useState,
} from 'react';
import io, { Socket } from 'socket.io-client';
import Cookies from 'js-cookie';
import { useRouter } from 'next/navigation';

interface SocketContextProps {
  socket: Socket | null;
  connectSocket: () => void;
  disconnectSocket: () => void;
}

const SocketContext = createContext<SocketContextProps>({
  socket: null,
  connectSocket: () => {},
  disconnectSocket: () => {},
});

export const SocketProvider: React.FC<{ children: React.ReactNode }> = ({
  children,
}) => {
  const [socket, setSocket] = useState<Socket | null>(null);
  const socketRef = useRef(null);

  const connectSocket = () => {
    const token = Cookies.get('accessToken');
    if (!token) {
      console.error('No access token found');
      return;
    }

    const socketUrl =
      process.env.NODE_ENV === 'production'
        ? 'wss://developer.superagi.com'
        : 'ws://localhost:8080';

    const socketInstance = io(socketUrl, {
      transports: ['websocket'],
      path: '/api/socket.io',
    });

    socketInstance.on('connect', () => {
      console.log('Connected to websocket server');
      setSocket(socketInstance);
      socketRef.current = socketInstance;
      if (typeof window !== 'undefined') {
        const project_id = localStorage.getItem('projectId');
        if (project_id)
          socketInstance.emit('workspace-start', { project_id: project_id });
      }
    });

    socketInstance.on('workspace-started', (message: string) => {
      console.log(message);
    });

    socketInstance.on('disconnect', (reason: string) => {
      console.log('Disconnected from websocket server', reason);
      setSocket(null);
    });

    socketInstance.on('error', (error: any) => {
      console.error('Socket error:', error);
    });

    socketInstance.on('connect_error', (error: any) => {
      console.error('Connection error:', error);
    });
  };

  const connectSocketTerminal = () => {
    const token = Cookies.get('accessToken');
    if (!token) {
      console.error('No access token found');
      return;
    }
    const socketUrl =
      process.env.NODE_ENV === 'production'
        ? 'wss://developer.superagi.com/api/terminal?EIO=4&transport=websocket'
        : 'ws://localhost:8084/api/terminal?EIO=4&transport=websocket';
    const socketInstance = new WebSocket(socketUrl);

    socketInstance.onopen = () => {
      console.log('Connected to websocket terminal server');
      setSocket(socketInstance);
      socketRef.current = socketInstance;
    };

    socketInstance.onclose = (event) => {
      console.log('Disconnected from websocket terminal server', event.reason);
      setSocket(null);
    };

    socketInstance.onerror = (error) => {
      console.error('Socket error:', error);
    };
  };

  const disconnectSocket = () => {
    if (socketRef.current) {
      socketRef.current.disconnect();
      setSocket(null);
    }
  };

  useEffect(() => {
    connectSocket();
    connectSocketTerminal();
    return () => {
      disconnectSocket();
    };
  }, []);

  return (
    <SocketContext.Provider value={{ socket, connectSocket, disconnectSocket }}>
      {children}
    </SocketContext.Provider>
  );
};
