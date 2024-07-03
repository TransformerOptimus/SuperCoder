import React from 'react';
import styles from './diff.module.css';
import CustomImage from '@/components/ImageComponents/CustomImage';
import imagePath from '@/app/imagePath';

interface Snippet {
  changed: boolean;
  snippet: string;
}

interface DiffProps {
  diffStrings: string[];
}

interface SideDiffProps {
  snippets: Snippet[];
  isLeft: boolean;
}

const parseDiffString = (
  diffString: string,
): { leftside: Snippet[]; rightside: Snippet[] } => {
  const leftside: Snippet[] = [];
  const rightside: Snippet[] = [];

  const lines = diffString.split('\n');
  let isInDiffSection = false;

  lines.forEach((line) => {
    if (line.startsWith('@@')) {
      isInDiffSection = true;
      return;
    }

    if (!isInDiffSection) {
      return;
    }

    if (line.startsWith('-')) {
      leftside.push({ changed: true, snippet: line.substring(1) });
      rightside.push({ changed: false, snippet: '' });
    } else if (line.startsWith('+')) {
      rightside.push({ changed: true, snippet: line.substring(1) });
      leftside.push({ changed: false, snippet: '' });
    } else {
      leftside.push({ changed: false, snippet: line.substring(1) });
      rightside.push({ changed: false, snippet: line.substring(1) });
    }
  });

  return { leftside, rightside };
};

const highlightSyntax = (code: string) => {
  const keywords =
    /\b(import|from|class|def|return|if|else|for|while|break|continue|pass|try|except|finally|with|as|lambda|yield|raise|assert|del|global|nonlocal|True|False|None)\b/g;
  const strings = /(["'`])(?:(?=(\\?))\2.)*?\1/g;
  const functions = /\b([a-zA-Z_][a-zA-Z0-9_]*)\s*(?=\()/g;
  const variables = /\b([a-zA-Z_][a-zA-Z0-9_]*)\b/g;
  const comments = /#.*/g;
  const numbers = /\b\d+(\.\d+)?\b/g;

  return code
    .replace(keywords, '<span class="keyword">$&</span>')
    .replace(strings, '<span class="string">$&</span>')
    .replace(functions, '<span class="function">$&</span>')
    .replace(variables, '<span class="variable">$&</span>')
    .replace(comments, '<span class="comment">$&</span>')
    .replace(numbers, '<span class="number">$&</span>');
};

const SideDiff: React.FC<SideDiffProps> = React.memo(({ snippets, isLeft }) => (
  <div className={`${styles.sideDiff} col-span-6 flex flex-col`}>
    {snippets.map((snippet, index) => (
      <div
        key={index}
        className={`flex flex-row items-center ${
          snippet.changed &&
          (isLeft ? styles.deletion_line_color : styles.insertion_line_color)
        }`}
      >
        <div
          className={`flex h-full w-24 flex-row items-center justify-center ${
            styles.indexContainer
          } ${
            snippet.changed &&
            (isLeft
              ? styles.deletion_index_color
              : styles.insertion_index_color)
          }`}
          style={{ width: '60px' }}
        >
          {index + 1}
        </div>
        <div
          className={`flex flex-row items-center gap-[10px] break-words px-4 py-1 ${styles.snippetContainer}`}
        >
          {snippet.changed && (
            <CustomImage
              className="size-3"
              src={isLeft ? imagePath.codeMinusIcon : imagePath.codeAddIcon}
              alt={isLeft ? 'minus_icon' : 'add_icon'}
            />
          )}
          <pre
            className={`${styles.snippet} ${styles.syntaxHighlight}`}
            dangerouslySetInnerHTML={{
              __html: highlightSyntax(snippet.snippet),
            }}
          />
        </div>
      </div>
    ))}
  </div>
));

const DiffViewer: React.FC<DiffProps> = ({ diffStrings }) => {
  return (
    <div
      className={`space_mono grid w-full grid-cols-12 border-1 text-xs font-normal text-white ${styles.code_diff_container}`}
    >
      {diffStrings.map((diffString, index) => {
        const { leftside, rightside } = parseDiffString(diffString);
        const maxLength = Math.max(leftside.length, rightside.length);
        const paddedLeftside = [
          ...leftside,
          ...Array(maxLength - leftside.length).fill({
            changed: false,
            snippet: '',
          }),
        ];
        const paddedRightside = [
          ...rightside,
          ...Array(maxLength - rightside.length).fill({
            changed: false,
            snippet: '',
          }),
        ];

        return (
          <React.Fragment key={index}>
            <SideDiff snippets={paddedLeftside} isLeft />
            <SideDiff snippets={paddedRightside} isLeft={false} />
          </React.Fragment>
        );
      })}
    </div>
  );
};

export default DiffViewer;
