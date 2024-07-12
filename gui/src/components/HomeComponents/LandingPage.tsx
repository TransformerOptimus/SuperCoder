'use client';
import CustomImage from '@/components/ImageComponents/CustomImage';
import imagePath from '@/app/imagePath';
import styles from './home.module.css';
import {
  checkHealth,
  checkUserEmailExists,
  login,
  signUp,
} from '@/api/DashboardService';
import { Button } from '@nextui-org/react';
import { useEffect, useState } from 'react';
import { API_BASE_URL } from '@/api/apiConfig';
import CustomInput from '@/components/CustomInput/CustomInput';
import { authPayload, userData } from '../../../types/authTypes';
import { useRouter } from 'next/navigation';
import { setUserData } from '@/app/utils';

export default function LandingPage() {
  const [email, setEmail] = useState<string>('');
  const [password, setPassword] = useState<string>('');
  const [showPassword, setShowPassword] = useState<boolean>(false);
  const [isEmailRegistered, setIsEmailRegistered] = useState<boolean | null>(
    false,
  );
  const [isButtonLoading, setIsButtonLoading] = useState<boolean>(false);

  const router = useRouter();

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

  const onSetEmail = (value: string) => {
    setEmail(value);
    setIsEmailRegistered(null);
    setPassword('');
    setShowPassword(false);
  };

  const toSetUserData = (user, access_token: string) => {
    const userData: userData = {
      userEmail: user.Email,
      userName: user.Name,
      organisationId: user.OrganisationID,
      accessToken: access_token,
    };
    setUserData(userData);
  };

  async function toCheckUserEmail() {
    try {
      setIsButtonLoading(true);
      const response = await checkUserEmailExists(email);
      if (response) {
        const data = response.data;
        if (data.user_exists) {
          setIsEmailRegistered(true);
        } else {
          setIsEmailRegistered(false);
        }
      }
      setIsButtonLoading(false);
    } catch (error) {
      setIsButtonLoading(false);
      console.error('Error: ', error);
    }
  }

  async function loginUser() {
    try {
      setIsButtonLoading(true);
      const payload: authPayload = {
        email: email,
        password: password,
      };
      const response = await login(payload);
      if (response) {
        const data = response.data;
        if (data.success) {
          toSetUserData(data.user, data.access_token);
          router.push('/projects');
        }
      }
      setIsButtonLoading(false);
    } catch (error) {
      console.error('Error: ', error);
      setIsButtonLoading(false);
    }
  }

  async function createAccount() {
    try {
      setIsButtonLoading(true);
      const payload: authPayload = {
        email: email,
        password: password,
      };
      const response = await signUp(payload);
      if (response) {
        const data = response.data;
        if (data.success) {
          toSetUserData(data.user, data.access_token);
          router.push('/projects');
        }
      }
      setIsButtonLoading(false);
    } catch (error) {
      console.error('Error: ', error);
      setIsButtonLoading(false);
    }
  }

  const getButtonFields = () => {
    switch (isEmailRegistered) {
      case null:
        return { text: 'Continue', onClick: toCheckUserEmail };
      case true:
        return { text: 'Sign In', onClick: loginUser };
      case false:
        return { text: 'Create Account', onClick: createAccount };
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

      <div className={'proxima_nova flex flex-col  self-center'}>
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
                setter={onSetEmail}
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
                  endIcon={
                    showPassword
                      ? imagePath.passwordUnhidden
                      : imagePath.passwordHidden
                  }
                  endIconSize={'size-4'}
                  endIconClick={() =>
                    setShowPassword((prevState) => !prevState)
                  }
                />
              </div>
            )}
          </div>
          <Button
            onClick={getButtonFields().onClick}
            className={`${styles.button} w-full`}
            isLoading={isButtonLoading}
          >
            {getButtonFields().text}
          </Button>
        </div>
      </div>
    </div>
  );
}
