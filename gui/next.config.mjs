const localurl = process.env.NEXT_PUBLIC_DOCKER
  ? 'http://server:8080/api/:path*'
  : 'http://0.0.0.0:8080/api/:path*';
// const backendUrl = process.env.NEXT_PUBLIC_BACKEND_URL
//   ? `${process.env.NEXT_PUBLIC_BACKEND_URL}/api/:path*`
//   : 'http://default-backend-url/api/:path*';


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
        destination: localurl,
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
