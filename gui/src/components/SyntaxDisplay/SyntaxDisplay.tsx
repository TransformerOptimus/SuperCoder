import React from 'react';
import { Prism as SyntaxHighlighter } from 'react-syntax-highlighter';
import { dracula } from 'react-syntax-highlighter/dist/esm/styles/prism';
import { rgba } from 'color2k';
import { ActivityLogType } from "@/app/constants/ActivityLogType";

interface SyntaxDisplayProps {
  type: string;
  msg?: string;
}

const SyntaxDisplay: React.FC<SyntaxDisplayProps> = ({ type, msg }) => {
  let title = '';
  let content = msg || '';

  if (type === ActivityLogType.CODE) {
    const parseMessage = (message: string) => {
      const titleMatch = message.match(/"title":\s*"(.*?)"/);
      const contentMatch = message.match(/"content":\s*"([\s\S]*?)"/);

      return {
        title: titleMatch ? titleMatch[1] : '',
        content: contentMatch ? contentMatch[1].replace(/\\n/g, '\n').replace(/\\"/g, '"') : ''
      };
    };

    try {
      const parsedMsg = parseMessage(msg || '');
      title = parsedMsg.title;
      content = parsedMsg.content;
    } catch (e) {
      console.error('Failed to parse activity log message:', e);
    }
  }

  const command = title.replace('Running command: ', '');
  
  return (
    <>
      {type === ActivityLogType.ERROR && (
        <span className={'text-sm font-normal'}>
          AI Developer has an error in the terminal.
        </span>
      )}
      {type === ActivityLogType.CODE && title && (
        <span className={'text-sm font-normal'}>
          Running command: <span className="green-text">{command}</span>
        </span>
      )}

      <SyntaxHighlighter
        language="python"
        style={dracula}
        customStyle={{
          fontSize: '12px',
          padding: '10px',
          borderRadius: '5px',
          width: '100%',
          background: rgba(0, 0, 0, 0.4),
        }}
      >
        {content}
      </SyntaxHighlighter>
    </>
  );
};

export default SyntaxDisplay;