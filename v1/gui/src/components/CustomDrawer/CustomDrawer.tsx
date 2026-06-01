import React, { useState, useEffect } from 'react';
import styles from './drawer.module.css';
import { CustomDrawerProps } from '../../../types/customComponentTypes';

const CustomDrawer: React.FC<CustomDrawerProps> = ({
  open,
  onClose,
  direction = 'bottom',
  top = '0',
  width,
  children,
  contentCSS = '',
}) => {
  const [isVisible, setIsVisible] = useState(open);
  const setViewportHeight = () => {
    // Check if visual viewport is available and adjust styles accordingly

    if (window.visualViewport?.height <= 500) {
      window.scrollTo({ top: 0 });

      document.documentElement.style.setProperty('--drawer-max-height', '60vh');
    } else {
      document.documentElement.style.setProperty('--drawer-max-height', '90%');
    }
  };

  useEffect(() => {
    window.addEventListener('resize', setViewportHeight);
    window.visualViewport?.addEventListener('resize', setViewportHeight);
    setViewportHeight(); // Initial call

    return () => {
      window.removeEventListener('resize', setViewportHeight);

      window.visualViewport?.removeEventListener('resize', setViewportHeight);
    };
  }, []);

  useEffect(() => {
    if (open) {
      setIsVisible(true);
      document.body.style.overflow = 'hidden';
      document.body.style.position = 'fixed';
      document.body.style.width = '100%';
    } else {
      document.body.style.removeProperty('overflow');
      document.body.style.removeProperty('position');
      document.body.style.removeProperty('width');
    }

    return () => {
      document.body.style.removeProperty('overflow');
      document.body.style.removeProperty('position');
      document.body.style.removeProperty('width');
    };
  }, [open]);

  useEffect(() => {
    if (!open && isVisible) {
      const timer = setTimeout(() => {
        setIsVisible(false);
      }, 300); // Ensure this matches the CSS transition time

      return () => clearTimeout(timer);
    }
  }, [open, isVisible]);

  const handleClose = () => {
    if (onClose) {
      onClose();
    }
  };

  return (
    <div
      className={`${styles.drawer_container} ${
        isVisible ? styles.open : styles.closed
      } ${styles[direction]}`}
    >
      <div className={styles.drawer_backdrop} onClick={handleClose} />

      <div
        className={`${styles.drawer_content} bg-white ${contentCSS}`}
        style={{ top: top, width: width }}
      >
        {children}
      </div>
    </div>
  );
};

export default CustomDrawer;
