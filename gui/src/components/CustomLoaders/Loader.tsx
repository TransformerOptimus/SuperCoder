import React from 'react';
import styles from './loader.module.css';

interface LoaderProps {
  size?: number;
  text?: string;
}

const Loader: React.FC<LoaderProps> = ({ size = 100, text = 'Loading...' }) => {
  return (
    <div className="flex flex-col items-center justify-center space-y-4">
      <div className={styles.loader} style={{ width: size, height: size }}>
        <div className={styles.dot}></div>
        <div className={styles.dot}></div>
        <div className={styles.dot}></div>
        <div className={styles.dot}></div>
      </div>
      <span className="text-gray-200">{text}</span>
    </div>
  );
};

export default Loader;
