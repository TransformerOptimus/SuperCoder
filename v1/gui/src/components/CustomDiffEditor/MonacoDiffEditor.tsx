import React, { useEffect, useRef, useState } from 'react';
import { DiffEditor } from '@monaco-editor/react';
import * as monaco from 'monaco-editor';
import styles from './diff.module.css';
import CustomImage from '@/components/ImageComponents/CustomImage';
import imagePath from '@/app/imagePath';
import { parse } from 'diff2html';

interface MonacoDiffEditorProps {
  diffString: string;
  fileName: string;
}

const MonacoDiffEditor: React.FC<MonacoDiffEditorProps> = ({
  diffString,
  fileName,
}) => {
  const [height, setHeight] = useState<string | null>(null);
  const [isCollapsed, setIsCollapsed] = useState(false);
  const editorRef = useRef<monaco.editor.IStandaloneDiffEditor | null>(null);
  const [original, setOriginal] = useState('');
  const [modified, setModified] = useState('');

  useEffect(() => {
    const diffJson = parse(diffString);
    if (diffJson.length > 0) {
      const file = diffJson[0];
      const originalContent = file.blocks
        .map((block) =>
          block.lines
            .filter((line) => line.type !== 'insert')
            .map((line) => line.content.slice(1))
            .join('\n'),
        )
        .join('\n');
      const modifiedContent = file.blocks
        .map((block) =>
          block.lines
            .filter((line) => line.type !== 'delete')
            .map((line) => line.content.slice(1))
            .join('\n'),
        )
        .join('\n');
      setOriginal(originalContent);
      setModified(modifiedContent);
    }
  }, [diffString]);

  useEffect(() => {
    if (editorRef.current) {
      const originalHeight = editorRef.current
        .getOriginalEditor()
        .getContentHeight();
      const modifiedHeight = editorRef.current
        .getModifiedEditor()
        .getContentHeight();
      const newHeight = Math.max(originalHeight, modifiedHeight);
      setHeight(`${newHeight}px`);
      editorRef.current.layout();
    }
  }, [original, modified]);

  const handleToggleCollapse = () => {
    setIsCollapsed(!isCollapsed);
  };

  return (
    <div className={styles.diff_container}>
      <div
        className={`${styles.diff_header} ${
          !isCollapsed ? styles.border_radius_top : 'rounded-[7px]'
        } cursor-pointer`}
        onClick={handleToggleCollapse}
      >
        <CustomImage
          className={'size-4'}
          src={
            isCollapsed
              ? imagePath.rightArrowThinGrey
              : imagePath.bottomArrowThinGrey
          }
          alt={isCollapsed ? 'top_arrow_thin_grey' : 'bottom_arrow_thin_grey'}
        />
        <span>{fileName}</span>
      </div>
      {!isCollapsed && (
        <DiffEditor
          height={height}
          original={original}
          modified={modified}
          language="javascript"
          theme="vs-dark"
          onMount={(editor: monaco.editor.IStandaloneDiffEditor) => {
            editorRef.current = editor;
            const originalHeight = editor
              .getOriginalEditor()
              .getContentHeight();
            const modifiedHeight = editor
              .getModifiedEditor()
              .getContentHeight();
            const newHeight = Math.max(originalHeight, modifiedHeight);
            setHeight(`${newHeight}px`);
            editor.layout();
          }}
          options={{
            renderSideBySide: true,
            readOnly: true,
            minimap: { enabled: false },
            scrollBeyondLastLine: false,
            scrollbar: {
              alwaysConsumeMouseWheel: false,
            },
          }}
        />
      )}
    </div>
  );
};

export default MonacoDiffEditor;
