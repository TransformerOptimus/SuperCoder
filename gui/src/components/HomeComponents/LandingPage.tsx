'use client';
import CustomImage from '@/components/ImageComponents/CustomImage';
import imagePath from '@/app/imagePath';
import styles from './home.module.css';
import { checkHealth } from '@/api/DashboardService';
import { Button } from '@nextui-org/react';
import { useEffect, useState } from 'react';
import { API_BASE_URL } from '@/api/apiConfig';
import CustomInput from '@/components/CustomInput/CustomInput';

export default function LandingPage() {
  const [email, setEmail] = useState<string>('');
  const [password, setPassword] = useState<string>('');
  const [showPassword, setShowPassword] = useState<boolean>(false);
  const [isEmailRegistered, setIsEmailRegistered] = useState<boolean | null>(
    null,
  );
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

  const handleCheckUserEmail = () => {
    toCheckUserEmail();
  };

  const handleLogin = () => {
    console.log('login');
  };

  const handleSignUp = () => {
    console.log('sign Up');
  };

  async function toCheckUserEmail() {
    try {
      setIsEmailRegistered(true);
    } catch (error) {
      console.error('Error: ', error);
    }
  }

  const getButtonFields = () => {
    switch (isEmailRegistered) {
      case null:
        return { text: 'Continue', onClick: handleCheckUserEmail };
      case true:
        return { text: 'Sign In', onClick: handleLogin };
      case false:
        return { text: 'Create Account', onClick: handleSignUp };
    }
  };

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

      <div className={'proxima_nova flex flex-col items-center self-center'}>
        <div className={styles.gradient_effect} />

        <CustomImage
          className={'h-[232px] w-[390px]'}
          src={imagePath.supercoderImage}
          alt={'super_coder_image'}
        />
        <div className={'mt-20 flex w-full flex-col gap-6'}>
          <Button
            onClick={() => forGithubSignIn()}
            className={`${styles.button} w-full`}
          >
            <CustomImage
              className={'size-5'}
              src={imagePath.githubLogo}
              alt={'github_logo'}
            />
            Continue with Github
          </Button>
          <div className="flex items-center">
            <div className={`h-px flex-grow ${styles.divider}`} />
            <span className="secondary_color px-2 text-sm">or</span>
            <div className={`h-px flex-grow ${styles.divider}`} />
          </div>
          <div className={'flex flex-col gap-4'}>
            <div className={'flex flex-col gap-2'}>
              <span className={'secondary_color text-sm'}>Email</span>
              <CustomInput
                placeholder={'Enter your email'}
                format={'text'}
                value={email}
                setter={setEmail}
                disabled={false}
              />
            </div>
            {isEmailRegistered !== null && (
              <div className={'flex flex-col gap-2'}>
                <span className={'secondary_color text-sm'}>Password</span>
                <CustomInput
                  placeholder={'Enter your email'}
                  format={showPassword ? 'text' : 'password'}
                  value={password}
                  setter={setPassword}
                />
              </div>
            )}
          </div>
          <Button
            onClick={getButtonFields().onClick}
            className={`${styles.button} w-full`}
          >
            {getButtonFields().text}
          </Button>
        </div>
      </div>
    </div>
  );
}
