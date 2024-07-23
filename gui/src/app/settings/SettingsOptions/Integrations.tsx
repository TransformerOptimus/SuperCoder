'use client';

import React, { useEffect, useState } from 'react';

import styles from '../settings.module.css';
import {
  deleteGithubIntegration,
  isGithubConnected,
} from '@/api/DashboardService';
import { API_BASE_URL } from '@/api/apiConfig';
import GithubIntegrationSymbol from '@/components/IntegrationComponents/GithubIntegrationSymbol';
import { Button } from '@nextui-org/react';

async function redirectToGithubIntegration() {
  try {
    window.location.href = `${API_BASE_URL}/integrations/github/authorize`;
  } catch (error) {
    console.error('Error: ', error);
  }
}

const Integrations = () => {
  const [isExternalGitIntegration, setIsExternalGitIntegration] = useState<
    boolean | undefined
  >(null);
  useEffect(() => {
    (async function () {
      const gitIntegrated = await isGithubConnected();
      setIsExternalGitIntegration(gitIntegrated);
    })();
  }, []);

  const deleteIntegration = async () => {
    await deleteGithubIntegration();
    const gitIntegrated = await isGithubConnected();
    setIsExternalGitIntegration(gitIntegrated);
  };

  return (
    <div className="min-h-screen text-white">
      <div className="mx-auto max-w-3xl">
        <h2 className="mb-4 text-xl font-semibold">Integrations</h2>
        <div>
          <div
            className={`mb-3 flex items-center justify-between rounded-lg p-3 ${styles.integration_container}`}
          >
            <div className="flex items-center">
              <GithubIntegrationSymbol className="h-12 w-12 rounded-lg p-1.5" />
              <div className="ml-3">
                <h3 className="font-semibold text-white">GitHub</h3>
                <p className="text-sm text-gray-400">
                  Integrate with GitHub to start working on your projects
                </p>
              </div>
            </div>
            {isExternalGitIntegration === undefined ? (
              <></>
            ) : isExternalGitIntegration === false ? (
              <Button
                onClick={() => redirectToGithubIntegration()}
                className="rounded-md bg-white px-4 py-2 font-semibold text-black transition-colors"
              >
                Connect
              </Button>
            ) : (
              <button
                onClick={() => deleteIntegration()}
                className="rounded-md px-4 py-2 font-semibold text-gray-200 transition-colors hover:text-white"
              >
                Delete
              </button>
            )}
          </div>
        </div>
      </div>
    </div>
  );
};

export default Integrations;
