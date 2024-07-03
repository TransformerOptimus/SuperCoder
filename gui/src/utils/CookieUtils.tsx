import Cookies from 'js-cookie';

interface CookieOptions {
  expires?: number | Date;
  path?: string;
  domain?: string;
  secure?: boolean;
  sameSite?: 'Strict' | 'Lax' | 'None';
}

const defaultOptions: CookieOptions = {
  path: '/',
  secure: process.env.NODE_ENV === 'production',
  sameSite: 'Strict',
};

export const setCookie = (
  name: string,
  value: string,
  options: CookieOptions = {},
): void => {
  Cookies.set(name, value, { ...defaultOptions, ...options });
};

export const getCookie = (name: string): string | undefined => {
  return Cookies.get(name);
};

export const removeCookie = (
  name: string,
  options: CookieOptions = {},
): void => {
  Cookies.remove(name, { ...defaultOptions, ...options });
};
