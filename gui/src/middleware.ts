import { NextResponse, NextRequest } from 'next/server';
import { parse } from 'cookie';

export function middleware(request: NextRequest) {
  const cookies = parse(request.headers.get('cookie') || '');
  const accessToken = cookies.accessToken;

  if (request.nextUrl.pathname === '/') {
    if (accessToken) {
      const url = request.nextUrl.clone();
      url.pathname = '/projects';
      return NextResponse.redirect(url);
    }
  } else {
    if (!accessToken) {
      const url = request.nextUrl.clone();
      url.pathname = '/';
      return NextResponse.redirect(url);
    }
  }

  return NextResponse.next();
}

export const config = {
  matcher: ['/', '/workbench', '/board', '/code', '/pull_request', '/projects'],
};
