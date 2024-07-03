import styles from './loader.module.css';
import SkeletonLoader from '@/components/CustomLoaders/SkeletonLoader';
import skeletonLoader from '@/components/CustomLoaders/SkeletonLoader';

interface CustomLoadersProps {
  type: string;
  size?: number;
  skeletonType?: string;
}

export default function CustomLoaders({
  type,
  size,
  skeletonType,
}: CustomLoadersProps) {
  return (
    <div id={`${type}_loader`}>
      {type === 'default' ? (
        <div
          className={`${styles.default_spinner}`}
          style={{ width: `${size}px`, height: `${size}px` }}
        />
      ) : (
        <SkeletonLoader type={skeletonType} />
      )}
    </div>
  );
}
