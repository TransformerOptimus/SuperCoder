import { NextRequest } from 'next/server';

export function middleware(request: NextRequest) {
  const token = request.cookies.get('token')?.value;
  if (!token) {
    if (request.nextUrl.pathname !== '/') {
      return Response.redirect(new URL('/', request.url));
    }
  } else {
    if (request.nextUrl.pathname === '/') {
      return Response.redirect(new URL('/projects', request.url));
    }
  }
}

export const config = {
  matcher: ['/', '/workbench', '/board', '/code', '/pull_request', '/projects'],
};
