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
  return (
    <>
      {type === ActivityLogType.ERROR && (
        <span className={'text-sm font-normal'}>
          AI Developer has an error in the terminal.
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
        {msg}
      </SyntaxHighlighter>
    </>
  );
};

export default SyntaxDisplay;
