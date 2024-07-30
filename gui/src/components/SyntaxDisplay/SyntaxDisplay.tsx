import React from 'react';
import { Prism as SyntaxHighlighter } from 'react-syntax-highlighter';
import { dracula } from 'react-syntax-highlighter/dist/esm/styles/prism';
import { rgba } from 'color2k';
import {ActivityLogType} from "@/app/constants/ActivityLogType";

interface SyntaxDisplayProps {
  type: string;
  msg?: string;
}

const SyntaxDisplay: React.FC<SyntaxDisplayProps> = ({ type, msg }) => {
  let title = '';
  let content = msg;

  try {
    const parsedMsg = JSON.parse(msg || '');
    if (parsedMsg.title && parsedMsg.content) {
      title = parsedMsg.title;
      content = parsedMsg.content;
    }
  } catch (e) {
    // If parsing fails, it's not a valid JSON, so we'll use the original msg
  }

  return (
    <>
      {type === ActivityLogType.ERROR && (
        <span className={'text-sm font-normal'}>
          AI Developer has an error in the terminal.
        </span>
      )}
      {type === ActivityLogType.CODE && title && (
        <span className={'text-sm font-normal'}>
          Running command: <span className="green-text">{title.replace('Running command: ', '')}</span>
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