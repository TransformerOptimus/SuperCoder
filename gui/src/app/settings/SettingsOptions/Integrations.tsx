'use client';

import React, {useCallback, useEffect, useState} from 'react';

import styles from '../settings.module.css';
import {deleteGithubIntegration, isGithubConnected} from "@/api/DashboardService";
import {API_BASE_URL} from "@/api/apiConfig";

async function redirectToGithubIntegration() {
  try {
    window.location.href = `${API_BASE_URL}/integrations/github/authorize`;
  } catch (error) {
    console.error('Error: ', error);
  }
}

const Integrations = () => {
  const [isExternalGitIntegration, setIsExternalGitIntegration] = useState<boolean | undefined>(null);
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
      <div className="max-w-3xl mx-auto">
        <h2 className="text-xl font-semibold mb-4">Integrations</h2>
        <div>
          <div className={`flex items-center justify-between p-4 rounded-lg mb-3 ${styles.integration_container}`}>
            <div className="flex items-center">
              <div className="w-10 h-10 mr-4">
                <svg viewBox="0 0 24 24" className="fill-current">
                  <path
                    d="M12 .297c-6.63 0-12 5.373-12 12 0 5.303 3.438 9.8 8.205 11.385.6.113.82-.258.82-.577 0-.285-.01-1.04-.015-2.04-3.338.724-4.042-1.61-4.042-1.61C4.422 18.07 3.633 17.7 3.633 17.7c-1.087-.744.084-.729.084-.729 1.205.084 1.838 1.236 1.838 1.236 1.07 1.835 2.809 1.305 3.495.998.108-.776.417-1.305.76-1.605-2.665-.3-5.466-1.332-5.466-5.93 0-1.31.465-2.38 1.235-3.22-.135-.303-.54-1.523.105-3.176 0 0 1.005-.322 3.3 1.23.96-.267 1.98-.399 3-.405 1.02.006 2.04.138 3 .405 2.28-1.552 3.285-1.23 3.285-1.23.645 1.653.24 2.873.12 3.176.765.84 1.23 1.91 1.23 3.22 0 4.61-2.805 5.625-5.475 5.92.42.36.81 1.096.81 2.22 0 1.606-.015 2.896-.015 3.286 0 .315.21.69.825.57C20.565 22.092 24 17.592 24 12.297c0-6.627-5.373-12-12-12"/>
                </svg>
              </div>
              <div>
                <h3 className="text-white font-semibold">GitHub</h3>
                <p className="text-gray-400 text-sm">
                  Integrate with GitHub to start working on your projects
                </p>
              </div>
            </div>
            {isExternalGitIntegration === undefined ? (<></>) : isExternalGitIntegration === false ? (
              <button
                onClick={() => redirectToGithubIntegration()}
                className="bg-white font-semibold text-black px-4 py-2 rounded-md transition-colors"
              >
                Connect
              </button>
            ) : (
              <button
                onClick={() => deleteIntegration()}
                className="font-semibold text-gray-200 hover:text-white px-4 py-2 rounded-md transition-colors"
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