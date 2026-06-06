import axios from 'axios'

import { apiConfig } from '@/config/api'

export const httpClient = axios.create({
  baseURL: apiConfig.baseURL,
  timeout: apiConfig.timeoutMs,
  withCredentials: apiConfig.withCredentials,
  headers: {
    Accept: 'application/json',
  },
})

httpClient.interceptors.response.use(
  (response) => response,
  (error) => Promise.reject(error)
)
