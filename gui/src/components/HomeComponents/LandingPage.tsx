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
import { authPayload } from '../../../types/authTypes';
import { useRouter } from 'next/navigation';
import { validateEmail } from '@/app/utils';
import { useUserContext } from '@/context/UserContext';

export default function LandingPage() {
  const userContext = useUserContext();

  const [email, setEmail] = useState<string>('');
  const [password, setPassword] = useState<string>('');
  const [showPassword, setShowPassword] = useState<boolean>(false);
  const [isEmailRegistered, setIsEmailRegistered] = useState<boolean | null>(
    null,
  );
  const [emailErrorMsg, setEmailErrorMsg] = useState<string>('');
  const [passwordErrorMsg, setPasswordErrorMsg] = useState<string>('');
  const [isButtonLoading, setIsButtonLoading] = useState<boolean>(false);

  const router = useRouter();

  useEffect(() => {
    toCheckHealth().then().catch();
  }, []);

  useEffect(() => {
    const handleKeyDown = (event) => {
      if (event.key === 'Enter') {
        if (isEmailRegistered === null) {
          toCheckUserEmail().then().catch();
        } else if (isEmailRegistered) {
          loginUser().then().catch();
        } else {
          createAccount().then().catch();
        }
      }
    };
    window.addEventListener('keydown', handleKeyDown);

    return () => {
      window.removeEventListener('keydown', handleKeyDown);
    };
  }, [toCheckUserEmail, loginUser, createAccount]);

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
    setEmailErrorMsg('');
    setPasswordErrorMsg('');
  };

  const onSetPassword = (value: string) => {
    setPassword(value);
    setPasswordErrorMsg('');
  };

  async function toCheckUserEmail() {
    try {
      if (!validateEmail(email)) {
        setEmailErrorMsg('Enter a Valid Email.');
        return;
      }
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
    } catch (error) {
      console.error('Error: ', error);
    } finally {
      setIsButtonLoading(false);
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
          userContext.fetchUserDetails();
          router.push('/projects');
        } else {
          setPasswordErrorMsg('Password entered is incorrect.');
        }
      }
    } catch (error) {
      console.error('Error: ', error);
    } finally {
      setIsButtonLoading(false);
    }
  }

  async function createAccount() {
    try {
      if (password.length < 8) {
        setPasswordErrorMsg('Password must be atleast 8 characters');
        return;
      }
      setIsButtonLoading(true);
      const payload: authPayload = {
        email: email,
        password: password,
      };
      const response = await signUp(payload);
      if (response) {
        const data = response.data;
        if (data.success) {
          await userContext.fetchUserDetails();
          router.push('/projects');
        }
      }
    } catch (error) {
      console.error('Error: ', error);
    } finally {
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
          className={'mt-14 h-[232px] w-[390px]'}
          src={imagePath.supercoderImage}
          alt={'super_coder_image'}
        />
        <div className={'mt-20 flex w-full flex-col gap-6'}>
          <Button
            onClick={() => forGithubSignIn()}
            className={`primary_medium`}
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
                isError={emailErrorMsg !== ''}
                errorMessage={emailErrorMsg}
              />
            </div>
            {isEmailRegistered !== null && (
              <div className={'flex flex-col gap-2'}>
                <span className={'secondary_color text-sm'}>Password</span>
                <CustomInput
                  placeholder={
                    isEmailRegistered ? 'Enter Password' : 'Set Password'
                  }
                  format={showPassword ? 'text' : 'password'}
                  value={password}
                  setter={onSetPassword}
                  endIcon={
                    showPassword
                      ? imagePath.passwordUnhidden
                      : imagePath.passwordHidden
                  }
                  alt={'password_icons'}
                  endIconSize={'size-4'}
                  endIconClick={() =>
                    setShowPassword((prevState) => !prevState)
                  }
                  errorMessage={passwordErrorMsg}
                  isError={passwordErrorMsg !== ''}
                />
              </div>
            )}
          </div>
          <Button
            onClick={getButtonFields().onClick}
            className={`primary_medium`}
            isLoading={isButtonLoading}
          >
            {getButtonFields().text}
          </Button>
        </div>
      </div>
    </div>
  );
}
