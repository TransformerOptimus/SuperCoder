import type { Metadata } from 'next';
import { Inter } from 'next/font/google';
import './_app.css';
import React from 'react';
import { Providers } from './providers';
import Script from 'next/script';
import imagePath from '@/app/imagePath';

const inter = Inter({ subsets: ['latin'] });

export const metadata: Metadata = {
  title: 'SuperCoder',
  description: 'Autonomous Software Development System',
};

export default function RootLayout({
  children,
}: Readonly<{ children: React.ReactNode }>) {
  return (
    <html lang="en">
      <head>
        <title>SuperCoder</title>
        <link rel={'icon'} href={imagePath.superagiLogoRound} />
        <link rel={'preconnect'} href={'https://fonts.googleapis.com'} />
        <link
          href={
            'https://fonts.googleapis.com/css2?family=Space+Mono:ital,wght@0,400;0,700;1,400;1,700&display=swap'
          }
          rel={'stylesheet'}
        />
        <Script id={'clarity-script'} strategy={'afterInteractive'}>
          {`
            (function(c,l,a,r,i,t,y){
              c[a]=c[a]||function(){(c[a].q=c[a].q||[]).push(arguments)};
              t=l.createElement(r);t.async=1;t.src="https://www.clarity.ms/tag/"+i;
              y=l.getElementsByTagName(r)[0];y.parentNode.insertBefore(t,y);
            })(window, document, "clarity", "script", "myczkgvcth");
          `}
        </Script>
      </head>
      <body className={inter.className}>
        <Providers>{children}</Providers>
      </body>
    </html>
  );
}
