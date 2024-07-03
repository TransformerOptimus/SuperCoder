'use client';
import CustomImage from '@/components/ImageComponents/CustomImage';
import imagePath from '@/app/imagePath';
import styles from './home.module.css';
import { checkHealth } from '@/api/DashboardService';
import { Button } from '@nextui-org/react';
import { useEffect } from 'react';
import { API_BASE_URL } from '@/api/apiConfig';

export default function LandingPage() {
  useEffect(() => {
    toCheckHealth().then().catch();
  }, []);

  async function toCheckHealth() {
    try {
      const response = await checkHealth();
      if (response) {
        console.log(response);
      }
    } catch (error) {
      console.error('Error: ', error);
    }
  }

  async function forGithubSignIn() {
    try {
      window.location.href = `${API_BASE_URL}/github/signin`;
    } catch (error) {
      console.error('Error: ', error);
    }
  }

  return (
    <div
      id={'landing_page'}
      className={`flex h-screen w-screen flex-col px-14 py-10 text-white ${styles.bg_color}`}
    >
      <CustomImage
        className={'h-[32px] w-[166px]'}
        src={imagePath.superagiLogo}
        alt={'superagi_logo'}
      />

      <div className={'flex flex-col items-center self-center'}>
        <div className={styles.gradient_effect} />

        <CustomImage
          className={'h-[232px] w-[390px]'}
          src={imagePath.supercoderImage}
          alt={'super_coder_image'}
        />

        <Button
          onClick={() => forGithubSignIn()}
          className={`${styles.github_icon} mt-20 w-full`}
        >
          <CustomImage
            className={'size-5'}
            src={imagePath.githubLogo}
            alt={'github_logo'}
          />
          Continue with Github
        </Button>
      </div>
    </div>
  );
}
