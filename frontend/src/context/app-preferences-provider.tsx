import { createContext, useContext, useState } from 'react'
import { getCookie, removeCookie, setCookie } from '@/lib/cookies'

const APP_PREFERENCES_COOKIE_NAME = 'app-preferences'
const APP_PREFERENCES_COOKIE_MAX_AGE = 60 * 60 * 24 * 365 // 1 year

export const appLanguages = [
  { value: 'zh-CN', label: '简体中文' },
  { value: 'en-US', label: 'English' },
] as const

export type AppLanguage = (typeof appLanguages)[number]['value']

export type AppPreferences = {
  brandName: string
  subtitle: string
  language: AppLanguage
}

type AppPreferencesContextType = AppPreferences & {
  defaultPreferences: AppPreferences
  resetPreferences: () => void
  setPreferences: (preferences: AppPreferences) => void
}

export const defaultAppPreferences: AppPreferences = {
  brandName: 'OV Dash',
  subtitle: '运营仪表盘',
  language: 'zh-CN',
}

const AppPreferencesContext =
  createContext<AppPreferencesContextType | null>(null)

function readSavedPreferences(): AppPreferences {
  const savedPreferences = getCookie(APP_PREFERENCES_COOKIE_NAME)

  if (!savedPreferences) return defaultAppPreferences

  try {
    const parsed = JSON.parse(decodeURIComponent(savedPreferences)) as Partial<
      Record<keyof AppPreferences, string>
    >

    const language = appLanguages.some(
      (item) => item.value === parsed.language
    )
      ? (parsed.language as AppLanguage)
      : defaultAppPreferences.language

    return {
      brandName: parsed.brandName?.trim() || defaultAppPreferences.brandName,
      subtitle: parsed.subtitle?.trim() || defaultAppPreferences.subtitle,
      language,
    }
  } catch {
    return defaultAppPreferences
  }
}

export function AppPreferencesProvider({
  children,
}: {
  children: React.ReactNode
}) {
  const [preferences, updatePreferences] = useState<AppPreferences>(
    readSavedPreferences
  )

  const setPreferences = (preferences: AppPreferences) => {
    setCookie(
      APP_PREFERENCES_COOKIE_NAME,
      encodeURIComponent(JSON.stringify(preferences)),
      APP_PREFERENCES_COOKIE_MAX_AGE
    )
    updatePreferences(preferences)
  }

  const resetPreferences = () => {
    removeCookie(APP_PREFERENCES_COOKIE_NAME)
    updatePreferences(defaultAppPreferences)
  }

  return (
    <AppPreferencesContext
      value={{
        ...preferences,
        defaultPreferences: defaultAppPreferences,
        resetPreferences,
        setPreferences,
      }}
    >
      {children}
    </AppPreferencesContext>
  )
}

// eslint-disable-next-line react-refresh/only-export-components
export const useAppPreferences = () => {
  const context = useContext(AppPreferencesContext)

  if (!context) {
    throw new Error(
      'useAppPreferences must be used within an AppPreferencesProvider'
    )
  }

  return context
}
