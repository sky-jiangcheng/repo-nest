import i18n from 'i18next'
import { initReactI18next } from 'react-i18next'
import zhCommon from '../locales/zh-CN/common.json'
import enCommon from '../locales/en/common.json'

const detected = navigator.language.startsWith('zh') ? 'zh-CN' : 'en'
const stored = localStorage.getItem('repotory-language')
const lng = stored === 'zh-CN' || stored === 'en' ? stored : detected

i18n
  .use(initReactI18next)
  .init({
    resources: {
      'zh-CN': { common: zhCommon },
      en:       { common: enCommon },
    },
    lng,
    fallbackLng: 'en',
    ns: ['common'],
    defaultNS: 'common',
    interpolation: { escapeValue: false },
    react: { useSuspense: false },
  })

export function setLanguage(lng: string) {
  i18n.changeLanguage(lng)
  localStorage.setItem('repotory-language', lng)
  document.documentElement.lang = lng === 'zh-CN' ? 'zh-CN' : 'en'
  window.dispatchEvent(new Event('repotory-lang-change'))
}

export function getCurrentLanguage() {
  return i18n.language
}

export default i18n
