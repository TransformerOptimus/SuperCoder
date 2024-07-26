const localurl = process.env.NEXT_PUBLIC_DOCKER
  ? 'http://server:8080/api/:path*'
  : 'http://0.0.0.0:8080/api/:path*';

let backendUrl = process.env.NEXT_PUBLIC_API_URL+'/api/:path*';

console.log("NEXT_PUBLIC_DOCKER", process.env.NEXT_PUBLIC_DOCKER)
console.log("LOCALURL", localurl)
const nextConfig = {
  compiler: {
    styledComponents: true,
  },
  async rewrites() {
    return [
      {
        source: '/api/:path*',
        destination: process.env.NEXT_PUBLIC_APP_ENV === 'development' ? localurl : backendUrl,
      },
    ];
  },
  images: {
    domains: [],
    formats: ['image/webp', 'image/avif'],
    remotePatterns: [
      {
        protocol: 'https',
        hostname: '**',
        port: '',
        pathname: '**',
      },
    ],
  },
  reactStrictMode: false,
  output: 'standalone',
};

export default nextConfig;
