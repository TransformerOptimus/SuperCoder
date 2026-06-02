interface AvatarProps {
  initials: string;
  color: string;
  size?: 'xs' | 'sm' | 'md' | 'lg';
  className?: string;
}

const sizeClasses = {
  xs: 'w-6 h-6 text-[10px]',
  sm: 'w-8 h-8 text-xs',
  md: 'w-10 h-10 text-sm',
  lg: 'w-12 h-12 text-base',
};

export default function Avatar({ initials, color, size = 'md', className = '' }: AvatarProps) {
  return (
    <div
      className={`${sizeClasses[size]} rounded-full flex items-center justify-center text-white font-semibold shrink-0 ${className}`}
      style={{ backgroundColor: color }}
    >
      {initials}
    </div>
  );
}
