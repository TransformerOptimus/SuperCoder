import React from 'react';
import { Prism as SyntaxHighlighter } from 'react-syntax-highlighter';
import { dracula } from 'react-syntax-highlighter/dist/esm/styles/prism';
import { rgba } from 'color2k';

interface SyntaxDisplayProps {
  error: string;
}

const SyntaxDisplay: React.FC<SyntaxDisplayProps> = ({ error }) => {
  return (
    <>
      <span className={'text-sm font-normal'}>
        AI Developer has an error in the terminal.
      </span>
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
        {error}
      </SyntaxHighlighter>
    </>
  );
};

export default SyntaxDisplay;
