import axios, { AxiosInstance } from 'axios';

export const API_BASE_URL = '/api';

const api: AxiosInstance = axios.create({
  baseURL: API_BASE_URL,
  timeout: 300000,
  withCredentials: true,
  headers: {
    common: {
      'Content-Type': 'application/json',
      'Access-Control-Allow-Origin': '*',
    },
  },
});

export default api;
