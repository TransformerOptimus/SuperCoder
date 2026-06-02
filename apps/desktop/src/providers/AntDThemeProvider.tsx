import React, { useEffect } from 'react';
import { ConfigProvider, theme as antdTheme, message, App } from 'antd';
import { useTheme } from '../context/ThemeContext';

declare global {
  interface Window {
    messageInstance: any;
  }
}

export const themedMessage = {
  success: (content: React.ReactNode, key?: string) => window.messageInstance?.success({ content, key }),
  error: (content: React.ReactNode, key?: string) => window.messageInstance?.error({ content, key }),
  warning: (content: React.ReactNode, key?: string) => window.messageInstance?.warning({ content, key }),
  info: (content: React.ReactNode, key?: string) => window.messageInstance?.info({ content, key }),
  loading: (content: React.ReactNode, key?: string, duration?: number) =>
    window.messageInstance?.loading({ content, key, duration: duration ?? 0 }),
};

export const AntDThemeProvider = ({ children }: { children: React.ReactNode }) => {
  const { dark } = useTheme();

  const primary = dark ? '#ffffff' : '#131315';
  const primaryHover = dark ? '#ccc' : '#555';

  const themeConfig = {
    algorithm: dark ? antdTheme.darkAlgorithm : antdTheme.defaultAlgorithm,
    token: {
      fontFamily: '"Proxima Nova", sans-serif',
      colorPrimary: primary,
      colorPrimaryHover: primaryHover,
      colorBorder: 'var(--white-opacity-8)',
      colorBgContainer: 'var(--white-opacity-4)',
    },
    components: {
      Message: { algorithm: true },
      Button: {
        algorithm: true,
        colorPrimary: primary,
        colorPrimaryHover: primaryHover,
        colorBorder: 'var(--white-opacity-8)',
        defaultShadow: 'none',
        primaryShadow: 'none',
        dangerShadow: 'none',
      },
      Input: {
        algorithm: true,
        colorPrimary: primary,
        colorPrimaryHover: primaryHover,
        colorBorder: 'var(--white-opacity-8)',
        activeBorderColor: primaryHover,
        hoverBg: 'var(--white-opacity-4)',
        activeBg: 'var(--white-opacity-4)',
      },
      Select: {
        algorithm: true,
        colorPrimary: primary,
        colorPrimaryHover: primaryHover,
        colorBorder: 'var(--white-opacity-8)',
      },
      Table: {
        algorithm: true,
        headerColor: '#888888',
        headerBg: 'var(--white-opacity-8)',
        headerSplitColor: 'transparent',
        colorSplit: 'transparent',
        controlItemBgHover: 'transparent',
        headerFilterHoverBg: 'transparent',
        rowHoverBg: 'var(--white-opacity-4)',
        rowSelectedHoverBg: 'transparent',
        rowSelectedBg: 'transparent',
        headerSortHoverBg: 'transparent',
        cellPaddingBlock: 8,
        borderRadius: 8,
      },
      Card: {
        paddingLG: 12,
        paddingSM: 8,
      },
      Alert: {
        defaultPadding: '8px 12px',
        withDescriptionPadding: '10px 12px',
      },
      Segmented: {
        borderRadius: 10,
        borderRadiusSM: 10,
        borderRadiusXS: 8,
      },
      Tabs: { algorithm: true, colorPrimary: primary },
      Radio: { algorithm: true, colorPrimary: primary },
      Checkbox: { algorithm: true, colorPrimary: primary, borderRadiusSM: 2 },
    },
  };

  return (
    <ConfigProvider theme={themeConfig}>
      <App>
        <AppMessageProvider>{children}</AppMessageProvider>
      </App>
    </ConfigProvider>
  );
};

const AppMessageProvider: React.FC<{ children: React.ReactNode }> = ({ children }) => {
  const [messageApi, contextHolder] = message.useMessage();

  useEffect(() => {
    window.messageInstance = messageApi;
  }, [messageApi]);

  return (
    <>
      {contextHolder}
      {children}
    </>
  );
};
