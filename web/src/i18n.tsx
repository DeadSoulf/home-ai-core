import { createContext, useContext, useEffect, useMemo, useState, type ReactNode } from "react";

export type Locale = "ru" | "en";

const strings = {
  en: {
    starting: "Starting Home-AI-Core…", controlPlane: "Control Plane", dashboard: "Dashboard", system: "System",
    modules: "Modules", updates: "Updates", jobs: "Jobs", audit: "Audit", realtime: "Realtime", signOut: "Sign out", user: "User",
    language: "Language", loading: "Loading…", loadError: "Unable to load data.", requestFailed: "Request failed",
    signIn: "Sign in", signInSubtitle: "Manage your private home infrastructure.", username: "Username",
    password: "Password", signingIn: "Signing in…", privateInfrastructure: "Private home infrastructure",
    createOwner: "Create the first owner", createOwnerSubtitle: "Initialize this Home-AI-Core node.",
    bootstrapNotice: "First setup is intentionally local-only. Read the one-time bootstrap token from the server state directory.",
    bootstrapToken: "Bootstrap token", displayName: "Display name", initializing: "Initializing…",
    initialize: "Initialize Home-AI-Core", bootstrapLocalOnly: "First-owner setup is restricted to localhost.",
    dashboardSubtitle: "Current state of this Home-AI-Core node.", node: "Node", cpu: "CPU", memory: "Memory",
    core: "Core", available: "available", registered: "registered", active: "active", logicalCpus: "logical CPUs",
    schema: "schema", storageOverview: "Storage overview", noBlockDevices: "No block devices reported.",
    recentJobs: "Recent jobs", noJobs: "No jobs yet.", systemSubtitle: "Read-only hardware and operating-system inventory.",
    hostname: "Hostname", nodeId: "Node ID", os: "OS", kernel: "Kernel", architecture: "Architecture",
    coreVersion: "Core version", compute: "Compute", ram: "RAM", availableRam: "Available RAM", gpuCount: "GPU count", gpu: "GPU", gpuDevices: "Graphics devices", vendor: "Vendor", driver: "Driver", driverMissing: "Driver not installed", pciAddress: "PCI address", deviceId: "Device ID",
    blockDevices: "Block devices", device: "Device", model: "Model", size: "Size", type: "Type", serial: "Serial",
    networkInterfaces: "Network interfaces", name: "Name", state: "State", addresses: "Addresses", speed: "Speed",
    flash: "SSD / flash", unknown: "Unknown", modulesSubtitle: "Registered platform capabilities. Installation arrives in Phase 9.",
    hostCapabilities: "Host capabilities", noDescription: "No description.",
    noModules: "No modules are registered yet. This is expected before the Module Store phase.",
    jobsSubtitle: "Persistent background operations and progress.", jobHistory: "Job history", status: "Status",
    progress: "Progress", created: "Created", auditSubtitle: "Security-sensitive actions recorded by Core.",
    recentEvents: "Recent events", time: "Time", actor: "Actor", action: "Action", target: "Target", outcome: "Outcome",
    noAudit: "No audit events yet.", updatesSubtitle: "Check and install Home-AI-Core updates from the Web UI.", updateStatus: "Core update status", currentVersion: "Current version", availableVersion: "Available version", published: "Published", checkUpdates: "Check for updates", checking: "Checking…", installUpdate: "Install update", installing: "Installing…", updateAvailable: "An update is available", upToDate: "Home-AI-Core is up to date", updateRestartNotice: "Core will restart automatically during installation. This page will reconnect.", updateStarted: "Update job started:", releaseNotes: "Release notes", noReleaseNotes: "No release notes were provided."
  },
  ru: {
    starting: "Запуск Home-AI-Core…", controlPlane: "Панель управления", dashboard: "Обзор", system: "Система",
    modules: "Модули", updates: "Обновления", jobs: "Задачи", audit: "Аудит", realtime: "События", signOut: "Выйти", user: "Пользователь",
    language: "Язык", loading: "Загрузка…", loadError: "Не удалось загрузить данные.", requestFailed: "Ошибка запроса",
    signIn: "Вход", signInSubtitle: "Управление вашей домашней инфраструктурой.", username: "Логин",
    password: "Пароль", signingIn: "Вход…", privateInfrastructure: "Личная домашняя инфраструктура",
    createOwner: "Создание первого владельца", createOwnerSubtitle: "Первичная настройка узла Home-AI-Core.",
    bootstrapNotice: "Первичная настройка разрешена только локально. Одноразовый bootstrap-токен хранится в каталоге состояния сервера.",
    bootstrapToken: "Bootstrap-токен", displayName: "Отображаемое имя", initializing: "Инициализация…",
    initialize: "Инициализировать Home-AI-Core", bootstrapLocalOnly: "Создание первого владельца разрешено только с localhost.",
    dashboardSubtitle: "Текущее состояние этого узла Home-AI-Core.", node: "Узел", cpu: "Процессор", memory: "Память",
    core: "Ядро", available: "доступно", registered: "зарегистрировано", active: "активно", logicalCpus: "логических CPU",
    schema: "схема", storageOverview: "Накопители", noBlockDevices: "Блочные устройства не обнаружены.",
    recentJobs: "Последние задачи", noJobs: "Задач пока нет.", systemSubtitle: "Аппаратная конфигурация и операционная система в режиме только для чтения.",
    hostname: "Имя хоста", nodeId: "ID узла", os: "ОС", kernel: "Ядро Linux", architecture: "Архитектура",
    coreVersion: "Версия Core", compute: "Вычислительные ресурсы", ram: "ОЗУ", availableRam: "Доступно ОЗУ", gpuCount: "Количество GPU", gpu: "GPU", gpuDevices: "Графические устройства", vendor: "Производитель", driver: "Драйвер", driverMissing: "Драйвер не установлен", pciAddress: "PCI-адрес", deviceId: "ID устройства",
    blockDevices: "Блочные устройства", device: "Устройство", model: "Модель", size: "Размер", type: "Тип", serial: "Серийный номер",
    networkInterfaces: "Сетевые интерфейсы", name: "Имя", state: "Состояние", addresses: "Адреса", speed: "Скорость",
    flash: "SSD / Flash", unknown: "Неизвестно", modulesSubtitle: "Зарегистрированные возможности платформы. Установка модулей появится на этапе 9.",
    hostCapabilities: "Возможности хоста", noDescription: "Описание отсутствует.",
    noModules: "Модули пока не зарегистрированы. Это нормально до этапа Module Store.",
    jobsSubtitle: "Фоновые операции и их прогресс.", jobHistory: "История задач", status: "Состояние",
    progress: "Прогресс", created: "Создана", auditSubtitle: "Журнал действий, важных для безопасности.",
    recentEvents: "Последние события", time: "Время", actor: "Инициатор", action: "Действие", target: "Объект", outcome: "Результат",
    noAudit: "Событий аудита пока нет.", updatesSubtitle: "Проверка и установка обновлений Home-AI-Core прямо из Web UI.", updateStatus: "Состояние обновления Core", currentVersion: "Текущая версия", availableVersion: "Доступная версия", published: "Опубликовано", checkUpdates: "Проверить обновления", checking: "Проверка…", installUpdate: "Установить обновление", installing: "Установка…", updateAvailable: "Доступно обновление", upToDate: "Установлена последняя версия Home-AI-Core", updateRestartNotice: "Во время установки Core автоматически перезапустится. Эта страница подключится снова.", updateStarted: "Запущена задача обновления:", releaseNotes: "Что изменилось", noReleaseNotes: "Описание изменений отсутствует."
  }
} as const;

export type StringKey = keyof typeof strings.en;
const storageKey = "home-ai-core.locale";

export function chooseInitialLocale(stored: string | null, languages: readonly string[]): Locale {
  if (stored === "ru" || stored === "en") return stored;
  return languages.some((language) => language.toLowerCase().startsWith("ru")) ? "ru" : "en";
}

type I18nValue = {
  locale: Locale;
  setLocale: (locale: Locale) => void;
  t: (key: StringKey) => string;
  status: (value: string) => string;
  date: (value: string) => string;
};

const Context = createContext<I18nValue | undefined>(undefined);

export function I18nProvider({children}: {children: ReactNode}) {
  const [locale, setLocaleState] = useState<Locale>(() => chooseInitialLocale(localStorage.getItem(storageKey), navigator.languages));

  useEffect(() => {
    document.documentElement.lang = locale;
  }, [locale]);

  const value = useMemo<I18nValue>(() => ({
    locale,
    setLocale(next) {
      localStorage.setItem(storageKey, next);
      setLocaleState(next);
    },
    t(key) {
      return strings[locale][key];
    },
    status(raw) {
      const ruStatus: Record<string, string> = {
        connected: "подключено", connecting: "подключение", disconnected: "отключено",
        succeeded: "успешно", failed: "ошибка", running: "выполняется", queued: "в очереди",
        cancel_requested: "отмена запрошена", cancelled: "отменено", enabled: "включён",
        disabled: "выключен", registered: "зарегистрирован", error: "ошибка", success: "успешно",
        denied: "отказ", up: "активен", down: "неактивен"
      };
      return locale === "ru" ? (ruStatus[raw] || raw.replaceAll("_", " ")) : raw.replaceAll("_", " ");
    },
    date(raw) {
      return new Date(raw).toLocaleString(locale === "ru" ? "ru-RU" : "en-US");
    }
  }), [locale]);

  return <Context.Provider value={value}>{children}</Context.Provider>;
}

export function useI18n() {
  const value = useContext(Context);
  if (!value) throw new Error("useI18n must be used inside I18nProvider");
  return value;
}

export function LanguageSwitch() {
  const {locale, setLocale, t} = useI18n();
  return <div className="language-switch" role="group" aria-label={t("language")}>
    <button type="button" className={locale === "ru" ? "active" : ""} onClick={() => setLocale("ru")}>RU</button>
    <button type="button" className={locale === "en" ? "active" : ""} onClick={() => setLocale("en")}>EN</button>
  </div>;
}
